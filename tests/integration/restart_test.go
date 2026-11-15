//go:build integration

/*
Purpose: Prove restart dry-run, conditional writes and controller reconciliation on local API servers.
Used by: Opt-in Phase 1D.2 integration verification.
Responsibilities: Revalidate fixture ownership and distinguish native acceptance from owned Pod replacement.
Important: Only owned workloads inside the two approved local contexts are eligible for test mutations.
*/
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/operation"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const restartNamespace = "kubesift-restart-check"
const marker = "kubectl.kubernetes.io/restartedAt"

type restartTarget struct {
	kind, plural, name string
	blocked, recreate  bool
}

func TestReviewedRestart(t *testing.T) {
	if os.Getenv("KUBESIFT_RESTART_INTEGRATION") != "1" {
		t.Skip("explicit restart opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	manager := connection.NewManager(nil)
	defer manager.CloseAll()
	coordinator := operation.NewCoordinator(manager, nil)
	defer coordinator.Close()
	service := operation.NewRestartService(manager, coordinator, nil)
	scale := operation.NewScaleService(manager, coordinator)
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			ref, client := openLocalOperationFixture(t, ctx, manager, dir, suffix, "")
			own, err := client.CoreV1().Namespaces().Get(ctx, restartNamespace, metav1.GetOptions{})
			if err != nil || own.Labels["kubesift.dev/fixture"] != "phase1d2-validation" {
				t.Fatal("restart fixture ownership unavailable", err)
			}
			denied, _ := openLocalOperationFixture(t, ctx, manager, dir, suffix, "-restart-reader")
			limited, _ := openLocalOperationFixture(t, ctx, manager, dir, suffix, "-restart-writer")
			cfg, err := manager.RESTConfig(ref)
			if err != nil {
				t.Fatal(err)
			}
			dyn, err := dynamic.NewForConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range []restartTarget{
				{"Deployment", "deployments", "reviewed-deployment", false, false},
				{"StatefulSet", "statefulsets", "reviewed-statefulset", false, false},
				{"DaemonSet", "daemonsets", "reviewed-daemonset", false, false},
				{"Deployment", "deployments", "recreate-deployment", false, true},
				{"Deployment", "deployments", "paused-deployment", true, false},
				{"StatefulSet", "statefulsets", "ondelete-statefulset", true, false},
				{"StatefulSet", "statefulsets", "partition-statefulset", true, false},
				{"DaemonSet", "daemonsets", "ondelete-daemonset", true, false},
			} {
				t.Run(item.name, func(t *testing.T) {
					api := dyn.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: item.plural}).Namespace(restartNamespace)
					read := func() *unstructured.Unstructured {
						t.Helper()
						obj, err := api.Get(ctx, item.name, metav1.GetOptions{})
						if err != nil {
							t.Fatal(err)
						}
						return obj
					}
					makeRequest := func(connection model.ConnectionRef, obj *unstructured.Unstructured) operation.RestartRequest {
						return operation.RestartRequest{Resource: kube.ResourceRef{Connection: connection, Kind: item.kind, APIVersion: "apps/v1", Namespace: restartNamespace, Name: item.name, UID: string(obj.GetUID())}, SourceVersion: obj.GetResourceVersion()}
					}
					before := read()
					if item.blocked {
						_, err := service.PrepareRestart(ctx, makeRequest(ref, before))
						restartErrorCode(t, err, "unsupported_strategy")
						if templateMarker(read()) != templateMarker(before) {
							t.Fatal("blocked action changed marker")
						}
						return
					}
					oldPod := waitRestartPod(t, ctx, client, item, before.GetUID(), templateMarker(before), "")
					before = read()
					request := makeRequest(ref, before)
					if !item.recreate {
						_, err := service.PrepareRestart(ctx, makeRequest(denied, before))
						restartErrorCode(t, err, "forbidden")
						delegated, err := service.PrepareRestart(ctx, makeRequest(limited, before))
						if err != nil || delegated.Permission != "unknown" || delegated.Token == "" {
							t.Fatalf("authorized dry-run with unavailable hint failed: %+v %v", delegated, err)
						}
						service.RevokeRestart(delegated.Token)
					}
					p, err := service.PrepareRestart(ctx, request)
					if err != nil || p.Token == "" || p.Impact.Recreate != item.recreate {
						t.Fatalf("prepare failed: %+v %v", p, err)
					}
					during := read()
					if during.GetResourceVersion() != before.GetResourceVersion() || templateMarker(during) != templateMarker(before) {
						t.Fatal("dry-run persisted a change")
					}
					if _, err := scale.ExecuteScale(ctx, p.Token, ""); err == nil {
						t.Fatal("Scale accepted a restart token")
					}
					ack := ""
					if item.recreate {
						_, err = service.ExecuteRestart(ctx, p.Token, "wrong")
						restartErrorCode(t, err, "acknowledgement")
						p, err = service.PrepareRestart(ctx, request)
						if err != nil {
							t.Fatal(err)
						}
						ack = item.name
					}
					result, err := service.ExecuteRestart(ctx, p.Token, ack)
					if err != nil || result.State != "accepted" || result.Restart == nil || result.Restart.ResponseDiffers {
						t.Fatalf("execution failed: %+v %v", result, err)
					}
					replay, err := service.ExecuteRestart(ctx, p.Token, ack)
					if err != nil || replay.OperationID != result.OperationID {
						t.Fatal("duplicate changed result", err)
					}
					if got := read(); templateMarker(got) != p.RestartAt || templateAnnotation(got, "kubesift.dev/sibling") != "retained" {
						t.Fatal("wrong template changes")
					}
					newPod := waitRestartPod(t, ctx, client, item, before.GetUID(), p.RestartAt, oldPod)
					if newPod == oldPod {
						t.Fatal("Pod identity did not change")
					}
					if !item.recreate {
						proveRestartPreconditions(t, ctx, api, read())
					}
				})
			}
		})
	}
}

func restartErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var problem *model.Problem
	if !errors.As(err, &problem) || problem.Code != code {
		t.Fatalf("expected %s: %v", code, err)
	}
}

func templateAnnotation(obj *unstructured.Unstructured, key string) string {
	value, _, _ := unstructured.NestedString(obj.Object, "spec", "template", "metadata", "annotations", key)
	return value
}
func templateMarker(obj *unstructured.Unstructured) string { return templateAnnotation(obj, marker) }

func waitRestartPod(t *testing.T, ctx context.Context, client kubernetes.Interface, item restartTarget, parent types.UID, expected string, exclude types.UID) types.UID {
	t.Helper()
	label := "app=" + item.name
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		owned := map[types.UID]bool{parent: true}
		if item.kind == "Deployment" {
			sets, err := client.AppsV1().ReplicaSets(restartNamespace).List(ctx, metav1.ListOptions{LabelSelector: label})
			if err != nil {
				t.Fatal(err)
			}
			for _, rs := range sets.Items {
				if owner := metav1.GetControllerOf(&rs); owner != nil && owner.UID == parent {
					owned[rs.UID] = true
				}
			}
		}
		pods, err := client.CoreV1().Pods(restartNamespace).List(ctx, metav1.ListOptions{LabelSelector: label})
		if err != nil {
			t.Fatal(err)
		}
		for _, pod := range pods.Items {
			owner := metav1.GetControllerOf(&pod)
			if owner == nil || !owned[owner.UID] || pod.UID == exclude || pod.DeletionTimestamp != nil || pod.Annotations[marker] != expected {
				continue
			}
			for _, condition := range pod.Status.Conditions {
				if condition.Type == core.PodReady && condition.Status == core.ConditionTrue {
					return pod.UID
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("ready owned replacement Pod did not appear")
	return ""
}

func proveRestartPreconditions(t *testing.T, ctx context.Context, api dynamic.ResourceInterface, before *unstructured.Unstructured) {
	t.Helper()
	name := before.GetName()
	advance := []byte(fmt.Sprintf(`{"metadata":{"annotations":{"kubesift.dev/race":%q}}}`, model.ID()))
	advanced, err := api.Patch(ctx, name, types.MergePatchType, advance, metav1.PatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// The version changes after the value used to build the conditional patch has been captured.
	testRejectedRestartPatch(t, ctx, api, advanced, string(before.GetUID()), before.GetResourceVersion())
	uid := before.GetUID()
	foreground := metav1.DeletePropagationForeground
	if err := api.Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &foreground, Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(30 * time.Second)
	for {
		_, err := api.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			break
		}
		if err != nil || time.Now().After(until) {
			t.Fatal("fixture replacement did not settle", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	replacement := before.DeepCopy()
	replacement.Object["metadata"] = map[string]any{"name": name, "namespace": restartNamespace}
	delete(replacement.Object, "status")
	current, err := api.Create(ctx, replacement, metav1.CreateOptions{})
	if err != nil || current.GetUID() == uid {
		t.Fatal("fixture was not recreated", err)
	}
	testRejectedRestartPatch(t, ctx, api, current, string(uid), current.GetResourceVersion())
}

func testRejectedRestartPatch(t *testing.T, ctx context.Context, api dynamic.ResourceInterface, current *unstructured.Unstructured, uid, version string) {
	t.Helper()
	patch, err := json.Marshal([]map[string]string{
		{"op": "test", "path": "/metadata/uid", "value": uid},
		{"op": "test", "path": "/metadata/resourceVersion", "value": version},
		{"op": "add", "path": "/spec/template/metadata/annotations/kubectl.kubernetes.io~1restartedAt", "value": "fixture-must-not-persist"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.Patch(ctx, current.GetName(), types.JSONPatchType, patch, metav1.PatchOptions{})
	if !apierrors.IsInvalid(err) && !apierrors.IsConflict(err) {
		t.Fatalf("API did not enforce the patch tests: %v", err)
	}
	after, err := api.Get(ctx, current.GetName(), metav1.GetOptions{})
	if err != nil || templateMarker(after) != templateMarker(current) || after.GetUID() != current.GetUID() {
		t.Fatal("rejected patch changed target", err)
	}
}
