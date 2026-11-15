/*
Purpose: Verify restart enters through the cancellable desktop bridge.
Used by: Desktop regression tests.
Responsibilities: Check request ownership and safe mixed-result reads.
Important: No cluster or credentials are used.
*/
package desktop

import (
	"testing"

	"github.com/sandjaie/KubeSift/internal/operation"
)

func TestRestartBridgeRejectsUnissuedTokenAndSharedRequestID(t *testing.T) {
	core := NewService()
	t.Cleanup(func() { _ = core.ServiceShutdown() })
	explorer := newExplorerService(core)
	_, done, err := explorer.beginWorkloadRead("reserved")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if _, err := explorer.PrepareRestart("reserved", operation.RestartRequest{}); err == nil {
		t.Fatal("accepted a duplicate request ID")
	}
	if _, err := explorer.ExecuteRestart("unissued", ""); err == nil {
		t.Fatal("accepted an unissued token")
	}
	if len(explorer.ListOperationResults()) != 0 {
		t.Fatal("invalid execution created a result")
	}
	if _, err := explorer.GetOperationResult("unknown"); err == nil {
		t.Fatal("invalid result ID succeeded")
	}
}
