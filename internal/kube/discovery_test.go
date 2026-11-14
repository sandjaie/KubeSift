/*
Purpose: Verify lazy and bounded discovery with connection-owned handles.
Used by: Kubernetes discovery tests.
Responsibilities: Check root failures, canonical routing, eviction and expiry.
Important: Only synthetic HTTP routes are involved.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDiscoveryPartialLazyAndProtected(t *testing.T) {
	ctx := context.Background()
	paths := []string{}
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != "GET" || r.Header.Get("Accept") != "application/json" {
			t.Error("expected unaggregated JSON GET")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			fmt.Fprint(w, `{"kind":"APIVersions","versions":["v1"]}`)
		case "/apis":
			w.WriteHeader(403)
		case "/api/v1":
			fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"secrets","kind":"Wrong","namespaced":false,"verbs":["list","get"]},{"name":"pods","kind":"Pod","namespaced":true,"verbs":["list","get"]},{"name":"pods/proxy","kind":"PodProxyOptions","verbs":["get"]},{"name":"endpoints","kind":"Endpoints","namespaced":true,"verbs":["get","list"]}]}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	groups, err := s.ReadAPIGroups(ctx, ref, false)
	if err != nil || len(groups.Groups) != 1 || groups.APIsProblem == nil || len(paths) != 2 {
		t.Fatalf("root %+v %v %v", groups, err, paths)
	}
	if _, err := s.ReadAPIResources(ctx, ref, "", "v2", false); err == nil || len(paths) != 2 {
		t.Fatal("unadvertised version reached network")
	}
	catalog, err := s.ReadAPIResources(ctx, ref, "", "v1", false)
	if err != nil || len(catalog.Resources) != 3 || len(paths) != 3 {
		t.Fatalf("catalog %+v %v %v", catalog, err, paths)
	}
	for _, entry := range catalog.Resources {
		switch entry.Resource {
		case "secrets":
			if entry.Mode != "unavailable" || entry.Handle != "" {
				t.Fatal("unsafe Secret descriptor")
			}
		case "pods":
			if entry.Mode != "dedicated" || entry.Handle != "" {
				t.Fatal("dedicated bypass")
			}
		case "endpoints":
			if entry.Mode != "generic" || entry.Handle == "" {
				t.Fatal("missing generic handle")
			}
			if _, err := s.resolveDescriptor(ref, entry.Handle); err != nil {
				t.Fatal(err)
			}
			s.discovery[ref].versions[versionKey{"", "v1"}].catalog.FetchedAt = time.Now().Add(-6 * time.Minute)
			if _, err := s.resolveDescriptor(ref, entry.Handle); err == nil {
				t.Fatal("expired handle accepted")
			}
		}
	}
	s.connections.Close(ref)
	if s.discovery[ref] != nil {
		t.Fatal("closed connection retained catalog")
	}
}

func TestDiscoveryLRUAndDocumentLimits(t *testing.T) {
	ctx := context.Background()
	versions := []string{"v1", "v2", "v3", "v4", "v5", "v6", "v7", "v8", "v9"}
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			fmt.Fprint(w, `{"kind":"APIVersions","versions":["v1","v2","v3","v4","v5","v6","v7","v8","v9"]}`)
		case "/apis":
			fmt.Fprint(w, `{"kind":"APIGroupList","groups":[]}`)
		default:
			fmt.Fprintf(w, `{"kind":"APIResourceList","groupVersion":%q,"resources":[{"name":"things","kind":"Thing","namespaced":false,"verbs":["list","get"]}]}`, strings.TrimPrefix(r.URL.Path, "/api/"))
		}
	})
	if _, err := s.ReadAPIGroups(ctx, ref, false); err != nil {
		t.Fatal(err)
	}
	var handles []string
	for i, version := range versions {
		catalog, err := s.ReadAPIResources(ctx, ref, "", version, false)
		if err != nil || len(catalog.Resources) != 1 {
			t.Fatal(catalog, err)
		}
		handles = append(handles, catalog.Resources[0].Handle)
		if i == 7 {
			if _, err := s.resolveDescriptor(ref, handles[0]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := s.resolveDescriptor(ref, handles[1]); err == nil {
		t.Fatal("least recently used descriptor survived")
	}
	if _, err := s.resolveDescriptor(ref, handles[0]); err != nil {
		t.Fatal("recently used descriptor was evicted")
	}
	if _, err := s.ReadAPIGroups(ctx, ref, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolveDescriptor(ref, handles[0]); err == nil {
		t.Fatal("root refresh did not invalidate descriptor")
	}
	for _, test := range []struct{ name, body, code string }{
		{"mismatch", `{"kind":"APIResourceList","groupVersion":"other/v1","resources":[]}`, "invalid"},
		{"duplicate", `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"things","kind":"Thing","namespaced":false},{"name":"things","kind":"Thing","namespaced":false}]}`, "invalid"},
		{"scope absent", `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"things","kind":"Thing","verbs":["get","list"]}]}`, "invalid"},
		{"path escape", `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"%2e%2e","kind":"Thing","namespaced":false}]}`, "invalid"},
		{"descriptor limit", `{"kind":"APIResourceList","groupVersion":"v1","resources":[` + strings.Repeat("{},", 1024) + "{}]}", "capacity"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := projectDiscovery([]byte(test.body), versionKey{"", "v1"}); err == nil || !strings.HasPrefix(err.Error(), test.code+":") {
				t.Fatal(err)
			}
		})
	}
}
