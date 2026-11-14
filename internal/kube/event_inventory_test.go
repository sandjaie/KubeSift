/*
Purpose: Verify namespace Event inventory and exact target captures.
Used by: Kubernetes reader tests.
Responsibilities: Check absence-only fallback, source version and target UID.
Important: Synthetic API only.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestEventInventoryTarget(t *testing.T) {
	ctx := context.Background()
	targetUID := "pod-1"
	modernStatus := 404
	modernCalls, podCalls := 0, 0
	event := `{"kind":"Event","apiVersion":"v1","metadata":{"name":"notice","namespace":"default","uid":"event-1","resourceVersion":"9"},"involvedObject":{"apiVersion":"v1","kind":"Pod","name":"sample","namespace":"default","uid":"pod-1"},"message":"Scheduled"}`
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/events.k8s.io/v1/namespaces/default/events":
			modernCalls++
			w.WriteHeader(modernStatus)
		case "/api/v1/namespaces/default/events":
			fmt.Fprintf(w, `{"kind":"EventList","apiVersion":"v1","metadata":{"continue":"next"},"items":[%s]}`, event)
		case "/api/v1/namespaces/default/events/notice":
			fmt.Fprint(w, event)
		case "/api/v1/namespaces/default/pods/sample":
			podCalls++
			fmt.Fprintf(w, `{"kind":"Pod","apiVersion":"v1","metadata":{"name":"sample","namespace":"default","uid":%q,"resourceVersion":"12"}}`, targetUID)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	req := PageRequest{Connection: ref, Kind: "Event", Namespace: "default"}
	page, err := s.ReadEventInventory(ctx, req)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("inventory %+v: %v", page, err)
	}
	item := page.Items[0]
	if item.Resource.APIVersion != "v1" || item.Event.Count != nil || item.Event.LastAt != nil || item.Target == nil {
		t.Fatalf("normalized evidence %+v", item)
	}
	req.Cursor = page.Page.NextCursor
	if _, err := s.ReadEventInventory(ctx, req); err != nil || modernCalls != 1 {
		t.Fatalf("continuation changed Event API: %d %v", modernCalls, err)
	}
	resolved, err := s.ResolveEventTarget(ctx, EventTargetRequest{Source: item.Resource, SourceVersion: "9"})
	if err != nil || resolved.Detail.Resource.UID != "pod-1" || podCalls != 1 {
		t.Fatalf("resolution %+v: %v, gets=%d", resolved, err, podCalls)
	}
	if _, err := s.ResolveEventTarget(ctx, EventTargetRequest{Source: item.Resource, SourceVersion: "8"}); err == nil || podCalls != 1 {
		t.Fatal("stale Event version read target")
	}
	targetUID = "recreated"
	if _, err := s.ResolveEventTarget(ctx, EventTargetRequest{Source: item.Resource, SourceVersion: "9"}); err == nil || !strings.Contains(err.Error(), "resource_replaced") {
		t.Fatalf("expected UID protection: %v", err)
	}
	modernStatus = 403
	req.Cursor = ""
	if _, err := s.ReadEventInventory(ctx, req); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected denial without fallback: %v", err)
	}
}

func TestLeaseResourceEvents(t *testing.T) {
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/coordination.k8s.io/v1/namespaces/default/leases/fixture":
			fmt.Fprint(w, `{"kind":"Lease","apiVersion":"coordination.k8s.io/v1","metadata":{"name":"fixture","namespace":"default","uid":"lease-id"}}`)
		case "/apis/events.k8s.io/v1/namespaces/default/events":
			if r.URL.Query().Get("fieldSelector") != "regarding.uid=lease-id" {
				t.Error("Event filter did not bind the Lease UID")
			}
			fmt.Fprint(w, `{"kind":"EventList","apiVersion":"events.k8s.io/v1","items":[]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	lease := ResourceRef{Connection: ref, Kind: "Lease", APIVersion: "coordination.k8s.io/v1", Namespace: "default", Name: "fixture", UID: "lease-id"}
	if _, err := s.ReadResourceEvents(context.Background(), EventRequest{Resource: lease}); err != nil {
		t.Fatal(err)
	}
}

func TestEventConfigMapTargetUsesOverviewProjection(t *testing.T) {
	gets := 0
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/default/events/fixture":
			fmt.Fprint(w, `{"kind":"Event","apiVersion":"v1","metadata":{"name":"fixture","namespace":"default","uid":"event-id","resourceVersion":"3"},"involvedObject":{"kind":"ConfigMap","apiVersion":"v1","name":"configuration","namespace":"default","uid":"map-id"}}`)
		case "/api/v1/namespaces/default/configmaps/configuration":
			gets++
			fmt.Fprint(w, `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"configuration","namespace":"default","uid":"map-id","resourceVersion":"8"},"data":{"literal":"synthetic-value"},"binaryData":{"bytes":"AP8="}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	resolved, err := s.ResolveEventTarget(context.Background(), EventTargetRequest{
		Source: ResourceRef{Connection: ref, Kind: "Event", APIVersion: "v1", Namespace: "default", Name: "fixture", UID: "event-id"}, SourceVersion: "3"})
	if err != nil {
		t.Fatal(err)
	}
	raw := resolved.Detail.ObjectJSON
	if gets != 1 || strings.Contains(raw, "synthetic-value") || strings.Contains(raw, "AP8=") ||
		!strings.Contains(raw, `"textKeys":1`) || !strings.Contains(raw, `"binaryKeys":1`) {
		t.Fatalf("incorrect Overview evidence: %s; gets=%d", raw, gets)
	}
}
