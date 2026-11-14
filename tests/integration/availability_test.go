//go:build integration

/*
Purpose: Verify availability investigation on both approved kind contexts.
Used by: Opt-in Phase 1C.3f integration checks.
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

func TestAvailabilityInvestigation(t *testing.T) {
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
	const ns = "kubesift-availability-check"
	find := func(c model.ConnectionRef, kind, name string) kube.ResourceRef {
		t.Helper()
		req := kube.PageRequest{Connection: c, Kind: kind, Namespace: ns}
		if kind == "ReplicationController" || kind == "StatefulSet" || kind == "DaemonSet" || kind == "Deployment" {
			page, err := reader.ReadWorkloads(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range page.Items {
				if row.Resource.Name == name {
					return row.Resource
				}
			}
		} else {
			page, err := reader.ReadAvailability(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range page.Items {
				if row.Resource.Name == name {
					return row.Resource
				}
			}
		}
		t.Fatal("fixture missing", kind, name)
		return kube.ResourceRef{}
	}
	detail := func(ref kube.ResourceRef) kube.ResourceDetail {
		t.Helper()
		got, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: ref})
		if err != nil || got.ResourceVersion == "" {
			t.Fatal(ref.Kind, err)
		}
		doc, err := reader.ReadResourceYAML(ctx, kube.DetailRequest{Resource: ref})
		if err != nil || !strings.Contains(doc.WithoutManagedFields, "kind: "+ref.Kind) {
			t.Fatal("YAML", ref.Kind, err)
		}
		return got
	}
	resolve := func(from kube.ResourceRef, version string, to kube.ResourceRef, association string) {
		t.Helper()
		got, err := reader.ResolveAvailabilityReference(ctx, kube.AvailabilityReferenceRequest{Source: from, SourceVersion: version, Target: to})
		if err != nil || got.Resource.Name != to.Name || got.Association != association || got.Detail.Resource != got.Resource || got.Detail.ResourceVersion != got.TargetVersion {
			t.Fatal(from.Kind, to.Kind, err)
		}
	}
	for _, suffix := range []string{"a", "b"} {
		t.Log("Checking explicit kind-dev-local-" + suffix)
		c := open(suffix, suffix)
		hpa := detail(find(c, "HorizontalPodAutoscaler", "fixture-hpa"))
		target := find(c, "Deployment", "fixture-scale")
		target.UID = ""
		resolve(hpa.Resource, hpa.ResourceVersion, target, "scale-target")
		pdb := detail(find(c, "PodDisruptionBudget", "fixture-selected"))
		pods, err := reader.ReadPDBSelectedPods(ctx, kube.RelatedRequest{Resource: pdb.Resource})
		if err != nil || len(pods.Items) != 1 || pods.Problem != nil {
			t.Fatal("selector page", pods, err)
		}
		resolve(pdb.Resource, pods.SourceVersion, pods.Items[0].Resource, "selector-match")
		for name, want := range map[string]int{"fixture-unset": 0, "fixture-all": 4} {
			got, err := reader.ReadPDBSelectedPods(ctx, kube.RelatedRequest{Resource: find(c, "PodDisruptionBudget", name)})
			if err != nil || len(got.Items) != want || !got.Complete {
				t.Fatal(name, got, err)
			}
		}
		rc := detail(find(c, "ReplicationController", "fixture-rc"))
		owned, err := reader.ReadRelatedPods(ctx, kube.RelatedRequest{Resource: rc.Resource})
		if err != nil || len(owned.Items) != 1 || owned.Items[0].Controller != rc.Resource || owned.Items[0].Pod.Resource != pods.Items[0].Resource {
			t.Fatal("owned RC Pods", owned, err)
		}
		for kind, name := range map[string]string{"StatefulSet": "fixture-stateful", "DaemonSet": "fixture-daemon"} {
			parent := find(c, kind, name)
			history, err := reader.ReadControllerRevisions(ctx, kube.RelatedRequest{Resource: parent})
			if err != nil || len(history.Items) != 1 || history.Items[0].Summary.Revision != "1" {
				t.Fatal("revision page", history, err)
			}
			rev := detail(history.Items[0].Resource)
			resolve(parent, history.SourceVersion, rev.Resource, "controller-owner")
			resolve(rev.Resource, rev.ResourceVersion, parent, "controller-owner")
			if kind == "StatefulSet" && history.UpdateRevision != rev.Resource.Name {
				t.Fatal("missing update reference")
			}
			if kind == "DaemonSet" && (history.CurrentRevision != "" || history.UpdateRevision != "") {
				t.Fatal("inferred DaemonSet revision")
			}
		}
		for _, r := range []kube.ResourceRef{hpa.Resource, pdb.Resource, rc.Resource} {
			if _, err := reader.ReadResourceEvents(ctx, kube.EventRequest{Resource: r}); err != nil {
				t.Fatal(r.Kind, err)
			}
		}
	}
	c := open("availability-restricted", "a")
	hpa := detail(find(c, "HorizontalPodAutoscaler", "fixture-hpa"))
	_, err := reader.ResolveAvailabilityReference(ctx, kube.AvailabilityReferenceRequest{Source: hpa.Resource, SourceVersion: hpa.ResourceVersion,
		Target: kube.ResourceRef{Connection: c, Kind: "Deployment", APIVersion: "apps/v1", Namespace: ns, Name: "fixture-scale"}})
	var problem *model.Problem
	if !errors.As(err, &problem) || problem.Code != "forbidden" {
		t.Fatal("expected target GET denial", err)
	}
	pods, err := reader.ReadPDBSelectedPods(ctx, kube.RelatedRequest{Resource: find(c, "PodDisruptionBudget", "fixture-selected")})
	if err != nil || pods.Problem == nil || pods.Problem.Code != "forbidden" || pods.Complete {
		t.Fatal("expected selected Pod LIST denial", pods, err)
	}
	revs, err := reader.ReadControllerRevisions(ctx, kube.RelatedRequest{Resource: find(c, "StatefulSet", "fixture-stateful")})
	if err != nil || revs.Problem == nil || revs.Problem.Code != "forbidden" || revs.Complete {
		t.Fatal("expected revision LIST denial", revs, err)
	}
}
