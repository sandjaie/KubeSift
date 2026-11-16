/*
Purpose: Verify lossless, template-only workload rollback.
Used by: Native rollback tests.
Responsibilities: Lock supported source encodings, no-op detection and conditional paths.
Important: ControllerRevision shapes follow the pinned Kubernetes 1.37 controller getPatch sources.
*/
package operation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/sandjaie/KubeSift/internal/kube"
)

func TestRollbackTemplate(t *testing.T) {
	const template = `{"metadata":{"labels":{"app":"demo"},"annotations":{"kept":"yes"}},"spec":{"containers":[{"name":"app","image":"demo:old"}],"extension":9007199254740993}}`
	for _, tc := range []struct {
		parent, source string
		bad            bool
	}{
		{"Deployment", `{"spec":{"replicas":5,"template":` + strings.ReplaceAll(template, `"app":"demo"`, `"app":"demo","pod-template-hash":"owned"`) + `}}`, false},
		{"StatefulSet", `{"data":{"spec":{"template":` + `{"$patch":"replace",` + template[1:] + `}}}`, false},
		{"DaemonSet", `{"data":{"spec":{"template":` + `{"$patch":"replace",` + template[1:] + `}}}`, false},
		{"StatefulSet", `{"data":{"spec":{"template":` + template + `}}}`, true},
		{"DaemonSet", `{"data":{"spec":{"template":{"$patch":"delete"}}}}`, true},
		{"StatefulSet", `{"data":{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"$patch":"delete","name":"app"}]}}}}}`, true},
	} {
		t.Run(tc.parent+"/"+fmt.Sprint(tc.bad), func(t *testing.T) {
			got, err := rollbackTemplate(kube.ResourceRef{Kind: tc.parent}, json.RawMessage(tc.source))
			if tc.bad {
				if err == nil {
					t.Fatal("unrecognized history was accepted")
				}
				return
			}
			if err != nil || strings.Contains(string(got), "$patch") || strings.Contains(string(got), "pod-template-hash") ||
				!strings.Contains(string(got), "9007199254740993") || !strings.Contains(string(got), `"kept":"yes"`) {
				t.Fatalf("template: %s %v", got, err)
			}
			parent := func(template string) json.RawMessage {
				return json.RawMessage(`{"metadata":{"uid":"parent","resourceVersion":"31"},"spec":{"replicas":7,"updateStrategy":{"type":"OnDelete"},"volumeClaimTemplates":[{}],"template":` + template + `}}`)
			}
			patch, err := rollbackPatch(parent(strings.ReplaceAll(template, "demo:old", "demo:new")), got)
			if err != nil {
				t.Fatal(err)
			}
			var operations []patchOperation
			if err := json.Unmarshal(patch, &operations); err != nil || len(operations) != 3 ||
				operations[0].Op != "test" || operations[0].Path != "/metadata/uid" || operations[0].Value != "parent" ||
				operations[1].Op != "test" || operations[1].Path != "/metadata/resourceVersion" || operations[1].Value != "31" ||
				operations[2].Op != "replace" || operations[2].Path != "/spec/template" || !strings.Contains(string(patch), "demo:old") {
				t.Fatalf("unexpected conditional patch: %s", patch)
			}
			_, err = rollbackPatch(parent(template), got)
			checkCode(t, err, "no_changes")
		})
	}
}
