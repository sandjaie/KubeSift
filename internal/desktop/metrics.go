/*
Purpose: Expose cancellable current metrics to the workspace.
Used by: Generated ExplorerService bindings.
Responsibilities: Forward only explicit captured resource targets.
Important: This endpoint reads metrics and never provisions a provider.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

func (s *ExplorerService) ReadCurrentMetrics(id string, req kube.MetricsRequest) (kube.MetricsBatch, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.MetricsBatch{}, err
	}
	defer done()
	return s.core.kube.ReadCurrentMetrics(ctx, req)
}

func (s *ExplorerService) HistorySource(ref model.ConnectionRef) (kube.HistorySource, error) {
	return s.core.kube.HistorySource(ref)
}
func (s *ExplorerService) SetHistorySource(ref model.ConnectionRef, revision string, c kube.HistoryConfig) (kube.HistorySource, error) {
	if c.CAFile != "" {
		s.mu.Lock()
		selected := s.historyCA == c.CAFile
		s.mu.Unlock()
		if !selected {
			return kube.HistorySource{}, model.Fail("invalid", "Select the history CA using the file picker.")
		}
	}
	return s.core.kube.SetHistorySource(ref, revision, c)
}
func (s *ExplorerService) ClearHistorySource(ref model.ConnectionRef, revision string) (kube.HistorySource, error) {
	return s.core.kube.ClearHistorySource(ref, revision)
}
func (s *ExplorerService) SuggestHistorySource(id string, ref model.ConnectionRef) (string, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return "", err
	}
	defer done()
	return s.core.kube.SuggestHistorySource(ctx, ref)
}

func (s *ExplorerService) ReadHistoricalMetrics(id string, r kube.HistoryRequest) (kube.HistoricalMetrics, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.HistoricalMetrics{}, err
	}
	defer done()
	return s.core.kube.ReadHistoricalMetrics(ctx, r)
}

func (s *ExplorerService) PickHistoryCA() (string, error) {
	path, err := pickHelmFile("Select historical metrics CA certificate", "*.pem;*.crt;*.cer")
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.historyCA = path
	s.mu.Unlock()
	return path, nil
}
