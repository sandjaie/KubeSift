//go:build integration

/*
Purpose: Prove reviewed creation against the two local API servers.
Used by: Opt-in Phase 1D.7 integration checks.
Responsibilities: Verify precise scope, dry-run non-persistence, create-only authorization and atomic conflicts.
Important: Only isolated approved contexts and UID-owned synthetic fixtures are used.
*/
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
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
	"k8s.io/client-go/tools/clientcmd"
)

const createFixtureGroup = "create-check.kubesift.dev"

func TestYAMLCreation(t *testing.T) {
	if os.Getenv("KUBESIFT_CREATE_INTEGRATION") != "1" {
		t.Skip("explicit creation opt-in required")
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
			ref, client := openLocalOperationFixture(t, context.Background(), manager, dir, suffix, "")
			sessions := session.NewManager(nil)
			t.Cleanup(sessions.Close)
			coordinator := operation.NewCoordinator(manager, nil)
			t.Cleanup(coordinator.Close)
			create := operation.NewCreateService(manager, kube.NewService(manager, sessions), coordinator)
			cfg, err := manager.RESTConfig(ref)
			if err != nil {
				t.Fatal(err)
			}
			dyn, err := dynamic.NewForConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			ns, err := client.CoreV1().Namespaces().Create(ctx, &core.Namespace{ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kubesift-create-check-", Labels: map[string]string{"kubesift.dev/fixture": "phase1d7-validation"},
			}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			deferCreatedObject(t, dyn.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}), ns.Name, string(ns.UID))
			installCreateCRD(t, ctx, client, dyn)
			creator, denied := createPermissionFixtures(t, ctx, manager, client, dir, suffix, ns.Name)
			for _, fixture := range []struct {
				group, plural, kind, name, fields string
				scoped                            bool
				identity                          model.ConnectionRef
			}{
				{"", "configmaps", "ConfigMap", "created-config", "data: {marker: reviewed}\n", true, ref},
				{"", "configmaps", "ConfigMap", "create-only", "data: {marker: permitted}\n", true, creator},
				{"", "secrets", "Secret", "created-secret", "data: {marker: b2xk}\nstringData: {marker: synthetic-text}\n", true, ref},
				{"apps", "deployments", "Deployment", "created-workload", "spec:\n  replicas: 0\n  selector: {matchLabels: {app: create-fixture}}\n  template:\n    metadata: {labels: {app: create-fixture}}\n    spec:\n      containers: [{name: app, image: 'registry.k8s.io/pause:3.10'}]\n", true, ref},
				{createFixtureGroup, "widgets", "Widget", "created-widget", "spec: {count: 9007199254740993}\n", true, ref},
				{"autoscaling", "horizontalpodautoscalers", "HorizontalPodAutoscaler", "created-hpa-v1", "spec: {scaleTargetRef: {apiVersion: apps/v1, kind: Deployment, name: absent-workload}, minReplicas: 1, maxReplicas: 1, targetCPUUtilizationPercentage: 50}\n", true, ref},
				{"scheduling.k8s.io", "priorityclasses", "PriorityClass", "ks-create-" + ns.Name, "value: 0\nglobalDefault: false\npreemptionPolicy: Never\n", false, ref},
			} {
				t.Run(fixture.kind+"/"+fixture.name, func(t *testing.T) {
					apiVersion := "v1"
					if fixture.group != "" {
						apiVersion = fixture.group + "/v1"
					}
					namespace := ""
					api := dynamic.ResourceInterface(dyn.Resource(schema.GroupVersionResource{Group: fixture.group, Version: "v1", Resource: fixture.plural}))
					if fixture.scoped {
						namespace = ns.Name
						api = dyn.Resource(schema.GroupVersionResource{Group: fixture.group, Version: "v1", Resource: fixture.plural}).Namespace(namespace)
					}
					manifest := fmt.Sprintf("apiVersion: %s\nkind: %s\nmetadata:\n  name: %s\n%s", apiVersion, fixture.kind, fixture.name, fixture.fields)
					p := prepareCreateFixture(t, ctx, create, fixture.identity, namespace, manifest)
					if p.Target.Namespace != namespace || p.Target.Resource != fixture.plural || strings.Contains(p.ServerYAML, `"uid":`) {
						t.Fatal("preview scope or simulated identity is wrong")
					}
					if _, err := api.Get(ctx, fixture.name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
						t.Fatalf("preview persisted or name conflicted: %v", err)
					}
					result, err := create.ExecuteCreate(p.Token)
					if err != nil || result.State != "accepted" || result.Resource.UID == "" {
						t.Fatalf("create not accepted: %+v %v", result, err)
					}
					deferCreatedObject(t, api, fixture.name, result.Resource.UID)
					live, err := api.Get(ctx, fixture.name, metav1.GetOptions{})
					if err != nil || string(live.GetUID()) != result.Resource.UID || result.Create.ResourceVersion == "" {
						t.Fatal("result did not carry the actual identity", err)
					}
					if fixture.kind == "Secret" {
						_, err := create.ReadCreatedDetail(ctx, result.OperationID)
						restartErrorCode(t, err, "unsupported")
						marker, _, _ := unstructured.NestedString(live.Object, "data", "marker")
						if marker != "c3ludGhldGljLXRleHQ=" || live.Object["stringData"] != nil {
							t.Fatal("Secret stringData precedence was lost")
						}
					}
					if fixture.kind == "Widget" {
						count, _, _ := unstructured.NestedInt64(live.Object, "spec", "count")
						if count != 9007199254740993 {
							t.Fatal("custom numeric value was not preserved")
						}
					}
					if fixture.kind == "HorizontalPodAutoscaler" {
						detail, err := create.ReadCreatedDetail(ctx, result.OperationID)
						if err != nil || detail.Resource.Group != "autoscaling" || detail.Resource.Version != "v1" || detail.Resource.UID != result.Resource.UID {
							t.Fatalf("alternate API version did not open by the accepted UID: %v", err)
						}
					}

					if fixture.identity == creator {
						_, err := create.ReadCreatedDetail(ctx, result.OperationID)
						restartErrorCode(t, err, "forbidden")
						observed, err := create.InspectCreateName(ctx, result.OperationID)
						if err != nil || observed.State != "unavailable" {
							t.Fatal("create-only fixture did not deny GET", err)
						}
						retained, err := coordinator.GetOperationResult(result.OperationID)
						if err != nil || retained.State != "accepted" {
							t.Fatal("GET denial downgraded accepted result", err)
						}
					}
				})
			}
			for _, identity := range []model.ConnectionRef{denied} {
				draft, err := create.OpenCreate(ctx, identity, ns.Name)
				if err != nil {
					t.Fatal(err)
				}
				defer create.CloseCreate(draft.ID)
				_, err = create.PrepareCreate(ctx, operation.CreateRequest{DraftID: draft.ID, Namespace: ns.Name, YAML: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: denied}\n"})
				restartErrorCode(t, err, "forbidden")
			}
			const conflict = "late-conflict"
			p := prepareCreateFixture(t, ctx, create, ref, ns.Name, "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: "+conflict+"}\ndata: {marker: proposed}\n")
			cms := client.CoreV1().ConfigMaps(ns.Name)
			competing, err := cms.Create(ctx, &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: conflict}, Data: map[string]string{"marker": "external"}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			deferCreatedObject(t, dyn.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace(ns.Name), conflict, string(competing.UID))
			rejected, err := create.ExecuteCreate(p.Token)
			if err != nil || rejected.State != "rejected" || rejected.Problem == nil || rejected.Problem.Code != "already_exists" {
				t.Fatalf("wrong conflict: %+v %v", rejected, err)
			}
			after, err := cms.Get(ctx, conflict, metav1.GetOptions{})
			if err != nil || after.UID != competing.UID || after.ResourceVersion != competing.ResourceVersion || after.Data["marker"] != "external" {
				t.Fatal("the conflict modified the existing resource", err)
			}
		})
	}
}

