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

func (s *ExplorerService) WorkflowDiscovery(id string, ref model.ConnectionRef, refresh bool) (kube.WorkflowDiscovery, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.WorkflowDiscovery{}, err
	}
	defer done()
	return s.core.kube.WorkflowDiscovery(ctx, ref, refresh)
}
func (s *ExplorerService) ReadWorkflow(id string, q kube.WorkflowQuery) (kube.WorkflowPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.WorkflowPage{}, err
	}
	defer done()
	return s.core.kube.ReadWorkflow(ctx, q)
}

func (s *ExplorerService) ReadRouteService(id string, q kube.WorkflowQuery, name string) (kube.RouteService, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.RouteService{}, err
	}
	defer done()
	return s.core.kube.ReadRouteService(ctx, q, name)
}

func (s *ExplorerService) PrepareRoute(id string, r operation.RouteRequest) (operation.WorkflowPreview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.WorkflowPreview{}, err
	}
	defer done()
	env, err := s.connectionEnvironment(r.Target.Connection)
	if err != nil {
		return operation.WorkflowPreview{}, err
	}
	return s.core.creation.PrepareRoute(ctx, r, env)
}
func (s *ExplorerService) ExecuteRoute(action, token, ack string) (operation.OperationResult, error) {
	return s.core.creation.ExecuteRoute(action, token, ack)
}
func (s *ExplorerService) RevokeRoute(action, token string) {
	s.core.creation.RevokeRoute(action, token)
}

func (s *ExplorerService) PrepareBuildAction(id string, r operation.BuildActionRequest) (operation.WorkflowPreview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.WorkflowPreview{}, err
	}
	defer done()
	env, err := s.connectionEnvironment(r.Target.Connection)
	if err != nil {
		return operation.WorkflowPreview{}, err
	}
	return s.core.creation.PrepareBuildAction(ctx, r, env)
}
func (s *ExplorerService) ExecuteBuildAction(action, token, ack string) (operation.OperationResult, error) {
	return s.core.creation.ExecuteBuildAction(action, token, ack)
}
func (s *ExplorerService) RevokeBuildAction(action, token string) {
	s.core.creation.RevokeBuildAction(action, token)
}

func (s *ExplorerService) PrepareOperator(id string, r operation.OperatorRequest) (operation.WorkflowPreview, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return operation.WorkflowPreview{}, err
	}
	defer done()
	return s.core.creation.PrepareOperator(ctx, r)
}
func (s *ExplorerService) ExecuteOperator(action, token, ack string) (operation.OperationResult, error) {
	return s.core.creation.ExecuteOperator(action, token, ack)
}
func (s *ExplorerService) RevokeOperator(action, token string) {
	s.core.creation.RevokeOperator(action, token)
}

func (s *ExplorerService) ReadWorkflowLink(id string, q kube.WorkflowQuery, l kube.WorkflowLink) (kube.WorkflowItem, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.WorkflowItem{}, err
	}
	defer done()
	return s.core.kube.ReadWorkflowLink(ctx, q, l)
}

func (s *ExplorerService) ReadOperatorInstance(id string, q kube.WorkflowQuery, a kube.OperatorAPI, ns, name string) (kube.WorkflowItem, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.WorkflowItem{}, err
	}
	defer done()
	return s.core.kube.ReadOperatorInstance(ctx, q, a, ns, name)
}
