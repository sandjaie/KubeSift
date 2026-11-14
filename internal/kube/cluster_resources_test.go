/*
Purpose: Verify fixed infrastructure inventory without API discovery.
Used by: Kubernetes reader tests.
Responsibilities: Check explicit routes, evidence presence, captures and paging scope.
Important: Synthetic responses only.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestClusterResourceRoutes(t *testing.T) {
	for _, tc := range []struct{ kind, version, path, ns, extra, contains string }{
		{"Node", "v1", "/api/v1/nodes", "", `"spec":{"unschedulable":false},"status":{"nodeInfo":{"kubeletVersion":"v1.33"}}`, `"unschedulable":false`},
		{"PriorityClass", "scheduling.k8s.io/v1", "/apis/scheduling.k8s.io/v1/priorityclasses", "", `"value":0,"globalDefault":false`, `"value":"0"`},
		{"RuntimeClass", "node.k8s.io/v1", "/apis/node.k8s.io/v1/runtimeclasses", "", `"handler":"test"`, `"handler":"test"`},
		{"Lease", "coordination.k8s.io/v1", "/apis/coordination.k8s.io/v1/namespaces/default/leases", "default", `"spec":{"leaseDurationSeconds":0}`, `"leaseDurationSeconds":0`},
		{"CustomResourceDefinition", "apiextensions.k8s.io/v1", "/apis/apiextensions.k8s.io/v1/customresourcedefinitions", "", `"spec":{"group":"example.test","names":{"kind":"Widget"},"scope":"Namespaced","versions":[{"schema":{"huge":"omit"}}]}`, `"scope":"Namespaced"`},
		{"APIService", "apiregistration.k8s.io/v1", "/apis/apiregistration.k8s.io/v1/apiservices", "", `"spec":{"group":"example.test","version":"v1"}`, `"version":"v1"`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			raw := fmt.Sprintf(`{"kind":%q,"apiVersion":%q,"metadata":{"name":"sample","namespace":%q,"uid":"id","resourceVersion":"7"},%s}`, tc.kind, tc.version, tc.ns, tc.extra)
			calls := 0
			s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case tc.path:
					if r.Method != "GET" || r.URL.Query().Get("limit") != "100" {
						t.Error("expected bounded GET")
					}
					fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"continue":"next"},"items":[%s]}`, tc.kind+"List", tc.version, `{`+raw[strings.Index(raw, `"metadata"`):])
				case tc.path + "/sample":
					fmt.Fprint(w, raw)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(403)
				}
			})
			ctx := context.Background()
			req := PageRequest{Connection: ref, Kind: tc.kind, Namespace: tc.ns}
			page, err := s.ReadClusterResources(ctx, req)
			if err != nil || len(page.Items) != 1 || page.Page.NextCursor == "" || !strings.Contains(page.Items[0].ObjectJSON, tc.contains) || strings.Contains(page.Items[0].ObjectJSON, "huge") {
				t.Fatalf("page = %+v, %v", page, err)
			}
			detail, err := s.ReadResource(ctx, DetailRequest{Resource: page.Items[0].Resource})
			if err != nil || detail.ObjectJSON != raw {
				t.Fatalf("detail = %+v, %v", detail, err)
			}
			req.Namespace = "wrong"
			if tc.ns != "" {
				req.Namespace = ""
			}
			if _, err = s.ReadClusterResources(ctx, req); err == nil || calls != 2 {
				t.Fatalf("scope failure %v, calls=%d", err, calls)
			}
		})
	}
}
