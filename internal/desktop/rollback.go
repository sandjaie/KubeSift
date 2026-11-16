/*
Purpose: Expose retained history and reviewed workload rollback to the desktop.
Used by: Generated ExplorerService bindings.
Responsibilities: Bound request lifetimes and forward immutable review inputs.
Important: Historical source payloads stay native.
*/
package desktop

import "github.com/sandjaie/KubeSift/internal/kube"

func (s *ExplorerService) ReadRolloutHistory(id string, req kube.RolloutHistoryRequest) (kube.RolloutHistoryPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.RolloutHistoryPage{}, err
	}
	defer done()
	return s.core.kube.ReadRolloutHistory(ctx, req)
}
