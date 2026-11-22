/*
Purpose: Exercise the OpenShift Project proxy contract on a deterministic HTTP server.
Used by: Native reader regression tests.
Responsibilities: Verify capped inventories and named scope validation.
Important: These cases use synthetic identities.
*/
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProjectsReader(t *testing.T) {
	for _, n := range []int{0, 130, 501} {
		s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path != "/apis/project.openshift.io/v1/projects" || r.URL.Query().Get("limit") != "100" {
				t.Error(r.URL)
			}
			fmt.Fprint(w, `{"kind":"ProjectList","apiVersion":"project.openshift.io/v1","items":[`)
			for i := 0; i < n; i++ {
				if i > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"metadata":{"name":"p-%d","uid":"u-%d","resourceVersion":"1"}}`, i, i)
			}
			fmt.Fprint(w, "]}")
		})
		p, e := s.ListProjects(context.Background(), ref, "")
		if e != nil || len(p.Items) != min(n, 500) || p.Partial != (n > 500) {
			t.Fatal(n, len(p.Items), p.Partial, e)
		}
	}
}

func TestProjectOnlyAccessAndNamedLookup(t *testing.T) {
	ctx := context.Background()
	const path = "/apis/project.openshift.io/v1"
	var deniedList atomic.Bool
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			fmt.Fprint(w, `{"kind":"APIVersions","versions":["v1"]}`)
		case "/apis":
			fmt.Fprint(w, `{"kind":"APIGroupList","groups":[{"name":"project.openshift.io","versions":[{"groupVersion":"project.openshift.io/v1","version":"v1"}]}]}`)
		case path:
			fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"project.openshift.io/v1","resources":[{"name":"projects","kind":"Project","namespaced":false,"verbs":["get","list","delete"]}]}`)
		case path + "/projects":
			if deniedList.Load() {
				w.WriteHeader(403)
			} else {
				fmt.Fprint(w, `{"kind":"ProjectList","apiVersion":"project.openshift.io/v1","items":[]}`)
			}
		case path + "/projects/team":
			fmt.Fprint(w, `{"kind":"Project","apiVersion":"project.openshift.io/v1","metadata":{"name":"team","uid":"one","resourceVersion":"7"},"status":{"phase":"Active"}}`)
		case "/api/v1/namespaces/team/resourcequotas":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"ResourceQuotaList","items":[{"metadata":{"name":"budget","namespace":"team","uid":"quota"},"status":{"hard":{"cpu":"1000m"},"used":{"cpu":"0"}}}]}`)
		case "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews":
			var request struct {
				Spec struct {
					ResourceAttributes struct{ Name, Namespace, Resource string }
				}
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Error("invalid SSAR")
			}
			a := request.Spec.ResourceAttributes
			switch a.Resource {
			case "projects":
				if a.Name != "team" || a.Namespace != "" {
					t.Error(a)
				}
				w.WriteHeader(403)
			case "pods", "resourcequotas":
				if a.Namespace != "team" || a.Name != "" {
					t.Error(a)
				}
				fmt.Fprintf(w, `{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":%t,"denied":%t}}`, a.Resource == "resourcequotas", a.Resource == "pods")
			default:
				t.Error(a)
			}
		case "/api/v1/namespaces", "/api/v1/namespaces/default/pods":
			w.WriteHeader(403)
		default:
			t.Error(r.URL)
			w.WriteHeader(404)
		}
	})
	a, e := s.CheckAccess(ctx, ref, "default")
	if e != nil || a.Probe != "projects" || !a.AuthorizedRead || len(a.Namespaces) != 0 || a.PodProblem == nil {
		t.Fatal(a, e)
	}
	caps, e := s.ProjectCapabilities(ctx, ref, false)
	if e != nil || !caps.Delete || caps.Request {
		t.Fatal(caps, e)
	}
	deniedList.Store(true)
	if _, e = s.ListProjects(ctx, ref, ""); e == nil {
		t.Fatal("denied list accepted")
	}
	p, e := s.ReadProject(ctx, ref, "team", "one")
	if e != nil || p.Phase != "Active" {
		t.Fatal(p, e)
	}
	if _, e = s.ReadProject(ctx, ref, "team", "two"); e == nil {
		t.Fatal("replacement accepted")
	}
	if _, e = s.ReadProject(ctx, ref, "../team", ""); e == nil {
		t.Fatal("invalid path accepted")
	}
	q, e := s.ProjectQuotas(ctx, p.Resource, "")
	if e != nil || len(q.Items) != 1 || q.Items[0].Hard["cpu"] != "1000m" {
		t.Fatal(q, e)
	}
	access, e := s.MyProjectAccess(ctx, p.Resource)
	if e != nil || access.Pods != "denied" || access.Quotas != "allowed" || access.Delete != "unknown" {
		t.Fatal(access, e)
	}
}

func TestProjectContinuationsAndDisconnect(t *testing.T) {
	ctx := context.Background()
	started := make(chan struct{})
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case projectPath:
			if token := r.URL.Query().Get("continue"); token != "" {
				if token != "next" {
					t.Error(token)
				}
				w.WriteHeader(410)
				fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","code":410,"reason":"Expired"}`)
				return
			}
			fmt.Fprint(w, `{"kind":"ProjectList","apiVersion":"project.openshift.io/v1","metadata":{"continue":"next"},"items":[{"metadata":{"name":"one","uid":"id","resourceVersion":"7"}}]}`)
		case projectPath + "/held":
			close(started)
			<-r.Context().Done()
		default:
			t.Error(r.URL)
			w.WriteHeader(404)
		}
	})
	p, e := s.ListProjects(ctx, ref, "")
	if e != nil || !p.Partial || p.Next == "" {
		t.Fatal(p, e)
	}
	if _, e := s.ListProjects(ctx, ref, p.Next); e == nil || !strings.Contains(e.Error(), "list_changed") {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := s.ReadProject(ctx, ref, "held", "id"); done <- e }()
	<-started
	if e := s.connections.Close(ref); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("disconnected response accepted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read not cancelled")
	}
}

func TestProjectInvalidIdentityAndDiscovery(t *testing.T) {
	ctx := context.Background()
	for _, obj := range []string{
		`{"kind":"Project","apiVersion":"project.openshift.io/v1","metadata":{"name":"wrong","uid":"one","resourceVersion":"7"}}`,
		`{"kind":"Project","apiVersion":"project.openshift.io/v1","metadata":{"name":"team","namespace":"team","uid":"one","resourceVersion":"7"}}`,
	} {
		s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, obj)
		})
		if _, e := s.ReadProject(ctx, ref, "team", "one"); e == nil {
			t.Fatal("accepted mismatched Project")
		}
	}
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
	})
	if _, e := s.ProjectCapabilities(ctx, ref, false); e == nil || !strings.Contains(e.Error(), "forbidden") {
		t.Fatal("denied discovery reported absence", e)
	}
}
