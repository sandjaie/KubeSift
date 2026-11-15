/*
Purpose: Dispatch one confirmed scale and retain target-bound feedback.
Used by: The scaling bridge and operation tests.
Responsibilities: Consume tokens atomically, revalidate preconditions and classify uncertain writes.
Important: Result lookup and duplicate confirmation cannot submit another write.
*/
package operation

import (
	"context"
	"errors"
	"net/http"

	"github.com/sandjaie/KubeSift/internal/model"
)

func scaleResult(result OperationResult) ScaleResult {
	if result.Scale == nil {
		return ScaleResult{}
	}
	return ScaleResult{Cluster: result.Cluster, OperationID: result.OperationID, Resource: result.Resource, Before: result.Scale.Before,
		Requested: result.Scale.Requested, Reported: result.Scale.Reported, StartedAt: result.StartedAt, FinishedAt: result.FinishedAt,
		State: result.State, Problem: result.Problem}
}

func (s *ScaleService) ExecuteScale(_ context.Context, token, acknowledgement string) (ScaleResult, error) {
	review, result, err := s.coordinator.begin("scale", token, acknowledgement)
	if review == nil {
		return scaleResult(result), err
	}
	ctx, cancel := context.WithTimeout(s.coordinator.ctx, operationDeadline)
	defer cancel()
	result.State, result.Scale.Reported, result.Problem = s.runScale(ctx, review.payload.(ScalePreview))
	return scaleResult(s.coordinator.finish(result)), nil
}

func (s *ScaleService) runScale(ctx context.Context, review ScalePreview) (string, *int32, *model.Problem) {
	cancelled := &model.Problem{Code: "cancelled", Message: "Scale was cancelled before submission"}
	stale := &model.Problem{Code: "stale_target", Message: "Workload changed. Refresh and prepare a new review."}
	ref := review.Resource
	path, _ := scaleTarget(ref)
	client, err := newOperationClient(s.connections, ref.Connection)
	if err != nil {
		return "cancelled-before-send", nil, cancelled
	}
	before, impact, err := client.inspect(ctx, path, ref, review.SourceVersion)
	if ctx.Err() != nil || s.connections.Validate(ref.Connection) != nil {
		return "cancelled-before-send", nil, cancelled
	}
	if err != nil {
		problem := scaleReadProblem(err)
		if problem.Code == "stale_target" {
			return "stale", nil, stale
		}
		if problem.Code == "forbidden" {
			return "rejected", nil, problem
		}
		return "cancelled-before-send", nil, problem
	}
	impact.Direction = scaleDirection(before, review.After)
	if before != review.Before || impact != review.Impact {
		return "stale", nil, stale
	}
	var response scalePayload
	_, err = client.call(ctx, http.MethodPut, path+"/scale", false, scaleInput(review), &response)
	if err == nil && matchingScale(response, ref) {
		count := response.Spec.desired()
		return "accepted", &count, nil
	}
	var status operationHTTPStatus
	if errors.As(err, &status) {
		switch status {
		case 404, 409, 410:
			return "stale", nil, stale
		case 400, 401, 403, 405, 415, 422, 429:
			return "rejected", nil, &model.Problem{Code: "rejected", Message: "Kubernetes rejected the scaling request"}
		}
	}
	var problem *model.Problem
	if errors.As(err, &problem) && (problem.Code == "stale_target" || problem.Code == "trust_required") {
		return "cancelled-before-send", nil, cancelled
	}
	return "unknown", nil, &model.Problem{Code: "uncertain", Message: "The scale outcome is unknown. Refresh the target to inspect its current state."}
}

func (s *ScaleService) RevokeScale(token string) { s.coordinator.revoke("scale", token) }
func (s *ScaleService) GetScaleResult(id string) (ScaleResult, error) {
	result, err := s.coordinator.GetOperationResult(id)
	if err != nil || result.Scale == nil {
		return ScaleResult{}, model.Fail("not_found", "Scale result is unavailable")
	}
	return scaleResult(result), nil
}
func (s *ScaleService) ListScaleResults() []ScaleResult {
	results := []ScaleResult{}
	for _, result := range s.coordinator.ListOperationResults() {
		if result.Scale != nil {
			results = append(results, scaleResult(result))
		}
	}
	return results
}
func (s *ScaleService) Close() { s.coordinator.Close() }
