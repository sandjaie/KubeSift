/*
Purpose: Expose the reviewed scaling flow to the product workspace.
Used by: Generated explorer bindings.
Responsibilities: Share bounded preparation slots and pass native execution tokens unchanged.
Important: Execution is owned by the native operation, independent of a view request.
*/
package desktop

import "github.com/sandjaie/KubeSift/internal/operation"

func (s *ExplorerService) PrepareScale(id string, req operation.ScaleRequest) (operation.ScalePreview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.ScalePreview{}, err
	}
	defer done()
	return s.core.scale.PrepareScale(ctx, req)
}
func (s *ExplorerService) ExecuteScale(token, acknowledgement string) (operation.ScaleResult, error) {
	return s.core.scale.ExecuteScale(s.core.ctx, token, acknowledgement)
}
func (s *ExplorerService) RevokeScale(token string) {
	s.core.scale.RevokeScale(token)
}
func (s *ExplorerService) GetScaleResult(operationID string) (operation.ScaleResult, error) {
	return s.core.scale.GetScaleResult(operationID)
}
func (s *ExplorerService) ListScaleResults() []operation.ScaleResult {
	return s.core.scale.ListScaleResults()
}
