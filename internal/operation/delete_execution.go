/*
Purpose: Execute a confirmed deletion once.
Used by: The native operation flow and desktop bridge.
Responsibilities: Recheck eligibility, send original options and classify the server outcome.
Important: A lost or invalid success may still have deleted the target.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sandjaie/KubeSift/internal/model"
)

func (s *DeleteService) ExecuteDelete(_ context.Context, token, acknowledgement string) (OperationResult, error) {
	review, result, err := s.coordinator.begin("delete", token, acknowledgement)
	if review == nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(s.coordinator.ctx, operationDeadline)
	defer cancel()
	result.State, result.Problem = s.runDelete(ctx, result, review.payload.(deleteReview))
	return s.coordinator.finish(result), nil
}

func (s *DeleteService) RevokeDelete(token string) {
	s.coordinator.revoke("delete", token)
}

func (s *DeleteService) runDelete(ctx context.Context, result OperationResult, review deleteReview) (string, *model.Problem) {
	cancelled := &model.Problem{Code: "cancelled", Message: "Deletion was cancelled before submission"}
	stale := &model.Problem{Code: "stale_target", Message: "Workload changed or disappeared. Refresh and review again."}
	rejected := &model.Problem{Code: "rejected", Message: "Kubernetes rejected the deletion request"}
	unknown := &model.Problem{Code: "uncertain", Message: "The deletion outcome is unknown. Use Check target to inspect its current state."}
	ref := result.Resource
	path, plural := deleteTarget(ref)
	client, err := newOperationClient(s.connections, ref.Connection)
	if err != nil {
		return "cancelled-before-send", cancelled
	}
	_, err = client.inspectDelete(ctx, path, ref, review.version)
	if ctx.Err() != nil || s.connections.Validate(ref.Connection) != nil {
		return "cancelled-before-send", cancelled
	}
	if err != nil {
		problem := deleteProblem(err, false)
		switch problem.Code {
		case "stale_target", "mirror_pod", "unsafe_retention":
			return "stale", stale
		case "forbidden":
			return "rejected", problem
		default:
			return "cancelled-before-send", problem
		}
	}
	var raw json.RawMessage
	_, err = client.send(ctx, http.MethodDelete, path, false, review.options, "application/json", &raw, nil)
	if err == nil && validDeleteResponse(raw, ref, plural) {
		return "accepted", nil
	}
	var status operationHTTPStatus
	if errors.As(err, &status) {
		switch status {
		case 404, 409, 410:
			return "stale", stale
		case 400, 401, 403, 405, 406, 412, 413, 415, 422, 429:
			return "rejected", rejected
		}
	}
	var problem *model.Problem
	if errors.As(err, &problem) && (problem.Code == "stale_target" || problem.Code == "trust_required") {
		return "cancelled-before-send", cancelled
	}
	return "unknown", unknown
}
