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
	"sync/atomic"
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
	for _, kind := range []string{"Pod", "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet", "Job", "CronJob"} {
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
				if kind != "Pod" {
					version = "apps/v1"
				}
				if kind == "Job" || kind == "CronJob" {
					version = "batch/v1"
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

func TestWorkloadDetails(t *testing.T) {
	for _, tc := range []struct {
		name, uid string
		huge      bool
	}{
		{"captured YAML", "uid-1", false}, {"replacement", "old", false}, {"oversize", "uid-1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				annotation := "fixture"
				if tc.huge {
					annotation = strings.Repeat("x", 2<<20)
				}
				fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"sample","namespace":"default","uid":"uid-1","resourceVersion":"9","annotations":{"note":%q},"managedFields":[{"manager":"fixture"}]},"spec":{"containers":[{"name":"main","image":"synthetic"}]}}`, annotation)
			})
			req := DetailRequest{Resource: ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Pod", Namespace: "default", Name: "sample", UID: tc.uid}}
			detail, err := s.ReadResource(context.Background(), req)
			if tc.uid == "old" || tc.huge {
				if err == nil {
					t.Fatal("unsafe detail accepted")
				}
				return
			}
			if err != nil || !strings.Contains(detail.ObjectJSON, "synthetic") {
				t.Fatal(detail, err)
			}
			doc, err := s.ReadResourceYAML(context.Background(), req)
			if err != nil || doc.ResourceVersion != "9" || !strings.Contains(doc.WithoutManagedFields, "apiVersion: v1") || !strings.Contains(doc.WithManagedFields, "kind: Pod") || strings.Contains(doc.WithoutManagedFields, "managedFields") || !strings.Contains(doc.WithManagedFields, "managedFields") {
				t.Fatal(doc, err)
			}
		})
	}
	t.Run("owned Pods and forbidden coverage", func(t *testing.T) {
		denied := false
		s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "deployments/sample"):
				fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"sample","namespace":"default","uid":"deploy"},"spec":{"replicas":0}}`)
			case strings.HasSuffix(r.URL.Path, "replicasets/rs"):
				fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"ReplicaSet","metadata":{"name":"rs","namespace":"default","uid":"rs","ownerReferences":[{"uid":"deploy","controller":true}]},"spec":{"selector":{"matchLabels":{"app":"fixture"}}}}`)
			case strings.HasSuffix(r.URL.Path, "replicasets"):
				if denied {
					w.WriteHeader(403)
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","reason":"Forbidden","code":403}`)
					return
				}
				fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"ReplicaSetList","items":[{"metadata":{"name":"rs","namespace":"default","uid":"rs","ownerReferences":[{"uid":"deploy","controller":true}]},"spec":{"selector":{"matchLabels":{"app":"fixture"}}}}]}`)
			case strings.HasSuffix(r.URL.Path, "pods"):
				if r.URL.Query().Get("labelSelector") != "app=fixture" {
					t.Error("selector missing")
				}
				fmt.Fprint(w, `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":"owned","namespace":"default","uid":"owned","ownerReferences":[{"uid":"rs","controller":true}]}},{"metadata":{"name":"lookalike","namespace":"default","uid":"other"}}]}`)
			default:
				t.Error("unexpected read", r.URL.Path)
			}
		})
		resource := ResourceRef{Connection: ref, APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "sample", UID: "deploy"}
		page, err := s.ReadRelatedPods(context.Background(), RelatedRequest{Resource: resource})
		if err != nil || !page.Complete || len(page.Items) != 1 || page.Items[0].Pod.Resource.Name != "owned" {
			t.Fatal(page, err)
		}
		denied = true
		page, err = s.ReadRelatedPods(context.Background(), RelatedRequest{Resource: resource})
		if err != nil || page.Complete || page.Problem == nil || page.Problem.Code != "forbidden" {
			t.Fatal(page, err)
		}
	})
}

func TestWorkloadEvents(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(fmt.Sprint(denied), func(t *testing.T) {
			s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "pods/sample") {
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"sample","namespace":"default","uid":"pod"}}`)
					return
				}
				if !strings.HasPrefix(r.URL.Path, "/apis/events.k8s.io/v1/") {
					t.Error("unexpected core fallback")
					w.WriteHeader(500)
					return
				}
				if denied {
					w.WriteHeader(403)
					fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","reason":"Forbidden","code":403}`)
					return
				}
				if r.URL.Query().Get("fieldSelector") != "regarding.uid=pod" {
					t.Error("missing UID filter")
				}
				fmt.Fprint(w, `{"apiVersion":"events.k8s.io/v1","kind":"EventList","metadata":{"continue":"next"},"items":[{"metadata":{"uid":"event"},"regarding":{"uid":"pod"},"reason":"Waiting"},{"metadata":{"uid":"event"},"regarding":{"uid":"pod"},"reason":"Updated","series":{"count":3,"lastObservedTime":"2026-11-12T01:00:00.000000Z"}},{"metadata":{"uid":"other"},"regarding":{"uid":"replacement"}}]}`)
			})
			page, err := s.ReadResourceEvents(context.Background(), EventRequest{Resource: ResourceRef{Connection: ref, Kind: "Pod", APIVersion: "v1", Namespace: "default", Name: "sample", UID: "pod"}})
			if denied {
				if err == nil {
					t.Fatal("denial hidden")
				}
				return
			}
			if err != nil || len(page.Items) != 1 || page.Page.NextCursor == "" || page.Items[0].Count == nil || *page.Items[0].Count != 3 || page.Items[0].FirstAt != nil {
				t.Fatal(page, err)
			}
		})
	}
}

