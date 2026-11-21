/*
Purpose: Read on-demand usage history for a verified Pod container or Node.
Used by: The Historical usage disclosure in resource Overview.
Responsibilities: Bind range queries to the selected source, target lifetime and fixed evaluation grid.
Important: Provider access never substitutes for a current named Kubernetes read.
*/
package kube

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/model"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type HistoryRequest struct {
	Resource  ResourceRef `json:"resource"`
	SourceID  string      `json:"sourceID"`
	Preset    string      `json:"preset"`
	Container string      `json:"container"`
}
type HistoryPoint struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}
type HistoryMetric struct {
	State     string         `json:"state"`
	Points    []HistoryPoint `json:"points"`
	Qualified bool           `json:"qualified"`
}
type HistoricalMetrics struct {
	SourceID       string        `json:"sourceID"`
	Resource       ResourceRef   `json:"resource"`
	Container      string        `json:"container"`
	Start          int64         `json:"start"`
	EffectiveStart int64         `json:"effectiveStart"`
	End            int64         `json:"end"`
	Step           int64         `json:"step"`
	CPUWindow      int64         `json:"cpuWindow"`
	FetchedAt      int64         `json:"fetchedAt"`
	CPU            HistoryMetric `json:"cpu"`
	Memory         HistoryMetric `json:"memory"`
}

func (s *Service) historyIdentity(ctx context.Context, r HistoryRequest) (time.Time, error) {
	t := r.Resource
	cfg, err := s.connections.RESTConfig(t.Connection)
	if err != nil {
		return time.Time{}, err
	}
	client, err := BoundedHTTPClient(cfg)
	if err != nil {
		return time.Time{}, err
	}
	var obj struct {
		Metadata metav1.ObjectMeta
		Spec     struct{ Containers, InitContainers, EphemeralContainers []struct{ Name string } }
	}
	path := "/api/v1/nodes/" + url.PathEscape(t.Name)
	if t.Kind == "Pod" {
		path = "/api/v1/namespaces/" + url.PathEscape(t.Namespace) + "/pods/" + url.PathEscape(t.Name)
	}
	if err := (metricsClient{client, cfg.Host}).get(ctx, path, 16<<20, &obj); err != nil {
		if errors.Is(err, metricsHTTP(http.StatusNotFound)) {
			return time.Time{}, model.Fail("stale_target", "The selected resource no longer exists.")
		}
		return time.Time{}, metricsProblem(ctx, err)
	}
	m := obj.Metadata
	if m.Name != t.Name || m.Namespace != t.Namespace || string(m.UID) != t.UID || m.CreationTimestamp.IsZero() {
		return time.Time{}, model.Fail("stale_target", "The selected resource identity changed.")
	}
	if t.Kind == "Pod" {
		members := append(obj.Spec.Containers, obj.Spec.InitContainers...)
		members = append(members, obj.Spec.EphemeralContainers...)
		for _, c := range members {
			if c.Name == r.Container {
				return m.CreationTimestamp.Time, nil
			}
		}
		return time.Time{}, model.Fail("stale_target", "Selected container is no longer declared.")
	}
	return m.CreationTimestamp.Time, nil
}
func (s *Service) ReadHistoricalMetrics(ctx context.Context, r HistoryRequest) (HistoricalMetrics, error) {
	return s.readHistoricalMetrics(ctx, r, time.Now().UTC())
}

