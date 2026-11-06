/*
Purpose: Verify Pod watch resynchronization after an expired resource version.
Used by: Focused Kubernetes adapter tests.
Responsibilities: Exercise real list/watch requests through a synthetic loopback API and assert cleanup.
Important: The fixture accepts GET only and performs no cluster mutation.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
)

func TestWatchPodsExpiredResourceVersionRelists(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	listCount := 0
	secondWatch := make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Query().Get("watch")+" "+r.URL.Query().Get("resourceVersion"))
		mu.Unlock()
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/namespaces/payments/pods" {
			http.Error(w, "unexpected request", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("watch") != "true" {
			listCount++
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"PodList","metadata":{"resourceVersion":"%d"},"items":[]}`, listCount)
			return
		}
		if r.URL.Query().Get("resourceVersion") == "1" {
			fmt.Fprintln(w, `{"type":"ERROR","object":{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Expired","code":410}}`)
			return
		}
		secondWatch <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	data := fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"synthetic","cluster":{"server":%q,"insecure-skip-tls-verify":true}}],"contexts":[{"name":"synthetic","context":{"cluster":"synthetic","user":"synthetic","namespace":"payments"}}],"users":[{"name":"synthetic","user":{}}]}`, server.URL)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := config.Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := snapshot.Resolve("synthetic")
	if err != nil {
		t.Fatal(err)
	}
	connections := connection.NewManager(nil)
	events := make(chan model.Event, 32)
	sessions := session.NewManager(func(event model.Event) { events <- event })
	service := NewService(connections, sessions)
	ref, err := connections.Open(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	watch, err := service.WatchPods(context.Background(), model.Target{Connection: ref, Namespace: "payments"})
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Activate(watch); err != nil {
		t.Fatal(err)
	}
	snapshots, stale := 0, false
	deadline := time.After(5 * time.Second)
	for snapshots < 2 || !stale {
		select {
		case event := <-events:
			switch event.Kind {
			case "snapshot":
				snapshots++
			case "state":
				if string(event.Data) == "stale: watch expired or overflowed; resynchronizing" {
					stale = true
				}
			case "error":
				t.Fatalf("watch error: %s", event.Data)
			}
		case <-deadline:
			t.Fatalf("recovery timeout: snapshots=%d stale=%v", snapshots, stale)
		}
	}
	select {
	case <-secondWatch:
	case <-deadline:
		t.Fatal("second watch did not start")
	}
	if err := sessions.Stop(watch); err != nil {
		t.Fatal(err)
	}
	if sessions.Stats().Sessions != 0 {
		t.Fatal("watch session remained after stop")
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"GET  ", "GET true 1", "GET  ", "GET true 2"}
	if len(requests) < len(want) {
		t.Fatalf("requests=%v", requests)
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("requests=%v", requests)
		}
	}
}
