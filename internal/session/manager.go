/*
Purpose: Own cancellable background sessions for explicit targets.
Used by: Kubernetes stream adapters and the desktop bridge.
Responsibilities: Start and activate workers, deliver bounded events, wait for cleanup and track terminal acknowledgements.
Important: Closing a connection must stop its workers; removing a frontend listener is not backend cancellation.
*/
// Package session owns bounded, cancellable workers tied to immutable targets.
package session

import (
	"context"
	"errors"
	"github.com/sandjaie/KubeSift/internal/model"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var ErrResync = errors.New("watch overflow requires resync")

type Stats struct {
	Sessions        int    `json:"sessions"`
	QueuedBytes     int    `json:"queuedBytes"`
	InFlightBytes   int    `json:"inFlightBytes"`
	DroppedLogBytes uint64 `json:"droppedLogBytes"`
}
type Manager struct {
	mu       sync.Mutex
	entries  map[string]*entry
	closed   bool
	emit     func(model.Event)
	validate func(model.Target) error
	dropped  atomic.Uint64
}
type entry struct {
	owner                 *Manager
	ref                   model.SessionRef
	target                model.Target
	kind                  string
	ctx                   context.Context
	cancel                context.CancelFunc
	mu                    sync.Mutex
	queue                 []queued
	bytes, inFlight       int
	dropPending           uint64
	sequence, expectedAck uint64
	wake, space           chan struct{}
	ack                   chan uint64
	start                 chan struct{}
	activated             bool
	workDone, done        chan struct{}
}
type queued struct {
	kind string
	data []byte
}
type Writer struct{ entry *entry }

func NewManager(emit func(model.Event)) *Manager {
	if emit == nil {
		emit = func(model.Event) {}
	}
	return &Manager{entries: map[string]*entry{}, emit: emit}
}
func (m *Manager) SetValidate(fn func(model.Target) error) {
	m.mu.Lock()
	m.validate = fn
	m.mu.Unlock()
}
func (m *Manager) Start(parent context.Context, target model.Target, kind string, run func(context.Context, *Writer) error) (model.SessionRef, error) {
	return m.start(parent, target, kind, run, false)
}

// StartPending lets the bridge register a returned identity before events begin.
func (m *Manager) StartPending(parent context.Context, target model.Target, kind string, run func(context.Context, *Writer) error) (model.SessionRef, error) {
	return m.start(parent, target, kind, run, true)
}
func (m *Manager) start(parent context.Context, target model.Target, kind string, run func(context.Context, *Writer) error, pending bool) (model.SessionRef, error) {
	if parent == nil || parent.Err() != nil || run == nil {
		return model.SessionRef{}, model.Fail("cancelled", "Session start cancelled")
	}
	if target.Connection.ID == "" || target.Connection.Generation == 0 || target.Namespace == "" {
		return model.SessionRef{}, model.Fail("invalid", "Explicit session target required")
	}
	if kind != "logs" && kind != "watch" && kind != "terminal" && kind != "forward" {
		return model.SessionRef{}, model.Fail("invalid", "Unknown session kind")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return model.SessionRef{}, model.Fail("cancelled", "Session manager closed")
	}
	if len(m.entries) >= 128 {
		return model.SessionRef{}, model.Fail("invalid", "Session limit reached")
	}
	if m.validate != nil {
		if err := m.validate(target); err != nil {
			return model.SessionRef{}, err
		}
	}
	ctx, cancel := context.WithCancel(parent)
	ref := model.SessionRef{ID: model.ID(), Connection: target.Connection}
	e := &entry{owner: m, ref: ref, target: target, kind: kind, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), space: make(chan struct{}), ack: make(chan uint64, 1), start: make(chan struct{}), workDone: make(chan struct{}), done: make(chan struct{})}
	m.entries[ref.ID] = e
	if !pending {
		close(e.start)
		e.activated = true
	}
	go func() {
		defer close(e.workDone)
		select {
		case <-ctx.Done():
			return
		case <-e.start:
		}
		w := &Writer{e}
		_ = w.Send("state", []byte("running"))
		err := run(ctx, w)
		if err != nil && ctx.Err() == nil {
			message := "Session disconnected or failed"
			var safe *model.Error
			if errors.As(err, &safe) {
				message = safe.Code + ": " + safe.Message
			}
			_ = w.Send("error", []byte(message))
		}
		if ctx.Err() == nil {
			_ = w.Send("state", []byte("ended"))
		}
	}()
	go func() {
		e.dispatch()
		<-e.workDone
		e.cancel()
		m.mu.Lock()
		delete(m.entries, ref.ID)
		m.mu.Unlock()
		close(e.done)
	}()
	return ref, nil
}
func (m *Manager) Activate(ref model.SessionRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entries[ref.ID]
	if e == nil || e.ref != ref || e.ctx.Err() != nil {
		return model.Fail("stale_target", "Unknown or cancelled session")
	}
	if !e.activated {
		e.activated = true
		close(e.start)
	}
	return nil
}
func (m *Manager) Acknowledge(ref model.SessionRef, sequence uint64) error {
	m.mu.Lock()
	e := m.entries[ref.ID]
	m.mu.Unlock()
	if e == nil || e.ref != ref {
		return model.Fail("stale_target", "Unknown session")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if sequence == 0 || e.expectedAck != sequence || e.ctx.Err() != nil {
		return model.Fail("invalid", "Invalid terminal acknowledgement")
	}
	e.expectedAck = 0
	select {
	case e.ack <- sequence:
		return nil
	default:
		return model.Fail("invalid", "Terminal acknowledgement already queued")
	}
}
func validRef(ref model.SessionRef) bool {
	return ref.ID != "" && ref.Connection.ID != "" && ref.Connection.Generation > 0
}
func (m *Manager) Stop(ref model.SessionRef) error {
	if !validRef(ref) {
		return model.Fail("invalid", "Explicit session identity required")
	}
	m.mu.Lock()
	e := m.entries[ref.ID]
	if e != nil && e.ref != ref {
		m.mu.Unlock()
		return model.Fail("stale_target", "Session connection does not match")
	}
	m.mu.Unlock()
	if e == nil {
		return nil
	}
	e.cancel()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-e.done:
		return nil
	case <-timer.C:
		return model.Fail("cancelled", "Session cleanup timed out")
	}
}
func (m *Manager) stopMatching(ref *model.ConnectionRef, closing bool) {
	m.mu.Lock()
	if closing {
		m.closed = true
	}
	entries := []*entry{}
	for _, e := range m.entries {
		if ref == nil || e.ref.Connection == *ref {
			e.cancel()
			entries = append(entries, e)
		}
	}
	m.mu.Unlock()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for _, e := range entries {
		select {
		case <-e.done:
		case <-timer.C:
			return
		}
	}
}
func (m *Manager) StopConnection(ref model.ConnectionRef) { m.stopMatching(&ref, false) }
func (m *Manager) Close()                                 { m.stopMatching(nil, true) }
func (m *Manager) Stats() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := Stats{Sessions: len(m.entries), DroppedLogBytes: m.dropped.Load()}
	for _, e := range m.entries {
		e.mu.Lock()
		stats.QueuedBytes += e.bytes
		stats.InFlightBytes += e.inFlight
		e.mu.Unlock()
	}
	return stats
}
func (e *entry) notify() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}
func (e *entry) take() ([]queued, uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	dropped := e.dropPending
	e.dropPending = 0
	events := []queued{}
	budget := 64 << 10
	for len(e.queue) > 0 && len(e.queue[0].data) <= budget {
		next := e.queue[0]
		e.queue[0] = queued{}
		e.queue = e.queue[1:]
		e.bytes -= len(next.data)
		budget -= len(next.data)
		if next.kind == "data" && len(events) > 0 && events[len(events)-1].kind == "data" {
			last := &events[len(events)-1]
			last.data = append(last.data, next.data...)
		} else {
			events = append(events, next)
		}
	}
	if len(events) > 0 {
		close(e.space)
		e.space = make(chan struct{})
	}
	return events, dropped
}
func (e *entry) deliver(kind string, data []byte) bool {
	if e.ctx.Err() != nil {
		return false
	}
	e.mu.Lock()
	e.sequence++
	sequence := e.sequence
	terminal := e.kind == "terminal" && kind == "data"
	if terminal {
		e.expectedAck = sequence
	}
	e.inFlight = len(data)
	e.mu.Unlock()
	e.owner.emit(model.Event{Session: e.ref, Sequence: sequence, Kind: kind, Data: data})
	if terminal {
		select {
		case <-e.ack:
		case <-e.ctx.Done():
			return false
		}
	}
	e.mu.Lock()
	e.inFlight = 0
	e.mu.Unlock()
	return true
}
func (e *entry) dispatch() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	workDone := e.workDone
	finished := false
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
		case <-e.wake:
		case <-workDone:
			finished = true
			workDone = nil
		}
		events, dropped := e.take()
		if dropped > 0 {
			if !e.deliver("truncated", []byte(strconv.FormatUint(dropped, 10))) {
				return
			}
		}
		for _, event := range events {
			if !e.deliver(event.kind, event.data) {
				return
			}
		}
		e.mu.Lock()
		remaining := e.bytes
		e.mu.Unlock()
		if finished && remaining == 0 {
			return
		}
		if remaining >= 64<<10 {
			e.notify()
		}
	}
}
