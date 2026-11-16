/*
Purpose: Prove single-dispatch deletion and exact post-confirmation checks.
Used by: Native operation regression tests.
Responsibilities: Count real writes independently of preview and verify retained outcomes.
Important: Synthetic operations never touch a Kubernetes cluster.
*/
package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
)

const deleteSuccess = `{"apiVersion":"v1","kind":"Status","status":"Success","details":{"group":"apps","kind":"deployments","name":"web","uid":"uid"}}`

func TestDeleteNeverReplays(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		state  string
		body   string
	}{
		{"accepted", 202, "accepted", deleteSuccess},
		{"rate limited", 429, "rejected", ""},
		{"redirect", 307, "unknown", ""},
		{"unauthorized", 401, "rejected", ""},
		{"precondition status", 412, "rejected", ""},
		{"conflict", 409, "stale", ""},
		{"upstream error", 503, "unknown", ""},
		{"timeout status", 408, "unknown", ""},
		{"lost response", 0, "unknown", ""},
		{"unexpected success status", 201, "unknown", deleteSuccess},
		{"malformed", 200, "unknown", "{"},
		{"oversized", 200, "unknown", strings.Repeat(" ", (16<<20)+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls [2]atomic.Int32
			scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					fmt.Fprint(w, restartFixture("Deployment", "{}", "{}"))
				case http.MethodPost:
					w.WriteHeader(http.StatusForbidden)
				case http.MethodDelete:
					var options map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
						t.Error(err)
					}
					if _, preview := options["dryRun"]; preview {
						calls[0].Add(1)
						fmt.Fprint(w, deleteSuccess)
						return
					}
					calls[1].Add(1)
					if len(options) != 4 || string(options["preconditions"]) != `{"uid":"uid","resourceVersion":"7"}` {
						t.Errorf("unsafe actual DELETE: %s", options)
					}
					if tc.status == 0 {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						_ = conn.Close()
						return
					}
					w.Header().Set("Location", scalePath)
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				}
			})
			ctx := context.Background()
			service := NewDeleteService(scale.connections, scale.coordinator, nil)
			request := DeleteRequest{req.Resource, req.SourceVersion}
			preview, err := service.PrepareDelete(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = scale.ExecuteScale(ctx, preview.Token, "web"); err == nil {
				t.Fatal("wrong action claimed delete token")
			}
			if _, err = service.ExecuteDelete(ctx, preview.Token, "Web"); err == nil || calls[1].Load() != 0 {
				t.Fatal("incorrect typed name was dispatched")
			}
			preview, err = service.PrepareDelete(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			abandoned, cancel := context.WithCancel(ctx)
			cancel()
			result, err := service.ExecuteDelete(abandoned, preview.Token, "web")
			if err != nil || result.State != tc.state || result.Delete == nil || result.Restart != nil || result.Scale != nil {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			recovered, err := service.ExecuteDelete(ctx, preview.Token, "web")
			if err != nil || recovered.OperationID != result.OperationID || calls[0].Load() != 2 || calls[1].Load() != 1 {
				t.Fatalf("replayed write or lost result: %+v %v", recovered, err)
			}
		})
	}
}

func TestDeleteExecutionRechecksIdentity(t *testing.T) {
	for _, change := range []string{"version", "replacement", "terminating", "closed"} {
		t.Run(change, func(t *testing.T) {
			var confirmed atomic.Bool
			scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					object := restartFixture("Deployment", "{}", "{}")
					if confirmed.Load() {
						switch change {
						case "version":
							object = strings.Replace(object, `"resourceVersion":"7"`, `"resourceVersion":"8"`, 1)
						case "replacement":
							object = strings.Replace(object, `"uid":"uid"`, `"uid":"other"`, 1)
						case "terminating":
							object = strings.Replace(object, `"resourceVersion":"7"`, `"resourceVersion":"7","deletionTimestamp":"2026-11-15T00:00:00Z"`, 1)
						}
					}
					fmt.Fprint(w, object)
				case http.MethodPost:
					w.WriteHeader(403)
				case http.MethodDelete:
					if confirmed.Load() {
						t.Error("dispatched after target changed")
					}
					fmt.Fprint(w, deleteSuccess)
				}
			})
			ctx := context.Background()
			service := NewDeleteService(scale.connections, scale.coordinator, nil)
			preview, err := service.PrepareDelete(ctx, DeleteRequest{req.Resource, req.SourceVersion})
			if err != nil {
				t.Fatal(err)
			}
			if change == "closed" {
				_ = scale.connections.Close(req.Resource.Connection)
			}
			confirmed.Store(true)
			result, err := service.ExecuteDelete(ctx, preview.Token, "web")
			if change == "closed" {
				if err == nil {
					t.Fatal("closing connection did not revoke review")
				}
			} else if err != nil || result.State != "stale" {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestDeleteSharesOperationCoordinator(t *testing.T) {
	coordinator := NewCoordinator(nil, nil)
	t.Cleanup(coordinator.Close)
	req := DeleteRequest{}
	for _, action := range []string{"scale", "restart"} {
		prior, _, err := coordinator.save(context.Background(), pendingReview{result: OperationResult{Action: action, Resource: req.Resource}})
		if err != nil {
			t.Fatal(err)
		}
		deletion, _, err := coordinator.save(context.Background(), pendingReview{result: OperationResult{Action: "delete", Resource: req.Resource}})
		if err != nil {
			t.Fatal(err)
		}
		coordinator.revoke("delete", prior)
		_, result, err := coordinator.begin(action, prior, "")
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = coordinator.begin("delete", deletion, "")
		problem, ok := err.(*model.Problem)
		if !ok || problem.Code != "busy" {
			t.Fatalf("delete bypassed %s lock: %v", action, err)
		}
		coordinator.finish(result)
	}
}

func TestDeleteConnectionClosesAfterDispatch(t *testing.T) {
	var writes atomic.Int32
	sent := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			fmt.Fprint(w, restartFixture("Deployment", "{}", "{}"))
		case http.MethodPost:
			w.WriteHeader(http.StatusForbidden)
		case http.MethodDelete:
			var options map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
				t.Error(err)
			}
			if _, dry := options["dryRun"]; dry {
				fmt.Fprint(w, deleteSuccess)
				return
			}
			writes.Add(1)
			select {
			case sent <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}
	})
	ctx := context.Background()
	service := NewDeleteService(scale.connections, scale.coordinator, nil)
	preview, err := service.PrepareDelete(ctx, DeleteRequest{req.Resource, req.SourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan OperationResult, 1)
	go func() {
		result, err := service.ExecuteDelete(ctx, preview.Token, "web")
		if err != nil {
			t.Error(err)
		}
		done <- result
	}()
	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		t.Fatal("real DELETE was not dispatched")
	}
	if err := scale.connections.Close(req.Resource.Connection); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if result.State != "unknown" || writes.Load() != 1 {
			t.Fatalf("post-dispatch close=%+v writes=%d", result, writes.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connection close did not end the write")
	}
}
