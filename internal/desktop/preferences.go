/*
Purpose: Expose a narrow non-sensitive preferences bridge.
Used by: Product settings and native geometry lifecycle.
Responsibilities: Serialize preference updates at a fixed user-config path.
Important: Invalid files are not overwritten by background geometry; UI reset is explicit.
*/
package desktop

import (
	"errors"
	"github.com/sandjaie/KubeSift/internal/preferences"
	"os"
	"path/filepath"
	"sync"
)

type PreferencesService struct {
	mu      sync.Mutex
	path    string
	value   preferences.Preferences
	problem error
}

func newPreferencesService() *PreferencesService {
	base, err := os.UserConfigDir()
	s := &PreferencesService{value: preferences.Default(), problem: err}
	if err == nil {
		s.path = filepath.Join(base, "kubesift", "preferences.json")
		s.value, s.problem = preferences.Load(s.path)
	}
	return s
}
func (s *PreferencesService) GetPreferences() (preferences.Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value, s.problem
}
func (s *PreferencesService) SetPreferences(p preferences.Preferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return errors.New("user configuration directory unavailable")
	}
	if err := preferences.Validate(p); err != nil {
		return err
	}
	p.Window = s.value.Window
	if err := preferences.Save(s.path, p); err != nil {
		return err
	}
	s.value = p
	s.problem = nil
	return nil
}
func (s *PreferencesService) saveWindow(r preferences.Rect) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.problem != nil {
		return
	}
	next := s.value
	next.Window = &r
	if err := preferences.Save(s.path, next); err != nil {
		s.problem = err
		return
	}
	s.value = next
}
