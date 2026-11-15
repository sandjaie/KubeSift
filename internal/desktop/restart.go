/*
Purpose: Bridge reviewed restart and mixed operation results to the desktop.
Used by: Generated ExplorerService bindings.
Responsibilities: Use cancellable preparation and forward immutable confirmations.
Important: Result reads and revocation never submit a write.
*/
package desktop

import "github.com/sandjaie/KubeSift/internal/operation"

func (s *ExplorerService) PrepareRestart(id string, req operation.RestartRequest) (operation.RestartPreview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.RestartPreview{}, err
	}
	defer done()
	return s.core.restart.PrepareRestart(ctx, req)
}

func (s *ExplorerService) ExecuteRestart(token, acknowledgement string) (operation.OperationResult, error) {
	return s.core.restart.ExecuteRestart(s.core.ctx, token, acknowledgement)
}

func (s *ExplorerService) RevokeRestart(token string) {
	s.core.restart.RevokeRestart(token)
}

func (s *ExplorerService) GetOperationResult(id string) (operation.OperationResult, error) {
	return s.core.coordinator.GetOperationResult(id)
}

func (s *ExplorerService) ListOperationResults() []operation.OperationResult {
	return s.core.coordinator.ListOperationResults()
}
