/*
Purpose: Protect independent cancellation of workload bridge requests.
Used by: Desktop adapter tests.
Responsibilities: Verify cancelled reads release capacity without cancelling another request.
Important: No native window or cluster is needed.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/kube"
	"testing"
)

func TestWorkloadReadCancellation(t *testing.T) {
	s := &ExplorerService{}
	a, doneA, err := s.beginWorkloadRead("a")
	if err != nil {
		t.Fatal(err)
	}
	b, doneB, err := s.beginWorkloadRead("b")
	if err != nil {
		t.Fatal(err)
	}
	defer doneB()
	if err = s.CancelWorkloadRead("a"); err != nil {
		t.Fatal(err)
	}
	if a.Err() == nil || b.Err() != nil {
		t.Fatal("cancellation crossed request ownership")
	}
	doneA()
	_, doneAgain, err := s.beginWorkloadRead("a")
	if err != nil {
		t.Fatal("slot not released", err)
	}
	defer doneAgain()
	doneA()
	if len(s.reads) != 2 {
		t.Fatal("old completion removed a different request")
	}
}

func TestHistoryReservationCancellation(t *testing.T) {
	s := &ExplorerService{}
	id, err := s.PrepareHistoricalMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CancelWorkloadRead(id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadHistoricalMetrics(id, kube.HistoryRequest{}); err == nil {
		t.Fatal("read started after cancellation")
	}
}
