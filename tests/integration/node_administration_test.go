//go:build integration

/*
Purpose: Prove conditional scheduling and eviction on isolated synthetic Node objects.
Used by: Opt-in Phase 1E.5 verification against the two local kind contexts.
Responsibilities: Separate administrative setup and UID-bound cleanup from restricted operation credentials.
Important: Never mutate a host Node or place fixture workloads on one.
*/
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/operation"
	clientapi "k8s.io/client-go/tools/clientcmd/api"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apps "k8s.io/api/apps/v1"
	auth "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	rbac "k8s.io/api/rbac/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
)

func checked[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}

type nodeFixture struct {
	t           *testing.T
	ctx         context.Context
	admin, user *kubernetes.Clientset
	cfg         *rest.Config
	node, ns    string
	uid         types.UID
}

func setupNode(t *testing.T, suffix string) *nodeFixture {
	t.Helper()
	if suffix != "a" && suffix != "b" {
		t.Fatal("invalid context")
	}
	name := "kind-dev-local-" + suffix
	cfg := checked(clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: name}).ClientConfig())
	admin := checked(kubernetes.NewForConfig(cfg))
	ctx := context.Background()
	hosts := checked(admin.CoreV1().Nodes().List(ctx, meta.ListOptions{}))
	if len(hosts.Items) != 1 || hosts.Items[0].Name != "dev-local-"+suffix+"-control-plane" {
		t.Fatal("unexpected host inventory")
	}
	host := hosts.Items[0]
	t.Cleanup(func() {
		after := checked(admin.CoreV1().Nodes().Get(ctx, host.Name, meta.GetOptions{}))
		if after.UID != host.UID || after.Spec.Unschedulable != host.Spec.Unschedulable {
			t.Error("host scheduling state changed")
		}
	})
	ns := checked(admin.CoreV1().Namespaces().Create(ctx, &core.Namespace{ObjectMeta: meta.ObjectMeta{GenerateName: "kubesift-node-proof-"}}, meta.CreateOptions{}))
	clean := func(name string, uid types.UID, del func(context.Context, string, meta.DeleteOptions) error) {
		t.Cleanup(func() {
			e := del(ctx, name, meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &uid}})
			if e != nil && !errors.IsNotFound(e) {
				t.Error(e)
			}
		})
	}
	clean(ns.Name, ns.UID, admin.CoreV1().Namespaces().Delete)
	labels := map[string]string{"kubesift.dev/node-proof": ns.Name}
	object := meta.ObjectMeta{Name: ns.Name, Labels: labels}
	node := checked(admin.CoreV1().Nodes().Create(ctx, &core.Node{ObjectMeta: object, Spec: core.NodeSpec{Taints: []core.Taint{{Key: "kubesift.dev/node-proof", Value: ns.Name, Effect: core.TaintEffectNoSchedule}}}}, meta.CreateOptions{}))
	clean(node.Name, node.UID, admin.CoreV1().Nodes().Delete)
	sa := checked(admin.CoreV1().ServiceAccounts(ns.Name).Create(ctx, &core.ServiceAccount{ObjectMeta: meta.ObjectMeta{Name: "operator"}}, meta.CreateOptions{}))
	subject := []rbac.Subject{{Kind: "ServiceAccount", Name: sa.Name, Namespace: ns.Name}}
	role := checked(admin.RbacV1().ClusterRoles().Create(ctx, &rbac.ClusterRole{ObjectMeta: object, Rules: []rbac.PolicyRule{{APIGroups: []string{""}, Resources: []string{"nodes"}, Verbs: []string{"get", "patch"}, ResourceNames: []string{node.Name}}, {APIGroups: []string{""}, Resources: []string{"nodes", "namespaces", "pods"}, Verbs: []string{"list", "watch"}}}}, meta.CreateOptions{}))
	clean(role.Name, role.UID, admin.RbacV1().ClusterRoles().Delete)
	binding := checked(admin.RbacV1().ClusterRoleBindings().Create(ctx, &rbac.ClusterRoleBinding{ObjectMeta: object, Subjects: subject, RoleRef: rbac.RoleRef{APIGroup: rbac.GroupName, Kind: "ClusterRole", Name: role.Name}}, meta.CreateOptions{}))
	clean(binding.Name, binding.UID, admin.RbacV1().ClusterRoleBindings().Delete)
	checked(admin.RbacV1().Roles(ns.Name).Create(ctx, &rbac.Role{ObjectMeta: object, Rules: []rbac.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}, {APIGroups: []string{"apps"}, Resources: []string{"statefulsets"}, Verbs: []string{"get"}}, {APIGroups: []string{""}, Resources: []string{"pods/eviction"}, Verbs: []string{"create"}}}}, meta.CreateOptions{}))
	checked(admin.RbacV1().RoleBindings(ns.Name).Create(ctx, &rbac.RoleBinding{ObjectMeta: object, Subjects: subject, RoleRef: rbac.RoleRef{APIGroup: rbac.GroupName, Kind: "Role", Name: role.Name}}, meta.CreateOptions{}))
	token := checked(admin.CoreV1().ServiceAccounts(ns.Name).CreateToken(ctx, sa.Name, &auth.TokenRequest{}, meta.CreateOptions{}))
	restricted := rest.AnonymousClientConfig(cfg)
	restricted.BearerToken = token.Status.Token
	t.Logf("%s server %s, synthetic Node %s UID %s", name, checked(admin.Discovery().ServerVersion()).GitVersion, node.Name, node.UID)
	return &nodeFixture{t, ctx, admin, checked(kubernetes.NewForConfig(restricted)), restricted, node.Name, ns.Name, node.UID}
}

