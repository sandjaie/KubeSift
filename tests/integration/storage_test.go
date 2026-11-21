//go:build integration

/*
Purpose: Verify storage investigation on both approved kind contexts.
Used by: Opt-in Phase 1C.3d integration checks.
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

func TestStorageInvestigation(t *testing.T) {
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
	const ns = "kubesift-storage-check"
	find := func(c model.ConnectionRef, kind, namespace, name string) kube.ResourceRef {
		t.Helper()
		page, err := reader.ReadStorage(ctx, kube.PageRequest{Connection: c, Kind: kind, Namespace: namespace})
		if err != nil {
			t.Fatal(kind, err)
		}
		for _, row := range page.Items {
			if row.Resource.Name == name {
				return row.Resource
			}
		}
		t.Fatal("fixture not in first page", kind, name)
		return kube.ResourceRef{}
	}
	detail := func(ref kube.ResourceRef) kube.ResourceDetail {
		t.Helper()
		capture, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: ref})
		if err != nil || capture.Resource != ref || capture.ResourceVersion == "" {
			t.Fatal(ref.Kind, err)
		}
		doc, err := reader.ReadResourceYAML(ctx, kube.DetailRequest{Resource: ref})
		if err != nil || !strings.Contains(doc.WithoutManagedFields, "kind: "+ref.Kind) {
			t.Fatal("YAML", ref.Kind, err)
		}
		return capture
	}
	resolve := func(from kube.ResourceDetail, to kube.ResourceRef, identity, binding string) {
		t.Helper()
		expected := to
		to.UID = ""
		result, err := reader.ResolveStorageReference(ctx, kube.StorageReferenceRequest{Source: from.Resource, SourceVersion: from.ResourceVersion, Target: to})
		if err != nil || result.Resource != expected || result.IdentityEvidence != identity || result.BindingEvidence != binding {
			t.Fatal(from.Resource.Kind, to.Kind, result, err)
		}
	}
	for _, suffix := range []string{"a", "b"} {
		t.Log("Checking explicit kind-dev-local-" + suffix)
		c := open(suffix, suffix)
		pods, err := reader.ReadWorkloads(ctx, kube.PageRequest{Connection: c, Kind: "Pod", Namespace: ns})
		if err != nil || len(pods.Items) != 1 || pods.Items[0].Resource.Name != "storage-reference" {
			t.Fatal("reference Pod unavailable", err)
		}
		pod := detail(pods.Items[0].Resource)
		claim := detail(find(c, "PersistentVolumeClaim", ns, "bound-claim"))
		pending := detail(find(c, "PersistentVolumeClaim", ns, "pending-claim"))
		volume := detail(find(c, "PersistentVolume", "", "kubesift-phase1c3d-volume"))
		class := detail(find(c, "StorageClass", "", "kubesift-phase1c3d-static"))
		if !strings.Contains(claim.ObjectJSON, `"phase":"Bound"`) || !strings.Contains(pending.ObjectJSON, `"phase":"Pending"`) {
			t.Fatal("unexpected storage fixture phases")
		}
		resolve(pod, claim.Resource, "name-resolved", "not-applicable")
		resolve(claim, volume.Resource, "name-resolved", "references-agree")
		resolve(volume, class.Resource, "name-resolved", "not-applicable")
		resolve(claim, class.Resource, "name-resolved", "not-applicable")
		resolve(volume, claim.Resource, "uid-verified", "references-agree")
		if _, err := reader.ReadResourceEvents(ctx, kube.EventRequest{Resource: claim.Resource}); err != nil {
			t.Fatal("PVC Events", err)
		}
		for _, kind := range []string{"VolumeAttachment", "CSIDriver", "CSINode"} {
			page, err := reader.ReadStorage(ctx, kube.PageRequest{Connection: c, Kind: kind})
			if err != nil {
				t.Fatal(kind, err)
			}
			t.Logf("%s: %d genuine objects on first page", kind, len(page.Items))
			for _, row := range page.Items {
				detail(row.Resource)
			}
		}
	}
	restricted := open("storage-restricted", "a")
	claim := detail(find(restricted, "PersistentVolumeClaim", ns, "bound-claim"))
	denied := func(err error) {
		t.Helper()
		var problem *model.Problem
		if !errors.As(err, &problem) || problem.Code != "forbidden" {
			t.Fatal("expected independent storage denial", err)
		}
	}
	for _, kind := range []string{"PersistentVolume", "StorageClass", "VolumeAttachment", "CSIDriver", "CSINode"} {
		_, err := reader.ReadStorage(ctx, kube.PageRequest{Connection: restricted, Kind: kind})
		denied(err)
	}
	_, err := reader.ResolveStorageReference(ctx, kube.StorageReferenceRequest{
		Source: claim.Resource, SourceVersion: claim.ResourceVersion,
		Target: kube.ResourceRef{Connection: restricted, Kind: "PersistentVolume", APIVersion: "v1", Name: "kubesift-phase1c3d-volume"},
	})
	denied(err)
	detail(claim.Resource)
}
