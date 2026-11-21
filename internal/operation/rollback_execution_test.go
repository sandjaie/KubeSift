/*
Purpose: Exercise reviewed rollback at its native HTTP boundary.
Used by: Workload operation tests.
Responsibilities: Verify dry-run, races, the shared lock and single dispatch.
Important: The test server has no live cluster credentials.
*/
package operation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sandjaie/KubeSift/internal/kube"
)

func TestRollbackExecution(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		writes     int32
	}{
		{"denied", "forbidden", 0},
		{"accepted", "accepted", 1},
		{"changed source", "rejected", 0},
		{"late conflict", "stale", 1},
		{"lost response", "unknown", 1},
		{"shared edit lock", "busy", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const sourcePath = "/apis/apps/v1/namespaces/default/replicasets/web-old"
			var stage, real atomic.Int32
			reply := func(w http.ResponseWriter, version, image string) {
				fmt.Fprintf(w, `{"kind":"Deployment","apiVersion":"apps/v1","metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":%q},"spec":{"replicas":0,"paused":true,"template":{"spec":{"containers":[{"name":"app","image":%q}]}}}}`, version, image)
			}
			base, scaleReq := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET " + scalePath:
					version := "7"
					if tc.name == "late conflict" && real.Load() > 0 {
						version = "8"
					}
					reply(w, version, "demo:new")
				case "GET " + sourcePath:
					version := "11"
					if tc.name == "changed source" && stage.Load() > 0 {
						version = "12"
					}
					fmt.Fprintf(w, `{"kind":"ReplicaSet","apiVersion":"apps/v1","metadata":{"name":"web-old","namespace":"default","uid":"source","resourceVersion":%q,"annotations":{"deployment.kubernetes.io/revision":"2"},"ownerReferences":[{"apiVersion":"apps/v1","kind":"Deployment","name":"web","uid":"uid","controller":true}]},"spec":{"template":{"spec":{"containers":[{"name":"app","image":"demo:old"}]}}}}`, version)
				case "PATCH " + scalePath:
					body, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(body), `"path":"/spec/template"`) || strings.Contains(string(body), "/spec/replicas") {
						t.Errorf("unexpected patch paths: %s", body)
					}
					if r.URL.Query().Get("dryRun") == "All" {
						if tc.name == "denied" {
							w.WriteHeader(403)
							return
						}
						reply(w, "7", "demo:admitted")
						return
					}
					real.Add(1)
					switch tc.name {
					case "late conflict":
						w.WriteHeader(422)
					case "lost response":
						w.WriteHeader(503)
					default:
						reply(w, "8", "demo:admitted")
					}
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			})
			ctx := context.Background()
			req := RollbackRequest{Resource: scaleReq.Resource, ResourceVersion: "7", SourceVersion: "11", Revision: "2",
				Source: kube.ResourceRef{Connection: scaleReq.Resource.Connection, APIVersion: "apps/v1", Kind: "ReplicaSet", Namespace: "default", Name: "web-old", UID: "source"}}
			s := NewRollbackService(base.connections, base.coordinator)
			preview, err := s.PrepareRollback(ctx, req)
			if tc.name == "denied" {
				checkCode(t, err, tc.want)
				if preview.Token != "" || real.Load() != 0 {
					t.Fatal("denied dry-run allowed a write")
				}
				return
			}
			if err != nil || preview.Token == "" || !strings.Contains(preview.BeforeYAML, "demo:new") ||
				!strings.Contains(preview.ProposedYAML, "demo:old") || !strings.Contains(preview.ServerYAML, "demo:admitted") ||
				!strings.Contains(strings.Join(preview.Notices, " "), "paused") {
				t.Fatalf("unexpected review: %+v %v", preview, err)
			}
			stage.Store(1)
			if tc.name == "shared edit lock" {
				edit := kube.EditTarget{Connection: req.Resource.Connection, Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespace: "default", Name: "web", UID: "uid"}
				token, _, err := base.coordinator.save(ctx, pendingReview{result: OperationResult{Action: "edit", Resource: req.Resource, Edit: &edit}})
				if err != nil {
					t.Fatal(err)
				}
				_, active, err := base.coordinator.begin("edit", token, "")
				if err != nil {
					t.Fatal(err)
				}
				defer base.coordinator.finish(active)
				_, err = s.ExecuteRollback(preview.Token)
				checkCode(t, err, "busy")
				if real.Load() != 0 {
					t.Fatal("rollback bypassed the edit lock")
				}
				return
			}
			result, err := s.ExecuteRollback(preview.Token)
			if err != nil || result.State != tc.want || result.Rollback == nil || result.Rollback.SourceUID != "source" {
				t.Fatalf("result %+v: %v", result, err)
			}
			twice, err := s.ExecuteRollback(preview.Token)
			if err != nil || twice.OperationID != result.OperationID || real.Load() != tc.writes {
				t.Fatalf("duplicate dispatched: %+v %v (%d real patches)", twice, err, real.Load())
			}
		})
	}
}
