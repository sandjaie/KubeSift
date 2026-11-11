/*
Purpose: Protect cancellation, replacement and independent source lifecycles.
Used by: Go tests.
Responsibilities: Hold probes at explicit barriers and inspect native attempt snapshots.
Important: No cluster, sleeps or credential processes are involved.
*/
package explorer

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	"github.com/sandjaie/KubeSift/internal/sources"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

type heldProbe struct {
	entered chan context.Context
	release chan struct{}
}

func (p *heldProbe) ProbeAccess(ctx context.Context, _ model.ConnectionRef, ns, mode string) (kube.AccessResult, error) {
	p.entered <- ctx
	<-p.release
	return kube.AccessResult{AuthorizedRead: true, Namespace: ns, Probe: "pods"}, nil
}
func TestConnectionLifecycle(t *testing.T) {
	t.Run("external renewal and stale namespace evidence", func(t *testing.T) { testRenewal(t, true) })
	t.Run("external renewal and stale Pod evidence", func(t *testing.T) { testRenewal(t, false) })
	t.Run("two pending contexts in one source", testPendingContexts)
	dir := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(dir, "missing"))
	path := filepath.Join(dir, "a")
	os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\nclusters:\n- name: c\n  cluster:\n    server: https://synthetic.invalid\ncontexts:\n- name: same\n  context:\n    cluster: c\n    user: u\nusers:\n- name: u\n  user:\n    token: synthetic\n"), 0600)
	registry, _ := sources.NewRegistry(filepath.Join(dir, "sources.json"))
	source, _ := registry.Add(path)
	probe := &heldProbe{make(chan context.Context, 4), make(chan struct{}, 4)}
	c := NewCoordinator(registry, connection.NewManager(nil), probe)
	defer c.Close()
	a, e := c.StartConnection(ConnectRequest{SourceID: source.SourceID, SnapshotID: source.SnapshotID, Context: "same"})
	if e != nil {
		t.Fatal(e)
	}
	if v, e := c.GetAttempt(a.ID); e != nil || v.State != "connecting" {
		t.Fatal("attempt missing before probe", v, e)
	}
	ctx := <-probe.entered
	if e = c.CancelAttempt(a.ID); e != nil {
		t.Fatal(e)
	}
	if ctx.Err() == nil {
		t.Fatal("probe not cancelled")
	}
	probe.release <- struct{}{}
	v, _ := c.GetAttempt(a.ID)
	if v.State != "not_connected" {
		t.Fatal("cancel resurrected", v)
	}
	source, _ = c.ReloadSource(source.SourceID)
	b, e := c.StartConnection(ConnectRequest{SourceID: source.SourceID, SnapshotID: source.SnapshotID, Context: "same"})
	if e != nil {
		t.Fatal(e)
	}
	<-probe.entered
	if e = c.RemoveSource(source.SourceID, false); e == nil {
		t.Fatal("removal did not require confirmation")
	}
	if v, _ = c.GetAttempt(b.ID); v.State != "connecting" {
		t.Fatal("cancelled removal changed connection")
	}
	if e = c.RemoveSource(source.SourceID, true); e != nil {
		t.Fatal(e)
	}
	probe.release <- struct{}{}
	v, _ = c.GetAttempt(b.ID)
	if v.State != "not_connected" {
		t.Fatal("removed source resurrected")
	}
	now := time.Now()
	c.now = func() time.Time { return now.Add(6 * time.Minute) }
	if _, e = c.GetAttempt(a.ID); e == nil {
		t.Fatal("completed attempt did not expire")
	}
}

