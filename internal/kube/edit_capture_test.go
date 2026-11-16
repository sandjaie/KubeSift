/*
Purpose: Verify edit discovery and targeted full reads.
Used by: Kubernetes service tests.
Responsibilities: Exercise explicit identity, advertised verbs, scope and server authorization.
Important: All objects and permissions are synthetic.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestEditCapture(t *testing.T) {
	for _, tc := range []struct {
		name, group, kind, resource, namespace, verbs, problem string
		status                                                 int
	}{
		{"pod", "", "Pod", "pods", "demo", `"get","patch"`, "", 200},
		{"custom", "example.test", "Widget", "widgets", "", `"get","patch"`, "", 200},
		{"secret", "", "Secret", "secrets", "demo", `"get","patch"`, "", 200},
		{"unsupported", "", "Pod", "pods", "demo", `"get"`, "unsupported", 0},
		{"forbidden", "", "Secret", "secrets", "demo", `"get","patch"`, "forbidden", 403},
		{"replaced", "", "Pod", "pods", "demo", `"get","patch"`, "resource_replaced", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := apiVersionPath(tc.group, "v1")
			api := apiResource{tc.group, "v1", tc.resource, tc.kind, tc.namespace != ""}
			path := prefix
			if tc.namespace != "" {
				path += "/namespaces/" + tc.namespace
			}
			path += "/" + tc.resource + "/sample"
			s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case prefix:
					fmt.Fprintf(w, `{"kind":"APIResourceList","groupVersion":%q,"resources":[{"name":%q,"kind":%q,"namespaced":%t,"verbs":[%s]}]}`, api.apiVersion(), tc.resource, tc.kind, api.namespaced, tc.verbs)
				case path:
					if tc.status == 0 {
						t.Error("unsupported resource was read")
						return
					}
					w.WriteHeader(tc.status)
					fmt.Fprintf(w, `{"apiVersion":%q,"kind":%q,"metadata":{"namespace":%q,"name":"sample","uid":"uid","resourceVersion":"7"},"data":{"value":"c2FtcGxl"}}`, api.apiVersion(), tc.kind, tc.namespace)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					w.WriteHeader(404)
				}
			})
			uid := "uid"
			if tc.name == "replaced" {
				uid = "prior"
			}
			result, err := s.CaptureEdit(context.Background(), EditTarget{Connection: ref, Group: tc.group, Version: "v1", Kind: tc.kind, Namespace: tc.namespace, Name: "sample", UID: uid})
			if tc.problem == "" {
				if err != nil || result.Target.Resource != tc.resource || result.ResourceVersion != "7" {
					t.Fatalf("capture failed: %v", err)
				}
			} else if p := ClassifyError(context.Background(), err); p == nil || p.Code != tc.problem {
				t.Fatalf("expected %s, got %v", tc.problem, err)
			}
		})
	}
}
