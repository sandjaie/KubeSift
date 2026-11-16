/*
Purpose: Verify operation-bound deletion readback.
Used by: Native operation regression tests.
Responsibilities: Distinguish object absence, replacement, termination and unavailable evidence.
Important: Checks are bounded GETs and never overwrite the accepted result.
*/
package operation

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDeleteObservation(t *testing.T) {
	object := restartFixture("Deployment", "{}", "{}")
	missing := `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"NotFound","code":404,"details":{"group":"apps","kind":"deployments","name":"web"}}`
	for _, tc := range []struct {
		state  string
		status int
		body   string
	}{
		{"present", 200, object},
		{"terminating", 200, strings.Replace(object, `"uid":"uid"`, `"uid":"uid","deletionTimestamp":"2026-11-15T01:00:00Z"`, 1)},
		{"absent", 404, missing},
		{"replaced", 200, strings.Replace(object, `"uid":"uid"`, `"uid":"replacement"`, 1)},
		{"unavailable", 404, strings.Replace(missing, `"deployments"`, `"Deployment"`, 1)},
		{"unavailable", 404, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"NotFound","code":404}`},
		{"unavailable", 403, missing},
		{"unavailable", 200, strings.Replace(object, `"namespace":"default"`, `"namespace":"other"`, 1)},
	} {
		t.Run(tc.state, func(t *testing.T) {
			ctx := context.Background()
			var checks atomic.Int32
			scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != scalePath {
					t.Errorf("unexpected call %s %s", r.Method, r.URL)
				}
				checks.Add(1)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			coordinator := scale.coordinator
			service := NewDeleteService(scale.connections, coordinator, nil)
			token, _, err := coordinator.save(ctx, pendingReview{result: OperationResult{Action: "delete", Resource: req.Resource, Delete: &DeleteResultData{}}})
			if err != nil {
				t.Fatal(err)
			}
			_, result, err := coordinator.begin("delete", token, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{result.OperationID, "unissued"} {
				if _, err := service.CheckDeleteTarget(ctx, id); err == nil {
					t.Fatal("invalid or executing result could be checked")
				}
			}
			result.State = "accepted"
			coordinator.finish(result)
			obs, err := service.CheckDeleteTarget(ctx, result.OperationID)
			if err != nil || obs.State != tc.state || obs.Resource != req.Resource || obs.OperationID != result.OperationID || checks.Load() != 1 {
				t.Fatalf("observation=%+v error=%v checks=%d", obs, err, checks.Load())
			}
			saved, err := coordinator.GetOperationResult(result.OperationID)
			if err != nil || saved.State != "accepted" {
				t.Fatalf("observation changed execution: %+v %v", saved, err)
			}
			if err := scale.connections.Close(req.Resource.Connection); err != nil {
				t.Fatal(err)
			}
			if _, err := service.CheckDeleteTarget(ctx, result.OperationID); err == nil || checks.Load() != 1 {
				t.Fatal("check used a closed connection")
			}
		})
	}
}
