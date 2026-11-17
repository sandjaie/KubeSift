//go:build integration

/*
Purpose: Prove that Kubernetes rejects unsafe existing-target server-side apply.
Used by: The opt-in Phase 1D.8 protocol gate on the approved kind contexts.
Responsibilities: Exercise UID and resourceVersion guards before product implementation.
Important: All writes target synthetic objects in a disposable test-owned namespace.
*/
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/operation"
	"github.com/sandjaie/KubeSift/internal/session"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func TestExistingApplyProtocol(t *testing.T) {
	if os.Getenv("KUBESIFT_APPLY_INTEGRATION") != "1" {
		t.Skip("explicit apply protocol opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			manager := connection.NewManager(nil)
			t.Cleanup(manager.CloseAll)
			ref, kubeClient := openLocalOperationFixture(t, context.Background(), manager, dir, suffix, "")
			cfg, err := manager.RESTConfig(ref)
			if err != nil {
				t.Fatal(err)
			}
			dyn, err := dynamic.NewForConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			client, err := rest.HTTPClientFor(cfg)
			if err != nil {
				t.Fatal(err)
			}
			ns, err := kubeClient.CoreV1().Namespaces().Create(ctx, &core.Namespace{ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kubesift-apply-check-", Labels: map[string]string{"kubesift.dev/fixture": "phase1d8-protocol"},
			}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("kind-dev-local-%s namespace %s", suffix, ns.Name)
			deferCreatedObject(t, dyn.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}), ns.Name, string(ns.UID))
			installCreateCRD(t, ctx, kubeClient, dyn)
			for _, fixture := range []struct{ group, resource, kind string }{
				{"", "configmaps", "ConfigMap"},
				{createFixtureGroup, "widgets", "Widget"},
			} {
				t.Run(fixture.kind, func(t *testing.T) {
					gvr := schema.GroupVersionResource{Group: fixture.group, Version: "v1", Resource: fixture.resource}
					api := dyn.Resource(gvr).Namespace(ns.Name)
					apiVersion := gvr.GroupVersion().String()
					prefix := "/api/" + apiVersion
					if gvr.Group != "" {
						prefix = "/apis/" + apiVersion
					}
					const name = "existing-apply"
					endpoint := cfg.Host + prefix + "/namespaces/" + ns.Name + "/" + gvr.Resource + "/" + name
					object := func(marker, uid, version string) *unstructured.Unstructured {
						value := &unstructured.Unstructured{Object: map[string]any{
							"apiVersion": apiVersion, "kind": fixture.kind,
						}}
						value.SetName(name)
						value.SetNamespace(ns.Name)
						value.SetAnnotations(map[string]string{"kubesift.dev/test": marker})
						if uid != "" {
							value.SetUID(types.UID(uid))
						}
						if version != "" {
							value.SetResourceVersion(version)
						}
						return value
					}
					create := func(marker string) *unstructured.Unstructured {
						fixture := object("", "", "")
						fixture.SetAnnotations(map[string]string{"kubesift.dev/baseline": marker})
						out, err := api.Create(ctx, fixture, metav1.CreateOptions{FieldManager: "kubesift-protocol-fixture"})
						if err != nil {
							t.Fatal(err)
						}
						deferCreatedObject(t, api, name, string(out.GetUID()))
						return out
					}
					patch := func(marker, uid, version string, force, dry bool) (int, *unstructured.Unstructured) {
						t.Helper()
						body, err := json.Marshal(object(marker, uid, version))
						if err != nil {
							t.Fatal(err)
						}
						url := endpoint + "?fieldManager=kubesift-apply&fieldValidation=Strict"
						if dry {
							url += "&dryRun=All"
						}
						if force {
							url += "&force=true"
						}
						req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
						if err != nil {
							t.Fatal(err)
						}
						req.Header.Set("Content-Type", "application/apply-patch+yaml")
						req.Header.Set("Accept", "application/json")
						response, err := client.Do(req)
						if err != nil {
							t.Fatal(err)
						}
						defer response.Body.Close()
						raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
						if err != nil {
							t.Fatal(err)
						}
						var out unstructured.Unstructured
						if err := out.UnmarshalJSON(raw); err != nil {
							t.Fatal(err)
						}
						reason, _, _ := unstructured.NestedString(out.Object, "reason")
						t.Logf("PATCH dryRun=%t force=%t: HTTP %d %s", dry, force, response.StatusCode, reason)
						return response.StatusCode, &out
					}
					read := func() *unstructured.Unstructured {
						t.Helper()
						live, err := api.Get(ctx, name, metav1.GetOptions{})
						if err != nil {
							t.Fatal(err)
						}
						return live
					}
					verify := func(live, before *unstructured.Unstructured, marker string) {
						t.Helper()
						if live.GetUID() != before.GetUID() || live.GetAnnotations()["kubesift.dev/baseline"] != before.GetAnnotations()["kubesift.dev/baseline"] || live.GetAnnotations()["kubesift.dev/test"] != marker {
							t.Fatalf("object identity or marker changed unexpectedly: UID=%s annotation=%q", live.GetUID(), live.GetAnnotations()["kubesift.dev/test"])
						}
					}
					remove := func(live *unstructured.Unstructured) {
						t.Helper()
						id := live.GetUID()
						if err := api.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &id}}); err != nil {
							t.Fatal(err)
						}
						awaitDeletionFixture(t, ctx, "protocol fixture removal", func(ctx context.Context) (bool, error) {
							_, err := api.Get(ctx, name, metav1.GetOptions{})
							return apierrors.IsNotFound(err), nil
						})
					}

					initial := create("original")
					code, preview := patch("applied", string(initial.GetUID()), initial.GetResourceVersion(), false, true)
					if code != http.StatusOK {
						msg, _, _ := unstructured.NestedString(preview.Object, "message")
						t.Fatalf("guarded dry-run returned %d: %s", code, msg)
					}
					verify(preview, initial, "applied")
					verify(read(), initial, "")
					code, applied := patch("applied", string(initial.GetUID()), initial.GetResourceVersion(), false, false)
					if code != http.StatusOK {
						t.Fatalf("expected HTTP 200 for existing target, got %d", code)
					}
					verify(applied, initial, "applied")
					verify(read(), initial, "applied")

					for _, force := range []bool{false, true} {
						t.Run(fmt.Sprintf("stale-version/force=%t", force), func(t *testing.T) {
							code, _ := patch("stale-attempt", string(initial.GetUID()), initial.GetResourceVersion(), force, false)
							if code != http.StatusConflict && code != http.StatusUnprocessableEntity {
								t.Errorf("stale resourceVersion was not rejected: %d", code)
							}
							live := read()
							verify(live, initial, "applied")
							if live.GetResourceVersion() != applied.GetResourceVersion() {
								t.Error("stale apply changed the resourceVersion")
							}
						})
					}

					remove(applied)
					replacement := create("replacement")
					for _, force := range []bool{false, true} {
						t.Run(fmt.Sprintf("wrong-uid/force=%t", force), func(t *testing.T) {
							code, _ := patch("wrong-uid-attempt", string(initial.GetUID()), replacement.GetResourceVersion(), force, false)
							if code != http.StatusConflict && code != http.StatusUnprocessableEntity {
								t.Errorf("wrong UID with a current version was not rejected: %d", code)
							}
							verify(read(), replacement, "")
						})
					}
					victim := read()
					remove(victim)
					for _, force := range []bool{false, true} {
						t.Run(fmt.Sprintf("deleted-after-get/force=%t", force), func(t *testing.T) {
							code, after := patch("recreation-attempt", string(victim.GetUID()), victim.GetResourceVersion(), force, false)
							if code == http.StatusCreated && after.GetUID() != "" {
								deferCreatedObject(t, api, name, string(after.GetUID()))
							}
							if code != http.StatusNotFound && code != http.StatusConflict && code != http.StatusUnprocessableEntity {
								t.Errorf("deleted target did not fail safely: %d", code)
							}
							if _, err := api.Get(ctx, name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
								t.Errorf("apply after deletion did not leave the target absent: %v", err)
							}
						})
					}
				})
			}
		})
	}
}

