/*
Purpose: Expose native Helm management without accepting filesystem or execution targets from UI state.
Used by: Generated ExplorerService bindings and Helm settings/actions.
Responsibilities: Compose source preferences, native pickers, cancellable preparation and token-only execution.
Important: Credentials remain session-only; execution outlives navigation but not native connection ownership.
*/
package desktop

import (
	"context"
	"time"

	"github.com/sandjaie/KubeSift/internal/helmmanager"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/preferences"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func (s *ExplorerService) loadHelmSourcesLocked() error {
	if s.helmSourcesLoaded {
		return nil
	}
	if s.prefs == nil {
		return model.Fail("unavailable", "Helm source preferences are unavailable.")
	}
	p, err := s.prefs.GetPreferences()
	if err != nil {
		return model.Fail("invalid", "Reset invalid preferences explicitly before editing Helm sources.")
	}
	for _, source := range p.HelmSources {
		if err := s.core.helmManagement.SaveSource(source); err != nil {
			return err
		}
	}
	s.helmSourcesLoaded = true
	return nil
}

func (s *ExplorerService) ListHelmSources() ([]helmmanager.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadHelmSourcesLocked(); err != nil {
		return nil, err
	}
	return s.core.helmManagement.ListSources(), nil
}

func (s *PreferencesService) saveHelmSources(sources []preferences.HelmSource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.problem != nil || s.path == "" {
		return model.Fail("unavailable", "Helm source preferences cannot be saved. Resolve the preferences error first.")
	}
	p := s.value
	p.HelmSources = sources
	if err := preferences.Save(s.path, p); err != nil {
		return model.Fail("unavailable", "Cannot save Helm source metadata; check available space and preference limits.")
	}
	s.value = p
	return nil
}

func (s *ExplorerService) SaveHelmSource(source helmmanager.Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadHelmSourcesLocked(); err != nil {
		return err
	}
	if err := preferences.ValidateHelmSource(source); err != nil {
		return model.Fail("invalid", err.Error())
	}
	sources := s.core.helmManagement.ListSources()
	previous := helmmanager.Source{}
	index := -1
	for i, existing := range sources {
		if existing.ID == source.ID {
			previous, index = existing, i
		}
	}
	for kind, pair := range map[string][2]string{"ca": {source.CAFile, previous.CAFile}, "cert": {source.CertFile, previous.CertFile}, "key": {source.KeyFile, previous.KeyFile}} {
		if pair[0] != "" && pair[0] != pair[1] && !s.helmTLS[kind+"\x00"+pair[0]] {
			return model.Fail("invalid", "Select TLS files using the native file picker.")
		}
	}
	if index < 0 {
		sources = append(sources, source)
	} else {
		sources[index] = source
	}
	if err := s.prefs.saveHelmSources(sources); err != nil {
		return err
	}
	return s.core.helmManagement.SaveSource(source)
}

func (s *ExplorerService) DeleteHelmSource(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadHelmSourcesLocked(); err != nil {
		return err
	}
	retained := []preferences.HelmSource{}
	for _, source := range s.core.helmManagement.ListSources() {
		if source.ID != id {
			retained = append(retained, source)
		}
	}
	if err := s.prefs.saveHelmSources(retained); err != nil {
		return err
	}
	return s.core.helmManagement.DeleteSource(id)
}

func (s *ExplorerService) SetHelmCredentials(id string, credentials helmmanager.SourceCredentials) error {
	if _, err := s.ListHelmSources(); err != nil {
		return err
	}
	return s.core.helmManagement.SetCredentials(id, credentials)
}
func (s *ExplorerService) ForgetHelmCredentials(id string) {
	s.core.helmManagement.ForgetCredentials(id)
}

func pickHelmFile(title, pattern string) (string, error) {
	app := application.Get()
	if app == nil {
		return "", model.Fail("unavailable", "Native file selection requires the desktop app.")
	}
	path, err := app.Dialog.OpenFile().SetTitle(title).CanChooseFiles(true).CanChooseDirectories(false).AddFilter(title, pattern).PromptForSingleSelection()
	if err != nil || path == "" {
		return "", model.Fail("cancelled", "File selection cancelled.")
	}
	return path, nil
}

func (s *ExplorerService) PickHelmChart() (string, error) {
	path, err := pickHelmFile("Select packaged Helm chart", "*.tgz")
	if err != nil {
		return "", err
	}
	return s.core.helmManagement.ImportChart(path)
}
func (s *ExplorerService) PickHelmValues() (string, error) {
	path, err := pickHelmFile("Select Helm override values", "*.yaml;*.yml")
	if err != nil {
		return "", err
	}
	return s.core.helmManagement.ImportValues(path)
}
func (s *ExplorerService) PickHelmTLSFile(kind string) (string, error) {
	if kind != "ca" && kind != "cert" && kind != "key" {
		return "", model.Fail("invalid", "Choose CA, client certificate or key.")
	}
	path, err := pickHelmFile("Select Helm TLS "+kind, "*.pem;*.crt;*.cer;*.key")
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.helmTLS == nil || len(s.helmTLS) >= 96 {
		s.helmTLS = map[string]bool{}
	}
	s.helmTLS[kind+"\x00"+path] = true
	return path, nil
}

func (s *ExplorerService) ListHelmCharts(id, source, filter, cursor string) (helmmanager.ChartPage, error) {
	ctx, done, err := s.beginNativeRead(id, 120*time.Second)
	if err != nil {
		return helmmanager.ChartPage{}, err
	}
	defer done()
	if _, err := s.ListHelmSources(); err != nil {
		return helmmanager.ChartPage{}, err
	}
	return s.core.helmManagement.ListCharts(ctx, source, filter, cursor)
}
func (s *ExplorerService) ResolveHelmChart(id string, request helmmanager.ChartRequest) (helmmanager.ChartSnapshot, error) {
	ctx, done, err := s.beginNativeRead(id, 120*time.Second)
	if err != nil {
		return helmmanager.ChartSnapshot{}, err
	}
	defer done()
	if request.SourceID != "" {
		if _, err := s.ListHelmSources(); err != nil {
			return helmmanager.ChartSnapshot{}, err
		}
	}
	return s.core.helmManagement.ResolveChart(ctx, request)
}
func (s *ExplorerService) PrepareHelmAction(id string, request helmmanager.PrepareRequest) (helmmanager.Review, error) {
	ctx, done, err := s.beginNativeRead(id, 60*time.Second)
	if err != nil {
		return helmmanager.Review{}, err
	}
	defer done()
	return s.core.helmManagement.Prepare(ctx, request)
}
func (s *ExplorerService) ReadHelmReview(id, token, section string, offset int) (helmmanager.ReviewSection, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return helmmanager.ReviewSection{}, err
	}
	defer done()
	return s.core.helmManagement.ReadReview(ctx, token, section, offset)
}
func (s *ExplorerService) ReadHelmValues(handle string, offset int) (helmmanager.ReviewSection, error) {
	return s.core.helmManagement.ReadValues(context.Background(), handle, offset)
}
func (s *ExplorerService) DiscardHelmDraft(id string) { s.core.helmManagement.Discard(id) }
func (s *ExplorerService) ExecuteHelmAction(token string) (helmmanager.Result, error) {
	return s.core.helmManagement.Execute(token)
}
func (s *ExplorerService) CancelHelmAction(id string) error { return s.core.helmManagement.Cancel(id) }
func (s *ExplorerService) ListHelmResults() []helmmanager.Result {
	return s.core.helmManagement.ListResults()
}
