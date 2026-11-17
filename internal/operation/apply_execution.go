/*
Purpose: Submit one reviewed server-side apply to an existing resource.
Used by: The desktop operation bridge and operation results.
Responsibilities: Recheck the frozen identity, dispatch exact bytes and classify the outcome.
Important: A lost response or unexpected creation is never replayed or rolled back.
*/
package operation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

func (s *ApplyService) ExecuteApply(token string) (OperationResult, error) {
	out, err := s.ExecuteApplyObserved(token)
	return out.OperationResult, err
}

func (s *ApplyService) ExecuteApplyObserved(token string) (ApplyExecution, error) {
	c := s.coordinator
	prepared, result, err := c.begin("apply", token, "")
	if prepared == nil {
		return ApplyExecution{OperationResult: result}, err
	}
	ctx, cancel := context.WithTimeout(c.ctx, operationDeadline)
	defer cancel()
	var applied json.RawMessage
	review := prepared.payload.(applyReviewPayload)
	result.State, result.Problem = s.runApply(ctx, *result.Apply, review, &applied)
	out := ApplyExecution{OperationResult: c.finish(result)}
	if out.State != "accepted" {
		return out, nil
	}
	object, err := applyProjection(applied)
	if err != nil {
		out.ResponseChanged = true
		return out, nil
	}
	observed, err := renderEdit(object)
	out.ResponseChanged = err != nil || sha256.Sum256([]byte(observed)) != review.previewHash
	if err == nil && out.ResponseChanged {
		// Show the observed values only when both sides fit the review budget.
		rendered, _ := json.Marshal(observed)
		if len(rendered)+review.previewSize+1024 <= editLimit {
			out.ObservedYAML = observed
		}
	}
	return out, nil
}

func (s *ApplyService) runApply(ctx context.Context, info ApplyResultData, review applyReviewPayload, applied *json.RawMessage) (string, *model.Problem) {
	target := info.Target
	address, err := s.reader.ResolveManifestTarget(ctx, target.Connection, target.Ref().APIVersion, target.Kind, target.Resource, target.Namespace, target.Name, "patch")
	if err != nil {
		return "cancelled-before-send", editProblem(err, editFeedback{}, false)
	}
	if address.Path() != target.Path() {
		return "stale", staleEdit()
	}
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
	if ctx.Err() != nil {
		return "cancelled-before-send", &model.Problem{Code: "cancelled", Message: "The apply was cancelled before submission."}
	}
	var raw json.RawMessage
	var feedback editFeedback
	_, err = client.applyPatch(ctx, target.Path(), false, info.Force, review.body, &raw, &feedback, nil)
	if err == nil {
		if _, err := target.ReadVersion(raw); err == nil {
			*applied = raw
			return "accepted", nil
		}
	}
	var status operationHTTPStatus
	if errors.As(err, &status) {
		switch status {
		case http.StatusCreated:
			return "unknown", applyProblem(ctx, client, target, review.version, err, feedback, false)
		case 400, 401, 403, 404, 405, 409, 410, 415, 422, 429:
			problem := applyProblem(ctx, client, target, review.version, err, feedback, false)
			if isStaleEdit(problem) {
				return "stale", staleEdit()
			}
			return "rejected", problem
		}
	}
	return "unknown", &model.Problem{Code: "outcome_unknown", Message: "The apply may have reached Kubernetes. Inspect the original target before reviewing again."}
}

func (s *ApplyService) ReadAppliedDetail(ctx context.Context, id string) (kube.GenericResourceDetail, error) {
	result, err := s.coordinator.GetOperationResult(id)
	if err != nil || result.Action != "apply" || result.Apply == nil || result.FinishedAt == nil || result.Resource.UID == "" {
		return kube.GenericResourceDetail{}, model.Fail("not_found", "The original apply target is unavailable")
	}
	target := result.Apply.Target
	address := kube.ManifestAddress{Connection: target.Connection, Group: target.Group, Version: target.Version,
		Resource: target.Resource, Kind: target.Kind, Namespace: target.Namespace, Name: target.Name}
	return s.reader.ReadCreatedDetail(ctx, address, result.Resource.UID)
}