func TestYAMLApply(t *testing.T) {
	if os.Getenv("KUBESIFT_APPLY_INTEGRATION") != "1" {
		t.Skip("explicit apply opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			mgr := connection.NewManager(nil)
			t.Cleanup(mgr.CloseAll)
			ref, client := openLocalOperationFixture(t, context.Background(), mgr, dir, suffix, "")
			cfg, err := mgr.RESTConfig(ref)
			if err != nil {
				t.Fatal(err)
			}
			dyn, err := dynamic.NewForConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			ns, err := client.CoreV1().Namespaces().Create(ctx, &core.Namespace{ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kubesift-apply-check-", Labels: map[string]string{"kubesift.dev/fixture": "phase1d8-service"},
			}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("kind-dev-local-%s namespace %s", suffix, ns.Name)
			deferCreatedObject(t, dyn.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}), ns.Name, string(ns.UID))
			installCreateCRD(t, ctx, client, dyn)
			sessions := session.NewManager(nil)
			t.Cleanup(sessions.Close)
			coordinator := operation.NewCoordinator(mgr, nil)
			t.Cleanup(coordinator.Close)
			service := operation.NewApplyService(mgr, kube.NewService(mgr, sessions), coordinator)

			for _, fixture := range []struct{ group, kind, plural, seed, desired string }{
				{"", "ConfigMap", "configmaps", `{"data":{"marker":"shared","foreign":"preserved"}}`, "data: {marker: shared}\n"},
				{"", "Secret", "secrets", `{}`, "data: {marker: d3Jvbmc=}\nstringData: {marker: synthetic}\n"},
				{createFixtureGroup, "Widget", "widgets", `{}`, "spec: {count: 9007199254740993, values: [first, second]}\n"},
				{"scheduling.k8s.io", "PriorityClass", "priorityclasses", `{"value":0,"globalDefault":false}`, "description: synthetic\n"},
			} {
				t.Run(fixture.kind, func(t *testing.T) {
					gvr := schema.GroupVersionResource{Group: fixture.group, Version: "v1", Resource: fixture.plural}
					namespace, name := ns.Name, "check"
					resource := dyn.Resource(gvr)
					api := dynamic.ResourceInterface(resource.Namespace(namespace))
					if fixture.kind == "PriorityClass" {
						namespace, name, api = "", "ks-apply-"+ns.Name, resource
					}
					var seed unstructured.Unstructured
					if err := json.Unmarshal([]byte(fixture.seed), &seed.Object); err != nil {
						t.Fatal(err)
					}
					seed.SetAPIVersion(gvr.GroupVersion().String())
					seed.SetKind(fixture.kind)
					seed.SetName(name)
					seed.SetNamespace(namespace)
					live, err := api.Create(ctx, &seed, metav1.CreateOptions{FieldManager: "fixture-owner"})
					if err != nil {
						t.Fatal(err)
					}
					deferCreatedObject(t, api, name, string(live.GetUID()))
					target := kube.EditTarget{Connection: ref, Group: gvr.Group, Version: gvr.Version, Resource: gvr.Resource, Kind: fixture.kind, Namespace: namespace, Name: name, UID: string(live.GetUID())}
					open := func() operation.ApplyDraft {
						t.Helper()
						draft, err := service.OpenApply(ctx, target)
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { service.CloseApply(draft.ID) })
						return draft
					}
					prepare := func(draft operation.ApplyDraft, desired string, force bool) operation.ApplyReview {
						t.Helper()
						review, err := service.PrepareApply(ctx, operation.ApplyRequest{DraftID: draft.ID, YAML: draft.YAML + desired, TakeOwnership: force})
						if err != nil {
							t.Fatal(err)
						}
						return review
					}
					confirm := func(review operation.ApplyReview) *unstructured.Unstructured {
						t.Helper()
						result, err := service.ExecuteApply(review.Token)
						if err != nil || result.State != "accepted" || result.Resource.UID != target.UID {
							t.Fatalf("apply not accepted: state=%s problem=%+v err=%v", result.State, result.Problem, err)
						}
						out, err := api.Get(ctx, name, metav1.GetOptions{})
						if err != nil {
							t.Fatal(err)
						}
						return out
					}
					fieldChange := func(review operation.ApplyReview, field, action string) bool {
						for _, evidence := range review.Ownership {
							if evidence.Field == field && evidence.Change == action {
								return true
							}
						}
						return false
					}
					value := func(record *unstructured.Unstructured, fields ...string) string {
						v, _, _ := unstructured.NestedString(record.Object, fields...)
						return v
					}
					draft := open()
					review := prepare(draft, fixture.desired, false)
					if review.Token == "" {
						t.Fatalf("review returned conflicts: %+v", review.Conflicts)
					}
					still, err := api.Get(ctx, name, metav1.GetOptions{})
					if err != nil || still.GetResourceVersion() != live.GetResourceVersion() {
						t.Fatalf("dry-run persisted a change: %v", err)
					}
					if fixture.kind == "ConfigMap" && (review.BeforeYAML != review.ServerYAML || !fieldChange(review, ".data.marker", "Shared")) {
						t.Fatalf("the API did not preview a value-invariant ownership change: %+v", review.Ownership)
					}
					applied := confirm(review)
					switch fixture.kind {
					case "ConfigMap":
						draft = open()
						conflict := prepare(draft, "data: {marker: takeover}\n", false)
						if conflict.Token != "" || len(conflict.Conflicts) != 1 || conflict.Conflicts[0].Manager != "fixture-owner" || conflict.Conflicts[0].Field != ".data.marker" {
							t.Fatalf("expected real ownership conflict: %+v", conflict.Conflicts)
						}
						forced := prepare(draft, "data: {marker: takeover}\n", true)
						applied = confirm(forced)
						if value(applied, "data", "marker") != "takeover" || value(applied, "data", "foreign") != "preserved" || !fieldChange(forced, ".data.marker", "Acquired") {
							t.Fatalf("takeover changed an unrelated field or did not change ownership: %+v", forced.Ownership)
						}
						omission := prepare(open(), "", false)
						applied = confirm(omission)
						if value(applied, "data", "marker") != "" || value(applied, "data", "foreign") != "preserved" || !fieldChange(omission, ".data.marker", "Released") {
							t.Fatalf("omission did not remove the owned value and preserve the other manager's field: %+v", omission.Ownership)
						}
						applyPatchDenied(t, ctx, mgr, client, service, target, dir, suffix)
					case "Secret":
						if value(applied, "data", "marker") != "c3ludGhldGlj" || strings.Contains(review.ProposedYAML, "stringData") {
							t.Fatal("Secret stringData precedence was lost")
						}
					case "Widget":
						count, _, _ := unstructured.NestedInt64(applied.Object, "spec", "count")
						if count != 9007199254740993 {
							t.Fatal("precision was lost")
						}
						applied = confirm(prepare(open(), "spec: {count: 9007199254740993, values: [second]}\n", false))
						items, _, _ := unstructured.NestedStringSlice(applied.Object, "spec", "values")
						if len(items) != 1 || items[0] != "second" {
							t.Fatalf("server did not apply the declared list: %v", items)
						}
					case "PriorityClass":
						if value(applied, "description") != "synthetic" {
							t.Fatal("cluster-scoped apply did not persist")
						}
					}
				})
			}
		})
	}
}

