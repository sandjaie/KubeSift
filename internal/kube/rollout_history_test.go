/*
Purpose: Validate retained history from bounded, owned controller sources.
Used by: Rollout history reader tests.
Responsibilities: Exercise owner identity, exact revisions, denied reads and continuation.
Important: Only synthetic workloads are read.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestRolloutHistory(t *testing.T) {
	for _, tc := range []struct {
		kind, plural, source, sources string
		denied, partial               bool
	}{
		{"Deployment", "deployments", "ReplicaSet", "replicasets", false, true},
		{"StatefulSet", "statefulsets", "ControllerRevision", "controllerrevisions", false, false},
		{"DaemonSet", "daemonsets", "ControllerRevision", "controllerrevisions", false, false},
		{"Deployment", "deployments", "ReplicaSet", "replicasets", true, false},
	} {
		t.Run(fmt.Sprintf("%s/denied=%t", tc.kind, tc.denied), func(t *testing.T) {
			const prefix = "/apis/apps/v1/namespaces/test/"
			const revision = "9007199254740993"
			var lists atomic.Int32
			s, connection := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case prefix + tc.plural + "/parent":
					fmt.Fprintf(w, `{"apiVersion":"apps/v1","kind":%q,"metadata":{"name":"parent","namespace":"test","uid":"parent-uid","resourceVersion":"7"},"status":{"currentRevision":"one","updateRevision":"two"}}`, tc.kind)
				case prefix + tc.sources:
					if r.URL.Query().Get("limit") != "100" {
						t.Errorf("unbounded list: %s", r.URL)
					}
					if tc.denied {
						w.WriteHeader(403)
						return
					}
					page := lists.Add(1)
					row := func(name, uid, ownerUID, number string) string {
						extra := `"revision":` + number
						if tc.source == "ReplicaSet" {
							extra = `"spec":{"template":{"spec":{"containers":[{"name":"app","image":"fixture"}]}}}`
						}
						return fmt.Sprintf(`{"apiVersion":"apps/v1","kind":%q,"metadata":{"name":%q,"uid":%q,"namespace":"test","resourceVersion":"8","labels":{"app":"same"},"annotations":{"deployment.kubernetes.io/revision":%q},"ownerReferences":[{"apiVersion":"apps/v1","kind":%q,"name":"parent","uid":%q,"controller":true}]},%s}`,
							tc.source, name, uid, number, tc.kind, ownerUID, extra)
					}
					items := row("imposter", "other", "replaced-parent", revision)
					next := ""
					if tc.partial && page <= 8 {
						next = fmt.Sprint(page)
					} else {
						if tc.partial && r.URL.Query().Get("continue") != "8" {
							t.Error("continuation was not retained")
						}
						items += "," + row("two", "b", "parent-uid", revision) + "," + row("one", "a", "parent-uid", revision) + "," + row("earlier", "c", "parent-uid", "3")
						if tc.source == "ReplicaSet" {
							items += "," + row("bad", "invalid", "parent-uid", "unusable")
						}
					}
					fmt.Fprintf(w, `{"apiVersion":"apps/v1","kind":%q,"metadata":{"continue":%q},"items":[%s]}`, tc.source+"List", next, items)
				default:
					t.Errorf("unexpected request: %s", r.URL)
					w.WriteHeader(404)
				}
			})
			ctx := context.Background()
			req := RolloutHistoryRequest{Resource: ResourceRef{Connection: connection, Kind: tc.kind, APIVersion: "apps/v1", Namespace: "test", Name: "parent", UID: "parent-uid"}}
			page, err := s.ReadRolloutHistory(ctx, req)
			if err != nil || page.SourceVersion != "7" {
				t.Fatalf("history: %+v, %v", page, err)
			}
			if tc.denied {
				if page.Problem == nil || page.Problem.Code != "forbidden" || page.Complete {
					t.Fatalf("denial looked like an empty history: %+v", page)
				}
				return
			}
			if tc.partial {
				if page.Complete || page.Page.NextCursor == "" || len(page.Items) != 0 || lists.Load() != 8 {
					t.Fatalf("bounded continuation: %+v", page)
				}
				req.Cursor = page.Page.NextCursor
				page, err = s.ReadRolloutHistory(ctx, req)
			}
			if err != nil || !page.Complete || len(page.Items) != 3 || page.Items[0].Revision != revision ||
				page.Items[0].Resource.UID != "a" || page.Items[1].Resource.UID != "b" || page.Items[0].ResourceVersion != "8" {
				t.Fatalf("explicit revisions: %+v, %v", page, err)
			}
			if tc.kind == "StatefulSet" && (page.CurrentRevision != "one" || page.UpdateRevision != "two") ||
				tc.kind != "StatefulSet" && (page.CurrentRevision != "" || page.UpdateRevision != "") {
				t.Fatalf("unexpected controller references: %+v", page)
			}
		})
	}
}
