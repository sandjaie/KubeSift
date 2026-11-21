/*
Purpose: Verify access-control transport, identity and inventory boundaries.
Used by: Explorer regression tests.
Responsibilities: Exercise every allowed resource endpoint without cluster credentials.
Important: All responses are synthetic.
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

func TestAccessControlInventory(t *testing.T) {
	const group = "rbac.authorization.k8s.io/v1"
	for _, tc := range []struct{ kind, version, namespace, path, name, fields, want string }{
		{"ServiceAccount", "v1", "test", "/api/v1/namespaces/test/serviceaccounts", "example", `"automountServiceAccountToken":false,"imagePullSecrets":[{"name":"sentinel"}],"secrets":[{"name":"sentinel"}]`, `"automount":false`},
		{"Role", group, "test", "/apis/" + group + "/namespaces/test/roles", "system:Fixture", `"rules":[{"verbs":["sentinel"]}]`, `"ruleCount":1`},
		{"RoleBinding", group, "test", "/apis/" + group + "/namespaces/test/rolebindings", "system:Fixture", `"subjects":[{"kind":"User","name":"sentinel"}]`, `"subjectCount":1`},
		{"ClusterRole", group, "", "/apis/" + group + "/clusterroles", "system:Fixture", `"aggregationRule":{},"rules":[{"verbs":["sentinel"]}]`, `"aggregation":true`},
		{"ClusterRoleBinding", group, "", "/apis/" + group + "/clusterrolebindings", "system:Fixture", `"subjects":[{"kind":"Group","name":"sentinel"}]`, `"subjectCount":1`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var calls atomic.Int32
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				object := fmt.Sprintf(`{"kind":%q,"apiVersion":%q,"metadata":{"name":%q,"namespace":%q,"uid":"uid","resourceVersion":"7","annotations":{"notes":"sentinel"}},%s}`, tc.kind, tc.version, tc.name, tc.namespace, tc.fields)
				if r.Method != "GET" {
					t.Error("non-read request", r.Method)
				}
				switch r.URL.Path {
				case tc.path:
					if r.URL.Query().Get("limit") != "100" {
						t.Error("unbounded request")
					}
					if r.URL.Query().Get("continue") == "next" {
						fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"continue":"last"},"items":[]}`, tc.kind+"List", tc.version)
						return
					}
					fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"continue":"next"},"items":[%s]}`, tc.kind+"List", tc.version, object)
				case tc.path + "/" + tc.name:
					fmt.Fprint(w, object)
				default:
					t.Errorf("unexpected GET %s", r.URL)
					http.NotFound(w, r)
				}
			})
			ctx := context.Background()
			req := PageRequest{Connection: c, Kind: tc.kind, Namespace: tc.namespace}
			page, err := s.ReadAccessControl(ctx, req)
			if err != nil || len(page.Items) != 1 || page.Page.NextCursor == "" {
				t.Fatal(page, err)
			}
			row := page.Items[0]
			if !strings.Contains(row.ObjectJSON, tc.want) || strings.Contains(row.ObjectJSON, "sentinel") {
				t.Fatal(row.ObjectJSON)
			}
			if _, err := s.ReadResourceYAML(ctx, DetailRequest{Resource: row.Resource}); err != nil {
				t.Fatal(err)
			}
			req.Cursor = page.Page.NextCursor
			if next, err := s.ReadAccessControl(ctx, req); err != nil || len(next.Items) != 0 || next.Page.NextCursor == "" {
				t.Fatal(next, err)
			}
			before := calls.Load()
			req.Connection.Generation++
			if _, err := s.ReadAccessControl(ctx, req); err == nil {
				t.Fatal("mixed cursor accepted")
			}
			for _, name := range []string{"", ".", "..", "a/b", "a%2fb"} {
				bad := row.Resource
				bad.Name = name
				if _, err := s.ReadResource(ctx, DetailRequest{Resource: bad}); err == nil {
					t.Fatal("accepted", name)
				}
			}
			if tc.namespace == "" {
				if _, err := s.ReadResourceEvents(ctx, EventRequest{Resource: row.Resource}); err == nil {
					t.Fatal("cluster events accepted")
				}
			}
			if calls.Load() != before {
				t.Fatal("invalid requests reached the server")
			}
		})
	}
	if validResourceName("Pod", "system:Fixture") || validResourceName("ServiceAccount", "system:Fixture") {
		t.Fatal("DNS rules widened")
	}
}

func TestAccessControlReadErrorsAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name, code, body string
		status           int
		detail           bool
	}{
		{"unsupported list", "api_unavailable", `{"kind":"Status","reason":"NotFound","code":404}`, 404, false},
		{"missing detail", "not_found", `{"kind":"Status","reason":"NotFound","code":404}`, 404, true},
		{"denied list", "forbidden", `{"kind":"Status","reason":"Forbidden","code":403}`, 403, false},
		{"denied detail", "forbidden", `{"kind":"Status","reason":"Forbidden","code":403}`, 403, true},
		{"expired cursor", "list_changed", `{"kind":"Status","reason":"Expired","code":410}`, 410, false},
		{"malformed identity", "invalid", `{"kind":"RoleBinding","apiVersion":"rbac.authorization.k8s.io/v1","metadata":{"name":"some-other-name","namespace":"test","uid":"uid"}}`, 200, true},
		{"recreated identity", "resource_replaced", `{"kind":"RoleBinding","apiVersion":"rbac.authorization.k8s.io/v1","metadata":{"name":"system:Fixture","namespace":"test","uid":"new-uid"}}`, 200, true},
		{"large inventory", "capacity", `{"kind":"RoleBindingList","apiVersion":"rbac.authorization.k8s.io/v1","items":[{"metadata":{"name":"system:Fixture","namespace":"test","uid":"uid"},"roleRef":{"name":"` + strings.Repeat("x", detailLimit) + `"}}]}`, 200, false},
		{"large detail", "capacity", `{"kind":"RoleBinding","apiVersion":"rbac.authorization.k8s.io/v1","metadata":{"name":"system:Fixture","namespace":"test","uid":"uid"},"subjects":[{"name":"` + strings.Repeat("x", detailLimit) + `"}]}`, 200, true},
		{"body cap", "capacity", `{"ignored":"` + strings.Repeat("x", readBodyLimit) + `"}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				want := "/apis/rbac.authorization.k8s.io/v1/namespaces/test/rolebindings"
				if tc.detail {
					want += "/system:Fixture"
				}
				if r.Method != "GET" || r.URL.Path != want {
					t.Error("unexpected request", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			ctx := context.Background()
			var err error
			if tc.detail {
				_, err = s.ReadResource(ctx, DetailRequest{Resource: ResourceRef{Connection: c, Kind: "RoleBinding", Namespace: "test", APIVersion: "rbac.authorization.k8s.io/v1", Name: "system:Fixture", UID: "uid"}})
			} else {
				_, err = s.ReadAccessControl(ctx, PageRequest{Connection: c, Kind: "RoleBinding", Namespace: "test"})
			}
			if err == nil || !strings.HasPrefix(err.Error(), tc.code+":") {
				t.Fatal("expected", tc.code, "got", err)
			}
		})
	}
}

func TestAccessControlEventsAndCancelledRead(t *testing.T) {
	var calls atomic.Int32
	s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/test/serviceaccounts/account":
			fmt.Fprint(w, `{"kind":"ServiceAccount","apiVersion":"v1","metadata":{"name":"account","namespace":"test","uid":"account-uid","resourceVersion":"1"}}`)
		case "/apis/events.k8s.io/v1/namespaces/test/events":
			if r.Method != "GET" || r.URL.Query().Get("fieldSelector") != "regarding.uid=account-uid" {
				t.Error("invalid account Events selector", r.URL)
			}
			fmt.Fprint(w, `{"kind":"EventList","apiVersion":"events.k8s.io/v1","items":[]}`)
		default:
			t.Error("unexpected request", r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()
	ref := ResourceRef{Connection: c, Kind: "ServiceAccount", APIVersion: "v1", Namespace: "test", Name: "account", UID: "account-uid"}
	if _, err := s.ReadResourceEvents(ctx, EventRequest{Resource: ref}); err != nil {
		t.Fatal(err)
	}
	before := calls.Load()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.ReadAccessControl(cancelled, PageRequest{Connection: c, Kind: "ServiceAccount", Namespace: "test"}); err == nil {
		t.Fatal("cancelled request accepted")
	}
	for _, req := range []PageRequest{{Connection: c, Kind: "Secret", Namespace: "test"}, {Connection: c, Kind: "Role"}, {Connection: c, Kind: "ClusterRole", Namespace: "test"}} {
		if _, err := s.ReadAccessControl(ctx, req); err == nil {
			t.Fatal("invalid inventory accepted", req)
		}
	}
	if calls.Load() != before {
		t.Fatal("cancelled or invalid reads touched the API")
	}
}
