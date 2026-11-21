/*
Purpose: Verify lossless YAML editing and conditional patch generation.
Used by: Native operation tests.
Responsibilities: Lock patch identity, omitted fields, and Secret normalization.
Important: Fixtures are synthetic and never sent to a cluster.
*/
package operation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sandjaie/KubeSift/internal/kube"
)

func TestBuildEditPatch(t *testing.T) {
	base := `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"sample","namespace":"demo","uid":"uid","resourceVersion":"7","annotations":{"old":"value"},"managedFields":[{}]},"data":{"key":"b2xk"},"extension":{"n":9007199254740993,"array":[1,2]},"status":{"ready":true}}`
	capture := kube.EditCapture{Target: kube.EditTarget{Version: "v1", Resource: "secrets"}, ResourceVersion: "7", ObjectJSON: json.RawMessage(base)}
	capture.Target.UID = "uid"
	projected := strings.ReplaceAll(strings.ReplaceAll(base, `,"managedFields":[{}]`, ""), `,"status":{"ready":true}`, "")
	for _, tc := range []struct {
		name, draft  string
		want, absent []string
		fail         bool
	}{
		{"changes", strings.ReplaceAll(strings.ReplaceAll(projected, `"old":"value"`, `"example.test/a~b":"value"`), `[1,2]`, `[2,1]`),
			[]string{`/metadata/annotations/example.test~1a~0b`, `"remove","path":"/metadata/annotations/old"`, `"value":[2,1]`}, []string{"managedFields", "/status", "9007199254740993"}, false},
		{"precise number", strings.ReplaceAll(projected, "9007199254740993", "184467440737095516160"), []string{"184467440737095516160"}, nil, false},
		{"secret stringData", strings.ReplaceAll(projected, `"data":`, `"stringData":{"key":"new"},"data":`), []string{"bmV3"}, []string{"stringData"}, false},
		{"no changes", projected, nil, nil, true},
		{"duplicate", projected[:len(projected)-1] + `,"kind":"Secret"}`, nil, nil, true},
		{"renamed", strings.ReplaceAll(projected, "sample", "different"), nil, nil, true},
		{"invalid", "metadata: [", nil, nil, true},
		{"multiple", projected + "\n---\n{}", nil, nil, true},
		{"alias", "a: &owned 1\nb: *owned", nil, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildEditPatch(capture, tc.draft)
			if tc.fail {
				if err == nil {
					t.Fatal("an invalid or empty edit produced a patch")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			patch := string(got.body)
			if !strings.HasPrefix(patch, `[{"op":"test","path":"/metadata/uid","value":"uid"},{"op":"test","path":"/metadata/resourceVersion","value":"7"}`) {
				t.Fatal("missing identity tests")
			}
			for _, want := range tc.want {
				if !strings.Contains(patch, want) {
					t.Errorf("missing %s in %s", want, patch)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(patch, absent) {
					t.Errorf("unexpected %s in %s", absent, patch)
				}
			}
		})
	}
}

func TestEditRoundTripsNumbersOutsideFloatRange(t *testing.T) {
	raw := json.RawMessage(`{"apiVersion":"example.test/v1","kind":"Widget","metadata":{"name":"sample","uid":"uid","resourceVersion":"7"},"spec":{"big":1e400,"text":"1e400","marker":"before"}}`)
	capture := kube.EditCapture{Target: kube.EditTarget{Group: "example.test", Version: "v1", Resource: "widgets", UID: "uid"}, ResourceVersion: "7", ObjectJSON: raw}
	object, err := editProjection(raw)
	if err != nil {
		t.Fatal(err)
	}
	editor, err := renderEdit(object)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := buildEditPatch(capture, strings.ReplaceAll(editor, `"before"`, `"after"`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(patch.body), "/spec/big") || strings.Contains(string(patch.body), "/spec/text") {
		t.Fatalf("unrelated change altered numeric or string values: %s", patch.body)
	}
}
