/*
Purpose: Verify creation uses the existing desktop request and token boundaries.
Used by: Desktop operation tests.
Responsibilities: Reject duplicate read IDs and unissued confirmation tokens.
Important: No test requires cluster access.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/operation"
	"testing"
)

func TestCreateBridgeChecksIDsAndTokens(t *testing.T) {
	s := NewService()
	t.Cleanup(func() { _ = s.ServiceShutdown() })
	e := newExplorerService(s)
	_, done, err := e.beginWorkloadRead("reserved")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if _, err := e.OpenCreate("reserved", model.ConnectionRef{}, "default"); err == nil {
		t.Fatal("duplicate request ID accepted")
	}
	if _, err := e.PrepareCreate("reserved", operation.CreateRequest{}); err == nil {
		t.Fatal("duplicate request ID accepted")
	}
	if _, err := e.ExecuteCreate("nonexistent"); err == nil || len(e.ListOperationResults()) != 0 {
		t.Fatal("unissued confirmation accepted")
	}
	if _, err := e.InspectCreateName("fresh", "missing"); err == nil {
		t.Fatal("unretained result accepted")
	}
}
