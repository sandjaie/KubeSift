/*
Purpose: Verify configuration scope, projections and cursor contracts.
Used by: Configuration reader regression tests.
Responsibilities: Assert invalid scopes never reach the API and ConfigMap inventories omit values.
Important: All captured objects are synthetic.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConfigurationInventoryScopes(t *testing.T) {
	for _, tc := range []struct{ kind, ns, path string }{
		{"Namespace", "", "/api/v1/namespaces"},
		{"ResourceQuota", "test", "/api/v1/namespaces/test/resourcequotas"},
		{"LimitRange", "test", "/api/v1/namespaces/test/limitranges"},
		{"ConfigMap", "test", "/api/v1/namespaces/test/configmaps"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var calls atomic.Int32
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case tc.path:
					if r.URL.Query().Get("limit") != "100" {
						t.Error("unbounded list")
					}
					if r.URL.Query().Get("continue") != "" {
						w.WriteHeader(410)
						fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","reason":"Expired","code":410}`)
						return
					}
					fmt.Fprintf(w, `{"apiVersion":"v1","kind":%q,"metadata":{"continue":"next"},"items":[{"metadata":{"name":"sample","namespace":%q,"uid":"uid"},"data":{"key":"value-sentinel"},"binaryData":{"key2":"AP8="}}]}`, tc.kind+"List", tc.ns)
				case tc.path + "/sample":
					fmt.Fprintf(w, `{"apiVersion":"v1","kind":%q,"metadata":{"name":"sample","namespace":%q,"uid":"uid","resourceVersion":"7"}}`, tc.kind, tc.ns)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			ctx := context.Background()
			req := PageRequest{Connection: c, Kind: tc.kind, Namespace: tc.ns}
			page, err := s.ReadConfiguration(ctx, req)
			if err != nil || len(page.Items) != 1 || page.Page.NextCursor == "" {
				t.Fatal(page, err)
			}
			row := page.Items[0]
			if strings.Contains(row.ObjectJSON, "value-sentinel") || strings.Contains(row.ObjectJSON, "AP8=") {
				t.Fatal("inventory exposed values")
			}
			doc, err := s.ReadResourceYAML(ctx, DetailRequest{Resource: row.Resource})
			if err != nil || !strings.Contains(doc.WithoutManagedFields, "kind: "+tc.kind) {
				t.Fatal(doc, err)
			}
			before := calls.Load()
			bad := req
			if tc.ns == "" {
				bad.Namespace = "test"
				if _, err = s.ReadResourceEvents(ctx, EventRequest{Resource: row.Resource}); err == nil {
					t.Fatal("Namespace Events allowed")
				}
			} else {
				bad.Namespace = ""
			}
			if _, err = s.ReadConfiguration(ctx, bad); err == nil {
				t.Fatal("invalid scope accepted")
			}
			ref := row.Resource
			ref.APIVersion = "v2"
			if _, err = s.ReadResource(ctx, DetailRequest{Resource: ref}); err == nil {
				t.Fatal("invalid API accepted")
			}
			bad = req
			bad.Cursor = page.Page.NextCursor
			bad.Connection.Generation++
			if _, err = s.ReadConfiguration(ctx, bad); err == nil || calls.Load() != before {
				t.Fatal("invalid request reached API")
			}
			req.Cursor = page.Page.NextCursor
			if _, err = s.ReadConfiguration(ctx, req); err == nil || !strings.HasPrefix(err.Error(), "list_changed:") {
				t.Fatal("expired cursor", err)
			}
		})
	}
}

func TestQuotaPreservesReportedSpelling(t *testing.T) {
	s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"kind":"ResourceQuota","apiVersion":"v1","metadata":{"namespace":"test","name":"sample","uid":"uid","resourceVersion":"7"},"spec":{"hard":{"cpu":"0.50"}},"status":{"used":{"cpu":"0"}}}`)
	})
	ref := ResourceRef{Connection: c, Kind: "ResourceQuota", APIVersion: "v1", Namespace: "test", Name: "sample", UID: "uid"}
	detail, err := s.ReadResource(context.Background(), DetailRequest{Resource: ref})
	if err != nil || !strings.Contains(detail.ObjectJSON, `"cpu":"0.50"`) {
		t.Fatal(detail, err)
	}
}
