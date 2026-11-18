/*
Purpose: Verify native shutdown requires one informed decision for active Helm work.
Used by: Desktop lifecycle tests.
Responsibilities: Preserve work on dismissal and authorize one confirmed quit.
Important: Dialog callbacks are synthetic; packaged checks cover native window events.
*/
package desktop

import "testing"

func TestQuitGuard(t *testing.T) {
	active := true
	warnings, quits := 0, 0
	var confirm, cancel func()
	guard := &quitGuard{
		active: func() bool { return active },
		warn:   func(yes, no func()) { warnings++; confirm, cancel = yes, no },
		quit:   func() { quits++ },
	}
	if guard.shouldQuit() || guard.shouldQuit() || warnings != 1 {
		t.Fatal("active work quit silently or received duplicate warnings")
	}
	cancel()
	if quits != 0 || guard.shouldQuit() || warnings != 2 {
		t.Fatal("dismissal did not preserve work and allow a later warning")
	}
	confirm()
	if quits != 1 || !guard.shouldQuit() {
		t.Fatal("confirmed quit asked twice or did not proceed")
	}
	active = false
	guard.approved = false
	if !guard.shouldQuit() || warnings != 2 {
		t.Fatal("idle quit asked unnecessarily")
	}
}