func (f *nodeFixture) patch(uid types.UID, version string, state bool) error {
	body := checked(json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": uid}, {"op": "test", "path": "/metadata/resourceVersion", "value": version}, {"op": "add", "path": "/spec/unschedulable", "value": state}}))
	_, err := f.user.CoreV1().Nodes().Patch(f.ctx, f.node, types.JSONPatchType, body, meta.PatchOptions{})
	return err
}
func (f *nodeFixture) getNode() *core.Node {
	return checked(f.user.CoreV1().Nodes().Get(f.ctx, f.node, meta.GetOptions{}))
}
func (f *nodeFixture) workload() *core.Pod {
	ctx := f.ctx
	labels := map[string]string{"kubesift.dev/node-proof": f.ns}
	set := checked(f.admin.AppsV1().StatefulSets(f.ns).Create(ctx, &apps.StatefulSet{ObjectMeta: meta.ObjectMeta{Name: "fixture"}, Spec: apps.StatefulSetSpec{ServiceName: "fixture", Replicas: ptr.To(int32(1)), Selector: &meta.LabelSelector{MatchLabels: labels}, Template: core.PodTemplateSpec{ObjectMeta: meta.ObjectMeta{Labels: labels}, Spec: core.PodSpec{NodeName: f.node, Containers: []core.Container{{Name: "fixture", Image: "registry.k8s.io/pause:3.10"}}, Tolerations: []core.Toleration{{Operator: core.TolerationOpExists}}}}}}, meta.CreateOptions{}))
	pods := f.admin.CoreV1().Pods(f.ns)
	f.t.Cleanup(func() {
		checkedDelete := func(e error) {
			if e != nil && !errors.IsNotFound(e) {
				f.t.Error(e)
			}
		}
		checkedDelete(f.admin.AppsV1().StatefulSets(f.ns).Delete(ctx, set.Name, meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &set.UID}, PropagationPolicy: ptr.To(meta.DeletePropagationOrphan)}))
		p, e := pods.Get(ctx, "fixture-0", meta.GetOptions{})
		if errors.IsNotFound(e) {
			return
		}
		if e != nil || p.Spec.NodeName != f.node || p.Labels["kubesift.dev/node-proof"] != f.ns {
			f.t.Error("unexpected cleanup identity")
			return
		}
		checkedDelete(pods.Delete(ctx, p.Name, meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &p.UID}, GracePeriodSeconds: ptr.To(int64(0))}))
	})
	var pod *core.Pod
	awaitDeletionFixture(f.t, ctx, "controller-created synthetic Pod", func(ctx context.Context) (bool, error) {
		var e error
		pod, e = pods.Get(ctx, "fixture-0", meta.GetOptions{})
		if errors.IsNotFound(e) {
			return false, nil
		}
		return e == nil, e
	})
	if pod.Spec.NodeName != f.node || meta.GetControllerOf(pod).UID != set.UID {
		f.t.Fatal("fixture ownership mismatch")
	}
	return pod
}
func (f *nodeFixture) evict(p *core.Pod, uid types.UID, rv string) error {
	return f.user.PolicyV1().Evictions(f.ns).Evict(f.ctx, &policy.Eviction{ObjectMeta: meta.ObjectMeta{Name: p.Name, Namespace: f.ns}, DeleteOptions: &meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &uid, ResourceVersion: &rv}}})
}
func TestNodeEvictionProtocol(t *testing.T) {
	if os.Getenv("KUBESIFT_NODE_INTEGRATION") != "1" {
		t.Skip("explicit opt-in required")
	}
	seen := map[types.UID]bool{}
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			f := setupNode(t, suffix)
			if seen[f.uid] {
				t.Fatal("same UID across contexts")
			}
			seen[f.uid] = true
			n := f.getNode()
			for _, e := range []error{f.patch("other", n.ResourceVersion, true), f.patch(n.UID, "1", true)} {
				if !errors.IsInvalid(e) && !errors.IsConflict(e) {
					t.Fatalf("stale patch: %v", e)
				}
			}
			if f.getNode().Spec.Unschedulable {
				t.Fatal("stale patch changed node")
			}
			if e := f.patch(n.UID, f.getNode().ResourceVersion, true); e != nil {
				t.Fatal(e)
			}
			if !f.getNode().Spec.Unschedulable {
				t.Fatal("cordon unobserved")
			}
			if e := f.patch(n.UID, f.getNode().ResourceVersion, false); e != nil {
				t.Fatal(e)
			}
			p := f.workload()
			pods := f.admin.CoreV1().Pods(f.ns)
			for _, e := range []error{f.evict(p, "other", p.ResourceVersion), f.evict(p, p.UID, "1")} {
				if !errors.IsConflict(e) {
					t.Fatalf("stale eviction: %v", e)
				}
			}
			current := checked(pods.Get(f.ctx, p.Name, meta.GetOptions{}))
			if current.UID != p.UID || current.DeletionTimestamp != nil {
				t.Fatal("stale eviction changed pod")
			}
			if e := pods.Delete(f.ctx, p.Name, meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &p.UID}, GracePeriodSeconds: ptr.To(int64(0))}); e != nil {
				t.Fatal(e)
			}
			var replacement *core.Pod
			awaitDeletionFixture(t, f.ctx, "same-name replacement", func(ctx context.Context) (bool, error) {
				var e error
				replacement, e = pods.Get(ctx, p.Name, meta.GetOptions{})
				if errors.IsNotFound(e) {
					return false, nil
				}
				return e == nil && replacement.UID != p.UID, e
			})
			if e := f.evict(replacement, p.UID, replacement.ResourceVersion); !errors.IsConflict(e) {
				t.Fatalf("old UID against replacement: %v", e)
			}
			budget := f.protect(replacement)
			replacement = checked(pods.Get(f.ctx, p.Name, meta.GetOptions{}))
			e := f.evict(replacement, replacement.UID, replacement.ResourceVersion)
			if !errors.IsTooManyRequests(e) || !strings.Contains(strings.ToLower(fmt.Sprint(e)), "disruption budget") {
				t.Fatalf("expected budget-specific 429, got %v", e)
			}
			current = checked(pods.Get(f.ctx, p.Name, meta.GetOptions{}))
			if current.UID != replacement.UID || current.DeletionTimestamp != nil {
				t.Fatal("PDB denial changed pod")
			}
			budget = checked(f.admin.PolicyV1().PodDisruptionBudgets(f.ns).Get(f.ctx, budget.Name, meta.GetOptions{}))
			budget.Spec.MinAvailable = ptr.To(intstr.FromInt(0))
			budget = checked(f.admin.PolicyV1().PodDisruptionBudgets(f.ns).Update(f.ctx, budget, meta.UpdateOptions{}))
			awaitDeletionFixture(t, f.ctx, "PDB permits one disruption", func(ctx context.Context) (bool, error) {
				b, e := f.admin.PolicyV1().PodDisruptionBudgets(f.ns).Get(ctx, budget.Name, meta.GetOptions{})
				return e == nil && b.Status.ObservedGeneration == b.Generation && b.Status.DisruptionsAllowed == 1, e
			})
			current = checked(pods.Get(f.ctx, p.Name, meta.GetOptions{}))
			if e := f.evict(current, current.UID, current.ResourceVersion); e != nil {
				t.Fatalf("matching request after PDB allows disruption: %v", e)
			}
			after, e := pods.Get(f.ctx, p.Name, meta.GetOptions{})
			if e != nil && !errors.IsNotFound(e) {
				t.Fatal(e)
			}
			if e == nil && after.UID == current.UID && after.DeletionTimestamp == nil {
				t.Fatal("eviction accepted without observed termination")
			}
			t.Log("stale UID/version and same-name eviction rejected; PDB denied with 429; allowed conditional eviction accepted")
		})
	}
}

