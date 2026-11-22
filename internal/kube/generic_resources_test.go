/*
Purpose: Verify generic resource projections and exact capture fidelity.
Used by: Kubernetes reader tests.
Responsibilities: Exercise capability checks before I/O and preserve literal numbers for YAML copy.
Important: Synthetic data only.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestGenericInventoryCaptureAndGuards(t *testing.T) {
	ctx := context.Background()
	resourceCalls := 0
	raw := `{"apiVersion":"example.test/v1","kind":"Pod","metadata":{"namespace":"default","name":"sample","uid":"widget-1","resourceVersion":"9"},"spec":{"private":"inventory-must-not-leak","big":9007199254740993123456789,"small":1.234567890123456789,"custom":null}}`
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			fmt.Fprint(w, `{"kind":"APIVersions","versions":["v1"]}`)
		case "/apis":
			fmt.Fprint(w, `{"kind":"APIGroupList","groups":[{"name":"example.test","versions":[{"version":"v1","groupVersion":"example.test/v1"}]}]}`)
		case "/apis/example.test/v1":
			fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"example.test/v1","resources":[{"name":"widgets","kind":"Pod","namespaced":true,"verbs":["get","list"]}]}`)
		case "/apis/example.test/v1/namespaces/default/widgets":
			resourceCalls++
			if r.Method != "GET" || r.URL.Query().Get("limit") != "100" {
				t.Error("expected bounded GET")
			}
			fmt.Fprintf(w, `{"kind":"WidgetCollection","apiVersion":"example.test/v1","metadata":{"continue":"next"},"items":[%s]}`, raw)
		case "/apis/example.test/v1/namespaces/default/widgets/sample":
			resourceCalls++
			fmt.Fprint(w, raw)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	if _, err := s.ReadAPIGroups(ctx, ref, false); err != nil {
		t.Fatal(err)
	}
	catalog, err := s.ReadAPIResources(ctx, ref, "example.test", "v1", false)
	if err != nil || len(catalog.Resources) != 1 {
		t.Fatalf("catalog %+v %v", catalog, err)
	}
	handle := catalog.Resources[0].Handle
	req := GenericPageRequest{Connection: ref, Handle: "forged", Namespace: "default"}
	if _, err := s.ReadGenericResources(ctx, req); err == nil {
		t.Fatal("forged handle")
	}
	req.Handle = handle
	req.Namespace = ""
	if _, err := s.ReadGenericResources(ctx, req); err == nil {
		t.Fatal("implicit namespace")
	}
	req.Namespace = "default"
	page, err := s.ReadGenericResources(ctx, req)
	if err != nil || len(page.Items) != 1 || page.Page.NextCursor == "" || strings.Contains(fmt.Sprintf("%+v", page), "inventory-must-not-leak") {
		t.Fatalf("page %+v %v", page, err)
	}
	req.Cursor = page.Page.NextCursor + "tampered"
	if _, err := s.ReadGenericResources(ctx, req); err == nil || resourceCalls != 1 {
		t.Fatal("modified cursor reached native API")
	}
	req.Cursor = page.Page.NextCursor
	req.Namespace = "other"
	if _, err := s.ReadGenericResources(ctx, req); err == nil {
		t.Fatal("cursor crossed namespaces")
	}
	detailReq := GenericDetailRequest{Connection: ref, Handle: handle, Namespace: "default", Name: "../sample", UID: "widget-1"}
	if _, err := s.ReadGenericDetail(ctx, detailReq); err == nil || resourceCalls != 1 {
		t.Fatalf("address validation: %v, requests=%d", err, resourceCalls)
	}
	detailReq.Name = "sample"
	detail, err := s.ReadGenericDetail(ctx, detailReq)
	if err != nil || detail.ObjectJSON != raw {
		t.Fatalf("detail %+v %v", detail, err)
	}
	for _, text := range []string{"9007199254740993123456789", "1.234567890123456789", `"custom": null`} {
		if !strings.Contains(detail.YAML, text) {
			t.Fatalf("YAML lost %q: %s", text, detail.YAML)
		}
	}
	detailReq.UID = "old"
	if _, err := s.ReadGenericDetail(ctx, detailReq); err == nil || !strings.Contains(err.Error(), "resource_replaced") {
		t.Fatalf("UID check %v", err)
	}
	if _, err := s.ReadAPIResources(ctx, ref, "example.test", "v1", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadGenericDetail(ctx, detailReq); err == nil || resourceCalls != 3 {
		t.Fatal("invalidated descriptor reached network")
	}
}

func TestCreatedDetailAllowsGetWithoutInventoryAndRejectsReplacement(t *testing.T) {
	ctx := context.Background()
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		switch r.URL.Path {
		case "/api/v1":
			fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"secrets","kind":"Secret","namespaced":true,"verbs":["get","list"]}]}`)
		case "/apis/example.test/v1":
			fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"example.test/v1","resources":[{"name":"widgets","kind":"Widget","namespaced":true,"verbs":["get","create"]},{"name":"proxy","kind":"Proxy","namespaced":true,"verbs":["get","create"]},{"name":"connect","kind":"Connect","namespaced":true,"verbs":["get","create"]},{"name":"sessions","kind":"Session","namespaced":true,"verbs":["get","create","connect"]}]}`)
		case "/apis/example.test/v1/namespaces/default/widgets/sample":
			fmt.Fprint(w, `{"apiVersion":"example.test/v1","kind":"Widget","metadata":{"namespace":"default","name":"sample","uid":"actual","resourceVersion":"2"}}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	target := ManifestAddress{Connection: ref, Group: "example.test", Version: "v1", Resource: "widgets", Kind: "Widget", Namespace: "default", Name: "sample"}
	detail, err := s.ReadCreatedDetail(ctx, target, "actual")
	if err != nil || detail.Resource.UID != "actual" {
		t.Fatalf("created detail = %+v, %v", detail, err)
	}
	if _, err := s.ReadCreatedDetail(ctx, target, "old"); err == nil || !strings.Contains(err.Error(), "resource_replaced") {
		t.Fatalf("replacement check: %v", err)
	}
	secret := ManifestAddress{Connection: ref, Version: "v1", Kind: "Secret", Namespace: "default", Name: "sample"}
	if _, err := s.ReadCreatedDetail(ctx, secret, "actual"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("resolved Secret must not enter the full object reader: %v", err)
	}
	for _, endpoint := range []struct{ resource, kind string }{{"proxy", "Proxy"}, {"connect", "Connect"}, {"sessions", "Session"}} {
		target.Resource, target.Kind = endpoint.resource, endpoint.kind
		if _, err := s.ReadCreatedDetail(ctx, target, "actual"); err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("excluded %s endpoint: %v", endpoint.resource, err)
		}
	}
}

func TestSensitiveOpenShiftCapture(t *testing.T) {
	for _, tc := range []struct{ group, kind, plural string }{
		{"route.openshift.io", "Route", "routes"}, {"build.openshift.io", "BuildConfig", "buildconfigs"}, {"build.openshift.io", "Build", "builds"},
		{"operators.coreos.com", "InstallPlan", "installplans"}, {"operators.coreos.com", "ClusterServiceVersion", "clusterserviceversions"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"apiVersion":"%s/v1","kind":"%s","metadata":{"name":"x","namespace":"default","uid":"a","resourceVersion":"2","annotations":{"copy":"private-sentinel"}},"spec":{"tls":{"key":"private-sentinel"},"unknown":"private-sentinel"}}`, tc.group, tc.kind)
			})
			d, err := s.readManifestDetail(context.Background(), ManifestAddress{ref, tc.group, "v1", tc.plural, tc.kind, "default", "x"}, "a")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(d.ObjectJSON+d.YAML, "private-sentinel") {
				t.Fatal("sensitive material crossed the bridge")
			}
		})
	}
}

func TestOLMv1ProjectionAndUnrelatedGenericAPI(t *testing.T) {
	for _, tc := range []struct {
		group, kind, plural, fields, want string
		namespaced, restricted            bool
	}{
		{"olm.operatorframework.io", "ClusterExtension", "clusterextensions", `"namespace":"operators","source":{"sourceType":"Catalog","catalog":{"packageName":"demo","channels":["stable"]}},"secret":"private-sentinel"`, "stable", false, true},
		{"olm.operatorframework.io", "ClusterCatalog", "clustercatalogs", `"source":{"sourceType":"Image","image":{"ref":"registry.test/catalog:latest"}},"secret":"private-sentinel"`, "registry.test/catalog:latest", false, true},
		{"operators.coreos.com", "OperatorCondition", "operatorconditions", `"overrides":[{"type":"Upgradeable"}]`, "Upgradeable", true, false},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			ns := ""
			if tc.namespaced {
				ns = "team"
			}
			s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"apiVersion":"%s/v1","kind":%q,"metadata":{"name":"sample","namespace":%q,"uid":"one","resourceVersion":"2"},"spec":{%s}}`, tc.group, tc.kind, ns, tc.fields)
			})
			d, err := s.readManifestDetail(context.Background(), ManifestAddress{ref, tc.group, "v1", tc.plural, tc.kind, ns, "sample"}, "one")
			if err != nil || d.Projected != tc.restricted || !strings.Contains(d.ObjectJSON, tc.want) || strings.Contains(d.ObjectJSON+d.YAML, "private-sentinel") {
				t.Fatal("wrong inspection projection", err)
			}
		})
	}
}
