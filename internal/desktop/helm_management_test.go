/*
Purpose: Verify the Helm bridge keeps targets explicit and source secrets native.
Used by: Desktop service tests.
Responsibilities: Reject invented file paths and stale connections while persisting only source metadata.
Important: No native dialog or cluster is opened in this test.
*/
package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandjaie/KubeSift/internal/helmmanager"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/preferences"
)

func TestHelmManagementBridge(t *testing.T) {
	core := NewService()
	defer core.ServiceShutdown()
	s := newExplorerService(core)
	s.prefs = &PreferencesService{path: filepath.Join(t.TempDir(), "preferences.json"), value: preferences.Default()}
	source := helmmanager.Source{ID: "private", Name: "Private charts", Kind: "https", URL: "https://charts.example.invalid"}
	invented := source
	invented.CAFile = "/not-selected.pem"
	if err := s.SaveHelmSource(invented); err == nil {
		t.Fatal("arbitrary frontend TLS path accepted")
	}
	if err := s.SaveHelmSource(source); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHelmCredentials(source.ID, helmmanager.SourceCredentials{Username: "synthetic", Password: "secret-sentinel"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.prefs.path)
	if err != nil || strings.Contains(string(data), "secret-sentinel") || !strings.Contains(string(data), source.URL) {
		t.Fatalf("bad persisted source metadata: %v", err)
	}
	p, err := preferences.Load(s.prefs.path)
	if err != nil || len(p.HelmSources) != 1 {
		t.Fatal("source metadata did not round trip")
	}
	response, err := s.ListHelmSources()
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(response)
	if strings.Contains(string(encoded), "secret-sentinel") {
		t.Fatal("bridge returned credentials")
	}
	valuesPath := filepath.Join(t.TempDir(), "values.yaml")
	if err := os.WriteFile(valuesPath, []byte("message: synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	handle, err := core.helmManagement.ImportValues(valuesPath)
	if err != nil {
		t.Fatal(err)
	}
	values, err := s.ReadHelmValues(handle, 0)
	if err != nil || values.Text != "message: synthetic\n" || !values.Done {
		t.Fatal("values handle import failed")
	}
	s.DiscardHelmDraft(handle)
	if _, err := s.ReadHelmValues(handle, 0); err == nil {
		t.Fatal("discarded values remained readable")
	}
	_, err = s.PrepareHelmAction("stale", helmmanager.PrepareRequest{Action: "uninstall", Target: helmmanager.Target{Connection: model.ConnectionRef{ID: "missing", Generation: 1}, Namespace: "owned", Driver: "secret", ReleaseName: "demo"}})
	if err == nil {
		t.Fatal("bridge accepted a disconnected target")
	}
	if _, err := s.ExecuteHelmAction("invented-token"); err == nil {
		t.Fatal("bridge accepted invented review token")
	}
}
