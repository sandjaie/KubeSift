//go:build integration

/*
Purpose: Verify reviewed workload deletion against both approved local API servers.
Used by: Opt-in Phase 1D.3 integration verification.
Responsibilities: Create isolated fixtures, prove dry-run isolation and enforce native identity.
Important: All writes are scoped to task-owned namespaces on kind-dev-local-a/b.
*/
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/operation"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const deletionFinalizer = "kubesift.dev/deletion-check"

type deletionFixture struct {
	group, kind, plural, name string
	spec                      map[string]any
}

func (f deletionFixture) ref(connection model.ConnectionRef, object *unstructured.Unstructured) operation.DeleteRequest {
	return operation.DeleteRequest{Resource: kube.ResourceRef{
		Connection: connection, APIVersion: object.GetAPIVersion(), Kind: f.kind,
		Namespace: object.GetNamespace(), Name: f.name, UID: string(object.GetUID()),
	}, SourceVersion: object.GetResourceVersion()}
}

func (f deletionFixture) api(client dynamic.Interface, namespace string) dynamic.ResourceInterface {
	return client.Resource(schema.GroupVersionResource{Group: f.group, Version: "v1", Resource: f.plural}).Namespace(namespace)
}

func (f deletionFixture) create(t *testing.T, ctx context.Context, client dynamic.Interface, ns string, owner *unstructured.Unstructured) *unstructured.Unstructured {
	t.Helper()
	version := "v1"
	if f.group != "" {
		version = f.group + "/v1"
	}
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": version, "kind": f.kind, "spec": f.spec,
		"metadata": map[string]any{"name": f.name, "namespace": ns},
	}}
	if f.kind == "Pod" {
		object.SetFinalizers([]string{deletionFinalizer})
	}
	if owner != nil {
		controller := true
		object.SetOwnerReferences([]metav1.OwnerReference{
			{APIVersion: owner.GetAPIVersion(), Kind: owner.GetKind(), Name: owner.GetName(), UID: owner.GetUID(), Controller: &controller},
		})
	}
	created, err := f.api(client, ns).Create(ctx, object, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func deleteRead(t *testing.T, ctx context.Context, api dynamic.ResourceInterface, name string) *unstructured.Unstructured {
	t.Helper()
	obj, err := api.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

func awaitDeletionFixture(t *testing.T, ctx context.Context, description string, condition wait.ConditionWithContextFunc) {
	t.Helper()
	if err := wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 45*time.Second, true, condition); err != nil {
		t.Fatalf("%s: %v", description, err)
	}
}

func deleteFixtureSpecs() []deletionFixture {
	container := map[string]any{"name": "pause", "image": "registry.k8s.io/pause:3.10", "imagePullPolicy": "Never"}
	pod := map[string]any{"containers": []any{container}, "terminationGracePeriodSeconds": int64(1)}
	template := func(name string, spec map[string]any) map[string]any {
		return map[string]any{"metadata": map[string]any{"labels": map[string]any{"fixture": name}}, "spec": spec}
	}
	controller := func(name string, replicas int64) map[string]any {
		return map[string]any{"replicas": replicas, "template": template(name, pod), "selector": map[string]any{"matchLabels": map[string]any{"fixture": name}}}
	}
	job := map[string]any{"template": template("job", map[string]any{
		"containers": []any{container}, "restartPolicy": "Never", "terminationGracePeriodSeconds": int64(1),
	})}
	stateful := controller("stateful", 0)
	stateful["serviceName"] = "validation"
	retain := controller("retain", 0)
	retain["serviceName"] = "validation"
	retain["persistentVolumeClaimRetentionPolicy"] = map[string]any{"whenDeleted": "Retain", "whenScaled": "Delete"}
	unsafe := controller("unsafe", 0)
	unsafe["serviceName"] = "validation"
	unsafe["persistentVolumeClaimRetentionPolicy"] = map[string]any{"whenDeleted": "Delete"}
	return []deletionFixture{
		{"", "Pod", "pods", "pod", map[string]any{"containers": []any{container}, "nodeSelector": map[string]any{"kubesift.dev/fixture": "never"}}},
		{"", "ReplicationController", "replicationcontrollers", "rc", map[string]any{
			"replicas": int64(0), "selector": map[string]any{"fixture": "rc"}, "template": template("rc", pod),
		}},
		{"apps", "Deployment", "deployments", "deployment", controller("deployment", 1)},
		{"apps", "ReplicaSet", "replicasets", "replicaset", controller("replicaset", 0)},
		{"apps", "StatefulSet", "statefulsets", "stateful", stateful},
		{"apps", "StatefulSet", "statefulsets", "retain", retain},
		{"apps", "StatefulSet", "statefulsets", "unsafe", unsafe},
		{"apps", "DaemonSet", "daemonsets", "daemonset", map[string]any{
			"selector": map[string]any{"matchLabels": map[string]any{"fixture": "daemonset"}},
			"template": template("daemonset", map[string]any{"containers": []any{container}, "nodeSelector": map[string]any{"kubesift.dev/fixture": "never"}}),
		}},
		{"batch", "Job", "jobs", "job", map[string]any{"template": job["template"], "suspend": true}},
		{"batch", "CronJob", "cronjobs", "cronjob", map[string]any{
			"schedule": "0 0 * * *", "suspend": true, "jobTemplate": map[string]any{"spec": job},
		}},
	}
}

func TestReviewedDeletion(t *testing.T) {
	if os.Getenv("KUBESIFT_DELETION_INTEGRATION") != "1" {
		t.Skip("explicit deletion opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			manager := connection.NewManager(nil)
			t.Cleanup(manager.CloseAll)
			coordinator := operation.NewCoordinator(manager, nil)
			t.Cleanup(coordinator.Close)
			service := operation.NewDeleteService(manager, coordinator, nil)
			ref, client := openLocalOperationFixture(t, context.Background(), manager, dir, suffix, "")
			cfg, err := manager.RESTConfig(ref)
			if err != nil {
				t.Fatal(err)
			}
			dyn, err := dynamic.NewForConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			ns, err := client.CoreV1().Namespaces().Create(ctx, &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "kubesift-delete-check-", Labels: map[string]string{"kubesift.dev/fixture": "phase1d3-validation"}},
			}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				api := client.CoreV1().Namespaces()
				if err := api.Delete(cleanup, ns.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}}); err != nil && !apierrors.IsNotFound(err) {
					t.Error(err)
					return
				}
				awaitDeletionFixture(t, cleanup, "fixture namespace removal", func(ctx context.Context) (bool, error) {
					_, err := api.Get(ctx, ns.Name, metav1.GetOptions{})
					return apierrors.IsNotFound(err), nil
				})
			})
			for _, fixture := range deleteFixtureSpecs() {
				t.Run(fixture.name, func(t *testing.T) {
					runDeletionFixture(t, ctx, manager, service, client, dyn, ref, dir, suffix, ns.Name, fixture)
				})
			}
			t.Run("post-GET preconditions", func(t *testing.T) {
				verifyLateDeletePreconditions(t, ctx, manager, service, dyn, ref, ns.Name)
			})
		})
	}
}

