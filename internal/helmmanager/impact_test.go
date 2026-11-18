/*
Purpose: Verify first-install CRD previews and destructive resource impact.
Used by: Native Helm review tests.
Responsibilities: Keep undeclared kinds fatal and retain scoped removal evidence.
Important: Synthetic discovery performs no Kubernetes writes.
*/
package helmmanager

import (
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
