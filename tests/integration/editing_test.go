//go:build integration

/*
Purpose: Exercise reviewed editing on the two approved kind contexts.
Used by: The opt-in YAML edit integration run.
Responsibilities: Verify discovery, dry-run, persistence, denial, immutable fields and stale saves.
Important: Mutations target only a test-owned namespace and its synthetic objects.
*/
package integration

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/operation"
	"github.com/sandjaie/KubeSift/internal/session"
	auth "k8s.io/api/authorization/v1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func TestYAMLEditing(t *testing.T) {
	if os.Getenv("KUBESIFT_EDIT_INTEGRATION") != "1" {
		t.Skip("explicit YAML edit opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			manager := connection.NewManager(nil)
			t.Cleanup(manager.CloseAll)
			coordinator := operation.NewCoordinator(manager, nil)
			t.Cleanup(coordinator.Close)
			reader := kube.NewService(manager, session.NewManager(nil))
			editor := operation.NewEditService(manager, reader, coordinator)
			ref, client := openLocalOperationFixture(t, context.Background(), manager, dir, suffix, "")
			cfg, err := manager.RESTConfig(ref)
			if err != nil {
				t.Fatal(err)
			}
			dyn, err := dynamic.NewForConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			namespaces := client.CoreV1().Namespaces()
			ns, err := namespaces.Create(ctx, &core.Namespace{ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kubesift-edit-check-", Labels: map[string]string{"kubesift.dev/fixture": "phase1d4-validation"},
			}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
				defer stop()
				if err := namespaces.Delete(cleanup, ns.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}}); err != nil {
					t.Error(err)
					return
				}
				awaitDeletionFixture(t, cleanup, "edit fixture namespace removal", func(ctx context.Context) (bool, error) {
					_, err := namespaces.Get(ctx, ns.Name, metav1.GetOptions{})
					return apierrors.IsNotFound(err), nil
				})
			})

			const before = "fixture-before"
			const after = "fixture-after"
			for _, fixture := range []struct {
				group, resource, kind string
				fields                map[string]any
			}{
				{"apps", "deployments", "Deployment", map[string]any{"spec": map[string]any{
					"replicas": int64(0), "selector": map[string]any{"matchLabels": map[string]any{"app": "edit"}},
					"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "edit"}},
						"spec": map[string]any{"containers": []any{map[string]any{"name": "app", "image": "registry.k8s.io/pause:3.10", "imagePullPolicy": "Never"}}}},
				}}},
				{"", "configmaps", "ConfigMap", map[string]any{"data": map[string]any{"marker": before}}},
				{"", "secrets", "Secret", map[string]any{"data": map[string]any{"marker": base64.StdEncoding.EncodeToString([]byte(before))}}},
				{"", "services", "Service", map[string]any{"spec": map[string]any{"ports": []any{map[string]any{"port": int64(8080)}}}}},
				{"explorer.kubesift.dev", "widgets", "Widget", map[string]any{"spec": map[string]any{"marker": before}}},
			} {
				t.Run(fixture.kind, func(t *testing.T) {
					api := dyn.Resource(schema.GroupVersionResource{Group: fixture.group, Version: "v1", Resource: fixture.resource}).Namespace(ns.Name)
					apiVersion := "v1"
					if fixture.group != "" {
						apiVersion = fixture.group + "/v1"
					}
					body := map[string]any{"apiVersion": apiVersion, "kind": fixture.kind}
					for k, value := range fixture.fields {
						body[k] = value
					}
					object := &unstructured.Unstructured{Object: body}
					object.SetName("sample")
					object.SetAnnotations(map[string]string{"edit.kubesift.dev/marker": before})
					created, err := api.Create(ctx, object, metav1.CreateOptions{})
					if err != nil {
						t.Fatal(err)
					}
					if fixture.kind == "Deployment" {
						awaitDeletionFixture(t, ctx, "fixture observed", func(ctx context.Context) (bool, error) {
							live, err := api.Get(ctx, created.GetName(), metav1.GetOptions{})
							if err != nil {
								return false, err
							}
							generation, _, _ := unstructured.NestedInt64(live.Object, "status", "observedGeneration")
							return generation == created.GetGeneration(), nil
						})
					}
					target := kube.EditTarget{Connection: ref, Group: fixture.group, Version: "v1", Kind: fixture.kind, Namespace: ns.Name, Name: created.GetName(), UID: string(created.GetUID())}
					if fixture.kind == "Widget" {
						target.Resource = fixture.resource
					}
					draft := openYAMLEdit(t, ctx, editor, target)
					text := strings.ReplaceAll(draft.YAML, before, after)
					if fixture.kind == "Secret" {
						text += "\nstringData:\n  marker: fixture-after\n"
					}
					preview, err := editor.PrepareEdit(ctx, operation.EditRequest{DraftID: draft.ID, YAML: text})
					if err != nil {
						t.Fatal(err)
					}
					current := deleteRead(t, ctx, api, target.Name)
					if current.GetResourceVersion() != draft.ResourceVersion || current.GetAnnotations()["edit.kubesift.dev/marker"] != before {
						t.Fatal("dry-run changed the stored object")
					}
					result, err := editor.ExecuteEdit(preview.Token)
					if err != nil || result.State != "accepted" {
						t.Fatalf("save: %s %v %v", result.State, result.Problem, err)
					}
					updated := deleteRead(t, ctx, api, target.Name)
					if updated.GetAnnotations()["edit.kubesift.dev/marker"] != after || updated.GetUID() != created.GetUID() {
						t.Fatal("saved object did not match the reviewed target and values")
					}
					if fixture.kind == "Secret" {
						value, _, _ := unstructured.NestedString(updated.Object, "data", "marker")
						if value != base64.StdEncoding.EncodeToString([]byte(after)) {
							t.Fatal("Secret stringData did not replace data")
						}
					}
					if fixture.kind == "ConfigMap" {
						checkDeniedYAMLEdit(t, ctx, manager, editor, client, dir, suffix, target)
					}
				})
			}

			target := kube.EditTarget{Connection: ref, Version: "v1", Kind: "Namespace", Name: ns.Name, UID: string(ns.UID)}
			draft := openYAMLEdit(t, ctx, editor, target)
			wanted := strings.ReplaceAll(draft.YAML, "phase1d4-validation", "phase1d4-revised")
			p, err := editor.PrepareEdit(ctx, operation.EditRequest{DraftID: draft.ID, YAML: wanted})
			if err != nil {
				t.Fatal(err)
			}
			result, err := editor.ExecuteEdit(p.Token)
			if err != nil || result.State != "accepted" {
				t.Fatalf("cluster-scope edit: %s %v %v", result.State, result.Problem, err)
			}
			draft = openYAMLEdit(t, ctx, editor, target)
			p, err = editor.PrepareEdit(ctx, operation.EditRequest{DraftID: draft.ID, YAML: strings.ReplaceAll(draft.YAML, "phase1d4-revised", "phase1d4-stale")})
			if err != nil {
				t.Fatal(err)
			}
			current, err := namespaces.Get(ctx, ns.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			current.Annotations = map[string]string{"edit.kubesift.dev/marker": "outside-change"}
			if _, err := namespaces.Update(ctx, current, metav1.UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
			result, err = editor.ExecuteEdit(p.Token)
			if err != nil || result.State != "stale" {
				t.Fatalf("conflict: %s %v", result.State, err)
			}

			immutable := true
			cm, err := client.CoreV1().ConfigMaps(ns.Name).Create(ctx, &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "immutable"},
				Immutable: &immutable, Data: map[string]string{"marker": before},
			}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			draft = openYAMLEdit(t, ctx, editor, kube.EditTarget{Connection: ref, Version: "v1", Kind: "ConfigMap", Namespace: ns.Name, Name: cm.Name, UID: string(cm.UID)})
			defer editor.CloseEdit(draft.ID)
			_, err = editor.PrepareEdit(ctx, operation.EditRequest{DraftID: draft.ID, YAML: strings.ReplaceAll(draft.YAML, before, after)})
			restartErrorCode(t, err, "immutable")
		})
	}
}

