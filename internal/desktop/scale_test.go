/*
Purpose: Verify reviewed scaling enters through the cancellable desktop bridge.
Used by: Desktop safety regression tests.
Responsibilities: Check typed count input, request slot ownership and invalid tokens.
Important: No cluster or credentials are used.
*/
package desktop

import (
	"encoding/json"
	"testing"

	"github.com/sandjaie/KubeSift/internal/operation"
)

func TestScaleBridgeSharesReadSlots(t *testing.T) {
	core := NewService()
	t.Cleanup(func() { _ = core.ServiceShutdown() })
	explorer := newExplorerService(core)
	_, done, err := explorer.beginWorkloadRead("owned")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if _, err := explorer.PrepareScale("owned", operation.ScaleRequest{}); err == nil {
		t.Fatal("scaling used a duplicate request ID")
	}
	var request operation.ScaleRequest
	if err := json.Unmarshal([]byte(`{"replicasText":1e2}`), &request); err == nil {
		t.Fatal("numeric count accepted over the bridge")
	}
	if _, err := explorer.ExecuteScale("unissued", ""); err == nil {
		t.Fatal("unissued review accepted")
	}
	if rows := explorer.ListScaleResults(); len(rows) != 0 {
		t.Fatal("invalid execution created a result")
	}
}
