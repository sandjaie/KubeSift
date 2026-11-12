/*
Purpose: Bound data waiting to cross the desktop bridge.
Used by: Session writers used by watches, logs and terminals.
Responsibilities: Batch output, report dropped log bytes, request watch resynchronization and apply terminal backpressure.
Important: Terminal bytes must wait or fail explicitly; they must never be silently discarded.
*/
package session

import (
	"github.com/sandjaie/KubeSift/internal/model"
)

func (w *Writer) Send(kind string, data []byte) error {
	switch kind {
	case "snapshot", "update", "data", "truncated", "state", "error":
	default:
		return model.Fail("invalid", "Unknown event kind")
	}
	if len(data) == 0 {
		return nil
	}
	if kind != "data" && len(data) > 64<<10 {
		return model.Fail("invalid", "Session event exceeds batch limit")
	}
	for len(data) > 0 {
		size := len(data)
		if size > 64<<10 {
			size = 64 << 10
		}
		if err := w.send(kind, data[:size]); err != nil {
			return err
		}
		data = data[size:]
	}
	return nil
}
func (w *Writer) send(kind string, data []byte) error {
	e := w.entry
	for {
		if e.ctx.Err() != nil {
			return model.Fail("cancelled", "Session cancelled")
		}
		e.mu.Lock()
		limit := 1 << 20
		items := 4096
		if e.kind == "terminal" {
			limit = 64 << 10
		}
		if e.kind == "watch" {
			items = 256
		}
		full := e.bytes+len(data) > limit || len(e.queue) >= items
		if full && e.kind == "logs" && kind == "data" {
			count := uint64(len(data))
			e.dropPending += count
			e.owner.dropped.Add(count)
			e.mu.Unlock()
			return nil
		}
		if full && e.kind == "watch" {
			e.queue = nil
			e.bytes = 0
			e.queue = append(e.queue, queued{"state", []byte("stale: watch overflow; resync")})
			e.bytes = len(e.queue[0].data)
			e.mu.Unlock()
			e.notify()
			return ErrResync
		}
		if !full {
			e.queue = append(e.queue, queued{kind, append([]byte(nil), data...)})
			e.bytes += len(data)
			wake := e.bytes >= 64<<10 || kind == "state" || kind == "error"
			e.mu.Unlock()
			if wake {
				e.notify()
			}
			return nil
		}
		space := e.space
		e.mu.Unlock()
		select {
		case <-space:
		case <-e.ctx.Done():
			return model.Fail("cancelled", "Session cancelled")
		}
	}
}