func openYAMLEdit(t *testing.T, ctx context.Context, editor *operation.EditService, target kube.EditTarget) operation.EditDraft {
	t.Helper()
	draft, err := editor.OpenEdit(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func checkDeniedYAMLEdit(t *testing.T, ctx context.Context, manager *connection.Manager, editor *operation.EditService, client kubernetes.Interface, dir, suffix string, target kube.EditTarget) {
	t.Helper()
	const identity = "yaml-edit-reader"
	role := rbac.Role{ObjectMeta: metav1.ObjectMeta{Name: identity}, Rules: []rbac.PolicyRule{{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{target.Name}}}}
	if _, err := client.RbacV1().Roles(target.Namespace).Create(ctx, &role, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	binding := rbac.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: identity},
		RoleRef:  rbac.RoleRef{APIGroup: rbac.GroupName, Kind: "Role", Name: identity},
		Subjects: []rbac.Subject{{APIGroup: rbac.GroupName, Kind: "User", Name: target.Namespace + "-" + identity}},
	}
	if _, err := client.RbacV1().RoleBindings(target.Namespace).Create(ctx, &binding, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	kc, err := clientcmd.LoadFromFile(filepath.Join(dir, suffix+".kubeconfig"))
	if err != nil {
		t.Fatal(err)
	}
	kc.AuthInfos[kc.Contexts["kind-dev-local-"+suffix].AuthInfo].Impersonate = binding.Subjects[0].Name
	isolated := t.TempDir()
	if err := clientcmd.WriteToFile(*kc, filepath.Join(isolated, suffix+".kubeconfig")); err != nil {
		t.Fatal(err)
	}
	ref, restricted := openLocalOperationFixture(t, ctx, manager, isolated, suffix, "")
	defer manager.Close(ref)
	status, err := restricted.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &auth.SelfSubjectAccessReview{Spec: auth.SelfSubjectAccessReviewSpec{
		ResourceAttributes: &auth.ResourceAttributes{Namespace: target.Namespace, Resource: "configmaps", Name: target.Name, Verb: "patch"},
	}}, metav1.CreateOptions{})
	if err != nil || status.Status.Allowed {
		t.Fatalf("fixture role must deny Patch: %v", err)
	}
	target.Connection = ref
	draft := openYAMLEdit(t, ctx, editor, target)
	defer editor.CloseEdit(draft.ID)
	_, err = editor.PrepareEdit(ctx, operation.EditRequest{DraftID: draft.ID, YAML: strings.ReplaceAll(draft.YAML, "fixture-after", "fixture-denied")})
	restartErrorCode(t, err, "forbidden")
}