func TestRelatedPodsBoundsEmptyTraversal(t *testing.T) {
	pages := 0
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "deployments/sample") {
			fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"sample","namespace":"default","uid":"deploy"}}`)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "replicasets") {
			t.Error("unexpected read", r.URL.Path)
		}
		pages++
		fmt.Fprintf(w, `{"apiVersion":"apps/v1","kind":"ReplicaSetList","metadata":{"continue":"page%d"},"items":[]}`, pages)
	})
	page, err := s.ReadRelatedPods(context.Background(), RelatedRequest{Resource: ResourceRef{Connection: ref, APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "sample", UID: "deploy"}})
	if err != nil || pages != 8 || page.Complete || page.Page.NextCursor == "" {
		t.Fatalf("unbounded or misleading traversal: pages=%d complete=%v cursor=%t err=%v", pages, page.Complete, page.Page.NextCursor != "", err)
	}
}

func TestReplicaSetRelationships(t *testing.T) {
	var forbidden atomic.Bool
	s, connection := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/default/deployments/web":
			fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default","uid":"deploy"}}`)
		case "/apis/apps/v1/namespaces/default/replicasets":
			if r.URL.Query().Get("limit") != "100" {
				t.Error("expected bounded ReplicaSet page")
			}
			fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"ReplicaSetList","metadata":{"continue":"sets-next"},"items":[{"metadata":{"name":"owned","namespace":"default","uid":"rs","ownerReferences":[{"uid":"deploy","controller":true}]},"spec":{"replicas":0},"status":{"observedGeneration":1,"replicas":0}},{"metadata":{"name":"foreign","uid":"other","ownerReferences":[{"uid":"deploy","controller":false}]}}]}`)
		case "/apis/apps/v1/namespaces/default/replicasets/owned":
			fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"ReplicaSet","metadata":{"name":"owned","namespace":"default","uid":"rs"},"spec":{"selector":{"matchLabels":{"app":"web"}}}}`)
		case "/api/v1/namespaces/default/pods":
			if forbidden.Load() {
				w.WriteHeader(403)
				fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","reason":"Forbidden","code":403}`)
				return
			}
			if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("labelSelector") != "app=web" {
				t.Error("expected bounded and narrowed Pod page")
			}
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":"owned-pod","namespace":"default","uid":"pod","ownerReferences":[{"uid":"rs","controller":true}]}},{"metadata":{"name":"lookalike","namespace":"default","uid":"lookalike","ownerReferences":[{"uid":"another","controller":true}]}}]}`)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	deploy := ResourceRef{Connection: connection, APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "web", UID: "deploy"}
	sets, err := s.ReadRelatedReplicaSets(ctx, RelatedRequest{Resource: deploy})
	if err != nil || len(sets.Items) != 1 || sets.Complete || sets.Page.NextCursor == "" {
		t.Fatalf("sets: %+v %v", sets, err)
	}
	row := sets.Items[0]
	if row.Desired == nil || *row.Desired != 0 || row.Current == nil || *row.Current != 0 || row.Updated != nil {
		t.Fatalf("zero/current semantics: %+v", row)
	}
	pods, err := s.ReadRelatedPods(ctx, RelatedRequest{Resource: row.Resource})
	if err != nil || !pods.Complete || len(pods.Items) != 1 || pods.Items[0].Controller != row.Resource {
		t.Fatalf("pods: %+v %v", pods, err)
	}
	forbidden.Store(true)
	pods, err = s.ReadRelatedPods(ctx, RelatedRequest{Resource: row.Resource})
	if err != nil || pods.Problem == nil || pods.Problem.Code != "forbidden" || pods.Complete {
		t.Fatalf("forbidden: %+v %v", pods, err)
	}
	if _, err := s.ReadRelatedPods(ctx, RelatedRequest{Resource: row.Resource, Cursor: sets.Page.NextCursor}); err == nil {
		t.Fatal("cursor crossed relation endpoints")
	}
	row.Resource.UID = "stale"
	if _, err := s.ReadResource(ctx, DetailRequest{Resource: row.Resource}); err == nil {
		t.Fatal("reused name accepted")
	}
}
