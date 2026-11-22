//go:build integration

/*
Purpose: Verify OpenShift API absence and retained Namespace browsing on ordinary Kubernetes.
Used by: Phase 1F.1 opt-in kind acceptance.
Responsibilities: Open each named local kind cluster using an isolated temporary config.
Important: Reads only, with explicit cluster names and loopback validation.
*/
package integration

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/session"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectsAbsentOnKind(t *testing.T) {
	if os.Getenv("KUBESIFT_PROJECTS_KIND") != "1" {
		t.Skip("explicit kind opt-in required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	manager := connection.NewManager(nil)
	defer manager.CloseAll()
	sessions := session.NewManager(nil)
	defer sessions.Close()
	reader := kube.NewService(manager, sessions)
	for _, suffix := range []string{"a", "b"} {
		t.Run("kind-dev-local-"+suffix, func(t *testing.T) {
			dir := t.TempDir()
			data, err := exec.CommandContext(ctx, "kind", "get", "kubeconfig", "--name", "dev-local-"+suffix).Output()
			if err != nil {
				t.Fatal("Cannot load named kind fixture", err)
			}
			if err := os.WriteFile(filepath.Join(dir, suffix+".kubeconfig"), data, 0600); err != nil {
				t.Fatal(err)
			}
			ref, _ := openLocalOperationFixture(t, ctx, manager, dir, suffix, "")
			caps, err := reader.ProjectCapabilities(ctx, ref, true)
			if err != nil || caps != (kube.ProjectCapabilities{}) {
				t.Fatal(caps, err)
			}
			workflows, err := reader.WorkflowDiscovery(ctx, ref, false)
			if err != nil || len(workflows.Resources) != 0 || len(workflows.Problems) != 0 {
				t.Fatalf("unexpected workflow discovery %+v %v", workflows, err)
			}
			page, err := reader.ReadConfiguration(ctx, kube.PageRequest{Connection: ref, Kind: "Namespace"})
			if err != nil || len(page.Items) == 0 {
				t.Fatal("Namespace inventory unavailable", err)
			}
			t.Logf("Projects, Routes, Builds and OLM absent; %d Namespaces visible", len(page.Items))
		})
	}
}
