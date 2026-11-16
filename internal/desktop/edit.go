/*
Purpose: Expose the common reviewed YAML editor to the desktop.
Used by: Generated ExplorerService bindings.
Responsibilities: Keep capture and review requests cancellable and forward the confirmation token.
Important: Retained results carry only target and outcome information.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/operation"
)

func (s *ExplorerService) OpenEdit(id string, target kube.EditTarget) (operation.EditDraft, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.EditDraft{}, err
	}
	defer done()
	return s.core.editing.OpenEdit(ctx, target)
}

func (s *ExplorerService) PrepareEdit(id string, req operation.EditRequest) (operation.EditPreview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.EditPreview{}, err
	}
	defer done()
	return s.core.editing.PrepareEdit(ctx, req)
}

func (s *ExplorerService) ExecuteEdit(token string) (operation.OperationResult, error) {
	return s.core.editing.ExecuteEdit(token)
}

func (s *ExplorerService) RevokeEdit(token string) { s.core.editing.RevokeEdit(token) }

func (s *ExplorerService) CloseEdit(draftID string) { s.core.editing.CloseEdit(draftID) }
