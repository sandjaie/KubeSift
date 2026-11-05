// Package desktop translates desktop calls to framework-independent services.
package desktop

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/model"
	"sync"
)

type Service struct {
	mu          sync.Mutex
	snapshot    *config.Snapshot
	connections *connection.Manager
}

func NewService() *Service    { return &Service{connections: connection.NewManager(nil)} }
func (*Service) Ping() string { return "ready" }
func (s *Service) LoadContexts(paths []string) ([]config.ContextSummary, error) {
	snapshot, err := config.Load(paths)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.snapshot = snapshot
	s.mu.Unlock()
	return snapshot.Contexts(), nil
}
func (s *Service) OpenConnection(name string) (model.ConnectionRef, error) {
	s.mu.Lock()
	snapshot := s.snapshot
	s.mu.Unlock()
	if snapshot == nil {
		return model.ConnectionRef{}, model.Fail("invalid", "Load a configuration first")
	}
	resolved, err := snapshot.Resolve(name)
	if err != nil {
		return model.ConnectionRef{}, err
	}
	return s.connections.Open(context.Background(), resolved)
}
func (s *Service) CloseConnection(ref model.ConnectionRef) error { return s.connections.Close(ref) }
func (s *Service) ServiceShutdown() error                        { s.connections.CloseAll(); return nil }
