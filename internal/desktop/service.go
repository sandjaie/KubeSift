/*
Purpose: Expose the narrow desktop API to the frontend.
Used by: Generated Wails bindings and the application lifecycle.
Responsibilities: Bind context previews, helper approvals and session controls to backend-issued identities.
Important: Do not accept arbitrary executable text from React; stale preview IDs must fail.
*/
// Package desktop translates desktop calls to framework-independent services.
package desktop

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	"github.com/sandjaie/KubeSift/internal/trust"
	"k8s.io/client-go/rest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type Service struct {
	mu          sync.Mutex
	snapshot    *config.Snapshot
	snapshotID  string
	connections *connection.Manager
	auth        *lazyAuth
	kube        *kube.Service
	ctx         context.Context
	cancel      context.CancelFunc
	sessions    *session.Manager
	emit        func(model.Event)
}

type lazyAuth struct {
	once  sync.Once
	guard *trust.Guard
	err   error
}

func (a *lazyAuth) get() (*trust.Guard, error) {
	a.once.Do(func() {
		directory, err := os.UserConfigDir()
		if err != nil {
			a.err = model.Fail("invalid", "Cannot locate application configuration")
			return
		}
		store, err := trust.OpenStore(filepath.Join(directory, "KubeSift", "trust.json"))
		if err != nil {
			a.err = err
			return
		}
		a.guard = trust.NewGuard(store)
	})
	return a.guard, a.err
}
func (a *lazyAuth) Check(r config.Resolved) error {
	if !r.HasHelper() {
		return nil
	}
	guard, err := a.get()
	if err != nil {
		return err
	}
	return guard.Check(r)
}
func (a *lazyAuth) Authenticate(ctx context.Context, r config.Resolved) (*rest.Config, error) {
	guard, err := a.get()
	if err != nil {
		return nil, err
	}
	return guard.Authenticate(ctx, r)
}
func NewService() *Service {
	auth := &lazyAuth{}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{connections: connection.NewManager(auth), auth: auth, ctx: ctx, cancel: cancel}
	s.sessions = session.NewManager(func(event model.Event) {
		s.mu.Lock()
		emit := s.emit
		s.mu.Unlock()
		if emit != nil {
			emit(event)
		}
	})
	s.sessions.RequireBridgeAcknowledgements()
	s.sessions.SetValidate(func(target model.Target) error { return s.connections.Validate(target.Connection) })
	s.connections.SetCleanup(s.sessions.StopConnection)
	s.kube = kube.NewService(s.connections, s.sessions)
	return s
}
func (*Service) Ping() string { return "ready" }
func (s *Service) LoadContexts(paths []string) ([]config.ContextSummary, error) {
	snapshot, err := config.Load(paths)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.snapshot = snapshot
	s.snapshotID = model.ID()
	summaries := snapshot.Contexts()
	for i := range summaries {
		summaries[i].SnapshotID = s.snapshotID
	}
	s.mu.Unlock()
	return summaries, nil
}
func (s *Service) resolve(snapshotID, name string) (config.Resolved, error) {
	s.mu.Lock()
	snapshot := s.snapshot
	valid := snapshotID != "" && snapshotID == s.snapshotID
	s.mu.Unlock()
	if snapshot == nil || !valid {
		return config.Resolved{}, model.Fail("stale_target", "Reload configuration; preview was replaced")
	}
	return snapshot.Resolve(name)
}
func (s *Service) CloseConnection(ref model.ConnectionRef) error { return s.connections.Close(ref) }
func (s *Service) ServiceShutdown() error {
	s.cancel()
	s.sessions.Close()
	s.connections.CloseAll()
	return nil
}

func (s *Service) OpenConnection(snapshotID, name string) (model.ConnectionRef, error) {
	r, err := s.resolve(snapshotID, name)
	if err != nil {
		return model.ConnectionRef{}, err
	}
	return s.connections.Open(s.ctx, r)
}
func (s *Service) ReviewHelper(snapshotID, name string) (trust.PreviewRecord, error) {
	r, err := s.resolve(snapshotID, name)
	if err != nil {
		return trust.PreviewRecord{}, err
	}
	guard, err := s.auth.get()
	if err != nil {
		return trust.PreviewRecord{}, err
	}
	return guard.Review(r)
}
func (s *Service) ApproveHelper(id, snapshotID, name string) error {
	r, err := s.resolve(snapshotID, name)
	if err != nil {
		return err
	}
	guard, err := s.auth.get()
	if err != nil {
		return err
	}
	return guard.ApprovePreview(id, r)
}
func (s *Service) TrustedHelpers() ([]trust.Entry, error) {
	guard, err := s.auth.get()
	if err != nil {
		return nil, err
	}
	return guard.Entries(), nil
}
func (s *Service) RevokeHelper(fingerprint string) error {
	guard, err := s.auth.get()
	if err != nil {
		return err
	}
	if err = guard.Revoke(fingerprint); err != nil {
		return err
	}
	s.connections.CloseAll()
	return nil
}

func (s *Service) setEmitter(emit func(model.Event))          { s.mu.Lock(); s.emit = emit; s.mu.Unlock() }
func (s *Service) ActivateSession(ref model.SessionRef) error { return s.sessions.Activate(ref) }
func (s *Service) StopSession(ref model.SessionRef) error     { return s.sessions.Stop(ref) }
func (s *Service) AcknowledgeSession(ref model.SessionRef, sequence uint64) error {
	return s.sessions.Acknowledge(ref, sequence)
}
func (s *Service) SessionStats() session.Stats { return s.sessions.Stats() }

func (s *Service) ListPods(ref model.ConnectionRef, namespace string) ([]kube.PodSummary, error) {
	pods, err := s.kube.Pods(s.ctx, ref, namespace)
	if err != nil {
		return nil, err
	}
	result := make([]kube.PodSummary, 0, len(pods))
	for _, pod := range pods {
		result = append(result, kube.Summarize(pod))
	}
	return result, nil
}
func (s *Service) WatchPods(target model.Target) (model.SessionRef, error) {
	return s.kube.WatchPods(s.ctx, target)
}
func (s *Service) StartLogs(target model.Target, previous bool, tail int64) (model.SessionRef, error) {
	return s.kube.Logs(s.ctx, target, previous, tail)
}
func (s *Service) StartTerminal(target model.Target) (model.SessionRef, error) {
	return s.kube.Exec(s.ctx, target)
}
func (s *Service) StartForward(target model.Target, local, remote uint16) (kube.ForwardResult, error) {
	return s.kube.Forward(s.ctx, target, local, remote)
}
func (s *Service) TerminalInput(ref model.SessionRef, data []byte) error {
	return s.kube.Input(ref, data)
}
func (s *Service) ResizeTerminal(ref model.SessionRef, cols, rows uint16) error {
	return s.kube.Resize(ref, cols, rows)
}

type RuntimeStats struct {
	Sessions   session.Stats `json:"sessions"`
	Goroutines int           `json:"goroutines"`
	HeapAlloc  uint64        `json:"heapAlloc"`
	HeapSys    uint64        `json:"heapSys"`
}

func (s *Service) RuntimeStats() RuntimeStats {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return RuntimeStats{s.sessions.Stats(), runtime.NumGoroutine(), memory.HeapAlloc, memory.HeapSys}
}
