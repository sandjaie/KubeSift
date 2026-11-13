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

func TestNetworkingReferenceChecksSourceBeforeTarget(t *testing.T) {
	var targetCalls atomic.Int32
	s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/networking.k8s.io/v1/namespaces/test/ingresses/route":
			fmt.Fprint(w, `{"apiVersion":"networking.k8s.io/v1","kind":"Ingress","metadata":{"name":"route","namespace":"test","uid":"source","resourceVersion":"2"},"spec":{"ingressClassName":"shared","defaultBackend":{"service":{"name":"web","port":{"name":"http"}}}}}`)
		case "/api/v1/namespaces/test/services/web":
			targetCalls.Add(1)
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Service","metadata":{"name":"web","namespace":"test","uid":"target"}}`)
		case "/apis/networking.k8s.io/v1/ingressclasses/shared":
			targetCalls.Add(1)
			fmt.Fprint(w, `{"apiVersion":"networking.k8s.io/v1","kind":"IngressClass","metadata":{"name":"shared","uid":"class"}}`)
		default:
			t.Errorf("unexpected API call %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	source := ResourceRef{Connection: c, Kind: "Ingress", APIVersion: "networking.k8s.io/v1", Namespace: "test", Name: "route", UID: "source"}
	target := ResourceRef{Connection: c, Kind: "Service", APIVersion: "v1", Namespace: "test", Name: "web"}
	for _, change := range []func(*NetworkReferenceRequest){
		func(r *NetworkReferenceRequest) { r.SourceVersion = "old" },
		func(r *NetworkReferenceRequest) { r.Target.Name = "unreferenced" },
		func(r *NetworkReferenceRequest) { r.Target.Namespace = "other" },
		func(r *NetworkReferenceRequest) { r.Target.Connection.Generation++ },
		func(r *NetworkReferenceRequest) { r.Target.UID = "forged" },
	} {
		req := NetworkReferenceRequest{Source: source, SourceVersion: "2", Target: target}
		change(&req)
		if _, err := s.ResolveNetworkReference(context.Background(), req); err == nil {
			t.Fatalf("unsafe reference accepted: %+v", req)
		}
	}
	if targetCalls.Load() != 0 {
		t.Fatal("invalid request reached target")
	}
	for _, ref := range []ResourceRef{target, {Connection: c, Kind: "IngressClass", APIVersion: "networking.k8s.io/v1", Name: "shared"}} {
		result, err := s.ResolveNetworkReference(context.Background(), NetworkReferenceRequest{Source: source, SourceVersion: "2", Target: ref})
		if err != nil || result.IdentityEvidence != "name-resolved" || result.Resource.UID == "" {
			t.Fatal(result, err)
		}
	}
}

func TestServiceSliceAssociation(t *testing.T) {
	s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/test/services/web":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Service","metadata":{"name":"web","namespace":"test","uid":"current"}}`)
		case "/apis/discovery.k8s.io/v1/namespaces/test/endpointslices":
			if r.URL.Query().Get("labelSelector") != "kubernetes.io/service-name=web" || r.URL.Query().Get("limit") != "100" {
				t.Error("unbounded association")
			}
			fmt.Fprint(w, `{"apiVersion":"discovery.k8s.io/v1","kind":"EndpointSliceList","metadata":{"continue":"next"},"items":[`)
			for i, owner := range []string{"", "current", "old"} {
				if i > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"metadata":{"name":"slice-%d","namespace":"test","uid":"s%d","labels":{"kubernetes.io/service-name":"web"},"ownerReferences":[{"apiVersion":"v1","kind":"Service","name":"web","uid":%q}]},"addressType":"IPv4","endpoints":[]}`, i, i, owner)
			}
			fmt.Fprint(w, `]}`)
		default:
			t.Errorf("unexpected API call %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	ref := ResourceRef{Connection: c, Kind: "Service", APIVersion: "v1", Namespace: "test", Name: "web", UID: "current"}
	page, e := s.ReadServiceEndpointSlices(context.Background(), RelatedRequest{Resource: ref})
	if e != nil || page.Complete || len(page.Items) != 3 {
		t.Fatal(page, e)
	}
	for i, want := range []string{"label-only", "owner-confirmed", "owner-conflict"} {
		if page.Items[i].Association != want {
			t.Fatal(page.Items[i])
		}
	}
	ref.UID = "old"
	if _, e = s.ReadServiceEndpointSlices(context.Background(), RelatedRequest{Resource: ref, Cursor: page.Page.NextCursor}); e == nil {
		t.Fatal("replaced Service accepted")
	}
}
