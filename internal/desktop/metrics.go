/*
Purpose: Expose cancellable current metrics to the workspace.
Used by: Generated ExplorerService bindings.
Responsibilities: Forward only explicit captured resource targets.
Important: This endpoint reads metrics and never provisions a provider.
*/
package desktop

import "github.com/sandjaie/KubeSift/internal/kube"

func (s *ExplorerService) ReadCurrentMetrics(id string, req kube.MetricsRequest) (kube.MetricsBatch, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.MetricsBatch{}, err
	}
	defer done()
	return s.core.kube.ReadCurrentMetrics(ctx, req)
}