func (s *Service) readHistoricalMetrics(ctx context.Context, r HistoryRequest, now time.Time) (HistoricalMetrics, error) {
	var empty HistoricalMetrics
	t := r.Resource
	if validResource(t) != nil || t.APIVersion != "v1" || (t.Kind != "Node" && t.Kind != "Pod") || (t.Kind == "Node" && (t.Namespace != "" || r.Container != "")) || (t.Kind == "Pod" && (r.Container == "" || t.Namespace == "")) {
		return empty, model.Fail("invalid", "Select an explicit Pod container or Node.")
	}
	preset, ok := map[string][2]int64{"1h": {3600, 30}, "6h": {21600, 120}, "24h": {86400, 300}}[r.Preset]
	if !ok {
		return empty, model.Fail("invalid", "Unknown history range.")
	}
	s.mu.Lock()
	h := s.history[t.Connection]
	s.mu.Unlock()
	if h == nil || h.client == nil || h.view.Revision != r.SourceID || h.ctx.Err() != nil {
		return empty, model.Fail("stale_target", "Select the current history source.")
	}
	if h.view.Mode == "project" && t.Kind == "Node" {
		return empty, model.Fail("unsupported", "Project endpoints only support Pod history.")
	}
	select {
	case s.historyReads <- struct{}{}:
		defer func() { <-s.historyReads }()
	default:
		return empty, model.Fail("busy", "Two history reads are already active.")
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	stop := context.AfterFunc(h.ctx, cancel)
	defer stop()
	step := preset[1]
	end := now.Unix() / step * step
	created, err := s.historyIdentity(ctx, r)
	if err != nil {
		return empty, err
	}
	earliest := created.Add(5 * time.Minute)
	grid := earliest.Truncate(time.Duration(step) * time.Second)
	if grid.Before(earliest) {
		grid = grid.Add(time.Duration(step) * time.Second)
	}
	out := HistoricalMetrics{Resource: t, Container: r.Container, SourceID: r.SourceID, Start: end - preset[0], EffectiveStart: max(end-preset[0], grid.Unix()), End: end, Step: step, CPUWindow: 300, CPU: HistoryMetric{State: "missing"}, Memory: HistoryMetric{State: "missing"}}
	labels := map[string]string{"node": t.Name, "id": "/"}
	if t.Kind == "Pod" {
		labels = map[string]string{"namespace": t.Namespace, "pod": t.Name, "container": r.Container}
	}
	if h.view.Cluster != "" {
		labels["cluster"] = h.view.Cluster
	}
	if out.EffectiveStart <= end {
		for _, cpu := range []bool{true, false} {
			m := h.query(ctx, r, out, labels, cpu)
			if m.State == "forbidden" {
				return empty, model.Fail("forbidden", "Provider denied this history read.")
			}
			if cpu {
				out.CPU = m
			} else {
				out.Memory = m
			}
		}
	}
	verified, err := s.historyIdentity(ctx, r)
	if err != nil {
		return empty, err
	}
	if !verified.Equal(created) || h.ctx.Err() != nil {
		return empty, model.Fail("stale_target", "The resource or history source changed.")
	}
	if ctx.Err() != nil {
		return empty, safeError(ctx, ctx.Err())
	}
	out.FetchedAt = time.Now().Unix()
	data, err := json.Marshal(out)
	if err != nil || len(data) > 128<<10 {
		return empty, model.Fail("capacity", "History exceeds the display limit.")
	}
	return out, nil
}
func (h *historySource) query(ctx context.Context, r HistoryRequest, b HistoricalMetrics, labels map[string]string, cpu bool) HistoryMetric {
	var match []string
	for k, v := range labels {
		match = append(match, k+"="+strconv.Quote(v))
	}
	sort.Strings(match)
	expression := "last_over_time(container_memory_working_set_bytes{" + strings.Join(match, ",") + "}[5m])"
	if cpu {
		match = append(match, `cpu=~"total|"`)
		expression = "rate(container_cpu_usage_seconds_total{" + strings.Join(match, ",") + "}[5m])"
	}
	q := url.Values{"query": {expression}, "start": {fmt.Sprint(b.EffectiveStart)}, "end": {fmt.Sprint(b.End)}, "step": {fmt.Sprint(b.Step)}, "dedup": {"false"}, "limit": {"2"}, "timeout": {"10s"}}
	if h.view.Mode == "project" {
		q.Set("namespace", r.Resource.Namespace)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.view.Origin+"/api/v1/query_range?"+q.Encode(), nil)
	if err != nil {
		return HistoryMetric{State: "unsupported"}
	}
	if h.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+h.bearer)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		var cert *tls.CertificateVerificationError
		if errors.As(err, &cert) {
			return HistoryMetric{State: "tls"}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return HistoryMetric{State: "timeout"}
		}
		return HistoryMetric{State: "transport"}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 401, 403:
		return HistoryMetric{State: "forbidden"}
	case 429:
		return HistoryMetric{State: "busy"}
	case 503, 504:
		return HistoryMetric{State: "timeout"}
	default:
		return HistoryMetric{State: "transport"}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return HistoryMetric{State: "timeout"}
		}
		return HistoryMetric{State: "transport"}
	}
	if len(raw) > 1<<20 {
		return HistoryMetric{State: "capacity"}
	}
	return projectHistory(raw, labels, r.Resource.UID, cpu, b.EffectiveStart, b.End, b.Step)
}
func projectHistory(raw []byte, labels map[string]string, uid string, cpu bool, start, end, step int64) HistoryMetric {
	bad := HistoryMetric{State: "unsupported"}
	var env struct {
		Status          string
		Warnings, Infos []string
		Data            struct {
			ResultType string
			Result     []struct {
				Metric map[string]string
				Values [][]json.RawMessage
			}
		}
	}
	if json.Unmarshal(raw, &env) != nil || env.Status != "success" || env.Data.ResultType != "matrix" || env.Data.Result == nil {
		return bad
	}
	m := HistoryMetric{State: "missing", Qualified: len(env.Warnings)+len(env.Infos) > 0}
	if len(env.Data.Result) == 0 {
		return m
	}
	if len(env.Data.Result) > 1 {
		m.State = "ambiguous"
		return m
	}
	series := env.Data.Result[0]
	for k, v := range labels {
		if series.Metric[k] != v {
			return bad
		}
	}
	for _, k := range []string{"uid", "pod_uid", "node_uid"} {
		if v, ok := series.Metric[k]; ok && v != uid {
			return bad
		}
	}
	if cpu && series.Metric["cpu"] != "" && series.Metric["cpu"] != "total" {
		return bad
	}
	if series.Values == nil {
		return bad
	}
	if len(series.Values) > 289 {
		return HistoryMetric{State: "capacity"}
	}
	prev := start - step
	for _, pair := range series.Values {
		if len(pair) != 2 {
			return bad
		}
		var ts float64
		var value string
		if string(pair[0]) == "null" || json.Unmarshal(pair[0], &ts) != nil || ts < float64(start) || ts > float64(end) || math.Trunc(ts) != ts {
			return bad
		}
		at := int64(ts)
		if at <= prev || at%step != 0 {
			return bad
		}
		prev = at
		if string(pair[1]) == "null" {
			continue
		}
		if json.Unmarshal(pair[1], &value) != nil {
			return bad
		}
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return bad
		}
		if math.IsInf(n, 0) || math.IsNaN(n) {
			continue
		}
		if n < 0 {
			return bad
		}
		if cpu {
			n *= 1000
		}
		if math.IsInf(n, 0) {
			return bad
		}
		m.Points = append(m.Points, HistoryPoint{Time: at, Value: n})
	}
	if len(m.Points) > 0 {
		m.State = "available"
	}
	return m
}
