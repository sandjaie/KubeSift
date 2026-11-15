/*
Purpose: Execute one confirmed restart and classify its outcome.
Used by: The native service and desktop bridge.
Responsibilities: Recheck the reviewed target and send the original conditional patch once.
Important: Response loss or an invalid success is uncertain and cannot trigger replay.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sandjaie/KubeSift/internal/model"
)

func (s *RestartService) ExecuteRestart(_ context.Context, token, acknowledgement string) (OperationResult, error) {
	review, result, err := s.coordinator.begin("restart", token, acknowledgement)
	if review == nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(s.coordinator.ctx, operationDeadline)
	defer cancel()
	result.State, result.Restart.ResponseDiffers, result.Problem = s.runRestart(ctx, result, review.payload.(restartReview))
	return s.coordinator.finish(result), nil
}

func (s *RestartService) RevokeRestart(token string) {
	s.coordinator.revoke("restart", token)
}

func (s *RestartService) runRestart(ctx context.Context, result OperationResult, review restartReview) (string, bool, *model.Problem) {
	cancelled := &model.Problem{Code: "cancelled", Message: "Restart was cancelled before submission"}
	stale := &model.Problem{Code: "stale_target", Message: "Workload changed. Refresh and prepare a new restart review."}
	rejected := &model.Problem{Code: "rejected", Message: "Kubernetes rejected the restart request"}
	unknown := &model.Problem{Code: "uncertain", Message: "The restart outcome is unknown. Refresh the target to inspect its current state."}
	ref := result.Resource
	path, _ := workloadTarget(ref)
	client, err := newOperationClient(s.connections, ref.Connection)
	if err != nil {
		return "cancelled-before-send", false, cancelled
	}
	parent, _, err := client.inspectRestart(ctx, path, ref, review.version)
	if ctx.Err() != nil || s.connections.Validate(ref.Connection) != nil {
		return "cancelled-before-send", false, cancelled
	}
	if err != nil {
		problem := restartProblem(err, false)
		switch problem.Code {
		case "stale_target", "unsupported_strategy":
			return "stale", false, stale
		case "forbidden":
			return "rejected", false, problem
		default:
			return "cancelled-before-send", false, problem
		}
	}
	current, err := specHash(parent.spec)
	if err != nil || current != review.liveHash {
		return "stale", false, stale
	}
	var raw json.RawMessage
	if ctx.Err() != nil {
		return "cancelled-before-send", false, cancelled
	}
	_, err = client.patch(ctx, path, false, review.patch, &raw)
	if err == nil {
		response, err := parseRestartObject(raw)
		if err != nil || !response.matches(ref, "") || response.annotations[restartAnnotation] != result.Restart.RequestedAt {
			return "unknown", false, unknown
		}
		hash, err := specHash(response.spec)
		return "accepted", err != nil || hash != review.hash, nil
	}
	var status operationHTTPStatus
	if errors.As(err, &status) {
		switch status {
		case 404, 409, 410:
			return "stale", false, stale
		case 422:
			// The API does not reliably distinguish a failed test operation from validation.
			_, _, after := client.inspectRestart(ctx, path, ref, review.version)
			if after != nil && restartProblem(after, false).Code == "stale_target" {
				return "stale", false, stale
			}
			return "rejected", false, rejected
		}
		// Timeout or client disconnect responses may describe an upstream wait, not a decision.
		if status >= 400 && status < 500 && status != http.StatusRequestTimeout && status != 499 {
			return "rejected", false, rejected
		}
	}
	var problem *model.Problem
	if errors.As(err, &problem) && (problem.Code == "stale_target" || problem.Code == "trust_required") {
		return "cancelled-before-send", false, cancelled
	}
	return "unknown", false, unknown
}