func applyPatchDenied(t *testing.T, ctx context.Context, mgr *connection.Manager, client kubernetes.Interface, service *operation.ApplyService, target kube.EditTarget, dir, suffix string) {
	t.Helper()
	const name = "apply-fixture-reader"
	ns := target.Namespace
	role := &rbac.Role{ObjectMeta: metav1.ObjectMeta{Name: name}, Rules: []rbac.PolicyRule{
		{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{target.Name}},
	}}
	if _, err := client.RbacV1().Roles(ns).Create(ctx, role, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	user := ns + "-reader"
	binding := &rbac.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name}, RoleRef: rbac.RoleRef{APIGroup: rbac.GroupName, Kind: "Role", Name: name},
		Subjects: []rbac.Subject{{APIGroup: rbac.GroupName, Kind: "User", Name: user}},
	}
	if _, err := client.RbacV1().RoleBindings(ns).Create(ctx, binding, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	copied, err := clientcmd.LoadFromFile(filepath.Join(dir, suffix+".kubeconfig"))
	if err != nil {
		t.Fatal(err)
	}
	copied.AuthInfos[copied.Contexts["kind-dev-local-"+suffix].AuthInfo].Impersonate = user
	temp := t.TempDir()
	if err := clientcmd.WriteToFile(*copied, filepath.Join(temp, suffix+".kubeconfig")); err != nil {
		t.Fatal(err)
	}
	target.Connection, _ = openLocalOperationFixture(t, context.Background(), mgr, temp, suffix, "")
	draft, err := service.OpenApply(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer service.CloseApply(draft.ID)
	_, err = service.PrepareApply(ctx, operation.ApplyRequest{DraftID: draft.ID, YAML: draft.YAML})
	restartErrorCode(t, err, "forbidden")
}
