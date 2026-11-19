/*
Purpose: Verify current resource metrics semantics and guarded reads.
Used by: Native Kubernetes reader tests.
Responsibilities: Cover exact quantities, coverage, freshness and identity.
Important: All samples are synthetic unless the opt-in integration test runs.
*/
package kube

import (
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
