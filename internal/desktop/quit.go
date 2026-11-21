/*
Purpose: Protect active native Helm operations from accidental application exit.
Used by: Application quit and last-window-close hooks.
Responsibilities: Show one warning and preserve work when the user cancels.
Important: Approval permits existing service shutdown to cancel and join owned workers.
*/
package desktop

import "sync"

type quitGuard struct {
	mu       sync.Mutex
	active   func() bool
	warn     func(confirm, cancel func())
	quit     func()
	pending  bool
	approved bool
}

func (g *quitGuard) shouldQuit() bool {
	g.mu.Lock()
	if g.approved || !g.active() {
		g.mu.Unlock()
		return true
	}
	if g.pending {
		g.mu.Unlock()
		return false
	}
	g.pending = true
	g.mu.Unlock()
	g.warn(func() {
		g.mu.Lock()
		g.approved, g.pending = true, false
		g.mu.Unlock()
		g.quit()
	}, func() {
		g.mu.Lock()
		g.pending = false
		g.mu.Unlock()
	})
	return false
}
