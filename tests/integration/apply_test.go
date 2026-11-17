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
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
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
