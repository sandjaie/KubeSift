// Package connection owns explicit generations and guarded Kubernetes transports.
package connection

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/model"
	"io"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
	"sync"
	"time"
)

type AuthGate interface{ Check(config.Resolved) error }
type entry struct {
	ref      model.ConnectionRef
	resolved config.Resolved
	ctx      context.Context
	cancel   context.CancelFunc
}
type Manager struct {
	mu      sync.Mutex
	gate    AuthGate
	entries map[string]*entry
	keys    map[string]string
	cleanup func(model.ConnectionRef)
}

func NewManager(gate AuthGate) *Manager {
	return &Manager{gate: gate, entries: map[string]*entry{}, keys: map[string]string{}}
}
func (m *Manager) SetCleanup(fn func(model.ConnectionRef)) {
	m.mu.Lock()
	m.cleanup = fn
	m.mu.Unlock()
}
func (m *Manager) Open(ctx context.Context, r config.Resolved) (model.ConnectionRef, error) {
	if ctx == nil || ctx.Err() != nil {
		return model.ConnectionRef{}, model.Fail("cancelled", "Connection cancelled")
	}
	if r.HasHelper() {
		if m.gate == nil {
			return model.ConnectionRef{}, model.Fail("trust_required", "Review credential helper")
		}
		if err := m.gate.Check(r); err != nil {
			return model.ConnectionRef{}, err
		}
	}
	key := r.Source() + "/" + r.Name()
	m.mu.Lock()
	id := m.keys[key]
	if id == "" {
		id = model.ID()
		m.keys[key] = id
	}
	generation := uint64(1)
	var old model.ConnectionRef
	if e := m.entries[id]; e != nil {
		generation = e.ref.Generation + 1
		old = e.ref
		e.cancel()
	}
	owned, cancel := context.WithCancel(ctx)
	ref := model.ConnectionRef{ID: id, Generation: generation}
	m.entries[id] = &entry{ref, r, owned, cancel}
	cleanup := m.cleanup
	m.mu.Unlock()
	if old.ID != "" && cleanup != nil {
		cleanup(old)
	}
	return ref, nil
}
func (m *Manager) lookup(ref model.ConnectionRef) (*entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entries[ref.ID]
	if ref.ID == "" || e == nil || e.ref != ref || e.ctx.Err() != nil {
		return nil, model.Fail("stale_target", "Connection is closed or replaced")
	}
	return e, nil
}

type guarded struct {
	next  http.RoundTripper
	check func() error
	owned context.Context
}

func (g guarded) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := g.check(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(g.owned, cancel)
	response, err := g.next.RoundTrip(req.Clone(ctx))
	cleanup := func() { stop(); cancel() }
	if err != nil {
		cleanup()
		return nil, err
	}
	response.Body = &ownedBody{ReadCloser: response.Body, cleanup: cleanup}
	return response, nil
}
func (m *Manager) RESTConfig(ref model.ConnectionRef) (*rest.Config, error) {
	e, err := m.lookup(ref)
	if err != nil {
		return nil, err
	}
	check := func() error {
		if _, err := m.lookup(ref); err != nil {
			return err
		}
		if e.resolved.HasHelper() {
			if m.gate == nil {
				return model.Fail("trust_required", "Review credential helper")
			}
			return m.gate.Check(e.resolved)
		}
		return nil
	}
	if err = check(); err != nil {
		return nil, err
	}
	cfg := e.resolved.RESTConfig()
	native, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, model.Fail("invalid", "Cannot create authenticated transport")
	}
	safe := &rest.Config{Host: cfg.Host, APIPath: cfg.APIPath, ContentConfig: cfg.ContentConfig, UserAgent: "KubeSift-Phase1A", Timeout: 30 * time.Second, Transport: guarded{native, check, e.ctx}}
	return safe, nil
}
func (m *Manager) Client(ref model.ConnectionRef) (kubernetes.Interface, error) {
	cfg, err := m.RESTConfig(ref)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, model.Fail("invalid", "Cannot create Kubernetes client")
	}
	return client, nil
}
func (m *Manager) Close(ref model.ConnectionRef) error {
	m.mu.Lock()
	e := m.entries[ref.ID]
	if e == nil || e.ref != ref {
		m.mu.Unlock()
		return model.Fail("stale_target", "Unknown connection")
	}
	e.cancel()
	cleanup := m.cleanup
	m.mu.Unlock()
	if cleanup != nil {
		cleanup(ref)
	}
	return nil
}
func (m *Manager) CloseAll() {
	m.mu.Lock()
	refs := []model.ConnectionRef{}
	for _, e := range m.entries {
		refs = append(refs, e.ref)
	}
	m.mu.Unlock()
	for _, ref := range refs {
		_ = m.Close(ref)
	}
}

// ownedBody retains cancellation until the caller has finished a streaming response.
type ownedBody struct {
	io.ReadCloser
	cleanup func()
}

func (b *ownedBody) Close() error { defer b.cleanup(); return b.ReadCloser.Close() }
