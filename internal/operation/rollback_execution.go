/*
Purpose: Confirm a reviewed workload rollback exactly once.
Used by: The desktop rollback bridge.
Responsibilities: Revalidate the source and parent, then classify one attempt.
Important: A rejected or ambiguous dispatch never triggers another write.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sandjaie/KubeSift/internal/model"
)

func (s *RollbackService) RevokeRollback(token string) {
	s.coordinator.revoke("rollback", token)
}

func (s *RollbackService) ExecuteRollback(token string) (OperationResult, error) {
	review, result, err := s.coordinator.begin("rollback", token, "")
	if review == nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(s.coordinator.ctx, operationDeadline)
	defer cancel()
	result.State, result.Problem = s.runRollback(ctx, review.payload.(rollbackReview))
	return s.coordinator.finish(result), nil
}

func (s *RollbackService) runRollback(ctx context.Context, review rollbackReview) (string, *model.Problem) {
	req := review.request
	path, sourcePath := rollbackPaths(req)
	client, err := newOperationClient(s.connections, req.Resource.Connection)
	if err != nil {
		return "cancelled-before-send", rollbackProblem(err, false)
	}
	if _, err := readRollbackSource(ctx, client, sourcePath, req); err != nil {
		return "rejected", rollbackProblem(err, false)
	}
	parent, err := readRollbackObject(ctx, client, path)
	if err == nil {
		err = checkRollbackParent(parent, req.Resource, req.ResourceVersion)
	}
	if err != nil {
		return "rejected", rollbackProblem(err, false)
	}
	if ctx.Err() != nil || s.connections.Validate(req.Resource.Connection) != nil {
		return "cancelled-before-send", &model.Problem{Code: "cancelled", Message: "Rollback was cancelled before submission."}
	}
	var raw json.RawMessage
	_, err = client.patch(ctx, path, false, review.patch, &raw)
	if err == nil {
		if checkRollbackParent(raw, req.Resource, "") == nil &&
			rollbackField(rollbackField(rollbackObject(raw), "spec"), "template") != nil {
			return "accepted", nil
		}
	} else {
		var status operationHTTPStatus
		if errors.As(err, &status) && status >= 400 && status < 500 && status != http.StatusRequestTimeout && status != 499 {
			p := rollbackWriteProblem(ctx, client, path, req, err, false)
			switch p.Code {
			case "stale_target":
				return "stale", p
			case "forbidden":
				return "rejected", p
			default:
				return "rejected", &model.Problem{Code: "rejected", Message: "Kubernetes rejected the rollback patch."}
			}
		}
	}
	return "unknown", &model.Problem{Code: "outcome_unknown", Message: "The rollback may have been saved. Refresh the original workload before reviewing another change."}
}
