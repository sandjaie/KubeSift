/*
Purpose: Verify that release sections show recorded data without rendering.
Used by: Helm reader unit tests.
Responsibilities: Cover stored and computed values, exact manifests, and inert notes.
Important: Fixtures contain no live release data.
*/
package helmreader

import (
	"context"
	"strings"
	"testing"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	stored "helm.sh/helm/v4/pkg/release/v1"
)

func TestReleaseSections(t *testing.T) {
	release := &stored.Release{
		Chart:    &chart.Chart{Metadata: &chart.Metadata{Name: "example"}, Values: map[string]any{"default": "stored"}},
		Config:   map[string]any{"enabled": true},
		Manifest: "---\nkind: ConfigMap\n---\nkind: Secret\n",
		Info:     &stored.Info{Notes: "<script>example</script>"},
	}
	for _, tc := range []struct {
		section, want string
		computed      bool
	}{
		{"Values", "enabled: true", false}, {"Values", "default: stored", true},
		{"Manifest", release.Manifest, false}, {"Notes", release.Info.Notes, false},
	} {
		out, err := projectSection(context.Background(), nil, DetailRequest{Section: tc.section, ComputedValues: tc.computed}, release)
		if err != nil || !strings.Contains(out.Text, tc.want) || (!tc.computed && strings.Contains(out.Text, "default: stored")) {
			t.Fatalf("%s: unexpected section %q (%v)", tc.section, out.Text, err)
		}
	}
	release.Chart.Values = map[string]any{"enabled": map[string]any{"token": "sensitive"}}
	release.Config = map[string]any{"enabled": map[string]any{"token": map[string]any{"child": true}}}
	if _, err := computedValues(release); err == nil {
		t.Fatal("a conflict that could put values into SDK logs was accepted")
	}
}

func TestManifestIdentitiesDoNotInventScope(t *testing.T) {
	rows, truncated := manifestResources(context.Background(), nil, RecordRef{Scope: Scope{Namespace: "release"}},
		"apiVersion: v1\nkind: ConfigMap\nmetadata: {name: named}\n---\napiVersion: v1\nkind: Namespace\nmetadata: {generateName: prefix-}\n")
	if truncated || len(rows) != 2 || rows[0].Name != "named" || rows[0].Resolvable || rows[1].Name != "" || rows[1].Issue == "" {
		t.Fatalf("unexpected stored identities: %+v", rows)
	}
}
