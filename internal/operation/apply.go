/*
Purpose: Prepare one existing resource for declarative server-side apply.
Used by: The desktop reviewed YAML workflow.
Responsibilities: Freeze the target, validate intent and review the API server's preview.
Important: Only an earlier conflict for the same draft text can authorize forced review.
*/
package operation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

type ApplyService struct {
	connections *connection.Manager
	reader      *kube.Service
	coordinator *Coordinator
}

func NewApplyService(connections *connection.Manager, reader *kube.Service, coordinator *Coordinator) *ApplyService {
	return &ApplyService{connections, reader, coordinator}
}

func applyIdentity(target kube.EditTarget) map[string]any {
	metadata := map[string]any{"name": target.Name}
	if target.Namespace != "" {
		metadata["namespace"] = target.Namespace
	}
	return map[string]any{"apiVersion": target.Ref().APIVersion, "kind": target.Kind, "metadata": metadata}
}

func applyProjection(raw []byte) (map[string]any, error) {
	object, err := yamlValueProjection(raw)
	if err != nil {
		return nil, err
	}
	metadata := object["metadata"].(map[string]any)
	delete(metadata, "uid")
	delete(metadata, "resourceVersion")
	return object, nil
}

func (s *ApplyService) OpenApply(ctx context.Context, target kube.EditTarget) (ApplyDraft, error) {
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return ApplyDraft{}, err
	}
	defer release()
	baseline, err := s.reader.CaptureEdit(ctx, target)
	if err != nil {
		return ApplyDraft{}, err
	}
	cluster, err := s.connections.ContextName(target.Connection)
	if err != nil {
		return ApplyDraft{}, err
	}
	object, err := applyProjection(baseline.ObjectJSON)
	if err != nil {
		return ApplyDraft{}, err
	}
	reference, err := renderEdit(object)
	if err != nil {
		return ApplyDraft{}, err
	}
	desired, err := renderEdit(applyIdentity(baseline.Target))
	if err != nil {
		return ApplyDraft{}, err
	}
	draft := ApplyDraft{EditDraft: EditDraft{ID: model.ID(), Target: baseline.Target, ResourceVersion: baseline.ResourceVersion, Cluster: cluster, YAML: desired}, BaselineYAML: reference}
	if err := checkApplySize(draft); err != nil {
		return ApplyDraft{}, err
	}
	if err := s.coordinator.storeDraft(ctx, draft.ID, operationDraft{action: "apply", connection: target.Connection, baseline: baseline, cluster: cluster}); err != nil {
		return ApplyDraft{}, err
	}
	return draft, nil
}

func checkApplySize(value any) error {
	if raw, err := json.Marshal(value); err != nil || len(raw)+128 > editLimit {
		return model.Fail("capacity", "The combined apply content exceeds the 2 MiB limit")
	}
	return nil
}

func buildApplyPayload(capture kube.EditCapture, input string) (string, []byte, error) {
	object, err := parseEditYAML(input)
	if err != nil {
		return "", nil, err
	}
	target := capture.Target
	metadata, ok := object["metadata"].(map[string]any)
	if !ok || object["apiVersion"] != target.Ref().APIVersion || object["kind"] != target.Kind || metadata["name"] != target.Name {
		return "", nil, model.Fail("identity_changed", "Keep the existing resource API, kind and name unchanged")
	}
	if namespace, present := metadata["namespace"]; present && (target.Namespace == "" || namespace != target.Namespace) {
		return "", nil, model.Fail("identity_changed", "Keep the existing resource namespace unchanged")
	}
	if target.Namespace != "" {
		metadata["namespace"] = target.Namespace
	}
	for key, expected := range map[string]string{"uid": target.UID, "resourceVersion": capture.ResourceVersion} {
		if value, present := metadata[key]; present && value != expected {
			return "", nil, model.Fail("identity_changed", "Remove UID and resourceVersion or keep the captured values")
		}
		delete(metadata, key)
	}
	if _, present := object["status"]; present {
		return "", nil, model.Fail("invalid_yaml", "Remove the server-owned status from the configuration")
	}
	for _, key := range append(serverEditMetadata[:len(serverEditMetadata):len(serverEditMetadata)], "generateName") {
		if _, present := metadata[key]; present {
			return "", nil, model.Fail("invalid_yaml", "Remove server-owned metadata and generateName from the configuration")
		}
	}
	if err := normalizeSecretEdit(target, object); err != nil {
		return "", nil, err
	}
	desired, err := renderEdit(object)
	if err != nil {
		return "", nil, err
	}
	metadata["uid"], metadata["resourceVersion"] = target.UID, capture.ResourceVersion
	body, err := json.Marshal(object)
	if err != nil || len(body) > editLimit {
		return "", nil, model.Fail("capacity", "The guarded apply document exceeds the 2 MiB limit")
	}
	return desired, body, nil
}

type applyReviewPayload struct {
	version     string
	body        []byte
	previewHash [32]byte
	previewSize int
}

