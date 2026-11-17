//go:build integration

/*
Purpose: Verify actual Helm 3 and 4 records on the two approved kind contexts.
Used by: The opt-in Helm inspection integration check.
Responsibilities: Check retained revisions, section evidence, driver isolation, live links and denied reads.
Important: This test only reads task-owned fixtures created separately.
*/
package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/helmreader"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/session"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHelmInspection(t *testing.T) {
	if os.Getenv("KUBESIFT_HELM_INTEGRATION") != "1" {
		t.Skip("explicit Helm inspection opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			manager := connection.NewManager(nil)
			t.Cleanup(manager.CloseAll)
			ref, client := openLocalOperationFixture(t, context.Background(), manager, dir, suffix, "")
			ns := os.Getenv("KUBESIFT_HELM_NAMESPACE_" + strings.ToUpper(suffix))
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			owned, err := client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
			if err != nil || owned.Labels["kubesift.dev/fixture"] != "phase1e1" {
				t.Fatalf("expected an owned fixture namespace: %v", err)
			}
			sessions := session.NewManager(nil)
			t.Cleanup(sessions.Close)
			service := helmreader.NewService(manager, kube.NewService(manager, sessions))
			var previousID string
			for _, driver := range []string{"secret", "configmap"} {
				scope := helmreader.Scope{Connection: ref, Namespace: ns, Driver: driver, ReleaseName: "phase1e1"}
				page, err := service.ListReleases(ctx, helmreader.ListRequest{Scope: scope})
				if err != nil || len(page.Items) != 1 || page.Items[0].Problem != nil || page.Items[0].Record.Revision != "2" || page.Items[0].Record.Driver != driver {
					t.Fatalf("expected one latest release in %s storage: %+v (%v)", driver, page.Items, err)
				}
				if previousID != "" && page.Items[0].Record.UID == previousID {
					t.Fatal("same-name releases must remain isolated by driver")
				}
				previousID = page.Items[0].Record.UID
				history, err := service.ReadHistory(ctx, helmreader.HistoryRequest{Record: page.Items[0].Record})
				if err != nil || len(history.Items) != 2 || history.Items[1].Record.Revision != "1" || history.Items[1].Problem != nil {
					t.Fatalf("expected retained revision one: %+v (%v)", history.Items, err)
				}
				old := history.Items[1].Record
				for _, tc := range []struct {
					section, value string
					computed       bool
				}{
					{"Values", "marker: one", false}, {"Values", "fixtureDefault: from-chart", true},
					{"Manifest", "marker: \"one\"", false}, {"Hooks", "fixture-hook", false}, {"Notes", "revision 1", false},
				} {
					out, err := service.ReadSection(ctx, helmreader.DetailRequest{Record: old, Section: tc.section, ComputedValues: tc.computed})
					if err != nil || !strings.Contains(out.Text, tc.value) || out.Record != old {
						t.Fatalf("revision-one %s not available: %v preview contains expected = %t", tc.section, err, strings.Contains(out.Text, tc.value))
					}
				}
				resources, err := service.ReadSection(ctx, helmreader.DetailRequest{Record: old, Section: "Resources"})
				if err != nil || len(resources.Resources) != 1 || resources.Resources[0].Namespace != ns || !resources.Resources[0].Resolvable {
					t.Fatalf("unexpected manifest resource identities: %+v (%v)", resources.Resources, err)
				}
				current, err := service.OpenResource(ctx, helmreader.OpenResourceRequest{Record: old, Resource: resources.Resources[0]})
				if err != nil || current.Resource.UID == "" || current.Resource.Name != "phase1e1-"+driver {
					t.Fatalf("could not resolve current resource: %+v (%v)", current, err)
				}
				mapData, err := client.CoreV1().ConfigMaps(ns).Get(ctx, current.Resource.Name, metav1.GetOptions{})
				if err != nil || mapData.Data["marker"] != "two" || string(mapData.UID) != current.Resource.UID {
					t.Fatalf("resource link did not resolve the current version: %v", err)
				}
				if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, current.Resource.Name+"-hook", metav1.GetOptions{}); err == nil {
					t.Fatal("fixture hook was executed")
				}
			}
			empty, err := service.ListReleases(ctx, helmreader.ListRequest{Scope: helmreader.Scope{
				Connection: ref, Namespace: ns, Driver: "configmap", ReleaseName: "kubesift-no-such-fixture",
			}})
			if err != nil || empty.Total != 0 || empty.SnapshotID == "" {
				t.Fatalf("an empty exact-name filter should produce a complete empty snapshot: %v", err)
			}
			denied, _ := openLocalOperationFixture(t, context.Background(), manager, dir, suffix, "-helm-denied")
			_, err = service.ListReleases(ctx, helmreader.ListRequest{Scope: helmreader.Scope{Connection: denied, Namespace: ns, Driver: "secret"}})
			restartErrorCode(t, err, "forbidden")
		})
	}
}
