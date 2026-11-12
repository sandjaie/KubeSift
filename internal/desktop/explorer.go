/*
Purpose: Expose product source and connection operations to the desktop UI.
Used by: Generated ExplorerService bindings.
Responsibilities: Compose one coordinator over the existing connection, trust and Kubernetes core.
Important: Credentials remain native; resetting corrupt metadata requires explicit user action.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/explorer"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/sources"
	"github.com/sandjaie/KubeSift/internal/trust"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ExplorerService struct {
	mu          sync.Mutex
	core        *Service
	coordinator *explorer.Coordinator
}

func newExplorerService(core *Service) *ExplorerService { return &ExplorerService{core: core} }
func sourcePath() (string, error) {
	dir, e := os.UserConfigDir()
	if e != nil {
		return "", model.Fail("invalid_source", "Cannot locate local configuration directory")
	}
	return filepath.Join(dir, "kubesift", "sources.json"), nil
}
func (s *ExplorerService) get() (*explorer.Coordinator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.coordinator != nil {
		return s.coordinator, nil
	}
	path, e := sourcePath()
	if e != nil {
		return nil, e
	}
	r, e := sources.NewRegistry(path)
	if e != nil {
		return nil, e
	}
	s.coordinator = explorer.NewCoordinator(r, s.core.connections, s.core.kube)
	return s.coordinator, nil
}
func (s *ExplorerService) ListSources() ([]sources.SourceResult, error) {
	c, e := s.get()
	if e != nil {
		return nil, e
	}
	return c.ListSources()
}
func (s *ExplorerService) ReloadSource(id string) (sources.SourceResult, error) {
	c, e := s.get()
	if e != nil {
		return sources.SourceResult{}, e
	}
	return c.ReloadSource(id)
}
func (s *ExplorerService) AddSource(path string) (sources.SourceResult, error) {
	c, e := s.get()
	if e != nil {
		return sources.SourceResult{}, e
	}
	return c.AddSource(path)
}
func (s *ExplorerService) SetSourceEnabled(id string, enabled, confirmed bool) error {
	c, e := s.get()
	if e != nil {
		return e
	}
	return c.SetSourceEnabled(id, enabled, confirmed)
}
func (s *ExplorerService) RemoveSource(id string, confirmed bool) error {
	c, e := s.get()
	if e != nil {
		return e
	}
	return c.RemoveSource(id, confirmed)
}
func (s *ExplorerService) SetLabel(source, name, label string) error {
	c, e := s.get()
	if e != nil {
		return e
	}
	return c.SetLabel(source, name, label)
}
func (s *ExplorerService) StartConnection(req explorer.ConnectRequest) (explorer.Attempt, error) {
	c, e := s.get()
	if e != nil {
		return explorer.Attempt{}, e
	}
	return c.StartConnection(req)
}
func (s *ExplorerService) GetAttempt(id string) (explorer.ConnectionView, error) {
	c, e := s.get()
	if e != nil {
		return explorer.ConnectionView{}, e
	}
	return c.GetAttempt(id)
}
func (s *ExplorerService) CancelAttempt(id string) error {
	c, e := s.get()
	if e != nil {
		return e
	}
	return c.CancelAttempt(id)
}
func (s *ExplorerService) GetConnection(ref model.ConnectionRef) (explorer.ConnectionView, error) {
	c, e := s.get()
	if e != nil {
		return explorer.ConnectionView{}, e
	}
	return c.GetConnection(ref)
}
func (s *ExplorerService) Disconnect(ref model.ConnectionRef) error {
	c, e := s.get()
	if e != nil {
		return e
	}
	return c.Disconnect(ref)
}
func (s *ExplorerService) ReadPods(ref model.ConnectionRef, namespace string) (explorer.ConnectionView, error) {
	c, e := s.get()
	if e != nil {
		return explorer.ConnectionView{}, e
	}
	return c.ReadPods(ref, namespace)
}
func (s *ExplorerService) ReviewHelper(source, snapshot, name string) (trust.PreviewRecord, error) {
	c, e := s.get()
	if e != nil {
		return trust.PreviewRecord{}, e
	}
	g, e := s.core.auth.get()
	if e != nil {
		return trust.PreviewRecord{}, e
	}
	return c.ReviewHelper(g, source, snapshot, name)
}
func (s *ExplorerService) ApproveHelper(source, snapshot, name, review string) error {
	c, e := s.get()
	if e != nil {
		return e
	}
	g, e := s.core.auth.get()
	if e != nil {
		return e
	}
	return c.ApproveHelper(g, source, snapshot, name, review)
}
func (s *ExplorerService) TrustedHelpers() ([]trust.Entry, error) { return s.core.TrustedHelpers() }
func (s *ExplorerService) RevokeHelper(fp string) error {
	c, e := s.get()
	if e != nil {
		return e
	}
	g, e := s.core.auth.get()
	if e != nil {
		return e
	}
	return c.RevokeHelper(g, fp)
}

// ResetSources only recovers an invalid store; valid configured sources use normal Settings controls.
func (s *ExplorerService) ResetSources() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.coordinator != nil {
		return model.Fail("invalid_source", "Use source removal controls for valid settings")
	}
	path, e := sourcePath()
	if e != nil {
		return e
	}
	if _, e = sources.NewRegistry(path); e == nil {
		return model.Fail("invalid_source", "Source settings are valid; reset is unnecessary")
	}
	if e = os.Rename(path, path+".invalid-"+time.Now().UTC().Format("20060102T150405.000000000")); e != nil {
		return model.Fail("invalid_source", "Could not preserve invalid settings; no reset performed")
	}
	return nil
}
func (s *ExplorerService) ServiceShutdown() error {
	s.mu.Lock()
	c := s.coordinator
	s.mu.Unlock()
	if c != nil {
		c.Close()
	}
	return nil
}
