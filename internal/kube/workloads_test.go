/*
Purpose: Protect bounded workload reads and immutable resource targeting.
Used by: Kubernetes service tests.
Responsibilities: Exercise pagination, sparse data and response limits against synthetic APIs.
Important: No live cluster or credentials are used.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
)

func workloadServer(t *testing.T, handler http.HandlerFunc) (*Service, model.ConnectionRef) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "config")
	data := fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"test","cluster":{"server":%q}}],"contexts":[{"name":"test","context":{"cluster":"test","user":"test"}}],"users":[{"name":"test","user":{}}]}`, server.URL)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := config.Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := snapshot.Resolve("test")
	if err != nil {
		t.Fatal(err)
	}
	connections := connection.NewManager(nil)
	sessions := session.NewManager(func(model.Event) {})
	service := NewService(connections, sessions)
	ref, err := connections.Open(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(connections.CloseAll)
	return service, ref
}

func TestWorkloadReads(t *testing.T) {
	for _, kind := range []string{"Pod", "Deployment"} {
		t.Run(kind+" pagination and scope", func(t *testing.T) {
			s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Query().Get("limit") != "100" {
					t.Error("unbounded or non-read request")
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("continue") != "" {
					w.WriteHeader(410)
					fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","reason":"Expired","code":410}`)
					return
				}
				version := "v1"
				if kind == "Deployment" {
					version = "apps/v1"
				}
				fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"continue":"next","resourceVersion":"7"},"items":[{"metadata":{"name":"sample","namespace":"default","uid":"uid-1"}}]}`, kind+"List", version)
			})
			req := PageRequest{Connection: ref, Namespace: "default", Kind: kind}
			page, err := s.ReadWorkloads(context.Background(), req)
			if err != nil || len(page.Items) != 1 || page.Page.NextCursor == "" {
				t.Fatalf("page: %+v %v", page, err)
			}
			if page.Items[0].Ready != nil {
				t.Fatal("missing readiness became zero")
			}
			req.Cursor = page.Page.NextCursor
			req.Namespace = "other"
			if _, err = s.ReadWorkloads(context.Background(), req); err == nil {
				t.Fatal("cursor crossed namespace")
			}
			req.Namespace = "default"
			req.Connection.Generation++
			if _, err = s.ReadWorkloads(context.Background(), req); err == nil {
				t.Fatal("cursor crossed generation")
			}
			req.Connection = ref
			if _, err = s.ReadWorkloads(context.Background(), req); err == nil || !strings.Contains(err.Error(), "List changed") {
				t.Fatalf("expired page: %v", err)
			}
		})
	}
	t.Run("bounded body", func(t *testing.T) {
		s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"kind":"PodList","apiVersion":"v1","metadata":{},"items":[],"padding":"`)
			for range 17 {
				fmt.Fprint(w, strings.Repeat("x", 1<<20))
			}
			fmt.Fprint(w, `"}`)
		})
		if _, err := s.ReadWorkloads(context.Background(), PageRequest{Connection: ref, Kind: "Pod", Namespace: "default"}); err == nil {
			t.Fatal("oversized body accepted")
		}
	})
}
