/*
Purpose: Expose reviewed Node operations to the workspace.
Used by: Generated ExplorerService bindings.
Responsibilities: Resolve the connection environment and separate review cancellation from execution.
Important: Confirm accepts only a native token and acknowledgement.
*/
package desktop

import "github.com/sandjaie/KubeSift/internal/operation"

func (s *ExplorerService) PrepareNodeAction(id string, req operation.NodeRequest) (operation.NodePreview, error) {
	ctx, done, e := s.beginWorkloadRead(id)
	if e != nil {
		return operation.NodePreview{}, e
	}
	defer done()
	view, e := s.GetConnection(req.Resource.Connection)
	if e != nil {
		return operation.NodePreview{}, e
	}
	sources, e := s.ListSources()
	if e != nil {
		return operation.NodePreview{}, e
	}
	env := ""
	for _, src := range sources {
		if src.SourceID == view.SourceID {
			for _, entry := range src.Contexts {
				if entry.Context == view.Context {
					env = entry.Environment
				}
			}
		}
	}
	return s.core.coordinator.PrepareNodeAction(ctx, req, env)
}
func (s *ExplorerService) ExecuteNodeAction(token, ack string) (operation.OperationResult, error) {
	return s.core.coordinator.ExecuteNodeAction(token, ack)
}
func (s *ExplorerService) RevokeNodeReview(token string) { s.core.coordinator.RevokeNodeReview(token) }
func (s *ExplorerService) CancelDrain(id string)         { s.core.coordinator.CancelDrain(id) }
