/*
Purpose: Verify the native deletion methods use the established desktop boundary.
Used by: Desktop regression tests.
Responsibilities: Reject duplicate request IDs and unissued operation inputs.
Important: No live cluster requests are made.
*/
package desktop

import (
	"testing"

	"github.com/sandjaie/KubeSift/internal/operation"
)

func TestDeleteBridgeUsesReadAdmissionAndRetainedResults(t *testing.T) {
	core := NewService()
	t.Cleanup(func() { _ = core.ServiceShutdown() })
	explorer := newExplorerService(core)
	_, done, err := explorer.beginWorkloadRead("reserved")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if _, err := explorer.PrepareDelete("reserved", operation.DeleteRequest{}); err == nil {
		t.Fatal("accepted duplicate read request ID")
	}
	if _, err := explorer.CheckDeleteTarget("check", "unissued"); err == nil {
		t.Fatal("checked an unissued operation")
	}
	if _, err := explorer.ExecuteDelete("unissued", "web"); err == nil || len(explorer.ListOperationResults()) != 0 {
		t.Fatal("unissued review created a result")
	}
}
