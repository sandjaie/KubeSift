/*
Purpose: Protect source isolation and strict metadata persistence.
Used by: Go tests.
Responsibilities: Exercise independent snapshots, label review and non-destructive file handling.
Important: All kubeconfigs and settings are synthetic temporary files.
*/
package sources

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourcesIsolateSnapshotsAndPersistMetadata(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(dir, "missing"))
	fixture := func(path, server string) {
		t.Helper()
		err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\nclusters:\n- name: cluster\n  cluster:\n    server: "+server+"\ncontexts:\n- name: same\n  context:\n    cluster: cluster\n    user: user\nusers:\n- name: user\n  user:\n    token: synthetic\n"), 0600)
		if err != nil {
			t.Fatal(err)
		}
	}
	aPath, bPath := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	fixture(aPath, "https://a.invalid")
	fixture(bPath, "https://b.invalid")
	appearance := filepath.Join(dir, "preferences.json")
	os.WriteFile(appearance, []byte("untouched"), 0600)
	store := filepath.Join(dir, "sources.json")
	r, err := NewRegistry(store)
	if err != nil {
		t.Fatal(err)
	}
	a, err := r.Add(aPath)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Add(bPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Add(filepath.Join(dir, ".", "a")); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err = r.SetLabel(a.SourceID, "same", "Development"); err != nil {
		t.Fatal(err)
	}
	fixture(aPath, "https://changed.invalid")
	fresh, err := r.Reload(a.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh.Contexts[0].LabelNeedsReview || fresh.Contexts[0].Environment != "" {
		t.Fatal("old label trusted after target change")
	}
	if _, err = r.Resolve(a.SourceID, a.SnapshotID, "same"); err == nil {
		t.Fatal("superseded snapshot resolved")
	}
	if _, err = r.Resolve(b.SourceID, b.SnapshotID, "same"); err != nil {
		t.Fatal("B invalidated", err)
	}
	os.WriteFile(aPath, []byte("malformed: ["), 0600)
	bad, _ := r.Reload(a.SourceID)
	if bad.Problem == nil {
		t.Fatal("malformed source hidden")
	}
	if _, err = r.Resolve(b.SourceID, b.SnapshotID, "same"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(bPath)
	if err = r.Remove(b.SourceID); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(bPath)
	if string(before) != string(after) {
		t.Fatal("kubeconfig changed")
	}
	bytes, _ := os.ReadFile(appearance)
	if string(bytes) != "untouched" {
		t.Fatal("appearance changed")
	}
	for _, invalid := range []string{`{"version":1,"version":1,"sources":[],"labels":[]}`, `{"version":1,"sources":null,"labels":[]}`, `{"version":1,"sources":[],"labels":[],"unknown":true}`, `{"version":1,"sources":[],"labels":[]} {}`} {
		os.WriteFile(store, []byte(invalid), 0600)
		if _, err = NewRegistry(store); err == nil {
			t.Fatalf("invalid store accepted: %s", invalid)
		}
		data, _ := os.ReadFile(store)
		if string(data) != invalid {
			t.Fatal("invalid store overwritten")
		}
	}
}
