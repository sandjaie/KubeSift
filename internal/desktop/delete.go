/*
Purpose: Bridge reviewed deletion and target checks to the desktop.
Used by: Generated ExplorerService bindings.
Responsibilities: Admit cancellable reads and pass only token and acknowledgement at confirmation.
Important: A dispatched deletion outlives the initiating view.
*/
package desktop

import "github.com/sandjaie/KubeSift/internal/operation"

func (s *ExplorerService) PrepareDelete(id string, req operation.DeleteRequest) (operation.DeletePreview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.DeletePreview{}, err
	}
	defer done()
	return s.core.deletion.PrepareDelete(ctx, req)
}

func (s *ExplorerService) ExecuteDelete(token, acknowledgement string) (operation.OperationResult, error) {
	return s.core.deletion.ExecuteDelete(s.core.ctx, token, acknowledgement)
}

func (s *ExplorerService) RevokeDelete(token string) {
	s.core.deletion.RevokeDelete(token)
}

func (s *ExplorerService) CheckDeleteTarget(id, operationID string) (operation.DeleteObservation, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.DeleteObservation{}, err
	}
	defer done()
	return s.core.deletion.CheckDeleteTarget(ctx, operationID)
}
