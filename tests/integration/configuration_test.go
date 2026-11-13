//go:build integration

/*
Purpose: Verify configuration investigation on both approved kind contexts.
Used by: Opt-in Phase 1C.3c integration checks.
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

func TestConfigurationInvestigation(t *testing.T) {
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
	const ns = "kubesift-config-check"
	for _, suffix := range []string{"a", "b"} {
		c := open(suffix, suffix)
		for _, fixture := range []struct{ kind, namespace, name string }{
			{"Namespace", "", ns}, {"ResourceQuota", ns, "fixture-quota"}, {"LimitRange", ns, "fixture-limits"}, {"ConfigMap", ns, "fixture-config"}, {"Secret", ns, "fixture-secret"},
		} {
			page, err := reader.ReadConfiguration(ctx, kube.PageRequest{Connection: c, Kind: fixture.kind, Namespace: fixture.namespace})
			if err != nil {
				t.Fatal(fixture.kind, err)
			}
			var ref kube.ResourceRef
			for _, row := range page.Items {
				if row.Resource.Name == fixture.name {
					ref = row.Resource
					if strings.Contains(row.ObjectJSON, "literal") || strings.Contains(row.ObjectJSON, "SYNTHETIC") {
						t.Fatal("inventory value leaked")
					}
				}
			}
			if ref.UID == "" {
				t.Fatal("fixture missing", fixture.kind)
			}
			detail, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: ref})
			if err != nil || detail.Resource != ref || detail.ResourceVersion == "" {
				t.Fatal(fixture.kind, err)
			}
			if strings.Contains(detail.ObjectJSON, "SYNTHETIC") {
				t.Fatal("Secret metadata projection leaked")
			}
			if fixture.kind == "Secret" || fixture.kind == "ConfigMap" {
				pods, err := reader.ReadWorkloads(ctx, kube.PageRequest{Connection: c, Kind: "Pod", Namespace: ns})
				if err != nil || len(pods.Items) != 1 {
					t.Fatal("fixture Pod unavailable", err)
				}
				source, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: pods.Items[0].Resource})
				if err != nil {
					t.Fatal(err)
				}
				target := ref
				target.UID = ""
				resolved, err := reader.ResolveConfigurationReference(ctx, kube.ConfigurationReferenceRequest{Source: source.Resource, SourceVersion: source.ResourceVersion, Target: target})
				if err != nil || resolved.Resource != ref || resolved.IdentityEvidence != "name-resolved" {
					t.Fatal(fixture.kind, err)
				}
			}
			if fixture.kind == "Secret" {
				if _, err := reader.ReadResourceYAML(ctx, kube.DetailRequest{Resource: ref}); err == nil {
					t.Fatal("Secret YAML allowed")
				}
				if _, err := reader.ReadResourceEvents(ctx, kube.EventRequest{Resource: ref}); err == nil {
					t.Fatal("Secret Events allowed")
				}
				continue
			}
			if fixture.kind == "ConfigMap" {
				if strings.Contains(detail.ObjectJSON, "literal") {
					t.Fatal("ConfigMap Overview leaked a value")
				}
				data, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: ref, IncludeConfigMapData: true})
				if err != nil || !strings.Contains(data.ObjectJSON, "AP8=") {
					t.Fatal("binary capture missing", err)
				}
			}
			if _, err := reader.ReadResourceYAML(ctx, kube.DetailRequest{Resource: ref}); err != nil {
				t.Fatal(fixture.kind, err)
			}
			if fixture.namespace != "" {
				if _, err := reader.ReadResourceEvents(ctx, kube.EventRequest{Resource: ref}); err != nil {
					t.Fatal(fixture.kind, err)
				}
			}
		}
	}
	c := open("configuration-restricted", "a")
	if _, err := reader.ReadConfiguration(ctx, kube.PageRequest{Connection: c, Kind: "ConfigMap", Namespace: ns}); err != nil {
		t.Fatal("permitted ConfigMap read failed", err)
	}
	for _, req := range []kube.PageRequest{{Connection: c, Kind: "Namespace"}, {Connection: c, Kind: "Secret", Namespace: ns}} {
		var problem *model.Problem
		_, err := reader.ReadConfiguration(ctx, req)
		if !errors.As(err, &problem) || problem.Code != "forbidden" {
			t.Fatal("expected independent denial", req.Kind, err)
		}
	}
}
