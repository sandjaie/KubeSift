/*
Purpose: Verify safe OpenShift Route inspection on synthetic API responses.
Used by: The scoped workflow and generic browser tests.
Responsibilities: Cover router disagreement, zero weights, field omission and stale backend links.
Important: No host, certificate or external cluster is accessed.
*/
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestReadRouteAndGuardService(t *testing.T) {
	service := "svc"
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/route.openshift.io/v1/namespaces/default/routes/site":
			fmt.Fprintf(w, `{"apiVersion":"route.openshift.io/v1","kind":"Route","metadata":{"name":"site","namespace":"default","uid":"r1","resourceVersion":"2"},"spec":{"host":"example.test","path":"/some","to":{"name":"%s"},"port":{"targetPort":"https"},"alternateBackends":[{"name":"other","weight":0}],"tls":{"termination":"reencrypt","externalCertificate":{"name":"cert"},"key":"private-sentinel"}},"status":{"ingress":[{"routerName":"first","conditions":[{"type":"Admitted","status":"True"}]},{"routerName":"second","conditions":[{"type":"Admitted","status":"False"}]}]}}`, service)
		case "/api/v1/namespaces/default/services/svc":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Service","metadata":{"name":"svc","namespace":"default","uid":"s1"},"spec":{"type":"ClusterIP"}}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	q := WorkflowQuery{Connection: ref, Namespace: "default", Kind: "Route", Name: "site", UID: "r1"}
	page, err := s.ReadWorkflow(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	var projected routeProjection
	if len(page.Items) != 1 || strings.Contains(page.Items[0].Fields, "private-sentinel") || json.Unmarshal([]byte(page.Items[0].Fields), &projected) != nil {
		t.Fatal("invalid projection")
	}
	if projected.Spec.Port.TargetPort.StrVal != "https" || *projected.Spec.AlternateBackends[0].Weight != 0 || projected.Spec.TLS.ExternalCertificate.Name != "cert" || len(projected.Status.Ingress) != 2 {
		t.Fatal("lost Route evidence")
	}
	target, err := s.ReadRouteService(context.Background(), q, "svc")
	if err != nil || target.Resource.UID != "s1" {
		t.Fatalf("Service lookup: %v", err)
	}
	service = "replacement"
	if _, err := s.ReadRouteService(context.Background(), q, "svc"); err == nil {
		t.Fatal("stale backend opened")
	}
	q.UID = "other"
	if _, err := s.ReadWorkflow(context.Background(), q); err == nil {
		t.Fatal("replaced source opened")
	}
}

func TestRouteDiscoveryWithRestrictedProjectsAndBuilds(t *testing.T) {
	ctx := context.Background()
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			fmt.Fprint(w, `{"kind":"APIVersions","versions":["v1"]}`)
		case "/apis":
			var groups []map[string]any
			for _, group := range []string{"project.openshift.io", "build.openshift.io", "route.openshift.io"} {
				groups = append(groups, map[string]any{"name": group, "versions": []map[string]string{{"groupVersion": group + "/v1", "version": "v1"}}})
			}
			json.NewEncoder(w).Encode(map[string]any{"kind": "APIGroupList", "groups": groups})
		case "/apis/project.openshift.io/v1", "/apis/build.openshift.io/v1":
			w.WriteHeader(http.StatusForbidden)
		case "/apis/route.openshift.io/v1":
			fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"route.openshift.io/v1","resources":[{"name":"routes","kind":"Route","namespaced":true,"verbs":["get","list"]}]}`)
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	if _, err := s.ProjectCapabilities(ctx, ref, false); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatal("expected denied Project discovery", err)
	}
	discovery, err := s.WorkflowDiscovery(ctx, ref, false)
	if err != nil || len(discovery.Resources) != 1 || discovery.Resources[0].Kind != "Route" || !discovery.Resources[0].List || !strings.Contains(discovery.Problems["build.openshift.io/v1"], "forbidden") {
		t.Fatalf("partial workflow discovery: %+v, %v", discovery, err)
	}
}
