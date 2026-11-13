/*
Purpose: Test networking reads against synthetic Kubernetes API responses.
Used by: Networking reader regression checks.
Responsibilities: Verify scope, bounded evidence and reference identity before any target read.
Important: No live contexts or credentials are used.
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

func TestNetworkingInventoryScope(t *testing.T) {
	for _, tc := range []struct{ kind, version, ns, path string }{
		{"Service", "v1", "test", "/api/v1/namespaces/test/services"},
		{"EndpointSlice", "discovery.k8s.io/v1", "test", "/apis/discovery.k8s.io/v1/namespaces/test/endpointslices"},
		{"Ingress", "networking.k8s.io/v1", "test", "/apis/networking.k8s.io/v1/namespaces/test/ingresses"},
		{"IngressClass", "networking.k8s.io/v1", "", "/apis/networking.k8s.io/v1/ingressclasses"},
		{"NetworkPolicy", "networking.k8s.io/v1", "test", "/apis/networking.k8s.io/v1/namespaces/test/networkpolicies"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var calls atomic.Int32
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if r.Method != "GET" {
					t.Errorf("unexpected method %s", r.Method)
				}
				switch r.URL.Path {
				case tc.path:
					if r.URL.Query().Get("limit") != "100" {
						t.Error("unbounded request")
					}
					fmt.Fprintf(w, `{"apiVersion":%q,"kind":%q,"metadata":{"continue":"next"},"items":[{"metadata":{"name":"sample","namespace":%q,"uid":"uid","annotations":{"private":"not projected"}}}]}`, tc.version, tc.kind+"List", tc.ns)
				case tc.path + "/sample":
					fmt.Fprintf(w, `{"apiVersion":%q,"kind":%q,"metadata":{"name":"sample","namespace":%q,"uid":"uid","resourceVersion":"2"}}`, tc.version, tc.kind, tc.ns)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			ctx := context.Background()
			req := PageRequest{Connection: c, Kind: tc.kind, Namespace: tc.ns}
			page, e := s.ReadNetworking(ctx, req)
			if e != nil || len(page.Items) != 1 || page.Page.NextCursor == "" {
				t.Fatalf("%+v %v", page, e)
			}
			row := page.Items[0]
			if row.Resource.APIVersion != tc.version || row.Resource.Namespace != tc.ns || strings.Contains(row.ObjectJSON, "not projected") {
				t.Fatal(row)
			}
			doc, e := s.ReadResourceYAML(ctx, DetailRequest{Resource: row.Resource})
			if e != nil || !strings.Contains(doc.WithoutManagedFields, "kind: "+tc.kind) {
				t.Fatal(doc, e)
			}
			before := calls.Load()
			bad := row.Resource
			bad.APIVersion = "v2"
			if _, e = s.ReadResource(ctx, DetailRequest{Resource: bad}); e == nil {
				t.Fatal("invalid API version accepted")
			}
			req.Cursor = page.Page.NextCursor
			req.Connection.Generation++
			if _, e = s.ReadNetworking(ctx, req); e == nil {
				t.Fatal("stale connection accepted")
			}
			req.Connection = c
			if tc.ns == "" {
				req.Namespace = "test"
				if _, e = s.ReadResourceEvents(ctx, EventRequest{Resource: row.Resource}); e == nil {
					t.Fatal("cluster Events accepted")
				}
			} else {
				req.Namespace = ""
			}
			if _, e = s.ReadNetworking(ctx, req); e == nil {
				t.Fatal("wrong scope accepted")
			}
			if calls.Load() != before {
				t.Fatal("invalid requests reached API")
			}
		})
	}
}
