package trust

import (
	"context"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHelperTrustLifecycle(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	calls := 0
	storePath := filepath.Join(dir, "trust.json")
	store, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	guard := NewGuard(store)
	guard.now = func() time.Time { return now }
	guard.runner = func(ctx context.Context, path string, args, env []string) ([]byte, error) {
		calls++
		return []byte(fmt.Sprintf(`{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{"token":"synthetic-fixture","expirationTimestamp":%q}}`, now.Add(time.Minute).Format(time.RFC3339))), nil
	}
	resolved := func(command, args, env, server string) config.Resolved {
		t.Helper()
		path := filepath.Join(dir, "config")
		fixture := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: c\n  cluster: {server: %q, insecure-skip-tls-verify: true}\nusers:\n- name: u\n  user:\n    exec:\n      apiVersion: client.authentication.k8s.io/v1\n      command: %q\n      interactiveMode: Never\n      args: [%q]\n      env: [{name: FIXTURE, value: %q}]\ncontexts:\n- name: fixture\n  context: {cluster: c, user: u}\n", server, command, args, env)
		if e := os.WriteFile(path, []byte(fixture), 0600); e != nil {
			t.Fatal(e)
		}
		s, e := config.Load([]string{path})
		if e != nil {
			t.Fatal(e)
		}
		r, e := s.Resolve("fixture")
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	r := resolved(helper, "secret-arg", "secret-env", "https://127.0.0.1:6443")
	preview, err := guard.Review(r)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || strings.Contains(strings.Join(preview.Args, " "), "secret-arg") {
		t.Fatal("preview executed or disclosed arguments")
	}
	if _, err = guard.Authenticate(context.Background(), r); err == nil || calls != 0 {
		t.Fatal("untrusted helper executed")
	}
	if err = guard.ApprovePreview(preview.ID, r); err != nil {
		t.Fatal(err)
	}
	cfg, err := guard.Authenticate(context.Background(), r)
	if err != nil || cfg.BearerToken != "synthetic-fixture" || cfg.ExecProvider != nil || calls != 1 {
		t.Fatalf("approved execution failed: %v, calls %d", err, calls)
	}
	reloaded, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = NewGuard(reloaded).Check(r); err != nil {
		t.Fatal("approval did not persist")
	}
	for _, variant := range []struct{ name, command, args, env string }{{"args", helper, "changed", "secret-env"}, {"env", helper, "secret-arg", "changed"}, {"path", "/bin/echo", "secret-arg", "secret-env"}} {
		t.Run(variant.name, func(t *testing.T) {
			changed := resolved(variant.command, variant.args, variant.env, "https://127.0.0.1:6443")
			if _, e := guard.Authenticate(context.Background(), changed); e == nil || calls != 1 {
				t.Fatal("changed helper executed")
			}
		})
	}
	if err = store.Revoke(preview.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err = guard.Authenticate(context.Background(), r); err == nil || calls != 1 {
		t.Fatal("revoked helper executed on refresh")
	}
	data, _ := os.ReadFile(storePath)
	if strings.Contains(string(data), "secret-") {
		t.Fatal("trust file disclosed helper values")
	}
	for _, invalid := range []string{`{"version":99,"entries":[]}`, `{"version":1,"entries":[],"bypass":true}`, `{broken`} {
		if e := os.WriteFile(storePath, []byte(invalid), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := OpenStore(storePath); e == nil {
			t.Fatal("invalid trust file accepted")
		}
	}
	// Revocation during a 401 response must not trigger native exec refresh.
	if err = store.Approve(preview.Fingerprint); err != nil {
		t.Fatal(err)
	}
	revokeFP := preview.Fingerprint
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = store.Revoke(revokeFP)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	remote := resolved(helper, "secret-arg", "secret-env", server.URL)
	p, err := guard.Review(remote)
	if err != nil {
		t.Fatal(err)
	}
	if err = guard.ApprovePreview(p.ID, remote); err != nil {
		t.Fatal(err)
	}
	revokeFP = p.Fingerprint
	manager := connection.NewManager(guard)
	defer manager.CloseAll()
	ref, err := manager.Open(context.Background(), remote)
	if err != nil {
		t.Fatal(err)
	}
	client, err := manager.Client(ref)
	if err != nil {
		t.Fatal(err)
	}
	before := calls
	_, _ = client.CoreV1().Pods("fixture").List(context.Background(), metav1.ListOptions{})
	if calls != before+1 {
		t.Fatalf("401 caused extra helper execution: %d", calls-before)
	}
	if _, err = guard.Authenticate(context.Background(), remote); err == nil {
		t.Fatal("response-time revocation did not block next execution")
	}
	t.Run("PATH-change", func(t *testing.T) {
		a := t.TempDir()
		b := t.TempDir()
		for _, dir := range []string{a, b} {
			if err := os.WriteFile(filepath.Join(dir, "exec-probe"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("PATH", a)
		current := resolved("exec-probe", "safe", "fixture", "https://127.0.0.1:6443")
		frozen, e := current.FreezeHelper()
		if e != nil {
			t.Fatal(e)
		}
		p, e := guard.Review(frozen)
		if e != nil {
			t.Fatal(e)
		}
		if e = guard.ApprovePreview(p.ID, frozen); e != nil {
			t.Fatal(e)
		}
		before := calls
		t.Setenv("PATH", b)
		if _, e = guard.Authenticate(context.Background(), frozen); e == nil || calls != before {
			t.Fatal("PATH change executed an unapproved helper")
		}
	})

}
