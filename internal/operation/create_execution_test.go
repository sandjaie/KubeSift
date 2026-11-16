/*
Purpose: Verify one immutable create dispatch and name-only reconciliation.
Used by: Native operation regression tests.
Responsibilities: Count wire calls and distinguish accepted, rejected and uncertain results.
Important: The synthetic endpoint never handles real credentials or Kubernetes objects.
*/
package operation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCreateSingleDispatch(t *testing.T) {
	for _, tc := range []struct{ mode, state string }{
		{"created", "accepted"}, {"already-exists", "rejected"}, {"unauthorized", "rejected"}, {"rate-limit", "rejected"},
		{"wrong-status", "unknown"}, {"mismatch", "unknown"}, {"no-version", "unknown"}, {"oversize", "unknown"}, {"eof", "unknown"},
		{"redirect", "unknown"}, {"proxy-error", "unknown"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			var mutations, dry, reads atomic.Int32
			var mu sync.Mutex
			var prepared string
			base, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /api/v1":
					fmt.Fprint(w, createDiscovery)
				case "GET /api/v1/namespaces/default/configmaps/demo":
					reads.Add(1)
					w.WriteHeader(http.StatusForbidden)
				case "POST /api/v1/namespaces/default/configmaps":
					raw, _ := io.ReadAll(r.Body)
					q := r.URL.Query()
					if q.Get("fieldManager") != "kubesift-create" || q.Get("fieldValidation") != "Strict" {
						t.Error("invalid query")
					}
					mu.Lock()
					defer mu.Unlock()
					if q.Get("dryRun") == "All" {
						dry.Add(1)
						prepared = string(raw)
						w.WriteHeader(201)
						fmt.Fprint(w, `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"demo","namespace":"default","uid":"simulated","resourceVersion":"0"},"data":{"key":"server-default"}}`)
						return
					}
					mutations.Add(1)
					if string(raw) != prepared || strings.Contains(string(raw), "server-default") {
						t.Error("confirmed payload is not the reviewed proposal")
					}
					sample := `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"demo","namespace":"default","uid":"real","resourceVersion":"71"}}`
					switch tc.mode {
					case "created":
						w.WriteHeader(201)
						fmt.Fprint(w, sample)
					case "wrong-status":
						fmt.Fprint(w, sample)
					case "mismatch":
						w.WriteHeader(201)
						fmt.Fprint(w, strings.ReplaceAll(sample, `"demo"`, `"other"`))
					case "no-version":
						w.WriteHeader(201)
						fmt.Fprint(w, strings.ReplaceAll(sample, `"71"`, `""`))
					case "already-exists":
						w.WriteHeader(409)
					case "unauthorized":
						w.WriteHeader(401)
					case "rate-limit":
						w.Header().Set("Retry-After", "0")
						w.WriteHeader(429)
					case "proxy-error":
						w.WriteHeader(502)
					case "redirect":
						http.Redirect(w, r, "/must-not-follow", 307)
					case "oversize":
						w.WriteHeader(201)
						fmt.Fprint(w, `{"value":"`, strings.Repeat("x", 16<<20), `"}`)
					case "eof":
						conn, _, err := w.(http.Hijacker).Hijack()
						if err == nil {
							_ = conn.Close()
						} else {
							t.Error(err)
						}
					}
				default:
					t.Error("Unexpected API request")
					w.WriteHeader(404)
				}
			})
			ctx := context.Background()
			service := fixtureCreateService(t, base)
			draft, err := service.OpenCreate(ctx, req.Resource.Connection, "default")
			if err != nil {
				t.Fatal(err)
			}
			p, err := service.PrepareCreate(ctx, CreateRequest{DraftID: draft.ID, YAML: createMap, Namespace: "default"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := base.ExecuteScale(ctx, p.Token, ""); err == nil {
				t.Fatal("Scale accepted a create token")
			}
			result, err := service.ExecuteCreate(p.Token)
			if err != nil || result.State != tc.state {
				t.Fatalf("bad result: %+v %v", result, err)
			}
			duplicate, err := service.ExecuteCreate(p.Token)
			if err != nil || duplicate.OperationID != result.OperationID || mutations.Load() != 1 || dry.Load() != 1 || reads.Load() != 0 {
				t.Fatalf("create replayed or required GET: %+v %v", duplicate, err)
			}
			if tc.state == "accepted" {
				if result.Resource.UID != "real" || result.Create.ResourceVersion != "71" {
					t.Fatal("stored wrong identity")
				}
			} else if result.Resource.UID != "" || result.Create.ResourceVersion != "" {
				t.Fatal("stored an unverified identity")
			}
			if tc.state != "rejected" {
				obs, err := service.InspectCreateName(ctx, result.OperationID)
				if err != nil || obs.State != "unavailable" || reads.Load() != 1 || mutations.Load() != 1 {
					t.Fatalf("wrong observation: %+v %v", obs, err)
				}
				saved, err := base.coordinator.GetOperationResult(result.OperationID)
				if err != nil || saved.State != tc.state {
					t.Fatal("inspection changed write outcome")
				}
			}
		})
	}
}
