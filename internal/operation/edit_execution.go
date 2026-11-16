/*
Purpose: Submit the reviewed patch once and classify the server outcome.
Used by: The native edit service and desktop operation results.
Responsibilities: Recheck target identity, dispatch the exact bytes, and preserve safe outcome summaries.
Important: A lost or ambiguous response never triggers another write.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

func checkEditVersion(ctx context.Context, client operationClient, target kube.EditTarget, expected string) error {
	var object json.RawMessage
	if _, err := client.call(ctx, http.MethodGet, target.Path(), false, nil, &object); err != nil {
		return editProblem(err, editFeedback{}, false)
	}
	version, err := target.ReadVersion(object)
	if err != nil {
		return err
	}
	if version != expected {
		return staleEdit()
	}
	return nil
}

func staleEdit() *model.Problem {
	return &model.Problem{Code: "stale_target", Message: "The resource changed or was replaced. Refresh it and review again."}
}

func editProblem(err error, feedback editFeedback, dry bool) *model.Problem {
	var status operationHTTPStatus
	if errors.As(err, &status) {
		switch status {
		case 401, 403:
			return &model.Problem{Code: "forbidden", Message: "Kubernetes denied this request with the current credentials."}
		case 404, 409, 410:
			return staleEdit()
		case 405, 415, 501:
			return &model.Problem{Code: "unsupported", Message: "This API does not support the requested JSON Patch or dry-run."}
		case 400, 422:
			if feedback.immutable {
				return &model.Problem{Code: "immutable", Message: "Kubernetes rejected a change to an immutable field. Check the proposed diff."}
			}
			if dry && status == 400 && feedback.dryRunUnsupported {
				return &model.Problem{Code: "unsupported", Message: "This API rejected dry-run. Editing requires a successful server preview."}
			}
			return &model.Problem{Code: "invalid", Message: "Kubernetes rejected the proposed fields or their values. Check the diff and the resource schema."}
		case 429:
			return &model.Problem{Code: "busy", Message: "Kubernetes rejected the request because it is rate limited."}
		}
		return &model.Problem{Code: "failed", Message: "Kubernetes did not return a usable response."}
	}
	problem := kube.ClassifyError(context.Background(), err)
	if problem == nil {
		return &model.Problem{Code: "failed", Message: "Kubernetes did not return a usable response."}
	}
	return problem
}

func editWriteProblem(ctx context.Context, client operationClient, target kube.EditTarget, version string, err error, feedback editFeedback, dry bool) *model.Problem {
	problem := editProblem(err, feedback, dry)
	var status operationHTTPStatus
	if errors.As(err, &status) && status == 422 {
		if recheck := checkEditVersion(ctx, client, target, version); isStaleEdit(recheck) {
			return staleEdit()
		}
	}
	return problem
}

func isStaleEdit(err error) bool {
	var problem *model.Problem
	return errors.As(err, &problem) && (problem.Code == "stale_target" || problem.Code == "resource_replaced")
}

func (s *EditService) ExecuteEdit(token string) (OperationResult, error) {
	c := s.coordinator
	prepared, result, err := c.begin("edit", token, "")
	if prepared == nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(c.ctx, operationDeadline)
	defer cancel()
	target, patch := *result.Edit, prepared.payload.(editReview)
	result.State, result.Problem = s.runEdit(ctx, target, patch)
	return c.finish(result), nil
}

func (s *EditService) runEdit(ctx context.Context, target kube.EditTarget, review editReview) (string, *model.Problem) {
	client, err := newOperationClient(s.connections, target.Connection)
	if err != nil {
		return "cancelled-before-send", editProblem(err, editFeedback{}, false)
	}
	if err := checkEditVersion(ctx, client, target, review.version); err != nil {
		if isStaleEdit(err) {
			return "stale", staleEdit()
		}
		problem := editProblem(err, editFeedback{}, false)
		if problem.Code == "forbidden" {
			return "rejected", problem
		}
		return "cancelled-before-send", problem
	}
	var raw json.RawMessage
	var feedback editFeedback
	_, err = client.editPatch(ctx, target.Path(), false, review.patch, &raw, &feedback)
	if err == nil {
		if _, err := target.ReadVersion(raw); err == nil {
			return "accepted", nil
		}
	} else {
		problem := editWriteProblem(ctx, client, target, review.version, err, feedback, false)
		switch problem.Code {
		case "stale_target":
			return "stale", problem
		case "forbidden", "invalid", "immutable", "busy", "unsupported":
			return "rejected", problem
		}
	}
	return "unknown", &model.Problem{Code: "outcome_unknown", Message: "The edit may have been saved. Refresh and inspect the resource before reviewing another change."}
}
