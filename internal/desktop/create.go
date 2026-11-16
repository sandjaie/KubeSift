/*
Purpose: Expose reviewed creation and explicit name inspection to the desktop.
Used by: Generated ExplorerService bindings.
Responsibilities: Use bounded cancellable reads and forward one opaque confirmation token.
Important: Result inspection never writes or adopts an observed UID.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/operation"
)

func (s *ExplorerService) OpenCreate(id string, ref model.ConnectionRef, namespace string) (operation.CreateDraft, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.CreateDraft{}, err
	}
	defer done()
	return s.core.creation.OpenCreate(ctx, ref, namespace)
}

func (s *ExplorerService) PrepareCreate(id string, req operation.CreateRequest) (operation.CreatePreview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.CreatePreview{}, err
	}
	defer done()
	return s.core.creation.PrepareCreate(ctx, req)
}

func (s *ExplorerService) ExecuteCreate(token string) (operation.OperationResult, error) {
	return s.core.creation.ExecuteCreate(token)
}
func (s *ExplorerService) RevokeCreate(token string)  { s.core.creation.RevokeCreate(token) }
func (s *ExplorerService) CloseCreate(draftID string) { s.core.creation.CloseCreate(draftID) }

func (s *ExplorerService) InspectCreateName(id, operationID string) (operation.CreateObservation, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.CreateObservation{}, err
	}
	defer done()
	return s.core.creation.InspectCreateName(ctx, operationID)
}
