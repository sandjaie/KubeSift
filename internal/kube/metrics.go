/*
Purpose: Read bounded current usage for explicitly selected Pods and Nodes.
Used by: Visible workspace metric batches.
Responsibilities: Discover served APIs, share read capacity and verify current core identity after each sample.
Important: No provider is installed and no sample is cached by this service.
*/
package kube

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type metricsDiscovery struct {
	ready       chan struct{}
	until       time.Time
	version     string
	pods, nodes bool
	problem     *model.Problem
}
type metricsClient struct {
	client *http.Client
	host   string
}
type metricsHTTP int

func (e metricsHTTP) Error() string { return http.StatusText(int(e)) }
func (c metricsClient) get(ctx context.Context, path string, limit int64, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.host, "/")+path, nil)
	if err != nil {
		return model.Fail("invalid", "Cannot build metrics read.")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return metricsHTTP(resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > limit {
		return model.Fail("capacity", "The metrics response exceeds the read limit.")
	}
	if json.Unmarshal(raw, out) != nil {
		return model.Fail("invalid_response", "Kubernetes returned malformed metrics data.")
	}
	return nil
}
func metricsProblem(ctx context.Context, err error) *model.Problem {
	var status metricsHTTP
	if errors.As(err, &status) {
		code, message := "unavailable", "The Metrics API is unavailable."
		switch status {
		case 401:
			code, message = "auth_required", "Kubernetes rejected metrics authentication."
		case 403:
			code, message = "forbidden", "Kubernetes denied this metrics read."
		case 404:
			code, message = "missing_sample", "No metrics sample is available for this resource."
		case 429:
			code, message = "busy", "The Metrics API is rate limited."
		}
		return &model.Problem{Code: code, Message: message}
	}
	return ClassifyError(ctx, err)
}
func (s *Service) discoverMetrics(ctx context.Context, ref model.ConnectionRef, c metricsClient, refresh bool) *metricsDiscovery {
	for {
		s.mu.Lock()
		entry := s.metrics[ref]
		if entry != nil {
			select {
			case <-entry.ready:
				if !refresh && time.Now().Before(entry.until) {
					s.mu.Unlock()
					return entry
				}
			default:
				s.mu.Unlock()
				select {
				case <-entry.ready:
					if entry.until.IsZero() {
						continue
					}
					return entry
				case <-ctx.Done():
					return &metricsDiscovery{problem: metricsProblem(ctx, ctx.Err())}
				}
			}
		}
		entry = &metricsDiscovery{ready: make(chan struct{})}
		s.metrics[ref] = entry
		s.mu.Unlock()
		var group metav1.APIGroup
		err := c.get(ctx, "/apis/metrics.k8s.io", 16<<20, &group)
		if err == nil {
			for _, v := range []string{"v1", "v1beta1"} {
				if slices.ContainsFunc(group.Versions, func(item metav1.GroupVersionForDiscovery) bool {
					return item.Version == v && item.GroupVersion == "metrics.k8s.io/"+v
				}) {
					entry.version = v
					break
				}
			}
			if entry.version == "" {
				entry.problem = &model.Problem{Code: "unsupported", Message: "No supported Metrics API version is served."}
			} else {
				var resources metav1.APIResourceList
				err = c.get(ctx, "/apis/metrics.k8s.io/"+entry.version, 16<<20, &resources)
				if err == nil {
					for _, r := range resources.APIResources {
						if !slices.Contains(r.Verbs, "get") {
							continue
						}
						if r.Name == "pods" && r.Kind == "PodMetrics" && r.Namespaced {
							entry.pods = true
						}
						if r.Name == "nodes" && r.Kind == "NodeMetrics" && !r.Namespaced {
							entry.nodes = true
						}
					}
				}
			}
		}
		if err != nil {
			entry.problem = metricsProblem(ctx, err)
			if errors.Is(err, metricsHTTP(404)) {
				entry.problem = &model.Problem{Code: "unsupported", Message: "The cluster does not serve the Metrics API."}
			}
		}
		s.mu.Lock()
		if ctx.Err() == nil {
			entry.until = time.Now().Add(time.Minute)
		} else if s.metrics[ref] == entry {
			delete(s.metrics, ref)
		}
		close(entry.ready)
		s.mu.Unlock()
		return entry
	}
}
func metricsUnavailable(target ResourceRef, problem *model.Problem) ResourceUsage {
	return ResourceUsage{Resource: target, State: "unavailable", FetchedAt: time.Now().UTC(), Problem: problem}
}
func (s *Service) ReadCurrentMetrics(ctx context.Context, req MetricsRequest) (MetricsBatch, error) {
	if len(req.Targets) == 0 || len(req.Targets) > 100 {
		return MetricsBatch{}, model.Fail("capacity", "Metrics batches require 1–100 targets.")
	}
	seen := map[ResourceRef]bool{}
	addresses := map[string]bool{}
	for _, t := range req.Targets {
		address := t.Kind + "/" + t.Namespace + "/" + t.Name
		if t.Connection != req.Connection || t.APIVersion != "v1" || (t.Kind != "Pod" && t.Kind != "Node") || validResource(t) != nil ||
			(t.Kind == "Node" && t.Namespace != "") || seen[t] || addresses[address] {
			return MetricsBatch{}, model.Fail("invalid", "Metrics requires unique explicit Pod or Node identities.")
		}
		seen[t] = true
		addresses[address] = true
	}
	if req.Detail != nil && !seen[*req.Detail] {
		return MetricsBatch{}, model.Fail("invalid", "Metrics detail must match a batch target.")
	}
	select {
	case s.metricsBatches <- struct{}{}:
		defer func() { <-s.metricsBatches }()
	default:
		return MetricsBatch{}, model.Fail("busy", "Two metrics batches are already running.")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	owned, err := s.connections.Lifetime(req.Connection)
	if err != nil {
		return MetricsBatch{}, err
	}
	stop := context.AfterFunc(owned, cancel)
	defer stop()
	cfg, err := s.connections.RESTConfig(req.Connection)
	if err != nil {
		return MetricsBatch{}, err
	}
	httpClient, err := BoundedHTTPClient(cfg)
	if err != nil {
		return MetricsBatch{}, err
	}
	client := metricsClient{httpClient, cfg.Host}
	discovered := s.discoverMetrics(ctx, req.Connection, client, req.RefreshDiscovery)
	result := MetricsBatch{Items: make([]ResourceUsage, len(req.Targets)), FetchedAt: time.Now().UTC(), Problem: discovered.problem}
	if discovered.problem != nil {
		for i, t := range req.Targets {
			result.Items[i] = metricsUnavailable(t, discovered.problem)
		}
		return boundedMetricsBatch(result)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	var rediscover sync.Once
	var failureLock sync.Mutex
	var sharedFailure *model.Problem
	for worker := 0; worker < min(4, len(req.Targets)); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				target := req.Targets[i]
				select {
				case s.metricsWorkers <- struct{}{}:
					failureLock.Lock()
					failure := sharedFailure
					failureLock.Unlock()
					out := metricsUnavailable(target, failure)
					if failure == nil {
						out = s.readMetric(ctx, client, discovered, target, req.Detail != nil && *req.Detail == target)
					}
					<-s.metricsWorkers
					if out.Problem != nil && out.Problem.Code == "missing_sample" {
						rediscover.Do(func() {
							updated := s.discoverMetrics(ctx, req.Connection, client, true)
							if updated.problem != nil {
								failureLock.Lock()
								sharedFailure = updated.problem
								failureLock.Unlock()
							}
						})
					}
					failureLock.Lock()
					if sharedFailure != nil && out.Problem != nil {
						out = metricsUnavailable(target, sharedFailure)
					}
					failureLock.Unlock()
					result.Items[i] = out
				case <-ctx.Done():
					result.Items[i] = metricsUnavailable(target, metricsProblem(ctx, ctx.Err()))
				}
			}
		}()
	}
	for i := range req.Targets {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	result.Problem = sharedFailure
	if s.connections.Validate(req.Connection) != nil {
		return MetricsBatch{}, model.Fail("stale_target", "The metrics connection was replaced.")
	}
	return boundedMetricsBatch(result)
}
func boundedMetricsBatch(result MetricsBatch) (MetricsBatch, error) {
	if raw, err := json.Marshal(result); err != nil || len(raw) > 2<<20 {
		return MetricsBatch{}, model.Fail("capacity", "The metrics batch exceeds the 2 MiB display limit.")
	}
	return result, nil
}
func (s *Service) readMetric(ctx context.Context, c metricsClient, discovered *metricsDiscovery, target ResourceRef, detail bool) ResourceUsage {
	unavailable := func(err error) ResourceUsage { return metricsUnavailable(target, metricsProblem(ctx, err)) }
	if (target.Kind == "Pod" && !discovered.pods) || (target.Kind == "Node" && !discovered.nodes) {
		return unavailable(model.Fail("unsupported", "The Metrics API does not advertise this resource read."))
	}
	plural := "nodes"
	prefix := ""
	if target.Kind == "Pod" {
		plural = "pods"
		prefix = "/namespaces/" + url.PathEscape(target.Namespace)
	}
	suffix := prefix + "/" + plural + "/" + url.PathEscape(target.Name)
	var sample metricsWire
	if err := c.get(ctx, "/apis/metrics.k8s.io/"+discovered.version+suffix, 256<<10, &sample); err != nil {
		return unavailable(err)
	}
	if sample.APIVersion != "metrics.k8s.io/"+discovered.version || sample.Kind != target.Kind+"Metrics" {
		return unavailable(model.Fail("invalid_response", "The Metrics API returned an unexpected sample type."))
	}
	var pod *core.Pod
	var node *core.Node
	if target.Kind == "Pod" {
		pod = &core.Pod{}
		if err := c.get(ctx, "/api/v1"+suffix, 16<<20, pod); err != nil {
			return unavailable(err)
		}
		if pod.Name != target.Name || pod.Namespace != target.Namespace {
			return unavailable(model.Fail("invalid_response", "Kubernetes returned a different Pod."))
		}
	} else {
		node = &core.Node{}
		if err := c.get(ctx, "/api/v1"+suffix, 16<<20, node); err != nil {
			return unavailable(err)
		}
		if node.Name != target.Name || node.Namespace != "" {
			return unavailable(model.Fail("invalid_response", "Kubernetes returned a different Node."))
		}
	}
	out, err := projectMetrics(target, sample, pod, node, detail, "metrics.k8s.io/"+discovered.version, time.Now().UTC())
	if err != nil {
		return unavailable(err)
	}
	return out
}
