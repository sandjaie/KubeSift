/*
Purpose: Verify server-reviewed YAML edits are submitted once.
Used by: Native operation tests.
Responsibilities: Exercise dry-run, exact conditional bytes, conflicts and ambiguous outcomes.
Important: The synthetic document must not enter retained operation history.
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

func TestEditExecution(t *testing.T) {
	const object = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":"7"},"spec":{"replicas":1}}`
	for _, tc := range []struct {
		name, want  string
		dry, actual int
	}{
		{"accepted", "accepted", 200, 200},
		{"denied", "forbidden", 403, 0},
		{"immutable", "immutable", 422, 0},
		{"conflict", "stale", 200, 409},
		{"unknown", "unknown", 200, 500},
		{"shared lock", "busy", 200, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var preview, writes atomic.Int32
			var reviewed atomic.Value
			scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/apis/apps/v1":
					fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"apps/v1","resources":[{"name":"deployments","kind":"Deployment","namespaced":true,"verbs":["get","patch"]}]}`)
				case r.Method == http.MethodGet && r.URL.Path == scalePath:
					fmt.Fprint(w, object)
				case r.Method == http.MethodPatch && r.URL.Path == scalePath:
					body, _ := io.ReadAll(r.Body)
					if r.URL.Query().Get("fieldValidation") != "Strict" {
						t.Error("missing field validation")
					}
					code := tc.actual
					if r.URL.Query().Get("dryRun") == "All" {
						preview.Add(1)
						reviewed.Store(string(body))
						code = tc.dry
					} else {
						writes.Add(1)
						if string(body) != reviewed.Load() {
							t.Error("real PATCH differed from dry-run")
						}
					}
					if code == 0 {
						t.Error("unexpected write")
						code = 500
					}
					w.WriteHeader(code)
					if code == 200 {
						fmt.Fprint(w, strings.ReplaceAll(object, `"replicas":1`, `"replicas":2,"server":"defaulted"`))
					} else {
						fmt.Fprint(w, `{"kind":"Status","status":"Failure","message":"immutable field: private-value"}`)
					}
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					w.WriteHeader(404)
				}
			})
			s := NewEditService(scale.connections, kube.NewService(scale.connections, session.NewManager(nil)), scale.coordinator)
			ctx := context.Background()
			draft, err := s.OpenEdit(ctx, kube.EditTarget{Connection: req.Resource.Connection, Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web", UID: "uid"})
			if err != nil {
				t.Fatal(err)
			}
			p, err := s.PrepareEdit(ctx, EditRequest{DraftID: draft.ID, YAML: strings.ReplaceAll(draft.YAML, `"replicas": 1`, `"replicas": 2`)})
			if tc.dry != 200 {
				checkCode(t, err, tc.want)
				if preview.Load() != 1 || writes.Load() != 0 || strings.Contains(err.Error(), "private-value") {
					t.Fatal("invalid preview handling")
				}
				return
			}
			if err != nil || !strings.Contains(p.ServerYAML, "defaulted") {
				t.Fatalf("prepare failed: %v", err)
			}
			if tc.name == "shared lock" {
				scaleToken := saveFixtureReview(t, scale.coordinator, "scale", req.Resource)
				_, running, err := scale.coordinator.begin("scale", scaleToken, "")
				if err != nil {
					t.Fatal(err)
				}
				defer scale.coordinator.finish(running)
				_, err = s.ExecuteEdit(p.Token)
				checkCode(t, err, "busy")
				return
			}
			result, err := s.ExecuteEdit(p.Token)
			if err != nil || result.State != tc.want {
				t.Fatalf("result %s: %v", result.State, err)
			}
			repeated, err := s.ExecuteEdit(p.Token)
			if err != nil || repeated.OperationID != result.OperationID || preview.Load() != 1 || writes.Load() != 1 {
				t.Fatal("execution replayed or lost its result")
			}
			raw, _ := json.Marshal(result)
			for _, value := range []string{"replicas", "private-value", "defaulted"} {
				if strings.Contains(string(raw), value) {
					t.Fatal("operation history retained content")
				}
			}
		})
	}
}