func runDeletionFixture(t *testing.T, ctx context.Context, manager *connection.Manager, service *operation.DeleteService, client kubernetes.Interface, dyn dynamic.Interface, ref model.ConnectionRef, dir, suffix, ns string, fixture deletionFixture) {
	t.Helper()
	api := fixture.api(dyn, ns)
	before := fixture.create(t, ctx, dyn, ns, nil)
	if fixture.kind == "Pod" {
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			removeDeletionFixtureFinalizer(t, cleanup, api, fixture.name, before.GetUID())
		})
	}
	listPodLabel := ""
	switch fixture.kind {
	case "Deployment":
		listPodLabel = "fixture=deployment"
	case "CronJob":
		listPodLabel = "fixture=job"
		jobSpec, _, err := unstructured.NestedMap(before.Object, "spec", "jobTemplate", "spec")
		if err != nil || jobSpec == nil {
			t.Fatal("missing CronJob template")
		}
		job := deletionFixture{"batch", "Job", "jobs", "cron-owned", jobSpec}
		job.create(t, ctx, dyn, ns, before)
	}
	pods := client.CoreV1().Pods(ns)
	var child *core.Pod
	if listPodLabel != "" {
		awaitDeletionFixture(t, ctx, "running owned descendant", func(ctx context.Context) (bool, error) {
			list, err := pods.List(ctx, metav1.ListOptions{LabelSelector: listPodLabel})
			if err != nil || len(list.Items) != 1 || list.Items[0].Status.Phase != core.PodRunning {
				return false, err
			}
			child = list.Items[0].DeepCopy()
			return metav1.GetControllerOf(child) != nil, nil
		})
	}
	// Status writes can continue after fixture creation. Wait for a stable post-controller version before reviewing.
	before = deleteRead(t, ctx, api, fixture.name)
	awaitDeletionFixture(t, ctx, "stable workload version", func(ctx context.Context) (bool, error) {
		next := deleteRead(t, ctx, api, fixture.name)
		unchanged := before.GetResourceVersion() == next.GetResourceVersion()
		before = next
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(450 * time.Millisecond):
		}
		return unchanged && deleteRead(t, ctx, api, fixture.name).GetResourceVersion() == before.GetResourceVersion(), nil
	})
	request := fixture.ref(ref, before)
	if fixture.name == "unsafe" {
		_, err := service.PrepareDelete(ctx, request)
		restartErrorCode(t, err, "unsafe_retention")
		return
	}
	if fixture.kind == "ReplicationController" {
		verifyDeletionPermissions(t, ctx, manager, service, client, dir, suffix, request)
	}
	p, err := service.PrepareDelete(ctx, request)
	if err != nil || p.Token == "" {
		t.Fatalf("prepare: %+v %v", p, err)
	}
	still := deleteRead(t, ctx, api, fixture.name)
	if still.GetUID() != before.GetUID() || still.GetResourceVersion() != before.GetResourceVersion() || still.GetDeletionTimestamp() != nil {
		t.Fatal("dry-run mutated the workload")
	}
	if child != nil {
		current, err := pods.Get(ctx, child.Name, metav1.GetOptions{})
		if err != nil || current.UID != child.UID || current.DeletionTimestamp != nil {
			t.Fatal("dry-run affected a dependent", err)
		}
	}
	if fixture.kind == "Pod" {
		_, err = service.ExecuteDelete(ctx, p.Token, "wrong")
		restartErrorCode(t, err, "acknowledgement")
		p, err = service.PrepareDelete(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := service.ExecuteDelete(ctx, p.Token, fixture.name)
	if err != nil || result.State != "accepted" || result.Delete == nil {
		t.Fatalf("execute: %+v %v", result, err)
	}
	if fixture.kind == "Pod" {
		checkDeletionState(t, ctx, service, result.OperationID, "terminating")
		_, err := service.PrepareDelete(ctx, fixture.ref(ref, deleteRead(t, ctx, api, fixture.name)))
		restartErrorCode(t, err, "stale_target")
		removeDeletionFixtureFinalizer(t, ctx, api, fixture.name, before.GetUID())
	}
	awaitDeletionFixture(t, ctx, "target removal", func(ctx context.Context) (bool, error) {
		observation, err := service.CheckDeleteTarget(ctx, result.OperationID)
		if err == nil && observation.State == "unavailable" {
			return false, fmt.Errorf("object absence could not be established")
		}
		return observation.State == "absent", err
	})
	if child != nil {
		awaitDeletionFixture(t, ctx, "background dependent removal", func(ctx context.Context) (bool, error) {
			_, err := pods.Get(ctx, child.Name, metav1.GetOptions{})
			return apierrors.IsNotFound(err), nil
		})
	}
	if fixture.kind == "Pod" {
		replacement := fixture.create(t, ctx, dyn, ns, nil)
		t.Cleanup(func() {
			removeDeletionFixtureFinalizer(t, context.Background(), api, fixture.name, replacement.GetUID())
		})
		checkDeletionState(t, ctx, service, result.OperationID, "replaced")
		if _, err := service.PrepareDelete(ctx, request); err == nil {
			t.Fatal("old request became a review of the replacement")
		}
	}
}

func removeDeletionFixtureFinalizer(t *testing.T, ctx context.Context, api dynamic.ResourceInterface, name string, uid types.UID) {
	t.Helper()
	current, err := api.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) || err == nil && current.GetUID() != uid {
		return
	}
	if err != nil {
		t.Error(err)
		return
	}
	if len(current.GetFinalizers()) == 0 {
		return
	}
	if len(current.GetFinalizers()) != 1 || current.GetFinalizers()[0] != deletionFinalizer {
		t.Error("unexpected finalizer state; will not change fixture")
		return
	}
	patch := []byte(fmt.Sprintf(`[{"op":"test","path":"/metadata/uid","value":%q},{"op":"test","path":"/metadata/finalizers","value":[%q]},{"op":"remove","path":"/metadata/finalizers/0"}]`, uid, deletionFinalizer))
	if _, err := api.Patch(ctx, name, types.JSONPatchType, patch, metav1.PatchOptions{}); err != nil && !apierrors.IsNotFound(err) {
		t.Error(err)
	}
}