func (f *nodeFixture) protect(pod *core.Pod) *policy.PodDisruptionBudget {
	pods := f.admin.CoreV1().Pods(f.ns)
	pod.Status.Phase = core.PodRunning
	pod.Status.Conditions = []core.PodCondition{{Type: core.PodReady, Status: core.ConditionTrue}}
	pod = checked(pods.UpdateStatus(f.ctx, pod, meta.UpdateOptions{}))
	budget := checked(f.admin.PolicyV1().PodDisruptionBudgets(f.ns).Create(f.ctx, &policy.PodDisruptionBudget{ObjectMeta: meta.ObjectMeta{Name: "protect"}, Spec: policy.PodDisruptionBudgetSpec{MinAvailable: ptr.To(intstr.FromInt(1)), Selector: &meta.LabelSelector{MatchLabels: map[string]string{"kubesift.dev/node-proof": f.ns}}}}, meta.CreateOptions{}))
	awaitDeletionFixture(f.t, f.ctx, "PDB accounts for Running Ready pod", func(ctx context.Context) (bool, error) {
		b, e := f.admin.PolicyV1().PodDisruptionBudgets(f.ns).Get(ctx, budget.Name, meta.GetOptions{})
		return e == nil && b.Status.ObservedGeneration == b.Generation && b.Status.CurrentHealthy == 1 && b.Status.DisruptionsAllowed == 0, e
	})
	return budget
}
func (f *nodeFixture) configFile(dir, name string) string {
	path := filepath.Join(dir, name+".kubeconfig")
	conf := clientapi.NewConfig()
	conf.Clusters[name] = &clientapi.Cluster{Server: f.cfg.Host, CertificateAuthorityData: f.cfg.CAData, CertificateAuthority: f.cfg.CAFile}
	conf.AuthInfos[name] = &clientapi.AuthInfo{Token: f.cfg.BearerToken}
	conf.Contexts[name] = &clientapi.Context{AuthInfo: name, Cluster: name, Namespace: f.ns}
	conf.CurrentContext = name
	if e := clientcmd.WriteToFile(*conf, path); e != nil {
		f.t.Fatal(e)
	}
	return path
}
func (f *nodeFixture) preflight(want ...types.UID) {
	pods := checked(f.admin.CoreV1().Pods("").List(f.ctx, meta.ListOptions{FieldSelector: "spec.nodeName=" + f.node}))
	if len(pods.Items) != len(want) {
		f.t.Fatal("unexpected assigned Pod count; abort before product writes")
	}
	allowed := map[types.UID]bool{}
	for _, id := range want {
		allowed[id] = true
	}
	for _, p := range pods.Items {
		if p.Namespace != f.ns || !allowed[p.UID] {
			f.t.Fatal("unexpected assigned Pod identity")
		}
	}
}
func TestNodeAdministration(t *testing.T) {
	if os.Getenv("KUBESIFT_NODE_INTEGRATION") != "1" {
		t.Skip("explicit opt-in required")
	}
	manager := connection.NewManager(nil)
	defer manager.CloseAll()
	c := operation.NewCoordinator(manager, nil)
	defer c.Close()
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			f := setupNode(t, suffix)
			name := "kind-dev-local-" + suffix
			path := f.configFile(t.TempDir(), name)
			resolved := checked(checked(config.Load([]string{path})).Resolve(name))
			ref := checked(manager.Open(f.ctx, resolved))
			defer manager.Close(ref)
			review := func(action string) operation.NodePreview {
				n := f.getNode()
				p, e := c.PrepareNodeAction(f.ctx, operation.NodeRequest{Resource: kube.ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Node", Name: f.node, UID: string(f.uid)}, Action: action, SourceVersion: n.ResourceVersion, WaitPreset: 2}, "Development")
				if e != nil {
					t.Fatal(e)
				}
				return p
			}
			start := func(p operation.NodePreview) operation.OperationResult {
				if p.Token == "" {
					t.Fatalf("missing token %+v", p)
				}
				ack := ""
				if p.Typed {
					ack = f.node
				}
				r := checked(c.ExecuteNodeAction(p.Token, ack))
				again := checked(c.ExecuteNodeAction(p.Token, ack))
				if again.OperationID != r.OperationID {
					t.Fatal("duplicate execution")
				}
				return r
			}
			wait := func(id string, state string) operation.OperationResult {
				var r operation.OperationResult
				awaitDeletionFixture(t, f.ctx, state, func(context.Context) (bool, error) {
					r = checked(c.GetOperationResult(id))
					if r.FinishedAt != nil && r.State != state {
						return false, fmt.Errorf("unexpected result %s: %s", r.State, r.Node.Reason)
					}
					return r.State == state || r.Node.Counts[state] > 0, nil
				})
				return r
			}
			f.preflight()
			for _, action := range []string{"cordon", "uncordon", "drain", "uncordon"} {
				p := review(action)
				r := wait(start(p).OperationID, "completed")
				if r.Node.Cordoned == nil || *r.Node.Cordoned != (action != "uncordon") {
					t.Fatal("incorrect final scheduling")
				}
			}
			bare := checked(f.admin.CoreV1().Pods(f.ns).Create(f.ctx, &core.Pod{ObjectMeta: meta.ObjectMeta{Name: "unmanaged"}, Spec: core.PodSpec{NodeName: f.node, Containers: []core.Container{{Name: "fixture", Image: "registry.k8s.io/pause:3.10"}}}}, meta.CreateOptions{}))
			f.preflight(bare.UID)
			p := review("drain")
			if p.Token != "" || p.Counts["blocked"] != 1 || f.getNode().Spec.Unschedulable {
				t.Fatal("unsafe initial drain review")
			}
			e := f.admin.CoreV1().Pods(f.ns).Delete(f.ctx, bare.Name, meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &bare.UID}, GracePeriodSeconds: ptr.To(int64(0))})
			if e != nil {
				t.Fatal(e)
			}
			pod := f.workload()
			budget := f.protect(pod)
			f.preflight(pod.UID)
			id := start(review("drain")).OperationID
			wait(id, "budget-blocked")
			c.CancelDrain(id)
			wait(id, "cancelled")
			if !f.getNode().Spec.Unschedulable {
				t.Fatal("drain lost cordon after cancellation")
			}
			e = f.admin.PolicyV1().PodDisruptionBudgets(f.ns).Delete(f.ctx, budget.Name, meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &budget.UID}})
			if e != nil {
				t.Fatal(e)
			}
			f.preflight(pod.UID)
			id = start(review("drain")).OperationID
			awaitDeletionFixture(t, f.ctx, "admitted Pod begins terminating", func(context.Context) (bool, error) {
				r := checked(c.GetOperationResult(id))
				if r.FinishedAt != nil {
					return false, fmt.Errorf("unexpected final state %s: %s", r.State, r.Node.Reason)
				}
				return r.Node.Counts["terminating"] == 1, nil
			})
			// A synthetic Node has no kubelet. End only the owned fixture, then observe the worker's final inventory.
			sets := f.admin.AppsV1().StatefulSets(f.ns)
			set := checked(sets.Get(f.ctx, "fixture", meta.GetOptions{}))
			if set.UID != meta.GetControllerOf(pod).UID {
				t.Fatal("unexpected fixture controller")
			}
			if e = sets.Delete(f.ctx, set.Name, meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &set.UID}, PropagationPolicy: ptr.To(meta.DeletePropagationOrphan)}); e != nil {
				t.Fatal(e)
			}
			awaitDeletionFixture(t, f.ctx, "fixture controller removed", func(ctx context.Context) (bool, error) {
				_, err := sets.Get(ctx, set.Name, meta.GetOptions{})
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			})
			if e = f.admin.CoreV1().Pods(f.ns).Delete(f.ctx, pod.Name, meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &pod.UID}, GracePeriodSeconds: ptr.To(int64(0))}); e != nil {
				t.Fatal(e)
			}
			final := wait(id, "completed")
			if final.Node.Counts["gone"] != 1 {
				t.Fatal("reviewed UID disappearance not observed")
			}
			wait(start(review("uncordon")).OperationID, "completed")
			t.Log("empty drain completed; unmanaged Pod blocked before cordon; budget wait cancelled; accepted Pod observed Terminating; fixture-only teardown enabled final gone observation")
		})
	}
}

