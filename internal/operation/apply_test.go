/*
Purpose: Verify that apply preserves declared intent and requires prior conflict evidence.
Used by: Native apply service checks.
Responsibilities: Cover lossless YAML, field ownership summaries and takeover authorization.
Important: These tests use synthetic data only.
*/
package operation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/kube"
)

func TestApplyPayload(t *testing.T) {
	capture := kube.EditCapture{Target: kube.EditTarget{Version: "v1", Resource: "secrets", Kind: "Secret", Name: "sample", Namespace: "sample-ns", UID: "sample-uid"}, ResourceVersion: "20"}
	input := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: sample\n  labels: null\nstringData:\n  marker: synthetic\ndata:\n  marker: d3Jvbmc=\ncustom: [1234567890123456789, null]\n"
	desired, raw, err := buildApplyPayload(capture, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"labels":null`, `"custom":[1234567890123456789,null]`, `"uid":"sample-uid"`, `"resourceVersion":"20"`, `"marker":"c3ludGhldGlj"`} {
		if !strings.Contains(string(raw), expected) {
			t.Errorf("missing %s in %s", expected, raw)
		}
	}
	for _, excluded := range []string{"stringData", "resourceVersion", "sample-uid"} {
		if strings.Contains(desired, excluded) {
			t.Errorf("unexpected %s in desired YAML", excluded)
		}
	}
	for _, altered := range []string{
		strings.Replace(input, "name: sample", "name: other", 1),
		strings.Replace(input, "labels: null", "managedFields: []", 1),
		strings.Replace(input, "labels: null", "resourceVersion: '21'", 1),
		input + "status: {}\n",
	} {
		if _, _, err := buildApplyPayload(capture, altered); err == nil {
			t.Errorf("invalid manifest passed")
		}
	}
}

func TestApplyEvidence(t *testing.T) {
	const manager = `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Conflict","code":409,"details":{"causes":[{"reason":"FieldManagerConflict","field":".data.marker","message":"conflict with \"setup\" using v1"}]}}`
	var status operationStatus
	if err := json.Unmarshal([]byte(manager), &status); err != nil {
		t.Fatal(err)
	}
	got := applyConflicts(status)
	if len(got) != 1 || got[0] != (ApplyConflict{Field: ".data.marker", Manager: "setup"}) {
		t.Fatalf("structured conflict not surfaced: %+v", got)
	}
	status.Details.Causes[0].Reason = "FieldValueInvalid"
	if got := applyConflicts(status); got != nil {
		t.Fatalf("unrelated conflict offered takeover: %+v", got)
	}
	const before = `{"metadata":{"managedFields":[{"apiVersion":"v1","manager":"kubesift-apply","operation":"Apply","fieldsType":"FieldsV1","fieldsV1":{"f:data":{"f:old":{}}}},{"apiVersion":"v1","manager":"other","operation":"Update","fieldsType":"FieldsV1","fieldsV1":{"f:data":{"f:common":{}}}}]}}`
	const after = `{"metadata":{"managedFields":[{"apiVersion":"v1","manager":"kubesift-apply","operation":"Apply","fieldsType":"FieldsV1","fieldsV1":{"f:data":{"f:new":{},"f:common":{}}}},{"apiVersion":"v1","manager":"other","operation":"Update","fieldsType":"FieldsV1","fieldsV1":{"f:data":{"f:common":{}}}}]}}`
	changes, incomplete, truncated := applyOwnership([]byte(before), []byte(after), "v1")
	if incomplete || truncated || len(changes) != 3 ||
		changes[0] != (ApplyOwnershipChange{Field: ".data.new", Change: "Acquired"}) ||
		changes[1] != (ApplyOwnershipChange{Field: ".data.old", Change: "Released"}) ||
		changes[2] != (ApplyOwnershipChange{Field: ".data.common", Change: "Shared"}) {
		t.Fatalf("unexpected ownership evidence: %+v (%t, %t)", changes, incomplete, truncated)
	}
}

func TestApplyTakeoverEvidenceCannotBeReused(t *testing.T) {
	c := NewCoordinator(nil, time.Now)
	defer c.Close()
	c.captures["test"] = operationDraft{action: "apply", expires: time.Now().Add(time.Minute)}
	s := NewApplyService(nil, nil, c)
	ctx := context.Background()
	req := ApplyRequest{DraftID: "test", YAML: "changed draft", TakeOwnership: true}
	digest := sha256.Sum256([]byte("original draft"))
	if _, err := c.rememberApplyConflict(ctx, "test", 0, digest); err != nil {
		t.Fatal(err)
	}
	_, err := s.PrepareApply(ctx, req)
	checkCode(t, err, "conflict_review_required")
	req.YAML = "original draft"
	_, err = s.PrepareApply(ctx, req)
	checkCode(t, err, "conflict_review_required")
	if _, err := c.rememberApplyConflict(ctx, "test", 0, digest); err == nil {
		t.Fatal("late conflict should not replace current revision evidence")
	}
}
