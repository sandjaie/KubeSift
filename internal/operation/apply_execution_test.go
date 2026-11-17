/*
Purpose: Exercise apply review and confirmation across an actual HTTP transport.
Used by: Native operation regression tests.
Responsibilities: Check takeover, race handling and single dispatch when the response is uncertain.
Important: The fake API tests application behavior; the kind gate tests Kubernetes behavior.
*/
package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/session"
)

func TestApplyExecution(t *testing.T) {
	for _, tc := range []struct {
		mode, state string
	}{
		{"updated", "accepted"},
		{"takeover", "accepted"},
		{"admission changed", "accepted"},
		{"stale before dispatch", "stale"},
		{"deleted after GET", "stale"},
		{"unexpected creation", "unknown"},
		{"invalid success", "unknown"},
		{"oversize success", "unknown"},
		{"lost response", "unknown"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			const path = "/api/v1/namespaces/default/configmaps/web"
			const live = `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":"7"},"data":{"marker":"old"}}`
			const preview = `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":"7"},"data":{"marker":"synthetic","default":"from-admission"}}`
			var dry, real atomic.Int32
			var reviewed atomic.Value
			base, request := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /api/v1":
					fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true,"verbs":["get","patch"]}]}`)
				case "GET " + path:
					if tc.mode == "deleted after GET" && real.Load() > 0 {
						w.WriteHeader(404)
					} else if tc.mode == "stale before dispatch" && dry.Load() > 0 {
						fmt.Fprint(w, strings.ReplaceAll(live, `"7"`, `"8"`))
					} else {
						fmt.Fprint(w, live)
					}
				case "PATCH " + path:
					raw, _ := io.ReadAll(r.Body)
					query := r.URL.Query()
					force := query.Get("force") == "true"
					if !strings.Contains(string(raw), `"uid":"uid"`) || !strings.Contains(string(raw), `"resourceVersion":"7"`) || strings.Contains(string(raw), "from-admission") {
						t.Errorf("lost guard or submitted unreviewed server value: %s", raw)
					}
					if query.Get("dryRun") == "All" {
						dry.Add(1)
						if tc.mode == "takeover" && !force {
							w.WriteHeader(409)
							fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Conflict","code":409,"details":{"causes":[{"reason":"FieldManagerConflict","field":".data.marker","message":"conflict with \"other\""}]}}`)
							return
						}
						reviewed.Store(string(raw))
						fmt.Fprint(w, preview)
						return
					}
					real.Add(1)
					if reviewed.Load() != string(raw) || force != (tc.mode == "takeover") {
						t.Error("confirmation did not keep the reviewed bytes and force setting")
					}
					switch tc.mode {
					case "deleted after GET":
						w.WriteHeader(409)
					case "unexpected creation":
						w.WriteHeader(201)
						fmt.Fprint(w, preview)
					case "admission changed":
						fmt.Fprint(w, strings.ReplaceAll(preview, "from-admission", "changed-on-execution"))
					case "invalid success":
						fmt.Fprint(w, strings.ReplaceAll(preview, `"uid"`, `"wrong"`))
					case "oversize success":
						fmt.Fprint(w, strings.ReplaceAll(preview, "synthetic", strings.Repeat("x", editLimit)))
					case "lost response":
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						_ = conn.Close()
					default:
						fmt.Fprint(w, strings.ReplaceAll(preview, `"7"`, `"8"`))
					}
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			})
			s := NewApplyService(base.connections, kube.NewService(base.connections, session.NewManager(nil)), base.coordinator)
			ctx := context.Background()
			draft, err := s.OpenApply(ctx, kube.EditTarget{Connection: request.Resource.Connection, Version: "v1", Kind: "ConfigMap", Namespace: "default", Name: "web", UID: "uid"})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(draft.YAML, "marker") || !strings.Contains(draft.BaselineYAML, "marker") {
				t.Fatal("the editor should start from identity only")
			}
			req := ApplyRequest{DraftID: draft.ID, YAML: draft.YAML + "data:\n  marker: synthetic\n"}
			p, err := s.PrepareApply(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			if tc.mode == "takeover" {
				if p.Token != "" || len(p.Conflicts) != 1 {
					t.Fatalf("invalid conflict review: %+v", p)
				}
				req.TakeOwnership = true
				p, err = s.PrepareApply(ctx, req)
			}
			if err != nil || p.Token == "" || !strings.Contains(p.ServerYAML, "from-admission") {
				t.Fatalf("invalid preview: %+v %v", p, err)
			}
			if _, err := base.ExecuteScale(ctx, p.Token, ""); err == nil {
				t.Fatal("Scale took an apply review")
			}
			out, err := s.ExecuteApplyObserved(p.Token)
			r := out.OperationResult
			if err != nil || r.State != tc.state {
				t.Fatalf("got %+v %v, want %s", r, err, tc.state)
			}
			if tc.mode == "admission changed" && (!out.ResponseChanged || !strings.Contains(out.ObservedYAML, "changed-on-execution")) {
				t.Fatal("the submitting editor did not receive a changed server response")
			}
			duplicate, err := s.ExecuteApplyObserved(p.Token)
			expectedWrites := int32(1)
			if tc.mode == "stale before dispatch" {
				expectedWrites = 0
			}
			if err != nil || r.OperationID != duplicate.OperationID || duplicate.ResponseChanged || duplicate.ObservedYAML != "" || real.Load() != expectedWrites {
				t.Fatalf("duplicate reapplied or did not recover original result: %+v %v, writes=%d", duplicate, err, real.Load())
			}
			summary, _ := json.Marshal(r)
			if strings.Contains(string(summary), "synthetic") || strings.Contains(string(summary), "from-admission") {
				t.Error("result leaked edited values")
			}
		})
	}
}
