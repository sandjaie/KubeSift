/*
Purpose: Expose retained history and reviewed workload rollback to the desktop.
Used by: Generated ExplorerService bindings.
Responsibilities: Bound request lifetimes and forward immutable review inputs.
Important: Historical source payloads stay native.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/operation"
)

func (s *ExplorerService) ReadRolloutHistory(id string, req kube.RolloutHistoryRequest) (kube.RolloutHistoryPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.RolloutHistoryPage{}, err
	}
	defer done()
	return s.core.kube.ReadRolloutHistory(ctx, req)
}

func (s *ExplorerService) PrepareRollback(id string, req operation.RollbackRequest) (operation.RollbackPreview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.RollbackPreview{}, err
	}
	defer done()
	return s.core.rollback.PrepareRollback(ctx, req)
}

func (s *ExplorerService) ExecuteRollback(token string) (operation.OperationResult, error) {
	return s.core.rollback.ExecuteRollback(token)
}

func (s *ExplorerService) RevokeRollback(token string) {
	s.core.rollback.RevokeRollback(token)
}
