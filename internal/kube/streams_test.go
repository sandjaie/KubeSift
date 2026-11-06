/*
Purpose: Verify Pod watch resynchronization after an expired resource version.
Used by: Focused Kubernetes adapter tests.
Responsibilities: Exercise real list/watch requests through a synthetic loopback API and assert cleanup.
Important: The fixture accepts GET only and performs no cluster mutation.
*/
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWatchPodsExpiredResourceVersionRelists(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	listCount := 0
	finalWatch := make(chan struct{}, 1)
	var closedConnections atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		version, _ := strconv.Atoi(r.URL.Query().Get("resourceVersion"))
		if version <= 3 {
			fmt.Fprintln(w, `{"type":"ERROR","object":{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Expired","code":410}}`)
			return
		}
		finalWatch <- struct{}{}
		<-r.Context().Done()
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closedConnections.Add(1)
		}
	}
	server.StartTLS()
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
	defer sessions.Stop(watch)
	snapshots, stale := 0, 0
	deadline := time.After(5 * time.Second)
	for snapshots < 4 || stale < 3 {
		select {
		case event := <-events:
			switch event.Kind {
			case "snapshot":
				snapshots++
			case "state":
				if string(event.Data) == "stale: watch expired or overflowed; resynchronizing" {
					stale++
				}
			case "error":
				t.Fatalf("watch error: %s", event.Data)
			}
		case <-deadline:
			t.Fatalf("recovery timeout: snapshots=%d stale=%v", snapshots, stale)
		}
	}
	select {
	case <-finalWatch:
	case <-deadline:
		t.Fatal("fourth watch did not start")
	}
	for closedConnections.Load() < 3 {
		select {
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			t.Fatalf("only %d old watch transports closed before final stop", closedConnections.Load())
		}
	}
	if err := sessions.Stop(watch); err != nil {
		t.Fatal(err)
	}
	if sessions.Stats().Sessions != 0 {
		t.Fatal("watch session remained after stop")
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"GET  ", "GET true 1", "GET  ", "GET true 2", "GET  ", "GET true 3", "GET  ", "GET true 4"}
	if len(requests) < len(want) {
		t.Fatalf("requests=%v", requests)
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("requests=%v", requests)
		}
	}
}

func TestWatchPodsRejectedSnapshotRelistsBeforeWatch(t *testing.T) {
	var mu sync.Mutex
	lists, watches := 0, map[int]bool{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/namespaces/payments/pods" {
			http.Error(w, "unexpected request", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("watch") == "true" {
			version, _ := strconv.Atoi(r.URL.Query().Get("resourceVersion"))
			mu.Lock()
			watches[version] = true
			mu.Unlock()
			fmt.Fprintln(w, `{"type":"ERROR","object":{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Expired","code":410}}`)
			return
		}
		mu.Lock()
		lists++
		version := lists
		mu.Unlock()
		pods := make([]corev1.Pod, 100)
		for i := range pods {
			pods[i] = corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%d", i), Namespace: "payments", Annotations: map[string]string{"kubesift.dev/hostile": strings.Repeat("x", 450)}}}
		}
		data, _ := json.Marshal(corev1.PodList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"}, ListMeta: metav1.ListMeta{ResourceVersion: strconv.Itoa(version)}, Items: pods})
		_, _ = w.Write(data)
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
	sessions.RequireBridgeAcknowledgements()
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
	defer sessions.Stop(watch)
	deadline := time.After(12 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Kind == "snapshot" {
				goto snapshotDelivered
			}
			if event.Kind == "error" {
				t.Fatalf("watch error: %s", event.Data)
			}
		case <-deadline:
			t.Fatal("first snapshot never delivered")
		}
	}
snapshotDelivered:
	seenHigh := false
	for {
		queued := sessions.Stats().QueuedBytes
		if queued > 800<<10 {
			seenHigh = true
		}
		if seenHigh && queued < 100<<10 {
			break
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			t.Fatalf("snapshot queue did not overflow: high=%v queued=%d", seenHigh, queued)
		}
	}
	mu.Lock()
	overflowList := lists
	mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	watchedOverflowVersion := watches[overflowList]
	mu.Unlock()
	if watchedOverflowVersion {
		t.Fatalf("watch opened at RV %d after its snapshot was rejected", overflowList)
	}
}
