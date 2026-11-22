/*
Purpose: Count Build action submissions against OCP 4.21-shaped loopback endpoints.
Used by: Reviewed operation tests.
Responsibilities: Prove cancel, source changes, rejection and ambiguous outcomes never replay a BuildRequest.
Important: The fixture performs no source fetch, image build or controller work.
*/
package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/kube"
	"net/http"
	"strings"
	"testing"
)

func TestReviewedBuildActions(t *testing.T) {
	for _, tc := range []struct {
		action, kind string
		code         int
		state        string
	}{{"start-build", "BuildConfig", 201, "accepted"}, {"rebuild", "Build", 201, "accepted"}, {"cancel-build", "Build", 200, "accepted"}, {"rebuild", "Build", 500, "unknown"}, {"start-build", "BuildConfig", 403, "rejected"}} {
		t.Run(tc.action+tc.state, func(t *testing.T) {
			ver, source, phase := "4", "Git", "Running"
			dry, writes := 0, 0
			base, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "selfsubjectaccessreviews"):
					fmt.Fprint(w, `{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":true}}`)
				case r.Method == "GET":
					fmt.Fprintf(w, `{"kind":"%s","apiVersion":"build.openshift.io/v1","metadata":{"name":"source","namespace":"default","uid":"orig","resourceVersion":"%s"},"spec":{"source":{"type":"%s","git":{"uri":"https://user:private-sentinel@example.test/repo?q=private-sentinel#private-sentinel"},"dockerfile":"private-sentinel"},"strategy":{"type":"Docker","dockerStrategy":{"env":[{"value":"private-sentinel"}]}}},"status":{"lastVersion":7,"phase":"%s"}}`, tc.kind, ver, source, phase)
				default:
					if r.URL.Query().Get("dryRun") != "" {
						dry++
						if tc.action != "cancel-build" {
							t.Fatal("imperative dry-run")
						}
					} else {
						writes++
					}
					name, uid := "source-8", "returned"
					if tc.action == "cancel-build" {
						name, uid = "source", "orig"
					} else {
						var b map[string]any
						if json.NewDecoder(r.Body).Decode(&b) != nil || b["kind"] != "BuildRequest" || tc.action == "start-build" && b["lastVersion"] != float64(7) {
							t.Error("wrong BuildRequest", b)
						}
					}
					w.WriteHeader(tc.code)
					fmt.Fprintf(w, `{"kind":"Build","apiVersion":"build.openshift.io/v1","metadata":{"name":"%s","namespace":"default","uid":"%s","resourceVersion":"8"},"status":{"cancelled":true}}`, name, uid)
				}
			})
			s := fixtureCreateService(t, base)
			q := kube.WorkflowQuery{Connection: req.Resource.Connection, Kind: tc.kind, Namespace: "default", Name: "source", UID: "orig"}
			request := BuildActionRequest{q, "4", tc.action}
			prepare := func() WorkflowPreview {
				p, e := s.PrepareBuildAction(context.Background(), request, "")
				if e != nil {
					t.Fatal(e)
				}
				if strings.Contains(p.Before, "private-sentinel") {
					t.Fatal("sensitive preview")
				}
				return p
			}
			p := prepare()
			s.RevokeBuildAction(tc.action, p.Token)
			if _, e := s.ExecuteBuildAction(tc.action, p.Token, ""); e == nil {
				t.Fatal("revoked token")
			}
			p = prepare()
			ver = "5"
			res, e := s.ExecuteBuildAction(tc.action, p.Token, "")
			if e != nil || res.State != "cancelled-before-send" || writes != 0 {
				t.Fatal(res, e, writes)
			}
			ver = "4"
			p = prepare()
			res, e = s.ExecuteBuildAction(tc.action, p.Token, "")
			if e != nil || res.State != tc.state {
				t.Fatal(res, e)
			}
			_, _ = s.ExecuteBuildAction(tc.action, p.Token, "")
			if writes != 1 {
				t.Fatal(writes)
			}
			if tc.action == "cancel-build" {
				phase = "Complete"
			} else {
				source = "Binary"
			}
			if _, e = s.PrepareBuildAction(context.Background(), request, ""); e == nil {
				t.Fatal("unsafe source accepted")
			}
		})
	}
}