func TestNodeDesktopFixture(t *testing.T) {
	dir := os.Getenv("KUBESIFT_NODE_DESKTOP_DIR")
	if os.Getenv("KUBESIFT_NODE_INTEGRATION") != "1" || dir == "" {
		t.Skip("desktop fixture opt-in required")
	}
	info := checked(os.Stat(dir))
	if !filepath.IsAbs(dir) || !info.IsDir() || info.Mode().Perm() != 0700 || len(checked(os.ReadDir(dir))) != 0 {
		t.Fatal("new private directory required")
	}
	gate := func(stage string) {
		t.Log("desktop fixture stage:", stage)
		deadline := time.Now().Add(20 * time.Minute)
		for time.Now().Before(deadline) {
			if _, e := os.Stat(filepath.Join(dir, stage)); e == nil {
				return
			}
			time.Sleep(time.Second)
		}
		t.Fatal("desktop fixture timed out")
	}
	a := setupNode(t, "a")
	b := setupNode(t, "b")
	a.configFile(dir, "kind-dev-local-a")
	b.configFile(dir, "kind-dev-local-b")
	bare := checked(a.admin.CoreV1().Pods(a.ns).Create(a.ctx, &core.Pod{ObjectMeta: meta.ObjectMeta{Name: "temporary-storage"}, Spec: core.PodSpec{NodeName: a.node, Containers: []core.Container{{Name: "fixture", Image: "registry.k8s.io/pause:3.10"}}, Volumes: []core.Volume{{Name: "scratch", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}}}}, meta.CreateOptions{}))
	a.preflight(bare.UID)
	b.preflight()
	gate("budget")
	if e := a.admin.CoreV1().Pods(a.ns).Delete(a.ctx, bare.Name, meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &bare.UID}, GracePeriodSeconds: ptr.To(int64(0))}); e != nil {
		t.Fatal(e)
	}
	pod := a.workload()
	a.protect(pod)
	a.preflight(pod.UID)
	gate("finished")
}
