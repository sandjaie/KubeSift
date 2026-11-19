/*
Purpose: Verify current resource metrics semantics and guarded reads.
Used by: Native Kubernetes reader tests.
Responsibilities: Cover exact quantities, coverage, freshness and identity.
Important: All samples are synthetic unless the opt-in integration test runs.
*/
package kube

import (
	"context"
	"fmt"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetricsProjection(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	pod := core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "sample", Namespace: "test", UID: "uid", CreationTimestamp: metav1.NewTime(now.Add(-time.Hour))},
		Spec:   core.PodSpec{Containers: []core.Container{{Name: "app", Resources: core.ResourceRequirements{Requests: core.ResourceList{core.ResourceCPU: resource.MustParse("500m")}}}}},
		Status: core.PodStatus{ContainerStatuses: []core.ContainerStatus{{Name: "app", State: core.ContainerState{Running: &core.ContainerStateRunning{}}}}}}
	target := ResourceRef{Kind: "Pod", APIVersion: "v1", Name: "sample", Namespace: "test", UID: "uid"}
	for _, tc := range []struct{ cpu, mem, want string }{{"125000000n", "80Mi", "125"}, {"1n", "1", "0.000001"}, {"0", "0", "0"}, {"2", "9007199254740993", "2000"}} {
		sample := metricsWire{Metadata: metav1.ObjectMeta{Name: "sample", Namespace: "test"}, Timestamp: metav1.NewTime(now), Window: "30s",
			Containers: []containerMetric{{Name: "app", Usage: map[string]string{"cpu": tc.cpu, "memory": tc.mem}}}}
		out, err := projectMetrics(target, sample, &pod, nil, true, "metrics.k8s.io/v1", now)
		if err != nil || out.CPU.Amount == nil || *out.CPU.Amount != tc.want || !out.CPUComplete {
			t.Fatalf("%+v %v", out, err)
		}
		if tc.want == "125" && (*out.Containers[0].CPU.RequestPercent != "25" || *out.Memory.Amount != "83886080") {
			t.Fatal("incorrect conversion")
		}
		if out.CPU.RequestPercent != nil {
			t.Fatal("Pod must not have a percentage")
		}
	}
	for _, bad := range []string{"-1", "invalid"} {
		if _, err := metricDecimal(bad, true); err == nil {
			t.Fatal("accepted invalid quantity")
		}
	}
	partial := metricsWire{Metadata: metav1.ObjectMeta{Name: "sample", Namespace: "test"}, Timestamp: metav1.NewTime(now), Window: "30s",
		Containers: []containerMetric{{Name: "app", Usage: map[string]string{"cpu": "1m"}}}}
	out, err := projectMetrics(target, partial, &pod, nil, true, "metrics.k8s.io/v1", now)
	if err != nil || out.MemoryComplete || out.Memory.Amount != nil || out.State != "partial" {
		t.Fatalf("lost missing memory %+v %v", out, err)
	}
	for _, variant := range []string{"coverage", "node", "old", "future", "before-creation", "unknown", "limit"} {
		t.Run(variant, func(t *testing.T) {
			p := pod.DeepCopy()
			s := partial
			s.Containers = append([]containerMetric(nil), partial.Containers...)
			var n *core.Node
			ref := target
			switch variant {
			case "coverage":
				p.Spec.InitContainers = []core.Container{{Name: "sidecar"}, {Name: "completed"}}
				p.Spec.EphemeralContainers = []core.EphemeralContainer{{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug"}}}
				p.Status.InitContainerStatuses = []core.ContainerStatus{{Name: "sidecar", State: core.ContainerState{Running: &core.ContainerStateRunning{}}}}
				p.Status.EphemeralContainerStatuses = []core.ContainerStatus{{Name: "debug", State: core.ContainerState{Running: &core.ContainerStateRunning{}}}}
				p.Spec.Resources = &core.ResourceRequirements{}
				s.Containers[0].Usage = map[string]string{"cpu": "1", "memory": "9007199254740993"}
				s.Containers = append(s.Containers, containerMetric{Name: "sidecar", Usage: map[string]string{"cpu": "1n", "memory": "1"}})
			case "node":
				n = &core.Node{ObjectMeta: p.ObjectMeta}
				n.Namespace = ""
				n.Status.Allocatable = core.ResourceList{core.ResourceCPU: resource.MustParse("2")}
				p = nil
				ref.Kind, ref.Namespace, s.Metadata.Namespace = "Node", "", ""
				s.Usage = map[string]string{"cpu": "3", "memory": "80Mi"}
			case "old":
				s.Timestamp = metav1.NewTime(now.Add(-61 * time.Second))
			case "future":
				s.Timestamp = metav1.NewTime(now.Add(31 * time.Second))
			case "before-creation":
				p.CreationTimestamp = metav1.NewTime(now.Add(-time.Second))
			case "unknown":
				s.Containers[0].Name = "unknown"
			case "limit":
				s.Containers = make([]containerMetric, 129)
			}
			got, err := projectMetrics(ref, s, p, n, true, "metrics.k8s.io/v1", now)
			if variant == "future" || variant == "before-creation" || variant == "unknown" || variant == "limit" {
				if err == nil {
					t.Fatal("accepted invalid sample")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if variant == "old" && got.State != "stale" {
				t.Fatal(got.State)
			}
			if variant == "node" && (*got.CPU.AllocatablePercent != "150" || got.Memory.Allocatable != nil) {
				t.Fatal(got)
			}
			if variant == "coverage" && (!got.PodLevelBudget || got.State != "partial" || len(got.MissingContainers) != 1 || got.MissingContainers[0] != "debug" || *got.Memory.Amount != "9007199254740994" || *got.Containers[0].CPU.RequestPercent != "200" || got.Containers[0].CPU.Limit != nil) {
				t.Fatalf("%+v", got)
			}
		})
	}

	partial.Containers = append(partial.Containers, partial.Containers[0])
	if _, err := projectMetrics(target, partial, &pod, nil, true, "metrics.k8s.io/v1", now); err == nil {
		t.Fatal("duplicate counted twice")
	}
}

func TestCurrentMetricsReader(t *testing.T) {
	t.Run("bridge payload bound", func(t *testing.T) {
		if _, err := boundedMetricsBatch(MetricsBatch{Items: []ResourceUsage{{WindowSeconds: strings.Repeat("x", 2<<20)}}}); err == nil {
			t.Fatal("accepted oversized bridge result")
		}
	})

	t.Run("shared capacity and cancellation", func(t *testing.T) {
		var active, peak atomic.Int32
		started := make(chan struct{}, 20)
		s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/apis/metrics.k8s.io":
				fmt.Fprint(w, `{"versions":[{"groupVersion":"metrics.k8s.io/v1","version":"v1"}]}`)
			case "/apis/metrics.k8s.io/v1":
				fmt.Fprint(w, `{"resources":[{"name":"pods","kind":"PodMetrics","namespaced":true,"verbs":["get"]}]}`)
			default:
				count := active.Add(1)
				defer active.Add(-1)
				for old := peak.Load(); count > old && !peak.CompareAndSwap(old, count); old = peak.Load() {
				}
				started <- struct{}{}
				<-r.Context().Done()
			}
		})
		targets := make([]ResourceRef, 10)
		for i := range targets {
			targets[i] = ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Pod", Namespace: "test", Name: fmt.Sprintf("pod-%d", i), UID: fmt.Sprint(i)}
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{}, 2)
		for range 2 {
			go func() {
				defer func() { done <- struct{}{} }()
				_, _ = s.ReadCurrentMetrics(ctx, MetricsRequest{Connection: ref, Targets: targets})
			}()
		}
		for range 4 {
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("workers did not start")
			}
		}
		_, err := s.ReadCurrentMetrics(ctx, MetricsRequest{Connection: ref, Targets: targets})
		if p := metricsProblem(ctx, err); p.Code != "busy" {
			t.Fatalf("expected batch capacity: %v", err)
		}
		cancel()
		for range 2 {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation did not release batch")
			}
		}
		if peak.Load() > 4 || len(s.metricsWorkers) != 0 || len(s.metricsBatches) != 0 {
			t.Fatal("capacity leaked", peak.Load())
		}
		if _, err = s.ReadCurrentMetrics(context.Background(), MetricsRequest{Connection: ref, Targets: make([]ResourceRef, 101)}); err == nil {
			t.Fatal("accepted 101 targets")
		}
	})

	t.Run("connection close cancels pending read", func(t *testing.T) {
		started := make(chan struct{})
		s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/apis/metrics.k8s.io":
				fmt.Fprint(w, `{"versions":[{"groupVersion":"metrics.k8s.io/v1","version":"v1"}]}`)
			case "/apis/metrics.k8s.io/v1":
				fmt.Fprint(w, `{"resources":[{"name":"pods","kind":"PodMetrics","namespaced":true,"verbs":["get"]}]}`)
			default:
				close(started)
				<-r.Context().Done()
			}
		})
		target := ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Pod", Namespace: "test", Name: "sample", UID: "uid"}
		done := make(chan error, 1)
		go func() {
			_, err := s.ReadCurrentMetrics(context.Background(), MetricsRequest{Connection: ref, Targets: []ResourceRef{target}})
			done <- err
		}()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("pending metric read did not start")
		}
		if err := s.connections.Close(ref); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("closed connection returned evidence")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("connection close did not cancel read")
		}
		if len(s.metricsWorkers) != 0 || len(s.metricsBatches) != 0 {
			t.Fatal("connection close leaked capacity")
		}
	})

	for _, mode := range []string{"v1", "v1beta1", "absent", "forbidden", "missing", "replacement", "oversize", "unavailable", "sample-forbidden", "before-creation"} {
		t.Run(mode, func(t *testing.T) {
			var groupReads atomic.Int32
			now := time.Now().UTC().Truncate(time.Second)
			s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("metrics wrote data")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/apis/metrics.k8s.io":
					groupReads.Add(1)
					if mode == "absent" {
						w.WriteHeader(404)
						return
					}
					if mode == "forbidden" {
						w.WriteHeader(403)
						return
					}
					version := "v1"
					if mode == "v1beta1" {
						version = "v1beta1"
					}
					fmt.Fprintf(w, `{"name":"metrics.k8s.io","versions":[{"groupVersion":"metrics.k8s.io/%s","version":"%s"}]}`, version, version)
				case "/apis/metrics.k8s.io/v1", "/apis/metrics.k8s.io/v1beta1":
					fmt.Fprint(w, `{"resources":[{"name":"pods","kind":"PodMetrics","namespaced":true,"verbs":["get"]},{"name":"nodes","kind":"NodeMetrics","namespaced":false,"verbs":["get"]}]}`)
				case "/apis/metrics.k8s.io/v1/namespaces/test/pods/sample", "/apis/metrics.k8s.io/v1beta1/namespaces/test/pods/sample":
					if mode == "unavailable" {
						w.WriteHeader(503)
						return
					}
					if mode == "sample-forbidden" {
						w.WriteHeader(403)
						return
					}
					if mode == "missing" {
						w.WriteHeader(404)
						return
					}
					if mode == "oversize" {
						fmt.Fprint(w, strings.Repeat("x", 262145))
						return
					}
					fmt.Fprintf(w, `{"apiVersion":"metrics.k8s.io/%s","kind":"PodMetrics","metadata":{"name":"sample","namespace":"test"},"timestamp":%q,"window":"30s","containers":[{"name":"app","usage":{"cpu":"125m","memory":"80Mi"}}]}`, map[bool]string{true: "v1beta1", false: "v1"}[mode == "v1beta1"], now.Format(time.RFC3339))
				case "/api/v1/namespaces/test/pods/sample":
					uid := "uid"
					if mode == "replacement" {
						uid = "new"
					}
					created := now.Add(-time.Hour)
					if mode == "before-creation" {
						created = now
					}
					fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"sample","namespace":"test","uid":%q,"creationTimestamp":%q},"spec":{"containers":[{"name":"app"}]},"status":{"containerStatuses":[{"name":"app","state":{"running":{}}}]}}`, uid, created.Format(time.RFC3339))
				default:
					t.Errorf("unexpected %s", r.URL.Path)
					w.WriteHeader(404)
				}
			})
			target := ResourceRef{Connection: ref, Kind: "Pod", APIVersion: "v1", Namespace: "test", Name: "sample", UID: "uid"}
			out, err := s.ReadCurrentMetrics(context.Background(), MetricsRequest{Connection: ref, Targets: []ResourceRef{target}, Detail: &target})
			if err != nil || len(out.Items) != 1 {
				t.Fatalf("%+v %v", out, err)
			}
			if mode == "v1" || mode == "v1beta1" {
				if out.Items[0].CPU.Amount == nil || *out.Items[0].CPU.Amount != "125" {
					t.Fatalf("%+v", out)
				}
			} else if out.Items[0].Problem == nil {
				t.Fatalf("expected classified unavailable sample %+v", out)
			}
			if mode == "forbidden" || mode == "absent" {
				_, _ = s.ReadCurrentMetrics(context.Background(), MetricsRequest{Connection: ref, Targets: []ResourceRef{target}})
				if groupReads.Load() != 1 {
					t.Fatal("repeated shared discovery failure", groupReads.Load())
				}
			}

		})
	}
}
