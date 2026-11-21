/*
Purpose: Verify creation routing from exact native discovery descriptors.
Used by: Kubernetes resource routing tests.
Responsibilities: Cover scope, ambiguity and create-only routes.
Important: No test sends a manifest or contacts an external API.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestResolveManifestTarget(t *testing.T) {
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("resolver wrote data")
		}
		switch r.URL.Path {
		case "/apis/example.test/v2":
			fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"example.test/v2","resources":[{"name":"widgets","kind":"Widget","namespaced":true,"verbs":["create"]},{"name":"widgetaliases","kind":"Widget","namespaced":true,"verbs":["create"]},{"name":"widgets/proxy","kind":"Proxy","namespaced":true,"verbs":["create"]},{"name":"scopes","kind":"Scope","namespaced":false,"verbs":["create"]},{"name":"views","kind":"View","namespaced":true,"verbs":["list"]}]}`)
		default:
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	for _, tc := range []struct{ api, kind, resource, ns, path string }{
		{"example.test/v2", "Widget", "widgets", "team", "/apis/example.test/v2/namespaces/team/widgets"},
		{"example.test/v2", "Widget", "", "team", ""},
		{"example.test/v2", "Widget", "widgetaliases", "team", "/apis/example.test/v2/namespaces/team/widgetaliases"},
		{"example.test/v2", "Widget", "widgets", "", ""},
		{"example.test/v2", "Scope", "", "", "/apis/example.test/v2/scopes"},
		{"example.test/v2", "Scope", "", "team", ""},
		{"example.test/v2", "Proxy", "widgets/proxy", "team", ""},
		{"example.test/v2", "View", "", "team", ""},
		{"example.test/v3", "Widget", "widgets", "team", ""},
	} {
		got, err := s.ResolveManifestTarget(ctx, ref, tc.api, tc.kind, tc.resource, tc.ns, "sample", "create")
		if tc.path == "" {
			if err == nil {
				t.Fatalf("unexpected route %+v", got)
			}
			continue
		}
		if err != nil || got.CollectionPath() != tc.path || got.Ref().Name != "sample" {
			t.Fatalf("route %+v: %v", got, err)
		}
	}
}
