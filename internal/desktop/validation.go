/*
Purpose: Record numeric evidence from the actual packaged validation renderer.
Used by: The explicit local Phase 1A measurement profile.
Responsibilities: Validate both owned fixtures before autorun and persist bounded scalar samples and frontend readiness.
Important: No frontend path, payload, executable or arbitrary local command is accepted; this is disabled without the isolated fixture environment.
*/
package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/model"
)

var validationEvidenceMu sync.Mutex

type ValidationSetup struct {
	Contexts []config.ContextSummary `json:"contexts"`
	Autorun  bool                    `json:"autorun"`
}
type ValidationSample struct {
	Phase              int       `json:"phase"`
	Cycle              int       `json:"cycle"`
	ElapsedMS          float64   `json:"elapsedMs"`
	Listeners          int       `json:"listeners"`
	ReceivedLogBytes   float64   `json:"receivedLogBytes"`
	RetainedLogBytes   int       `json:"retainedLogBytes"`
	TrimmedLogBytes    float64   `json:"trimmedLogBytes"`
	MaxHistoryBytes    int       `json:"maxHistoryBytes"`
	TerminalRTTMS      []float64 `json:"terminalRttMs"`
	ForwardPort        int       `json:"forwardPort"`
	SessionErrors      int       `json:"sessionErrors"`
	HostileContentSeen bool      `json:"hostileContentSeen"`
}

func validationSource() (string, error) {
	path := os.Getenv("KUBESIFT_TEST_KUBECONFIG")
	if !filepath.IsAbs(path) || filepath.Base(path) != "kubeconfig" || filepath.Base(filepath.Dir(path)) != "phase1a" || filepath.Base(filepath.Dir(filepath.Dir(path))) != ".local" {
		return "", model.Fail("invalid", "Packaged validation requires the isolated fixture source")
	}
	snapshot, err := config.Load([]string{path})
	if err != nil {
		return "", err
	}
	for _, name := range []string{"kind-dev-local", "kind-kubesift-b"} {
		r, e := snapshot.Resolve(name)
		if e != nil {
			return "", e
		}
		if _, e = localFixture(r); e != nil {
			return "", e
		}
	}
	return path, nil
}

// FrontendReady is called only after the renderer has completed Ping.
func (*Service) FrontendReady() error {
	path, err := validationSource()
	if err != nil {
		return nil
	}
	data, _ := json.Marshal(struct {
		UnixNano int64 `json:"unixNano"`
	}{time.Now().UnixNano()})
	return os.WriteFile(filepath.Join(filepath.Dir(path), "frontend-ready.json"), append(data, '\n'), 0600)
}
func (s *Service) ValidationConfiguration() (ValidationSetup, error) {
	if os.Getenv("KUBESIFT_VALIDATION_PROFILE") != "1" {
		return ValidationSetup{}, nil
	}
	path, err := validationSource()
	if err != nil {
		return ValidationSetup{}, err
	}
	contexts, err := s.LoadContexts([]string{path})
	if err != nil {
		return ValidationSetup{}, err
	}
	selected := []config.ContextSummary{}
	for _, name := range []string{"kind-dev-local", "kind-kubesift-b"} {
		for _, item := range contexts {
			if item.Name == name {
				selected = append(selected, item)
			}
		}
	}
	return ValidationSetup{selected, true}, nil
}
func (s *Service) RecordValidation(sample ValidationSample) error {
	if os.Getenv("KUBESIFT_VALIDATION_PROFILE") != "1" {
		return model.Fail("invalid", "Validation recording is disabled")
	}
	path, err := validationSource()
	if err != nil {
		return err
	}
	if sample.Phase < 0 || sample.Phase > 4 || sample.Cycle < 0 || sample.Cycle > 20 || sample.ElapsedMS < 0 || sample.Listeners < 0 || sample.Listeners > 130 || sample.RetainedLogBytes < 0 || sample.RetainedLogBytes > 128<<20 || sample.MaxHistoryBytes < 0 || sample.MaxHistoryBytes > 1<<20 || len(sample.TerminalRTTMS) > 20 {
		return model.Fail("invalid", "Invalid numeric validation sample")
	}
	record := struct {
		UnixNano int64            `json:"unixNano"`
		Native   RuntimeStats     `json:"native"`
		UI       ValidationSample `json:"ui"`
	}{time.Now().UnixNano(), s.RuntimeStats(), sample}
	data, err := json.Marshal(record)
	if err != nil {
		return model.Fail("invalid", "Invalid numeric validation sample")
	}
	validationEvidenceMu.Lock()
	defer validationEvidenceMu.Unlock()
	file, err := os.OpenFile(filepath.Join(filepath.Dir(path), "renderer-measurements.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size()+int64(len(data))+1 > 4<<20 {
		return model.Fail("invalid", "Validation evidence reached its 4 MiB limit")
	}
	_, err = file.Write(append(data, '\n'))
	return err
}
