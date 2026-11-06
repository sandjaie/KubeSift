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
	t.Run("packaged-evidence-boundary", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), ".local", "phase1a")
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, "kubeconfig")
		t.Setenv("KUBESIFT_TEST_KUBECONFIG", path)
		raw := map[string]any{"apiVersion": "v1", "kind": "Config", "clusters": []any{map[string]any{"name": "a", "cluster": map[string]any{"server": "https://127.0.0.1:12345"}}, map[string]any{"name": "b", "cluster": map[string]any{"server": "https://127.0.0.1:54321"}}}, "contexts": []any{map[string]any{"name": "kind-dev-local", "context": map[string]any{"cluster": "a", "namespace": "payments"}}, map[string]any{"name": "kind-kubesift-b", "context": map[string]any{"cluster": "b", "namespace": "payments"}}}}
		saveJSON(t, path, raw)
		saveJSON(t, path+".targets.json", map[string]any{"version": 1, "runId": "run-a", "a": map[string]any{"context": "kind-dev-local", "namespace": "payments", "server": "https://127.0.0.1:12345"}, "b": map[string]any{"context": "kind-kubesift-b", "namespace": "payments", "server": "https://127.0.0.1:54321"}})
		saveJSON(t, filepath.Join(directory, "ownership.json"), map[string]any{"version": 1, "runId": "run-a", "contextA": "kind-dev-local", "contextB": "kind-kubesift-b", "phase": "ready", "createdB": true, "containerB": "synthetic", "resources": []any{map[string]any{"context": "kind-dev-local", "kind": "ConfigMap", "name": "ks-marker", "uid": "a"}, map[string]any{"context": "kind-kubesift-b", "kind": "ConfigMap", "name": "ks-marker", "uid": "b"}}})
		service := NewService()
		defer service.ServiceShutdown()
		if err := service.RecordValidation(ValidationSample{}); err == nil {
			t.Fatal("recording enabled without explicit profile flag")
		}
		t.Setenv("KUBESIFT_VALIDATION_PROFILE", "1")
		setup, err := service.ValidationConfiguration()
		if err != nil || !setup.Autorun || len(setup.Contexts) != 2 {
			t.Fatalf("owned setup failed: %v", err)
		}
		if err = service.RecordValidation(ValidationSample{Cycle: 21}); err == nil {
			t.Fatal("unbounded cycle accepted")
		}
		if err = service.RecordValidation(ValidationSample{MaxHistoryBytes: (1 << 20) + 1}); err == nil {
			t.Fatal("unbounded renderer history accepted")
		}
		if err = service.RecordValidation(ValidationSample{Phase: 3, Cycle: 20}); err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(filepath.Join(directory, "renderer-measurements.jsonl")); err != nil {
			t.Fatal("numeric evidence was not persisted")
		}

		evidencePath := filepath.Join(directory, "renderer-measurements.jsonl")
		if err = os.Truncate(evidencePath, 4<<20); err != nil {
			t.Fatal(err)
		}
		if err = service.RecordValidation(ValidationSample{}); err == nil {
			t.Fatal("numeric evidence exceeded its file cap")
		}
		if err = service.FrontendReady(); err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(filepath.Join(directory, "frontend-ready.json")); err != nil {
			t.Fatal("frontend ready marker was not persisted")
		}
		delete(raw, "contexts")
		saveJSON(t, path, raw)
		if _, err = service.ValidationConfiguration(); err == nil {
			t.Fatal("profile accepted missing owned contexts")
		}
	})

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
