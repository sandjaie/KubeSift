/*
Purpose: Count OpenShift ProjectRequest submissions and conditional Project deletes.
Used by: Lifecycle contract tests.
Responsibilities: Exercise preview, cancellation, confirmation and read-only observation.
Important: Every server is a synthetic loopback fixture.
*/
package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/kube"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const projectObject = `{"apiVersion":"project.openshift.io/v1","kind":"Project","metadata":{"name":"demo","uid":"original","resourceVersion":"7"},"status":{"phase":"Active"}}`

func TestProjectRequestLifecycle(t *testing.T) {
	for _, tc := range []struct {
		code  int
		state string
	}{{201, "accepted"}, {403, "rejected"}, {409, "rejected"}, {500, "unknown"}} {
		t.Run(tc.state+fmt.Sprint(tc.code), func(t *testing.T) {
			var posts atomic.Int32
			base, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /api":
					fmt.Fprint(w, `{"kind":"APIVersions","versions":["v1"]}`)
				case "GET /apis":
					fmt.Fprint(w, `{"kind":"APIGroupList","groups":[{"name":"project.openshift.io","versions":[{"groupVersion":"project.openshift.io/v1","version":"v1"}]}]}`)
				case "GET /apis/project.openshift.io/v1":
					fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"project.openshift.io/v1","resources":[{"name":"projects","kind":"Project","namespaced":false,"verbs":["get","list","delete"]},{"name":"projectrequests","kind":"ProjectRequest","namespaced":false,"verbs":["list","create"]}]}`)
				case "GET " + requestPath:
					fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Success"}`)
				case "GET /apis/project.openshift.io/v1/projects/demo":
					if posts.Load() == 0 {
						w.WriteHeader(404)
					} else {
						fmt.Fprint(w, projectObject)
					}
				case "POST " + requestPath:
					posts.Add(1)
					var obj struct {
						Kind, APIVersion, DisplayName, Description string
						Metadata                                   struct{ Name string }
					}
					if json.NewDecoder(r.Body).Decode(&obj) != nil || obj.Kind != "ProjectRequest" || obj.APIVersion != kube.ProjectVersion || obj.Metadata.Name != "demo" || obj.DisplayName != "Demo" || obj.Description != "Example" || r.URL.RawQuery != "" {
						t.Error("unexpected POST", obj)
					}
					w.WriteHeader(tc.code)
					fmt.Fprint(w, projectObject)
				default:
					t.Error(r.Method, r.URL)
					w.WriteHeader(404)
				}
			})
			ctx := context.Background()
			s := fixtureCreateService(t, base)
			ref := req.Resource.Connection
			f := ProjectFields{"demo", "Demo", "Example"}
			for _, bad := range []string{"openshift", "kubernetes-test", "kube", "../demo", strings.Repeat("x", 64)} {
				if _, e := s.PrepareProjectRequest(ctx, ref, ProjectFields{Name: bad}, ""); e == nil {
					t.Fatal("accepted reserved or invalid name")
				}
			}
			prepare := func() ProjectRequestPreview {
				p, e := s.PrepareProjectRequest(ctx, ref, f, "prod")
				if e != nil || !p.Typed {
					t.Fatal(p, e)
				}
				return p
			}
			p := prepare()
			s.RevokeProjectRequest(p.Token)
			if _, e := s.ExecuteProjectRequest(p.Token, "demo"); e == nil {
				t.Fatal("revoked token")
			}
			p = prepare()
			if _, e := s.ExecuteProjectRequest(p.Token, ""); e == nil {
				t.Fatal("missing acknowledgement")
			}
			p = prepare()
			if posts.Load() != 0 {
				t.Fatal("review submitted", posts.Load())
			}
			result, e := s.ExecuteProjectRequest(p.Token, "demo")
			if e != nil || result.State != tc.state {
				t.Fatal(result, e)
			}
			again, e := s.ExecuteProjectRequest(p.Token, "demo")
			if e != nil || again.OperationID != result.OperationID || posts.Load() != 1 {
				t.Fatal("duplicate confirmation", again, e)
			}
			if tc.code == 201 || tc.code == 500 {
				obs, e := s.InspectCreateName(ctx, result.OperationID)
				if e != nil || tc.code == 201 && obs.State != "verified" || tc.code == 500 && (obs.State != "present" || result.Resource.UID != "") {
					t.Fatal(obs, e)
				}
			}
		})
	}
}
func TestConditionalProjectDelete(t *testing.T) {
	var sends atomic.Int32
	var version atomic.Int32
	version.Store(7)
	base, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews" {
			w.WriteHeader(403)
			return
		}
		if r.URL.Path != "/apis/project.openshift.io/v1/projects/demo" {
			t.Error(r.URL)
			w.WriteHeader(404)
			return
		}
		switch r.Method {
		case "GET":
			fmt.Fprint(w, strings.Replace(projectObject, `"7"`, fmt.Sprintf("%q", fmt.Sprint(version.Load())), 1))
		case "DELETE":
			var opts struct {
				DryRun        []string
				Preconditions struct{ UID, ResourceVersion string }
			}
			if json.NewDecoder(r.Body).Decode(&opts) != nil || opts.Preconditions.UID != "original" || opts.Preconditions.ResourceVersion != "7" {
				t.Error(opts)
			}
			n := sends.Add(1)
			if n <= 3 && (len(opts.DryRun) != 1 || opts.DryRun[0] != "All") || n == 4 && len(opts.DryRun) != 0 {
				t.Error("unexpected dry-run options", opts)
			}
			fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Success"}`)
		default:
			t.Error(r.Method)
		}
	})
	s := NewDeleteService(base.connections, base.coordinator, nil)
	ctx := context.Background()
	r := kube.ResourceRef{Connection: req.Resource.Connection, APIVersion: kube.ProjectVersion, Kind: "Project", Name: "demo", UID: "original"}
	prepare := func() DeletePreview {
		p, e := s.PrepareDelete(ctx, DeleteRequest{r, "7"})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	p := prepare()
	if _, e := s.ExecuteDelete(ctx, p.Token, "wrong"); e == nil {
		t.Fatal("wrong typed name")
	}
	p = prepare()
	version.Store(8)
	res, e := s.ExecuteDelete(ctx, p.Token, "demo")
	if e != nil || res.State != "stale" || sends.Load() != 2 {
		t.Fatal(res, e)
	}
	version.Store(7)
	p = prepare()
	res, e = s.ExecuteDelete(ctx, p.Token, "demo")
	if e != nil || res.State != "accepted" || sends.Load() != 4 {
		t.Fatal(res, e)
	}
}
