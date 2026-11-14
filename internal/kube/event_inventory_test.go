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
