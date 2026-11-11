//go:build integration

/*
Purpose: Validate independent product connections against the two approved local clusters.
Used by: The explicit opt-in Phase 1C.1 integration command.
Responsibilities: Check source ownership, namespace/Pod evidence and clean disconnects.
Important: Only isolated kubeconfigs naming kind-dev-local-a/b and loopback endpoints are accepted.
*/
package integration

import (
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/explorer"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	"github.com/sandjaie/KubeSift/internal/sources"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExplorerConnections(t *testing.T) {
	if os.Getenv("KUBESIFT_EXPLORER_INTEGRATION") != "1" {
		t.Skip("explicit KUBESIFT_EXPLORER_INTEGRATION=1 required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("explicit absolute isolated config directory required")
	}
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "absent"))
	r, e := sources.NewRegistry(filepath.Join(t.TempDir(), "sources.json"))
	if e != nil {
		t.Fatal(e)
	}
	manager := connection.NewManager(nil)
	sessions := session.NewManager(func(model.Event) {})
	defer sessions.Close()
	reader := kube.NewService(manager, sessions)
	c := explorer.NewCoordinator(r, manager, reader)
	defer c.Close()
	refs := []model.ConnectionRef{}
	uids := []string{}
	for _, suffix := range []string{"a", "b"} {
		source, e := c.AddSource(filepath.Join(dir, suffix+".kubeconfig"))
		if e != nil {
			t.Fatal(e)
		}
		if len(source.Contexts) != 1 || source.Contexts[0].Context != "kind-dev-local-"+suffix {
			t.Fatal("unexpected context in isolated source")
		}
		u, _ := url.Parse(source.Contexts[0].Endpoint)
		if u == nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
			t.Fatal("nonlocal endpoint rejected")
		}
		attempt, e := c.StartConnection(explorer.ConnectRequest{SourceID: source.SourceID, SnapshotID: source.SnapshotID, Context: source.Contexts[0].Context, Namespace: "kubesift-explorer"})
		if e != nil {
			t.Fatal(e)
		}
		var view explorer.ConnectionView
		deadline := time.After(35 * time.Second)
		tick := time.NewTicker(20 * time.Millisecond)
	wait:
		for {
			view, e = c.GetAttempt(attempt.ID)
			if e != nil {
				t.Fatal(e)
			}
			if view.State != "connecting" {
				break
			}
			select {
			case <-deadline:
				t.Fatal("connection deadline exceeded")
			case <-tick.C:
				continue wait
			}
		}
		tick.Stop()
		if view.State != "connected" || view.Ref == nil {
			t.Fatalf("connection failed: %s %+v", view.State, view.Problem)
		}
		found := false
		for _, p := range view.Pods {
			if p.Name == "same-pod" {
				if p.Marker != "local-"+suffix {
					t.Fatal("cross-cluster marker")
				}
				uids = append(uids, p.UID)
				found = true
			}
		}
		if !found {
			t.Fatal("owned fixture same-pod missing; run scripts/dev-clusters.sh --explorer")
		}
		refs = append(refs, *view.Ref)
	}
	if uids[0] == uids[1] || refs[0] == refs[1] {
		t.Fatal("A/B identity collision")
	}
	restricted, err := c.AddSource(filepath.Join(dir, "restricted.kubeconfig"))
	if err != nil {
		t.Fatal(err)
	}
	if len(restricted.Contexts) != 1 || restricted.Contexts[0].Context != "kind-dev-local-a" {
		t.Fatal("unexpected restricted context")
	}
	attempt, err := c.StartConnection(explorer.ConnectRequest{SourceID: restricted.SourceID, SnapshotID: restricted.SnapshotID, Context: "kind-dev-local-a", Namespace: "default"})
	if err != nil {
		t.Fatal(err)
	}
	await := func() explorer.ConnectionView {
		t.Helper()
		deadline := time.After(35 * time.Second)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			v, e := c.GetAttempt(attempt.ID)
			if e != nil {
				t.Fatal(e)
			}
			if v.State != "connecting" && !v.PodLoading {
				return v
			}
			select {
			case <-deadline:
				t.Fatal("restricted read timed out")
			case <-tick.C:
			}
		}
	}
	denied := await()
	if denied.State != "restricted" || denied.Ref == nil || denied.LastSuccessAt != "" {
		t.Fatalf("denied identity incorrectly proved access: %+v", denied.Problem)
	}
	if _, err = c.ReadPods(*denied.Ref, "kubesift-explorer"); err != nil {
		t.Fatal(err)
	}
	allowed := await()
	if allowed.State != "connected" || allowed.NamespacesComplete || len(allowed.Pods) == 0 {
		t.Fatal("manual namespace did not establish restricted read access", allowed.State)
	}
	refs = append(refs, *allowed.Ref)

	for _, ref := range refs {
		if e = c.Disconnect(ref); e != nil {
			t.Fatal(e)
		}
		if manager.Validate(ref) == nil {
			t.Fatal("closed connection remains usable")
		}
	}
}