func (s *ApplyService) PrepareApply(ctx context.Context, req ApplyRequest) (ApplyReview, error) {
	c := s.coordinator
	ctx, release, err := c.prepare(ctx)
	if err != nil {
		return ApplyReview{}, err
	}
	defer release()
	draft, err := c.reviseDraft("apply", req.DraftID)
	if err != nil {
		return ApplyReview{}, err
	}
	digest := sha256.Sum256([]byte(req.YAML))
	if req.TakeOwnership && (draft.applyConflict == nil || *draft.applyConflict != digest) {
		return ApplyReview{}, model.Fail("conflict_review_required", "Review this configuration's ownership conflicts before choosing takeover.")
	}
	proposed, body, err := buildApplyPayload(draft.baseline, req.YAML)
	if err != nil {
		return ApplyReview{}, err
	}
	target := draft.baseline.Target
	version := draft.baseline.ResourceVersion
	client, err := newOperationClient(s.connections, target.Connection)
	if err != nil {
		return ApplyReview{}, err
	}
	if err := checkEditVersion(ctx, client, target, version); err != nil {
		return ApplyReview{}, err
	}
	var raw json.RawMessage
	var feedback editFeedback
	var status operationStatus
	warnings, err := client.applyPatch(ctx, target.Path(), true, req.TakeOwnership, body, &raw, &feedback, &status)
	if err != nil {
		if conflicts := applyConflicts(status); !req.TakeOwnership && len(conflicts) != 0 {
			if err := checkEditVersion(ctx, client, target, version); err != nil {
				return ApplyReview{}, err
			}
			expires, err := c.rememberApplyConflict(ctx, req.DraftID, draft.revision, digest)
			if err != nil {
				return ApplyReview{}, err
			}
			return ApplyReview{EditPreview: EditPreview{Target: target, ExpiresAt: expires}, Conflicts: conflicts}, nil
		}
		return ApplyReview{}, applyProblem(ctx, client, target, version, err, feedback, true)
	}
	if _, err := target.ReadVersion(raw); err != nil {
		return ApplyReview{}, err
	}
	review := ApplyReview{EditPreview: EditPreview{Target: target, Warnings: warnings, ProposedYAML: proposed}, Force: req.TakeOwnership}
	for _, value := range []struct {
		raw  []byte
		dest *string
	}{{draft.baseline.ObjectJSON, &review.BeforeYAML}, {raw, &review.ServerYAML}} {
		projected, err := applyProjection(value.raw)
		if err != nil {
			return ApplyReview{}, err
		}
		*value.dest, err = renderEdit(projected)
		if err != nil {
			return ApplyReview{}, err
		}
	}
	review.Ownership, review.OwnershipIncomplete, review.OwnershipTruncated = applyOwnership(draft.baseline.ObjectJSON, raw, target.Ref().APIVersion)
	if err := checkApplySize(review); err != nil {
		return ApplyReview{}, err
	}
	review.Token, review.ExpiresAt, err = c.save(ctx, pendingReview{
		result:  OperationResult{Action: "apply", Cluster: draft.cluster, Resource: target.Ref(), Apply: &ApplyResultData{Target: target, Manager: applyManager, Force: req.TakeOwnership}},
		payload: applyReviewPayload{version: version, body: body, previewHash: sha256.Sum256([]byte(review.ServerYAML)), previewSize: len(review.ServerYAML)}, draftID: req.DraftID, revision: draft.revision,
	})
	if err != nil {
		return ApplyReview{}, err
	}
	return review, nil
}

func (s *ApplyService) RevokeApply(token string) { s.coordinator.revoke("apply", token) }
func (s *ApplyService) CloseApply(id string)     { s.coordinator.closeDraft("apply", id) }

func (c *Coordinator) rememberApplyConflict(ctx context.Context, id string, revision uint64, digest [32]byte) (time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	draft, ok := c.captures[id]
	if ctx.Err() != nil || c.ctx.Err() != nil || !ok || draft.action != "apply" || draft.revision != revision || !c.now().Before(draft.expires) {
		return time.Time{}, model.Fail("expired", "The editor capture changed or expired")
	}
	draft.applyConflict = &digest
	c.captures[id] = draft
	return draft.expires, nil
}

func applyProblem(ctx context.Context, client operationClient, target kube.EditTarget, version string, err error, feedback editFeedback, dry bool) *model.Problem {
	var status operationHTTPStatus
	if errors.As(err, &status) {
		switch status {
		case http.StatusCreated:
			return &model.Problem{Code: "unsupported_safety", Message: "This API returned a creation response for existing-resource apply. KubeSift cannot safely apply to this endpoint."}
		case 405, 415, 501:
			return &model.Problem{Code: "unsupported", Message: "This API does not support the requested server-side apply."}
		case 400, 422:
			if feedback.dryRunUnsupported || feedback.strictUnsupported {
				return &model.Problem{Code: "unsupported", Message: "This API does not support strict server-side apply review."}
			}
		case 409:
			if recheck := checkEditVersion(ctx, client, target, version); isStaleEdit(recheck) {
				return staleEdit()
			}
			return &model.Problem{Code: "conflict", Message: "Kubernetes rejected the apply with a conflict. Inspect the resource and review again."}
		}
	}
	return editWriteProblem(ctx, client, target, version, err, feedback, dry)
}
