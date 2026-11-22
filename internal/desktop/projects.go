/*
Purpose: Expose cancellable OpenShift Project reads and reviewed self-service requests.
Used by: The Projects desktop workspace.
Responsibilities: Capture the explicit connection and forward safe native calls.
Important: Confirmations accept only a saved token.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/operation"
)

func (s *ExplorerService) ProjectCapabilities(id string, ref model.ConnectionRef, refresh bool) (kube.ProjectCapabilities, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ProjectCapabilities{}, err
	}
	defer done()
	return s.core.kube.ProjectCapabilities(ctx, ref, refresh)
}
func (s *ExplorerService) ListProjects(id string, ref model.ConnectionRef, cursor string) (kube.ProjectPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ProjectPage{}, err
	}
	defer done()
	return s.core.kube.ListProjects(ctx, ref, cursor)
}
func (s *ExplorerService) ReadProject(id string, ref model.ConnectionRef, name, uid string) (kube.Project, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.Project{}, err
	}
	defer done()
	return s.core.kube.ReadProject(ctx, ref, name, uid)
}
func (s *ExplorerService) MyProjectAccess(id string, r kube.ResourceRef) (kube.ProjectAccess, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ProjectAccess{}, err
	}
	defer done()
	return s.core.kube.MyProjectAccess(ctx, r)
}
func (s *ExplorerService) ProjectQuotas(id string, r kube.ResourceRef, cursor string) (kube.ProjectQuotaPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ProjectQuotaPage{}, err
	}
	defer done()
	return s.core.kube.ProjectQuotas(ctx, r, cursor)
}
func (s *ExplorerService) PrepareProjectRequest(id string, ref model.ConnectionRef, f operation.ProjectFields) (operation.ProjectRequestPreview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.ProjectRequestPreview{}, err
	}
	defer done()
	env, err := s.connectionEnvironment(ref)
	if err != nil {
		return operation.ProjectRequestPreview{}, err
	}
	return s.core.creation.PrepareProjectRequest(ctx, ref, f, env)
}
func (s *ExplorerService) ExecuteProjectRequest(token, ack string) (operation.OperationResult, error) {
	return s.core.creation.ExecuteProjectRequest(token, ack)
}
func (s *ExplorerService) RevokeProjectRequest(token string) {
	s.core.creation.RevokeProjectRequest(token)
}
