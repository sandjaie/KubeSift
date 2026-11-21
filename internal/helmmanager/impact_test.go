/*
Purpose: Verify first-install CRD previews and destructive resource impact.
Used by: Native Helm review tests.
Responsibilities: Keep undeclared kinds fatal and retain scoped removal evidence.
Important: Synthetic discovery performs no Kubernetes writes.
*/
package helmmanager

import (
	"helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	"io"
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/kube/fake"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type recordingBuilder struct {
	fake.PrintingKubeClient
	text string
}

func (b *recordingBuilder) Build(reader io.Reader, _ bool) (kube.ResourceList, error) {
	raw, err := io.ReadAll(reader)
	b.text = string(raw)
	return nil, err
}

func TestPreviewCRDAndResourceImpact(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	custom := schema.GroupVersionKind{Group: "example.test", Version: "v1", Kind: "Example"}
	builder := &recordingBuilder{}
	preview := &crdPreviewClient{Interface: builder, mapper: mapper, declared: map[schema.GroupVersionKind]bool{custom: true}}
	manifest := "apiVersion: example.test/v1\nkind: Example\nmetadata:\n  name: first\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: retained\n"
	if _, err := preview.Build(strings.NewReader(manifest), true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(builder.text, "kind: Example") || !strings.Contains(builder.text, "kind: ConfigMap") || !preview.deferred[custom] {
		t.Fatal("new CRD dependent was not explicitly deferred while known resources stayed validated")
	}
	if _, err := preview.Build(strings.NewReader(strings.ReplaceAll(manifest, "kind: Example", "kind: Undeclared")), true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(builder.text, "Undeclared") {
		t.Fatal("unrelated unknown kind escaped normal SDK validation")
	}
	r := &reviewData{Review: Review{Action: "upgrade", Target: Target{Namespace: "owned"}}, mapper: mapper, sections: map[string]string{
		"manifest":        manifest,
		"currentManifest": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: removed\n---\napiVersion: v1\nkind: Namespace\nmetadata:\n  name: cluster-wide\n",
	}, offsets: map[string]map[int]bool{}, declared: map[schema.GroupVersionKind]bool{custom: true}}
	if err := reviewContent(r); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"removed", "effect: removed", "namespace: owned", "scope: cluster", "name: first"} {
		if !strings.Contains(r.sections["resources"], text) {
			t.Fatalf("missing %q in impact: %s", text, r.sections["resources"])
		}
	}
}

func TestDisabledDependencyCannotSupplyPreviewCRD(t *testing.T) {
	ch := &chartv2.Chart{Metadata: &chartv2.Metadata{Name: "parent", Version: "1.0.0", APIVersion: "v2", Dependencies: []*chartv2.Dependency{{Name: "provider", Version: "1.0.0", Condition: "provider.enabled"}}}, Values: map[string]any{"provider": map[string]any{"enabled": false}}}
	ch.AddDependency(&chartv2.Chart{Metadata: &chartv2.Metadata{Name: "provider", Version: "1.0.0", APIVersion: "v2"}, Files: []*common.File{{Name: "crds/example.yaml", Data: []byte("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nspec:\n  group: example.test\n  scope: Namespaced\n  names:\n    kind: Example\n  versions:\n  - name: v1\n    served: true\n")}}})
	declared, err := declaredCRDs(ch)
	if err != nil || len(declared) != 1 {
		t.Fatal("fixture missing CRD", err)
	}
	builder := &recordingBuilder{}
	preview := &crdPreviewClient{Interface: builder, chart: ch, mapper: meta.NewDefaultRESTMapper(nil), declared: declared}
	if err := chartutil.ProcessDependencies(ch, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err := preview.Build(strings.NewReader("apiVersion: example.test/v1\nkind: Example\nmetadata:\n  name: parent-resource\n"), true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(builder.text, "kind: Example") || len(preview.deferred) > 0 || len(preview.declared) > 0 {
		t.Fatal("disabled dependency improperly deferred unknown kind")
	}
}

func TestCRDPreviewPreservesKubernetesScalars(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	custom := schema.GroupVersionKind{Group: "example.test", Version: "v1", Kind: "Example"}
	original := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: 2026-11-18\n"
	for _, withCRD := range []bool{false, true} {
		builder := &recordingBuilder{}
		preview := &crdPreviewClient{Interface: builder, mapper: mapper}
		manifest := original
		if withCRD {
			preview.declared = map[schema.GroupVersionKind]bool{custom: true}
			manifest += "---\napiVersion: example.test/v1\nkind: Example\nmetadata:\n  name: deferred\n"
		}
		if _, err := preview.Build(strings.NewReader(manifest), true); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(builder.text, original) {
			t.Fatalf("retained identity changed: %q", builder.text)
		}
	}
	builder := &recordingBuilder{}
	preview := &crdPreviewClient{Interface: builder, mapper: mapper, declared: map[schema.GroupVersionKind]bool{custom: true}}
	manifest := "apiVersion: v1\nkind: List\nitems:\n- apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: 2026-11-18\n  number: 9007199254740993\n- apiVersion: example.test/v1\n  kind: Example\n  metadata:\n    name: deferred\n"
	if _, err := preview.Build(strings.NewReader(manifest), true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(builder.text, "T00:") || !strings.Contains(builder.text, "9007199254740993") || strings.Contains(builder.text, "deferred") {
		t.Fatalf("retained List semantics changed: %s", builder.text)
	}
}
