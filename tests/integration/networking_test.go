//go:build integration

/*
Purpose: Verify networking investigation on both approved kind contexts.
Used by: Opt-in Phase 1C.3b integration checks.
Responsibilities: Exercise typed reads, captured references and restricted credentials.
Important: Uses read requests against isolated loopback configurations only.
*/
package integration

import (
	"context"
	"encoding/json"
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

func TestNetworkingInvestigation(t *testing.T) {
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
	const ns = "kubesift-explorer"
	seen := map[string]bool{}
	for _, suffix := range []string{"a", "b"} {
		conn := open(suffix, suffix)
		inventory := func(kind, namespace, name string) kube.ResourceRef {
			t.Helper()
			page, err := reader.ReadNetworking(ctx, kube.PageRequest{Connection: conn, Kind: kind, Namespace: namespace})
			if err != nil {
				t.Fatal(kind, err)
			}
			for _, row := range page.Items {
				if name != "" && row.Resource.Name == name {
					return row.Resource
				}
			}
			if name != "" {
				t.Fatal("missing fixture", suffix, kind, name)
			}
			return kube.ResourceRef{}
		}
		capture := func(target kube.ResourceRef) kube.ResourceDetail {
			t.Helper()
			if target.UID == "" || seen[target.UID] {
				t.Fatal("invalid or duplicate fixture UID", target.Kind)
			}
			seen[target.UID] = true
			details, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: target})
			if err != nil || details.Resource != target || details.ResourceVersion == "" {
				t.Fatal(target.Kind, err)
			}
			yaml, err := reader.ReadResourceYAML(ctx, kube.DetailRequest{Resource: target})
			if err != nil || !strings.Contains(yaml.WithoutManagedFields, "kind: "+target.Kind) {
				t.Fatal(target.Kind, err)
			}
			if _, err = reader.ReadResourceEvents(ctx, kube.EventRequest{Resource: target}); err != nil {
				t.Fatal(target.Kind, err)
			}
			return details
		}
		inventory("IngressClass", "", "")
		capture(inventory("NetworkPolicy", ns, "allow-fixture-network"))
		ingress := capture(inventory("Ingress", ns, "web"))
		svc := inventory("Service", ns, "web")
		capture(svc)
		target := svc
		target.UID = ""
		resolved, err := reader.ResolveNetworkReference(ctx, kube.NetworkReferenceRequest{Source: ingress.Resource, SourceVersion: ingress.ResourceVersion, Target: target})
		if err != nil || resolved.Resource != svc || resolved.IdentityEvidence != "name-resolved" {
			t.Fatal(resolved, err)
		}
		slices, err := reader.ReadServiceEndpointSlices(ctx, kube.RelatedRequest{Resource: svc})
		if err != nil || slices.Problem != nil || len(slices.Items) == 0 || slices.Items[0].Association != "owner-confirmed" {
			t.Fatal(slices, err)
		}
		slice := slices.Items[0].Slice.Resource
		if inventory("EndpointSlice", ns, slice.Name) != slice {
			t.Fatal("slice inventory and association disagree")
		}
		resolved, err = reader.ResolveNetworkReference(ctx, kube.NetworkReferenceRequest{Source: svc, SourceVersion: slices.SourceVersion, Target: slice})
		if err != nil || resolved.Resource != slice || resolved.Association != "owner-confirmed" {
			t.Fatal(resolved, err)
		}
		detail := capture(slice)
		var raw struct {
			Endpoints []struct {
				TargetRef *struct{ Kind, Name, UID string }
			}
		}
		if err = json.Unmarshal([]byte(detail.ObjectJSON), &raw); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, endpoint := range raw.Endpoints {
			if p := endpoint.TargetRef; p != nil && p.Kind == "Pod" && p.UID != "" {
				pod := kube.ResourceRef{Connection: conn, Kind: "Pod", APIVersion: "v1", Namespace: ns, Name: p.Name, UID: p.UID}
				resolved, err = reader.ResolveNetworkReference(ctx, kube.NetworkReferenceRequest{Source: slice, SourceVersion: detail.ResourceVersion, Target: pod})
				if err != nil || resolved.Resource != pod || resolved.IdentityEvidence != "uid-verified" {
					t.Fatal(resolved, err)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatal("no UID-backed fixture Pod reference")
		}
	}
	restricted := open("restricted", "a")
	if page, err := reader.ReadWorkloads(ctx, kube.PageRequest{Connection: restricted, Kind: "Pod", Namespace: ns}); err != nil || len(page.Items) == 0 {
		t.Fatal("fixture Pod access missing", err)
	}
	for _, req := range []kube.PageRequest{{Connection: restricted, Kind: "IngressClass"}, {Connection: restricted, Kind: "Service", Namespace: ns}} {
		var problem *model.Problem
		_, err := reader.ReadNetworking(ctx, req)
		if !errors.As(err, &problem) || problem.Code != "forbidden" {
			t.Fatal("expected explicit networking denial", req.Kind, err)
		}
	}
}
