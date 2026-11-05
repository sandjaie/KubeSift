/*
Purpose: Keep the validation write surface confined to recorded local fixtures.
Used by: Repository checks for the desktop adapter.
Responsibilities: Reject copied labels, substituted sources, remote endpoints and mismatched ownership.
Important: Synthetic temporary configuration only; no ambient kubeconfig or cluster access.
*/
package desktop

import (
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFixtureRestriction(t *testing.T) {
	for _, name := range []string{"owned", "different-source", "remote", "wrong-run", "context-label"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "kubeconfig")
			t.Setenv("KUBESIFT_TEST_KUBECONFIG", path)
			contextName := "kind-dev-local"
			server := "https://127.0.0.1:12345"
			run := "run-a"
			if name == "remote" {
				server = "https://example.com:6443"
			}
			if name == "context-label" {
				contextName = "copied-label"
			}
			if name == "wrong-run" {
				run = "other-run"
			}
			raw := map[string]any{"apiVersion": "v1", "kind": "Config", "clusters": []any{map[string]any{"name": "a", "cluster": map[string]any{"server": server}}}, "contexts": []any{map[string]any{"name": contextName, "context": map[string]any{"cluster": "a", "namespace": "payments"}}}}
			saveJSON(t, path, raw)
			saveJSON(t, path+".targets.json", map[string]any{"version": 1, "runId": "run-a", "a": map[string]any{"context": "kind-dev-local", "namespace": "payments", "server": server}, "b": map[string]any{"context": "kind-kubesift-b", "namespace": "payments", "server": "https://127.0.0.1:54321"}})
			saveJSON(t, filepath.Join(dir, "ownership.json"), map[string]any{"version": 1, "runId": run, "contextA": "kind-dev-local", "contextB": "kind-kubesift-b", "phase": "ready", "createdB": true, "containerB": "synthetic", "resources": []any{map[string]any{"context": "kind-dev-local", "kind": "ConfigMap", "name": "ks-marker", "uid": "owned-uid"}}})
			selected := path
			if name == "different-source" {
				selected = filepath.Join(dir, "copied")
				saveJSON(t, selected, raw)
			}
			snapshot, err := config.Load([]string{selected})
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := snapshot.Resolve(contextName)
			if err != nil {
				t.Fatal(err)
			}
			fixture, err := localFixture(resolved)
			if name == "owned" {
				if err != nil {
					t.Fatal(err)
				}
				if fixture.UID != "owned-uid" || fixture.Target.Namespace != "payments" || fixture.Target.Name != "ks-marker" {
					t.Fatalf("incorrect fixture: %+v", fixture)
				}
			} else if err == nil {
				t.Fatal("unverified fixture was allowed")
			}
		})
	}
}
func saveJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
