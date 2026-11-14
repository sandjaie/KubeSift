//go:build integration

/*
Purpose: Verify infrastructure and generic discovery readers on both approved kind clusters.
Used by: Opt-in Phase 1C.3g integration verification.
Responsibilities: Check dedicated inventory, real discovery, exact captures and active read permission.
Important: Opens loopback-only kubeconfigs; the test itself never writes.
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

func TestClusterDiscovery(t *testing.T) {
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
	const ns = "kubesift-discovery-check"
	var firstHandle string
	for _, suffix := range []string{"a", "b"} {
		t.Log("Checking explicit kind-dev-local-" + suffix)
		c := open(suffix, suffix)
		for _, target := range []struct{ kind, name, scope string }{
			{"Node", "", ""}, {"PriorityClass", "kubesift-discovery-check", ""}, {"RuntimeClass", "kubesift-discovery-check", ""}, {"Lease", "discovery-lease", ns},
			{"CustomResourceDefinition", "widgets.discovery.kubesift.dev", ""}, {"APIService", "v1.", ""},
		} {
			page, err := reader.ReadClusterResources(ctx, kube.PageRequest{Connection: c, Kind: target.kind, Namespace: target.scope})
			if err != nil {
				t.Fatal(target.kind, err)
			}
			found := false
			for _, item := range page.Items {
				if target.name != "" && item.Resource.Name != target.name {
					continue
				}
				detail, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: item.Resource})
				if err != nil || detail.CapturedYAML == nil || detail.ResourceVersion == "" {
					t.Fatal(target.kind, "capture", err)
				}
				found = true
				break
			}
			if !found {
				t.Fatal("fixture absent", target.kind, target.name)
			}
		}
		events, err := reader.ReadEventInventory(ctx, kube.PageRequest{Connection: c, Kind: "Event", Namespace: ns})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, event := range events.Items {
			if event.Resource.Name != "discovery-event" {
				continue
			}
			detail, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: event.Resource})
			if err != nil || detail.EventTarget == nil {
				t.Fatal("event capture", err)
			}
			resolved, err := reader.ResolveEventTarget(ctx, kube.EventTargetRequest{Source: event.Resource, SourceVersion: detail.ResourceVersion})
			if err != nil || resolved.Detail.Resource != *detail.EventTarget {
				t.Fatal("event target", err)
			}
			found = true
		}
		if !found {
			t.Fatal("Event missing")
		}
		root, err := reader.ReadAPIGroups(ctx, c, false)
		if err != nil || root.CoreProblem != nil || root.APIsProblem != nil {
			t.Fatalf("root discovery: core=%+v apis=%+v err=%v", root.CoreProblem, root.APIsProblem, err)
		}
		descriptor := func(ref model.ConnectionRef, group, resource string) kube.APIResourceDescriptor {
			t.Helper()
			page, err := reader.ReadAPIResources(ctx, ref, group, "v1", false)
			if err != nil {
				t.Fatal(group, err)
			}
			for _, item := range page.Resources {
				if item.Resource == resource {
					return item
				}
			}
			t.Fatal("descriptor missing", group, resource)
			return kube.APIResourceDescriptor{}
		}
		if secret := descriptor(c, "", "secrets"); secret.Mode != "dedicated" || secret.Handle != "" {
			t.Fatal("Secret generic route advertised")
		}
		for _, target := range []struct{ group, resource, name, scope string }{
			{"discovery.kubesift.dev", "widgets", "sample", ns}, {"other-discovery.kubesift.dev", "widgets", "sample", ""}, {"", "endpoints", "discovery-endpoints", ns},
		} {
			d := descriptor(c, target.group, target.resource)
			if d.Mode != "generic" || d.Namespaced != (target.scope != "") {
				t.Fatal("incorrect generic mode or scope", d.Resource)
			}
			page, err := reader.ReadGenericResources(ctx, kube.GenericPageRequest{Connection: c, Handle: d.Handle, Namespace: target.scope})
			if err != nil {
				t.Fatal(target.resource, err)
			}
			found := false
			for _, item := range page.Items {
				if item.Resource.Name != target.name {
					continue
				}
				detail, err := reader.ReadGenericDetail(ctx, kube.GenericDetailRequest{Connection: c, Handle: d.Handle, Name: item.Resource.Name, Namespace: target.scope, UID: item.Resource.UID})
				if err != nil || detail.Resource != item.Resource {
					t.Fatal("generic capture", err)
				}
				if target.resource == "widgets" && !strings.Contains(detail.YAML, "9007199254740993") {
					t.Fatal("captured YAML rounded a large number")
				}
				found = true
			}
			if !found {
				t.Fatal("generic object absent", target.group, target.resource)
			}
		}
		d := descriptor(c, "discovery.kubesift.dev", "widgets")
		if suffix == "a" {
			firstHandle = d.Handle
		} else if _, err := reader.ReadGenericResources(ctx, kube.GenericPageRequest{Connection: c, Handle: firstHandle, Namespace: ns}); err == nil {
			t.Fatal("cross-connection handle")
		}
		empty, err := reader.ReadGenericResources(ctx, kube.GenericPageRequest{Connection: c, Handle: d.Handle, Namespace: "default"})
		if err != nil || len(empty.Items) != 0 {
			t.Fatal("namespace isolation", err)
		}
	}
	restricted := open("discovery-restricted", "a")
	if _, err := reader.ReadAPIGroups(ctx, restricted, false); err != nil {
		t.Fatal(err)
	}
	catalog, err := reader.ReadAPIResources(ctx, restricted, "discovery.kubesift.dev", "v1", false)
	if err != nil || len(catalog.Resources) != 1 {
		t.Fatal("restricted discovery", err)
	}
	handle := catalog.Resources[0].Handle
	assertDenied := func(err error) {
		t.Helper()
		var p *model.Problem
		if !errors.As(err, &p) || p.Code != "forbidden" {
			t.Fatalf("expected RBAC read denial, got %v", err)
		}
	}
	_, err = reader.ReadGenericResources(ctx, kube.GenericPageRequest{Connection: restricted, Handle: handle, Namespace: ns})
	assertDenied(err)
	_, err = reader.ReadGenericDetail(ctx, kube.GenericDetailRequest{Connection: restricted, Handle: handle, Namespace: ns, Name: "sample", UID: "not-authorized"})
	assertDenied(err)
}