func checkDeletionState(t *testing.T, ctx context.Context, service *operation.DeleteService, operationID, want string) {
	t.Helper()
	observed, err := service.CheckDeleteTarget(ctx, operationID)
	if err != nil || observed.State != want {
		t.Fatalf("wanted %s, got %+v %v", want, observed, err)
	}
}

func verifyDeletionPermissions(t *testing.T, ctx context.Context, manager *connection.Manager, service *operation.DeleteService, client kubernetes.Interface, dir, suffix string, req operation.DeleteRequest) {
	t.Helper()
	ns := req.Resource.Namespace
	for _, mode := range []string{"reader", "writer"} {
		user := ns + "-" + mode
		verbs := []string{"get"}
		if mode == "writer" {
			verbs = append(verbs, "delete")
		}
		roles := client.RbacV1().Roles(ns)
		if _, err := roles.Create(ctx, &rbac.Role{
			ObjectMeta: metav1.ObjectMeta{Name: mode},
			Rules:      []rbac.PolicyRule{{Verbs: verbs, APIGroups: []string{""}, Resources: []string{"replicationcontrollers"}, ResourceNames: []string{req.Resource.Name}}},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := client.RbacV1().RoleBindings(ns).Create(ctx, &rbac.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: mode},
			Subjects:   []rbac.Subject{{APIGroup: rbac.GroupName, Kind: "User", Name: user}},
			RoleRef:    rbac.RoleRef{APIGroup: rbac.GroupName, Kind: "Role", Name: mode},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		raw, err := clientcmd.LoadFromFile(filepath.Join(dir, suffix+".kubeconfig"))
		if err != nil {
			t.Fatal(err)
		}
		auth := raw.AuthInfos[raw.Contexts["kind-dev-local-"+suffix].AuthInfo]
		auth.Impersonate = user
		if mode == "writer" {
			auth.ImpersonateGroups = []string{"system:unauthenticated"}
		}
		isolated := t.TempDir()
		if err := clientcmd.WriteToFile(*raw, filepath.Join(isolated, suffix+".kubeconfig")); err != nil {
			t.Fatal(err)
		}
		restricted, _ := openLocalOperationFixture(t, ctx, manager, isolated, suffix, "")
		defer manager.Close(restricted)
		req.Resource.Connection = restricted
		p, err := service.PrepareDelete(ctx, req)
		if mode == "reader" {
			restartErrorCode(t, err, "forbidden")
		} else {
			if err != nil || p.Permission != "unknown" || p.Token == "" {
				t.Fatalf("unavailable hint: %+v %v", p, err)
			}
			service.RevokeDelete(p.Token)
		}
	}
}

// Delay forwarding the app's real DELETE until an owned target changes after the app's final GET.
func verifyLateDeletePreconditions(t *testing.T, ctx context.Context, manager *connection.Manager, service *operation.DeleteService, dyn dynamic.Interface, ref model.ConnectionRef, ns string) {
	t.Helper()
	fixture := deleteFixtureSpecs()[3]
	fixture.name = "late"
	api := fixture.api(dyn, ns)
	fixture.create(t, ctx, dyn, ns, nil)
	path := "/apis/apps/v1/namespaces/" + ns + "/replicasets/" + fixture.name
	cfg, err := manager.RESTConfig(ref)
	if err != nil {
		t.Fatal(err)
	}
	upstream, err := kube.BoundedHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var mode atomic.Int32
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path != path || (r.Method != http.MethodGet && r.Method != http.MethodDelete) {
			t.Error("request outside the owned deletion fixture")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Method == http.MethodDelete {
			var options metav1.DeleteOptions
			if json.Unmarshal(raw, &options) != nil || options.Preconditions == nil ||
				options.Preconditions.UID == nil || options.Preconditions.ResourceVersion == nil {
				t.Error("conditional options were lost")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if len(options.DryRun) == 0 {
				writes.Add(1)
				switch mode.Load() {
				case 1:
					patch := []byte(`{"metadata":{"annotations":{"kubesift.dev/delete-race":"changed"}}}`)
					_, err = api.Patch(r.Context(), fixture.name, types.MergePatchType, patch, metav1.PatchOptions{})
				case 2:
					uid := options.Preconditions.UID
					policy := metav1.DeletePropagationBackground
					err = api.Delete(r.Context(), fixture.name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: uid}, PropagationPolicy: &policy})
					if err == nil {
						_, err = api.Get(r.Context(), fixture.name, metav1.GetOptions{})
						if apierrors.IsNotFound(err) {
							replacement := &unstructured.Unstructured{Object: map[string]any{
								"apiVersion": "apps/v1", "kind": "ReplicaSet", "spec": fixture.spec,
								"metadata": map[string]any{"name": fixture.name, "namespace": ns},
							}}
							_, err = api.Create(r.Context(), replacement, metav1.CreateOptions{})
						} else {
							err = fmt.Errorf("old fixture remained: %v", err)
						}
					}
				}
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, strings.TrimRight(cfg.Host, "/")+path, bytes.NewReader(raw))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		response, err := upstream.Do(req)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer response.Body.Close()
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}))
	defer server.Close()
	kubeconfig := filepath.Join(t.TempDir(), "proxy.kubeconfig")
	raw := fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"proxy","cluster":{"server":%q}}],"contexts":[{"name":"fixture-proxy","context":{"cluster":"proxy","user":"proxy"}}],"users":[{"name":"proxy","user":{}}]}`, server.URL)
	if err := os.WriteFile(kubeconfig, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := config.Load([]string{kubeconfig})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := snapshot.Resolve("fixture-proxy")
	if err != nil {
		t.Fatal(err)
	}
	routed, err := manager.Open(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(routed)

	for _, race := range []int32{1, 2} {
		mode.Store(race)
		var review operation.DeletePreview
		var original *unstructured.Unstructured
		awaitDeletionFixture(t, ctx, "ready to review late-race fixture", func(ctx context.Context) (bool, error) {
			original = deleteRead(t, ctx, api, fixture.name)
			// A new ReplicaSet has no status until its controller has observed it.
			observed, _, _ := unstructured.NestedInt64(original.Object, "status", "observedGeneration")
			if observed != original.GetGeneration() {
				return false, nil
			}
			preview, err := service.PrepareDelete(ctx, fixture.ref(routed, original))
			review = preview
			return err == nil, err
		})
		result, err := service.ExecuteDelete(ctx, review.Token, fixture.name)
		if err != nil || result.State != "stale" || writes.Load() != race {
			t.Fatalf("server did not reject the late race: %+v %v; writes=%d", result, err, writes.Load())
		}
		current := deleteRead(t, ctx, api, fixture.name)
		if current.GetDeletionTimestamp() != nil || (current.GetUID() != original.GetUID()) != (race == 2) ||
			current.GetResourceVersion() == original.GetResourceVersion() {
			t.Fatal("changed or replaced target was not preserved")
		}
	}
}
