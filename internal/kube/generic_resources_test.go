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
			fmt.Fprintf(w, `{"kind":"PodList","apiVersion":"example.test/v1","metadata":{"continue":"next"},"items":[%s]}`, raw)
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