// Exercise actual guarded HTTP reads, with an injected scheduler rather than wall-clock waits.
func testRenewal(t *testing.T, namespaceListing bool) {
	var lastPath atomic.Value
	var expected atomic.Value
	expected.Store("Bearer first")
	var requests atomic.Int64
	var forbidPods atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		lastPath.Store(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != expected.Load().(string) {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Unauthorized","code":401}`)
			return
		}
		if (!namespaceListing && r.URL.Path == "/api/v1/namespaces") || (forbidPods.Load() && r.URL.Path != "/api/v1/namespaces") {
			w.WriteHeader(403)
			fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403}`)
			return
		}
		if r.URL.Path == "/api/v1/namespaces" {
			fmt.Fprint(w, `{"kind":"NamespaceList","apiVersion":"v1","items":[{"metadata":{"name":"default"}}]}`)
		} else {
			fmt.Fprint(w, `{"kind":"PodList","apiVersion":"v1","items":[]}`)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(dir, "missing"))
	path := filepath.Join(dir, "source")
	write := func(token string) {
		t.Helper()
		data := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: c\n  cluster:\n    server: %s\n    certificate-authority-data: %s\ncontexts:\n- name: same\n  context:\n    cluster: c\n    user: u\nusers:\n- name: u\n  user:\n    token: %s\n", server.URL, base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})), token)
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("first")
	r, _ := sources.NewRegistry(filepath.Join(dir, "sources.json"))
	source, _ := r.Add(path)
	manager := connection.NewManager(nil)
	sessions := session.NewManager(func(model.Event) {})
	defer sessions.Close()
	c := NewCoordinator(r, manager, kube.NewService(manager, sessions))
	defer c.Close()
	now := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	ticks := make(chan time.Time, 4)
	scheduled := make(chan time.Duration, 4)
	c.after = func(d time.Duration) <-chan time.Time { scheduled <- d; return ticks }
	start := func(source sources.SourceResult) Attempt {
		t.Helper()
		a, e := c.StartConnection(ConnectRequest{SourceID: source.SourceID, SnapshotID: source.SnapshotID, Context: "same"})
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	await := func(a Attempt, want string) ConnectionView {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			v, e := c.GetAttempt(a.ID)
			if e != nil {
				t.Fatal(e)
			}
			if v.State == want {
				return v
			}
			select {
			case <-deadline:
				t.Fatalf("wanted %s got %s %+v", want, v.State, v.Problem)
			default:
				runtime.Gosched()
			}
		}
	}
	a := start(source)
	if d := <-scheduled; d != 30*time.Second {
		t.Fatal("check cadence", d)
	}
	connected := await(a, "connected")
	if connected.LastSuccessAt == "" {
		t.Fatal("resource evidence missing")
	}

	forbidPods.Store(true)
	if _, e := c.ReadPods(*connected.Ref, "denied"); e != nil {
		t.Fatal(e)
	}
	waitDenied := time.After(2 * time.Second)
	for {
		v, _ := c.GetAttempt(a.ID)
		if !v.PodLoading {
			if v.State != "connected" || v.PodProblem == nil || v.PodProblem.Code != "forbidden" {
				t.Fatal("Pod 403 discarded established namespace evidence", v.State)
			}
			break
		}
		select {
		case <-waitDenied:
			t.Fatal("Pod read not completed")
		default:
			runtime.Gosched()
		}
	}
	<-scheduled
	forbidPods.Store(false)
	c.mu.Lock()
	now = now.Add(91 * time.Second)
	c.mu.Unlock()
	await(a, "stale")
	expected.Store("Bearer renewed")
	ticks <- now
	await(a, "auth_required")
	if !namespaceListing && lastPath.Load() != "/api/v1/namespaces/default/pods" {
		t.Fatal("periodic check lost successful namespace", lastPath.Load())
	}
	count := requests.Load()
	c.GetAttempt(a.ID)
	c.GetConnection(*connected.Ref)
	if requests.Load() != count {
		t.Fatal("snapshot polling made API requests")
	}
	select {
	case <-scheduled:
		t.Fatal("terminal auth failure scheduled retry")
	default:
	}
	write("renewed")
	source, _ = c.ReloadSource(source.SourceID)
	await(a, "stale")
	b := start(source)
	<-scheduled
	replacement := await(b, "connected")
	if *replacement.Ref == *connected.Ref {
		t.Fatal("Retry reused old generation")
	}
	if manager.Validate(*connected.Ref) == nil {
		t.Fatal("old reference survived Retry")
	}
	if e := c.Disconnect(*replacement.Ref); e != nil {
		t.Fatal(e)
	}
	await(b, "not_connected")
}

func testPendingContexts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(dir, "absent"))
	path := filepath.Join(dir, "config")
	data := `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster: {server: "https://synthetic.invalid"}
users:
- name: u
  user: {token: synthetic}
contexts:
- name: a
  context: {cluster: c, user: u}
- name: b
  context: {cluster: c, user: u}
`
	os.WriteFile(path, []byte(data), 0600)
	r, _ := sources.NewRegistry(filepath.Join(dir, "sources.json"))
	source, _ := r.Add(path)
	probe := &heldProbe{make(chan context.Context, 4), make(chan struct{}, 4)}
	c := NewCoordinator(r, connection.NewManager(nil), probe)
	defer c.Close()
	a, e := c.StartConnection(ConnectRequest{SourceID: source.SourceID, SnapshotID: source.SnapshotID, Context: "a"})
	if e != nil {
		t.Fatal(e)
	}
	ctx := <-probe.entered
	sources, _ := c.ListSources()
	for _, item := range sources {
		if item.SourceID == source.SourceID {
			source = item
		}
	}
	b, e := c.StartConnection(ConnectRequest{SourceID: source.SourceID, SnapshotID: source.SnapshotID, Context: "b"})
	if e != nil {
		probe.release <- struct{}{}
		t.Fatal(e)
	}
	<-probe.entered
	cancelled := ctx.Err() != nil
	c.CancelAttempt(a.ID)
	c.CancelAttempt(b.ID)
	probe.release <- struct{}{}
	probe.release <- struct{}{}
	if cancelled {
		t.Fatal("opening B cancelled unchanged pending A")
	}
}
