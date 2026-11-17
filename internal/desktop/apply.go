/*
Purpose: Expose reviewed server-side apply to the desktop.
Used by: Generated ExplorerService bindings.
Responsibilities: Keep capture and review requests cancellable and forward the confirmation token.
Important: Retained results carry only target and outcome information.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/operation"
)

func (s *ExplorerService) OpenApply(id string, target kube.EditTarget) (operation.ApplyDraft, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.ApplyDraft{}, err
	}
	defer done()
	return s.core.applying.OpenApply(ctx, target)
}

func (s *ExplorerService) PrepareApply(id string, req operation.ApplyRequest) (operation.ApplyReview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.ApplyReview{}, err
	}
	defer done()
	return s.core.applying.PrepareApply(ctx, req)
}

func (s *ExplorerService) ExecuteApply(token string) (operation.OperationResult, error) {
	return s.core.applying.ExecuteApply(token)
}

func (s *ExplorerService) RevokeApply(token string) { s.core.applying.RevokeApply(token) }

func (s *ExplorerService) CloseApply(draftID string) { s.core.applying.CloseApply(draftID) }

func (s *ExplorerService) ReadAppliedDetail(id, operationID string) (kube.GenericResourceDetail, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.GenericResourceDetail{}, err
	}
	defer done()
	return s.core.applying.ReadAppliedDetail(ctx, operationID)
}
