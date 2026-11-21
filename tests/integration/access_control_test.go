//go:build integration

/*
Purpose: Verify access control investigation on both approved kind contexts.
Used by: Opt-in Phase 1C.3e integration checks.
Responsibilities: Exercise typed reads, captured references and restricted credentials.
Important: Uses read requests against isolated loopback configurations only.
*/
package integration

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
)

func TestAccessControlInvestigation(t *testing.T) {
	if os.Getenv("KUBESIFT_EXPLORER_INTEGRATION") != "1" {
		t.Skip("explicit opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("absolute isolated configuration directory required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	manager := connection.NewManager(nil)
	defer manager.CloseAll()
	sessions := session.NewManager(func(model.Event) {})
	defer sessions.Close()
	reader := kube.NewService(manager, sessions)
	open := func(file, suffix string) model.ConnectionRef {
		t.Helper()
		snapshot, err := config.Load([]string{filepath.Join(dir, file+".kubeconfig")})
		if err != nil {
			t.Fatal(err)
		}
		target := "kind-dev-local-" + suffix
		if entries := snapshot.Contexts(); len(entries) != 1 || entries[0].Name != target {
			t.Fatal("unexpected context")
		}
		resolved, err := snapshot.Resolve(target)
		if err != nil {
			t.Fatal(err)
		}
		endpoint, err := url.Parse(resolved.RESTConfig().Host)
		if err != nil || endpoint.Scheme != "https" || endpoint.User != nil {
			t.Fatal("invalid local endpoint")
		}
		ip := net.ParseIP(endpoint.Hostname())
		if endpoint.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			t.Fatal("nonlocal endpoint rejected")
		}
		ref, err := manager.Open(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	const ns = "kubesift-access-check"
	const other = "kubesift-access-subject"
	const api = "rbac.authorization.k8s.io/v1"
	const localBinding = "kubesift:fixture:local-binding"
	const localRole = "kubesift:fixture:local"
	const aggregate = "kubesift:fixture:phase1c3e-aggregate"
	const member = "kubesift:fixture:phase1c3e-member"
	const clusterBinding = "kubesift:fixture:phase1c3e-cluster-binding"
	find := func(c model.ConnectionRef, kind, namespace, name string) kube.ResourceRef {
		t.Helper()
		req := kube.PageRequest{Connection: c, Kind: kind, Namespace: namespace}
		for n := 0; n < 20; n++ {
			page, err := reader.ReadAccessControl(ctx, req)
			if err != nil {
				t.Fatal(kind, err)
			}
			for _, row := range page.Items {
				if row.Resource.Name == name {
					return row.Resource
				}
			}
			if page.Page.NextCursor == "" {
				break
			}
			req.Cursor = page.Page.NextCursor
		}
		t.Fatal("fixture missing", kind, name)
		return kube.ResourceRef{}
	}
	detail := func(ref kube.ResourceRef) kube.ResourceDetail {
		t.Helper()
		got, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: ref})
		if err != nil || got.Resource != ref || got.ResourceVersion == "" {
			t.Fatal(ref.Kind, err)
		}
		doc, err := reader.ReadResourceYAML(ctx, kube.DetailRequest{Resource: ref})
		if err != nil || !strings.Contains(doc.WithoutManagedFields, "kind: "+ref.Kind) {
			t.Fatal("YAML", ref.Kind, err)
		}
		return got
	}
	resolve := func(from kube.ResourceDetail, to kube.ResourceRef) {
		t.Helper()
		want := to
		to.UID = ""
		result, err := reader.ResolveAccessControlReference(ctx, kube.AccessControlReferenceRequest{Source: from.Resource, SourceVersion: from.ResourceVersion, Target: to})
		if err != nil || result.Resource != want || result.Detail.Resource != want || result.TargetVersion != result.Detail.ResourceVersion || result.IdentityEvidence != "name-resolved" {
			t.Fatal(from.Resource.Kind, to.Kind, err)
		}
	}
	for _, suffix := range []string{"a", "b"} {
		t.Log("Checking explicit kind-dev-local-" + suffix)
		c := open(suffix, suffix)
		pods, err := reader.ReadWorkloads(ctx, kube.PageRequest{Connection: c, Kind: "Pod", Namespace: ns})
		if err != nil || len(pods.Items) != 1 || pods.Items[0].Resource.Name != "access-reference" {
			t.Fatal("unscheduled reference Pod unavailable", err)
		}
		pod := detail(pods.Items[0].Resource)
		sa := detail(find(c, "ServiceAccount", ns, "fixture-reader"))
		subject := detail(find(c, "ServiceAccount", other, "fixture-subject"))
		role := detail(find(c, "Role", ns, localRole))
		rb := detail(find(c, "RoleBinding", ns, localBinding))
		cr := detail(find(c, "ClusterRole", "", aggregate))
		crb := detail(find(c, "ClusterRoleBinding", "", clusterBinding))
		if !strings.Contains(sa.ObjectJSON, `"automountServiceAccountToken":false`) || !strings.Contains(cr.ObjectJSON, "clusterRoleSelectors") {
			t.Fatal("configured fixture evidence missing")
		}
		resolve(pod, sa.Resource)
		resolve(rb, role.Resource)
		resolve(rb, sa.Resource)
		resolve(rb, subject.Resource)
		resolve(detail(find(c, "RoleBinding", ns, "kubesift:fixture:cluster-role-binding")), cr.Resource)
		resolve(crb, find(c, "ClusterRole", "", member))
		resolve(crb, subject.Resource)
		for _, from := range []kube.ResourceDetail{sa, role, rb} {
			if _, err := reader.ReadResourceEvents(ctx, kube.EventRequest{Resource: from.Resource}); err != nil {
				t.Fatal(from.Resource.Kind, "Events", err)
			}
		}
	}
	c := open("access-restricted", "a")
	detail(find(c, "ServiceAccount", ns, "fixture-reader"))
	rb := detail(find(c, "RoleBinding", ns, localBinding))
	role := find(c, "Role", ns, localRole)
	denied := func(err error) {
		t.Helper()
		var problem *model.Problem
		if !errors.As(err, &problem) || problem.Code != "forbidden" {
			t.Fatal("expected independent authorization denial", err)
		}
	}
	_, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: role})
	denied(err)
	_, err = reader.ResolveAccessControlReference(ctx, kube.AccessControlReferenceRequest{
		Source: rb.Resource, SourceVersion: rb.ResourceVersion, Target: kube.ResourceRef{Connection: c, Kind: "Role", APIVersion: api, Namespace: ns, Name: localRole},
	})
	denied(err)
	for _, kind := range []string{"ClusterRole", "ClusterRoleBinding"} {
		_, err = reader.ReadAccessControl(ctx, kube.PageRequest{Connection: c, Kind: kind})
		denied(err)
	}
}
