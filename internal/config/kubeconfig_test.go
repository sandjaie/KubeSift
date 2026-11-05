package config_test

import (
	"bytes"
	"context"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndConnectionIdentity(t *testing.T) {
	fixture := []byte(`apiVersion: v1
kind: Config
current-context: same
extensions:
- name: vendor
  extension: {retained: true}
clusters:
- name: cluster
  cluster: {server: "https://127.0.0.1:6443"}
users:
- name: fixture
  user: {token: synthetic}
contexts:
- name: same
  context: {cluster: cluster, user: fixture, namespace: ks-validation}
`)
	manager := connection.NewManager(nil)
	var firstID string
	for _, name := range []string{"a", "b"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, fixture, 0600); err != nil {
				t.Fatal(err)
			}
			snapshot, err := config.Load([]string{path})
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Contexts()) != 1 {
				t.Fatal("missing preview")
			}
			if _, err = snapshot.Resolve("absent"); err == nil {
				t.Fatal("missing context accepted")
			}
			resolved, err := snapshot.Resolve("same")
			if err != nil {
				t.Fatal(err)
			}
			ref, err := manager.Open(context.Background(), resolved)
			if err != nil {
				t.Fatal(err)
			}
			if ref.ID == firstID {
				t.Fatal("same-named contexts from different files aliased")
			}
			firstID = ref.ID
			if err = manager.Close(ref); err != nil {
				t.Fatal(err)
			}
			next, err := manager.Open(context.Background(), resolved)
			if err != nil {
				t.Fatal(err)
			}
			if next.Generation <= ref.Generation {
				t.Fatal("generation did not advance")
			}
			if _, err = manager.Client(ref); err == nil {
				t.Fatal("stale reference accepted")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(after, fixture) {
				t.Fatal("input changed")
			}
		})
	}
}
