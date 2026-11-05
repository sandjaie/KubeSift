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
	snapshotID  string
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
	s.snapshotID = model.ID()
	summaries := snapshot.Contexts()
	for i := range summaries {
		summaries[i].SnapshotID = s.snapshotID
	}
	s.mu.Unlock()
	return summaries, nil
}
func (s *Service) OpenConnection(snapshotID, name string) (model.ConnectionRef, error) {
	s.mu.Lock()
	snapshot := s.snapshot
	valid := snapshotID != "" && snapshotID == s.snapshotID
	s.mu.Unlock()
	if snapshot == nil || !valid {
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
