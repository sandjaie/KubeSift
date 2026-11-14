/*
Purpose: Verify availability reference allowlists and exact target snapshots.
Used by: Resolver regression tests.
Responsibilities: Reject stale sources, forged routes and replaced or unrelated targets.
Important: All requests go to a synthetic read-only API.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestAvailabilityReferenceRoutes(t *testing.T) {
	const goodOwner = `"ownerReferences":[{"apiVersion":"apps/v1","kind":"StatefulSet","name":"object","uid":"uid","controller":true}]`
	for _, tc := range []struct {
		from, to, version, body, targetMeta, association string
		valid                                            bool
	}{
		{"HorizontalPodAutoscaler", "Deployment", "apps/v1", `"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"Deployment","name":"object"}}`, "", "scale-target", true},
		{"HorizontalPodAutoscaler", "StatefulSet", "apps/v1", `"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"StatefulSet","name":"object"}}`, "", "scale-target", true},
		{"HorizontalPodAutoscaler", "ReplicaSet", "apps/v1", `"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"ReplicaSet","name":"object"}}`, "", "scale-target", true},
		{"HorizontalPodAutoscaler", "ReplicationController", "v1", `"spec":{"scaleTargetRef":{"apiVersion":"v1","kind":"ReplicationController","name":"object"}}`, "", "scale-target", true},
		{"HorizontalPodAutoscaler", "DaemonSet", "apps/v1", `"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"DaemonSet","name":"object"}}`, "", "", false},
		{"PodDisruptionBudget", "Pod", "v1", `"spec":{"selector":{"matchLabels":{"app":"selected"}}}`, `"labels":{"app":"selected"}`, "selector-match", true},
		{"PodDisruptionBudget", "Pod", "v1", `"spec":{"selector":null},"status":{"disruptedPods":{"object":"2026-11-14T00:00:00Z"}}`, "", "", false},
		{"StatefulSet", "ControllerRevision", "apps/v1", `"spec":{}`, goodOwner, "controller-owner", true},
		{"DaemonSet", "ControllerRevision", "apps/v1", `"spec":{}`, `"ownerReferences":[{"apiVersion":"apps/v1","kind":"DaemonSet","name":"object","uid":"uid","controller":true}]`, "controller-owner", true},
		{"ControllerRevision", "StatefulSet", "apps/v1", `"revision":9`, "", "controller-owner", true},
	} {
		t.Run(tc.from+"-"+tc.to+"-"+tc.association, func(t *testing.T) {
			versions := map[string]string{"HorizontalPodAutoscaler": "autoscaling/v2", "PodDisruptionBudget": "policy/v1", "StatefulSet": "apps/v1", "DaemonSet": "apps/v1", "ControllerRevision": "apps/v1"}
			plurals := map[string]string{"HorizontalPodAutoscaler": "horizontalpodautoscalers", "PodDisruptionBudget": "poddisruptionbudgets", "Deployment": "deployments", "ReplicaSet": "replicasets", "ReplicationController": "replicationcontrollers", "StatefulSet": "statefulsets", "DaemonSet": "daemonsets", "ControllerRevision": "controllerrevisions", "Pod": "pods"}
			source := ResourceRef{Kind: tc.from, APIVersion: versions[tc.from], Name: "object", Namespace: "test", UID: "uid"}
			target := ResourceRef{Kind: tc.to, APIVersion: tc.version, Name: "object", Namespace: "test"}
			if tc.from != "HorizontalPodAutoscaler" && tc.from != "ControllerRevision" {
				target.UID = "target"
			}
			path := func(ref ResourceRef) string {
				prefix := "/apis/"
				if ref.APIVersion == "v1" {
					prefix = "/api/"
				}
				return prefix + ref.APIVersion + "/namespaces/test/" + plurals[ref.Kind] + "/object"
			}
			var reads atomic.Int32
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method != "GET" {
					t.Error(r.Method)
				}
				switch r.URL.Path {
				case path(source):
					meta := ""
					if tc.from == "ControllerRevision" {
						meta = `,"ownerReferences":[{"apiVersion":"apps/v1","kind":"StatefulSet","name":"object","uid":"target","controller":true}]`
					}
					fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"name":"object","namespace":"test","uid":"uid","resourceVersion":"7"%s},%s}`, source.Kind, source.APIVersion, meta, tc.body)
				case path(target):
					reads.Add(1)
					meta := ""
					if tc.targetMeta != "" {
						meta = "," + tc.targetMeta
					}
					fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"name":"object","namespace":"test","uid":"target","resourceVersion":"8"%s}}`, target.Kind, target.APIVersion, meta)
				default:
					t.Error(r.URL)
					http.NotFound(w, r)
				}
			})
			source.Connection, target.Connection = c, c
			req := AvailabilityReferenceRequest{Source: source, Target: target, SourceVersion: "7"}
			stale := req
			stale.SourceVersion = "6"
			ctx := context.Background()
			if _, err := s.ResolveAvailabilityReference(ctx, stale); err == nil || reads.Load() != 0 {
				t.Fatal("stale reached target")
			}
			forged := req
			forged.Target.Namespace = "other"
			if _, err := s.ResolveAvailabilityReference(ctx, forged); err == nil || reads.Load() != 0 {
				t.Fatal("forged scope reached target")
			}
			got, err := s.ResolveAvailabilityReference(ctx, req)
			if !tc.valid {
				if err == nil || reads.Load() != 0 {
					t.Fatal(got, err)
				}
				return
			}
			if err != nil || reads.Load() != 1 || got.Resource.UID != "target" || got.Detail.Resource != got.Resource || got.Detail.ResourceVersion != "8" || got.SourceVersion != "7" || got.Association != tc.association {
				t.Fatal(got, err)
			}
			req.Target.UID = "replaced"
			if _, err := s.ResolveAvailabilityReference(ctx, req); err == nil {
				t.Fatal("replaced target accepted")
			}
		})
	}
}
