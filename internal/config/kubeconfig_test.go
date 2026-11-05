/*
Purpose: Check configuration compatibility and connection targeting safeguards.
Used by: The Go unit suite and review gates.
Responsibilities: Exercise synthetic sources, stale previews, anonymous contexts and cancellation of in-flight requests.
Important: Only temporary fixtures and loopback HTTP servers are used; no real cluster is queried.
*/
package config_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/desktop"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	t.Run("rejects-replaced-preview", func(t *testing.T) {
		service := desktop.NewService()
		defer service.ServiceShutdown()
		a := filepath.Join(t.TempDir(), "a")
		b := filepath.Join(t.TempDir(), "b")
		for _, path := range []string{a, b} {
			if err := os.WriteFile(path, fixture, 0600); err != nil {
				t.Fatal(err)
			}
		}
		first, err := service.LoadContexts([]string{a})
		if err != nil {
			t.Fatal(err)
		}
		second, err := service.LoadContexts([]string{b})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.OpenConnection(first[0].SnapshotID, "same"); err == nil {
			t.Fatal("stale preview opened the replacement source")
		}
		if _, err := service.OpenConnection(second[0].SnapshotID, "same"); err != nil {
			t.Fatalf("current preview rejected: %v", err)
		}
	})

	t.Run("anonymous-context", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "anonymous")
		data := bytes.ReplaceAll(fixture, []byte("user: fixture, "), nil)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		snapshot, err := config.Load([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = snapshot.Resolve("same"); err != nil {
			t.Fatalf("anonymous context rejected: %v", err)
		}
	})
	t.Run("close-cancels-in-flight-request", func(t *testing.T) {
		started := make(chan struct{})
		stopped := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(stopped) }))
		defer server.Close()
		callCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		path := filepath.Join(t.TempDir(), "local-http-fixture")
		data := bytes.ReplaceAll(fixture, []byte("https://127.0.0.1:6443"), []byte(server.URL))
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		snapshot, err := config.Load([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := snapshot.Resolve("same")
		if err != nil {
			t.Fatal(err)
		}
		manager := connection.NewManager(nil)
		defer manager.CloseAll()
		ref, err := manager.Open(context.Background(), resolved)
		if err != nil {
			t.Fatal(err)
		}
		client, err := manager.Client(ref)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, e := client.CoreV1().Pods("ks-validation").List(callCtx, metav1.ListOptions{}); done <- e }()
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("request did not start")
		}
		if err = manager.Close(ref); err != nil {
			t.Fatal(err)
		}
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Fatal("connection close left network request running")
		}
		if err = <-done; err == nil {
			t.Fatal("cancelled request returned success")
		}
	})

	t.Run("freeze-certificate-files", func(t *testing.T) {
		dir := t.TempDir()
		ca := filepath.Join(dir, "ca.pem")
		original := []byte("synthetic-ca-before")
		if err := os.WriteFile(ca, original, 0600); err != nil {
			t.Fatal(err)
		}
		data := bytes.Replace(fixture, []byte(`cluster: {server: "https://127.0.0.1:6443"}`), []byte(fmt.Sprintf(`cluster: {server: "https://127.0.0.1:6443", certificate-authority: %q}`, ca)), 1)
		path := filepath.Join(dir, "config")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		snapshot, err := config.Load([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := snapshot.Resolve("same")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(ca, []byte("changed-ca-after-resolution"), 0600); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(resolved.RESTConfig().CAData, original) {
			t.Fatal("resolved configuration retained a mutable certificate file")
		}
	})

}
