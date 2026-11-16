/*
Purpose: Exercise the product ownership boundary for interactive sessions.
Used by: Kubernetes adapter tests.
Responsibilities: Verify exact targets, retained capacity, cancellation and cleanup.
Important: The synthetic API never runs a shell or opens a cluster forward.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
)

func TestInteractiveOwnership(t *testing.T) {
	var runs atomic.Int32
	s, connection := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/namespaces/demo/pods/work" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"work","namespace":"demo","uid":"present"},"status":{"phase":"Running"}}`)
	})
	target := PodSessionTarget{Resource: ResourceRef{Connection: connection, APIVersion: "v1", Kind: "Pod", Namespace: "demo", Name: "work", UID: "present"}}
	open := func(ctx context.Context, target PodSessionTarget) (InteractiveEntry, error) {
		return s.reserveInteractive(ctx, InteractiveEntry{Kind: "forward", Target: target}, func(context.Context, *session.Writer, func(uint16) error) error {
			runs.Add(1)
			return nil
		})
	}
	ctx := context.Background()
	stale := target
	stale.Resource.UID = "replaced"
	if _, err := open(ctx, stale); err == nil {
		t.Fatal("same-name replacement admitted")
	}
	entries := make([]InteractiveEntry, 8)
	for i := range entries {
		entry, err := open(ctx, target)
		if err != nil || entry.State != "connecting" {
			t.Fatalf("pending reservation %d: %+v %v", i, entry, err)
		}
		entries[i] = entry
	}
	ref := entries[0].Session
	forged := model.SessionRef{ID: ref.ID, Connection: model.ConnectionRef{ID: connection.ID, Generation: connection.Generation + 1}}
	if err := s.CloseInteractive(forged); err == nil {
		t.Fatal("mismatched generation closed the session")
	}
	if err := s.StopInteractive(ref); err != nil {
		t.Fatal(err)
	}
	if err := s.sessions.Activate(ref); err == nil || runs.Load() != 0 {
		t.Fatal("stopped pending session activated")
	}
	if _, err := open(ctx, target); err == nil {
		t.Fatal("ninth retained entry admitted")
	}
	if err := s.CloseInteractive(ref); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := open(cancelled, target); err == nil {
		t.Fatal("cancelled reservation admitted")
	}
	if _, err := open(ctx, target); err != nil {
		t.Fatal(err)
	}
	if err := s.connections.Close(connection); err != nil || s.sessions.Stats().Sessions != 0 || len(s.interactive) != 0 {
		t.Fatalf("connection cleanup: %v, stats %+v, retained %d", err, s.sessions.Stats(), len(s.interactive))
	}
}
