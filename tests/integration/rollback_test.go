//go:build integration

/*
Purpose: Prove rollback against actual controller-generated revision history.
Used by: Opt-in Phase 1D.5 verification on the two approved kind contexts.
Responsibilities: Evolve and restore templates and verify unrelated fields stay intact.
Important: Only test-owned, UID-guarded namespaces contain mutation fixtures.
*/
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/operation"
	"github.com/sandjaie/KubeSift/internal/session"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const rollbackMarker = "kubesift.dev/rollback-marker"

func TestWorkloadRollback(t *testing.T) {
	if os.Getenv("KUBESIFT_ROLLBACK_INTEGRATION") != "1" {
		t.Skip("explicit workload rollback opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			manager := connection.NewManager(nil)
			t.Cleanup(manager.CloseAll)
			coordinator := operation.NewCoordinator(manager, nil)
			t.Cleanup(coordinator.Close)
			reader := kube.NewService(manager, session.NewManager(nil))
			service := operation.NewRollbackService(manager, coordinator)
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
				GenerateName: "kubesift-rollback-check-", Labels: map[string]string{"kubesift.dev/fixture": "phase1d5-validation"},
			}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("kind-dev-local-%s fixture: %s", suffix, ns.Name)
			t.Cleanup(func() {
				cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
				defer stop()
				if err := namespaces.Delete(cleanup, ns.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}}); err != nil {
					t.Error(err)
					return
				}
				awaitDeletionFixture(t, cleanup, "rollback fixture namespace removal", func(ctx context.Context) (bool, error) {
					_, err := namespaces.Get(ctx, ns.Name, metav1.GetOptions{})
					return apierrors.IsNotFound(err), nil
				})
			})
			for _, fixture := range []struct {
				kind, plural, notice string
				settings             map[string]any
			}{
				{"Deployment", "deployments", "paused", map[string]any{"replicas": int64(0)}},
				{"StatefulSet", "statefulsets", "partition 1", map[string]any{
					"replicas": int64(0), "serviceName": "sample",
					"updateStrategy": map[string]any{"type": "RollingUpdate", "rollingUpdate": map[string]any{"partition": int64(1)}},
					"volumeClaimTemplates": []any{map[string]any{"metadata": map[string]any{"name": "data"}, "spec": map[string]any{
						"accessModes": []any{"ReadWriteOnce"}, "resources": map[string]any{"requests": map[string]any{"storage": "1Mi"}},
					}}},
				}},
				{"DaemonSet", "daemonsets", "OnDelete", map[string]any{"updateStrategy": map[string]any{"type": "OnDelete"}}},
			} {
				t.Run(fixture.kind, func(t *testing.T) {
					api := dyn.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: fixture.plural}).Namespace(ns.Name)
					const name = "sample"
					spec := fixture.settings
					spec["selector"] = map[string]any{"matchLabels": map[string]any{"app": fixture.plural}}
					spec["template"] = map[string]any{
						"metadata": map[string]any{"labels": map[string]any{"app": fixture.plural}, "annotations": map[string]any{rollbackMarker: "fixture-old"}},
						"spec": map[string]any{"nodeSelector": map[string]any{"kubesift.dev/rollback-fixture": "unused"},
							"containers": []any{map[string]any{"name": "app", "image": "registry.k8s.io/pause:3.10", "imagePullPolicy": "Never"}}},
					}
					created, err := api.Create(ctx, &unstructured.Unstructured{Object: map[string]any{
						"apiVersion": "apps/v1", "kind": fixture.kind, "metadata": map[string]any{"name": name}, "spec": spec,
					}}, metav1.CreateOptions{})
					if err != nil {
						t.Fatal(err)
					}
					target := kube.ResourceRef{Connection: ref, APIVersion: "apps/v1", Kind: fixture.kind, Namespace: ns.Name, Name: name, UID: string(created.GetUID())}
					waitHistory := func(count int, generation int64) kube.RolloutHistoryPage {
						t.Helper()
						var captured kube.RolloutHistoryPage
						awaitDeletionFixture(t, ctx, "observed workload with retained history", func(ctx context.Context) (bool, error) {
							object, err := api.Get(ctx, name, metav1.GetOptions{})
							if err != nil {
								return false, err
							}
							observed, _, _ := unstructured.NestedInt64(object.Object, "status", "observedGeneration")
							if observed != generation {
								return false, nil
							}
							captured, err = reader.ReadRolloutHistory(ctx, kube.RolloutHistoryRequest{Resource: target})
							return err == nil && captured.Problem == nil && len(captured.Items) >= count && captured.SourceVersion == object.GetResourceVersion(), err
						})
						return captured
					}
					old := waitHistory(1, created.GetGeneration()).Items[0]
					patch := []byte(fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{%q:"fixture-new"}}}}}`, rollbackMarker))
					changed, err := api.Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
					if err != nil {
						t.Fatal(err)
					}
					history := waitHistory(2, changed.GetGeneration())
					if fixture.kind == "Deployment" {
						changed, err = api.Patch(ctx, name, types.MergePatchType, []byte(`{"spec":{"paused":true}}`), metav1.PatchOptions{})
						if err != nil {
							t.Fatal(err)
						}
						history = waitHistory(2, changed.GetGeneration())
					}
					request := operation.RollbackRequest{Resource: target, ResourceVersion: history.SourceVersion}
					for _, row := range history.Items {
						if row.Resource.UID == old.Resource.UID {
							request.Source, request.SourceVersion, request.Revision = row.Resource, row.ResourceVersion, row.Revision
						}
					}
					if request.SourceVersion == "" {
						t.Fatal("original retained revision disappeared")
					}
					if fixture.kind == "StatefulSet" {
						stale := request
						stale.ResourceVersion = created.GetResourceVersion()
						_, err := service.PrepareRollback(ctx, stale)
						restartErrorCode(t, err, "stale_target")
					}
					preview, err := service.PrepareRollback(ctx, request)
					if err != nil || preview.Token == "" || !strings.Contains(preview.BeforeYAML, "fixture-new") ||
						!strings.Contains(preview.ProposedYAML, "fixture-old") || !strings.Contains(strings.Join(preview.Notices, " "), fixture.notice) {
						t.Fatalf("review failed: notices %v, error %v", preview.Notices, err)
					}
					before := deleteRead(t, ctx, api, name)
					if before.GetResourceVersion() != request.ResourceVersion || templateAnnotation(before, rollbackMarker) != "fixture-new" {
						t.Fatal("dry-run persisted a change")
					}
					result, err := service.ExecuteRollback(preview.Token)
					if err != nil || result.State != "accepted" || result.Rollback == nil || result.Rollback.SourceUID != request.Source.UID {
						t.Fatalf("rollback result %s: %v %v", result.State, result.Problem, err)
					}
					after := deleteRead(t, ctx, api, name)
					if after.GetUID() != before.GetUID() || templateAnnotation(after, rollbackMarker) != "fixture-old" {
						t.Fatal("the reviewed template was not restored to the original workload")
					}
					specWithoutTemplate := func(obj *unstructured.Unstructured) map[string]any {
						spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
						delete(spec, "template")
						return spec
					}
					if !reflect.DeepEqual(specWithoutTemplate(before), specWithoutTemplate(after)) {
						t.Fatal("rollback changed non-template spec fields")
					}
				})
			}
		})
	}
}
