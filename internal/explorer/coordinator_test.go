/*
Purpose: Protect cancellation, replacement and independent source lifecycles.
Used by: Go tests.
Responsibilities: Hold probes at explicit barriers and inspect native attempt snapshots.
Important: No cluster, sleeps or credential processes are involved.
*/
package explorer

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/sources"
	"os"
	"path/filepath"
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
