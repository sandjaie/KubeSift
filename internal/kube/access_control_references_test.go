/*
Purpose: Verify the exact allowlist and single target capture for access navigation.
Used by: Access control resolver checks.
Responsibilities: Reject forged fields, stale sources and replaced targets.
Important: No request reaches an actual cluster or authorization API.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestAccessControlReferenceRoutes(t *testing.T) {
	for _, tc := range []struct {
		from, to, ns, body string
		allowed            bool
	}{
		{"Pod", "ServiceAccount", "local", `"spec":{"serviceAccountName":"target"}`, true},
		{"Pod", "ServiceAccount", "other", `"spec":{"serviceAccountName":"target"}`, false},
		{"Pod", "ServiceAccount", "local", `"spec":{"serviceAccount":"target"}`, false},
		{"RoleBinding", "Role", "local", `"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"target"}`, true},
		{"RoleBinding", "Role", "other", `"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"target"}`, false},
		{"RoleBinding", "Role", "local", `"roleRef":{"apiGroup":"wrong","kind":"Role","name":"target"}`, false},
		{"RoleBinding", "ClusterRole", "", `"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"ClusterRole","name":"target"}`, true},
		{"ClusterRoleBinding", "ClusterRole", "", `"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"ClusterRole","name":"target"}`, true},
		{"ClusterRoleBinding", "Role", "local", `"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"target"}`, false},
		{"RoleBinding", "ServiceAccount", "local", `"subjects":[{"kind":"ServiceAccount","name":"target"},{"kind":"ServiceAccount","name":"target"}]`, true},
		{"RoleBinding", "ServiceAccount", "other", `"subjects":[{"kind":"ServiceAccount","namespace":"other","name":"target"}]`, true},
		{"ClusterRoleBinding", "ServiceAccount", "other", `"subjects":[{"kind":"ServiceAccount","namespace":"other","name":"target"}]`, true},
		{"ClusterRoleBinding", "ServiceAccount", "local", `"subjects":[{"kind":"ServiceAccount","name":"target"}]`, false},
		{"RoleBinding", "ServiceAccount", "local", `"subjects":[{"kind":"ServiceAccount","apiGroup":"wrong","name":"target"}]`, false},
		{"RoleBinding", "ServiceAccount", "local", `"subjects":[{"kind":"User","name":"system:serviceaccount:local:target"},{"kind":"Group","name":"target"}]`, false},
	} {
		t.Run(tc.from+"-"+tc.to+"-"+tc.ns, func(t *testing.T) {
			ns := "local"
			if tc.from == "ClusterRoleBinding" {
				ns = ""
			}
			version := func(kind string) string {
				if kind == "Pod" || kind == "ServiceAccount" {
					return "v1"
				}
				return "rbac.authorization.k8s.io/v1"
			}
			source := ResourceRef{Kind: tc.from, APIVersion: version(tc.from), Namespace: ns, Name: "source", UID: "source-uid"}
			target := ResourceRef{Kind: tc.to, APIVersion: version(tc.to), Namespace: tc.ns, Name: "target"}
			path := func(ref ResourceRef) string {
				prefix := "/api/v1"
				plurals := map[string]string{"Pod": "pods", "ServiceAccount": "serviceaccounts", "Role": "roles", "RoleBinding": "rolebindings", "ClusterRole": "clusterroles", "ClusterRoleBinding": "clusterrolebindings"}
				if ref.APIVersion != "v1" {
					prefix = "/apis/" + ref.APIVersion
				}
				if ref.Namespace != "" {
					prefix += "/namespaces/" + ref.Namespace
				}
				return prefix + "/" + plurals[ref.Kind] + "/" + ref.Name
			}
			var reads atomic.Int32
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method != "GET" {
					t.Error("non-read request", r.Method)
				}
				switch r.URL.Path {
				case path(source):
					fmt.Fprintf(w, `{"apiVersion":%q,"kind":%q,"metadata":{"name":"source","namespace":%q,"uid":"source-uid","resourceVersion":"7"},%s}`, source.APIVersion, source.Kind, source.Namespace, tc.body)
				case path(target):
					reads.Add(1)
					fmt.Fprintf(w, `{"apiVersion":%q,"kind":%q,"metadata":{"name":"target","namespace":%q,"uid":"target-uid","resourceVersion":"8"}}`, target.APIVersion, target.Kind, target.Namespace)
				default:
					t.Error("unexpected GET", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			source.Connection, target.Connection = c, c
			ctx := context.Background()
			req := AccessControlReferenceRequest{Source: source, Target: target, SourceVersion: "7"}
			for _, forge := range []func(*AccessControlReferenceRequest){
				func(r *AccessControlReferenceRequest) { r.SourceVersion = "6" },
				func(r *AccessControlReferenceRequest) { r.Source.UID = "replaced" },
				func(r *AccessControlReferenceRequest) { r.Target.Connection.Generation++ },
				func(r *AccessControlReferenceRequest) { r.Target.Name = "forged" },
				func(r *AccessControlReferenceRequest) { r.Target.APIVersion = "v9" },
			} {
				bad := req
				forge(&bad)
				if _, err := s.ResolveAccessControlReference(ctx, bad); err == nil || reads.Load() != 0 {
					t.Fatal("forged or stale request accepted", err)
				}
			}
			got, err := s.ResolveAccessControlReference(ctx, req)
			if !tc.allowed {
				if err == nil || reads.Load() != 0 {
					t.Fatal("disallowed reference read", err)
				}
				return
			}
			target.UID = "target-uid"
			if err != nil || reads.Load() != 1 || got.Resource != target || got.Detail.Resource != target || got.Detail.ResourceVersion != "8" ||
				got.SourceVersion != "7" || got.TargetVersion != "8" || got.IdentityEvidence != "name-resolved" {
				t.Fatal(got, err)
			}
			req.Target.UID = "old-uid"
			if _, err := s.ResolveAccessControlReference(ctx, req); err == nil {
				t.Fatal("accepted a replaced target")
			}
		})
	}
}