func prepareCreateFixture(t *testing.T, ctx context.Context, service *operation.CreateService, ref model.ConnectionRef, namespace, manifest string) operation.CreatePreview {
	t.Helper()
	draft, err := service.OpenCreate(ctx, ref, namespace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.CloseCreate(draft.ID) })
	p, err := service.PrepareCreate(ctx, operation.CreateRequest{DraftID: draft.ID, YAML: manifest, Namespace: namespace})
	if err != nil || p.Token == "" {
		t.Fatalf("server preview failed: %v", err)
	}
	return p
}

func deferCreatedObject(t *testing.T, api dynamic.ResourceInterface, name, id string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		uid := types.UID(id)
		err := api.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Error(err)
			return
		}
		awaitDeletionFixture(t, ctx, "create fixture removal", func(ctx context.Context) (bool, error) {
			_, err := api.Get(ctx, name, metav1.GetOptions{})
			return apierrors.IsNotFound(err), nil
		})
	})
}

func installCreateCRD(t *testing.T, ctx context.Context, client kubernetes.Interface, dyn dynamic.Interface) {
	t.Helper()
	api := dyn.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"})
	var obj unstructured.Unstructured
	raw := `{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"widgets.create-check.kubesift.dev"},
 "spec":{"group":"create-check.kubesift.dev","scope":"Namespaced","names":{"plural":"widgets","kind":"Widget"},
 "versions":[{"name":"v1","served":true,"storage":true,"schema":{"openAPIV3Schema":{"type":"object","properties":{"spec":{"type":"object","x-kubernetes-preserve-unknown-fields":true}}}}}]}}`
	if err := obj.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	created, err := api.Create(ctx, &obj, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	deferCreatedObject(t, api, created.GetName(), string(created.GetUID()))
	awaitDeletionFixture(t, ctx, "create fixture discovery", func(_ context.Context) (bool, error) {
		catalog, err := client.Discovery().ServerResourcesForGroupVersion(createFixtureGroup + "/v1")
		return err == nil && catalog != nil && len(catalog.APIResources) > 0, nil
	})
}

func createPermissionFixtures(t *testing.T, ctx context.Context, manager *connection.Manager, client kubernetes.Interface, dir, suffix, ns string) (model.ConnectionRef, model.ConnectionRef) {
	t.Helper()
	const name = "configmap-create-only"
	role := &rbac.Role{ObjectMeta: metav1.ObjectMeta{Name: name}, Rules: []rbac.PolicyRule{
		{Verbs: []string{"create"}, APIGroups: []string{""}, Resources: []string{"configmaps"}},
	}}
	if _, err := client.RbacV1().Roles(ns).Create(ctx, role, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	binding := &rbac.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name}, RoleRef: rbac.RoleRef{APIGroup: rbac.GroupName, Kind: "Role", Name: name},
		Subjects: []rbac.Subject{{APIGroup: rbac.GroupName, Kind: "User", Name: ns + "-creator"}},
	}
	if _, err := client.RbacV1().RoleBindings(ns).Create(ctx, binding, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	open := func(user string) model.ConnectionRef {
		t.Helper()
		cfg, err := clientcmd.LoadFromFile(filepath.Join(dir, suffix+".kubeconfig"))
		if err != nil {
			t.Fatal(err)
		}
		cfg.AuthInfos[cfg.Contexts["kind-dev-local-"+suffix].AuthInfo].Impersonate = user
		temp := t.TempDir()
		if err := clientcmd.WriteToFile(*cfg, filepath.Join(temp, suffix+".kubeconfig")); err != nil {
			t.Fatal(err)
		}
		ref, _ := openLocalOperationFixture(t, ctx, manager, temp, suffix, "")
		return ref
	}
	return open(ns + "-creator"), open(ns + "-denied")
}
