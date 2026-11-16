/*
Purpose: Dispatch the prepared named POST once and reconcile names on explicit request.
Used by: The native create service and desktop result bridge.
Responsibilities: Classify actual server identity, definitive rejection and unresolved outcomes.
Important: Dry-run identities and observed same-name objects never become created identities.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sandjaie/KubeSift/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *CreateService) ExecuteCreate(token string) (OperationResult, error) {
	pending, result, err := s.coordinator.begin("create", token, "")
	if pending == nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(s.coordinator.ctx, operationDeadline)
	defer cancel()
	s.runCreate(ctx, &result, pending.payload.([]byte))
	return s.coordinator.finish(result), nil
}

func (s *CreateService) runCreate(ctx context.Context, result *OperationResult, payload []byte) {
	target := result.Create.Target
	result.State = "cancelled-before-send"
	address, err := s.reader.ResolveManifestTarget(ctx, target.Connection, result.Resource.APIVersion, target.Kind, target.Resource, target.Namespace, target.Name, "create")
	if err != nil || address != target {
		result.Problem = &model.Problem{Code: "stale_target", Message: "The reviewed connection or API is unavailable. Reconcile the target and review again."}
		return
	}
	client, err := newOperationClient(s.connections, target.Connection)
	if err != nil || ctx.Err() != nil {
		result.Problem = &model.Problem{Code: "cancelled", Message: "The create was cancelled before submission"}
		return
	}
	var raw json.RawMessage
	_, err = client.createPost(ctx, target.CollectionPath(), false, payload, &raw, nil)
	if err == nil {
		uid, version, identityErr := target.ReadIdentity(raw)
		if identityErr == nil && version != "" && len(version) <= 128 {
			result.Resource.UID = uid
			result.Create.ResourceVersion = version
			result.State = "accepted"
			return
		}
	}
	var status operationHTTPStatus
	if errors.As(err, &status) && status >= 400 && status < 500 && status != http.StatusRequestTimeout && status != 499 {
		result.State = "rejected"
		if status == http.StatusConflict {
			result.Problem = &model.Problem{Code: "already_exists", Message: "A resource with this name already exists"}
		} else {
			result.Problem = &model.Problem{Code: "rejected", Message: "Kubernetes rejected the create request"}
		}
		return
	}
	var safe *model.Problem
	if errors.As(err, &safe) && (safe.Code == "stale_target" || safe.Code == "trust_required") {
		result.Problem = &model.Problem{Code: "cancelled", Message: "The create was cancelled before submission"}
		return
	}
	result.State = "unknown"
	result.Problem = &model.Problem{Code: "uncertain", Message: "The create outcome is unknown. Inspect the name and reconcile the draft before reviewing again."}
}

func (s *CreateService) InspectCreateName(ctx context.Context, id string) (CreateObservation, error) {
	result, err := s.coordinator.GetOperationResult(id)
	if err != nil || result.Create == nil || result.Action != "create" || result.FinishedAt == nil ||
		(result.State != "accepted" && result.State != "unknown") {
		return CreateObservation{}, model.Fail("not_found", "The accepted or uncertain create result is unavailable")
	}
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return CreateObservation{}, err
	}
	defer release()
	target := result.Create.Target
	client, err := newOperationClient(s.connections, target.Connection)
	if err != nil {
		return CreateObservation{}, err
	}
	var raw json.RawMessage
	var missing operationStatus
	secret := target.Group == "" && target.Resource == "secrets"
	_, err = client.observeCreate(ctx, target.Path(), secret, &raw, &missing)
	obs := CreateObservation{OperationID: id, CheckedAt: s.coordinator.now().UTC()}
	if errors.Is(err, operationHTTPStatus(http.StatusNotFound)) && objectNotFound(missing, target.Name, target.Group, target.Resource) {
		obs.State = "absent"
		return obs, nil
	}
	var uid, version string
	if err == nil {
		if secret {
			var metadata metav1.PartialObjectMetadata
			err = json.Unmarshal(raw, &metadata)
			if err == nil && metadata.APIVersion == "meta.k8s.io/v1" && metadata.Kind == "PartialObjectMetadata" &&
				metadata.Name == target.Name && metadata.Namespace == target.Namespace {
				uid, version = string(metadata.UID), metadata.ResourceVersion
			}
		} else {
			uid, version, err = target.ReadIdentity(raw)
		}
	}
	switch {
	case err != nil || uid == "" || len(uid) > 128 || version == "" || len(version) > 128:
		obs.State = "unavailable"
		obs.Problem = &model.Problem{Code: "unavailable", Message: "Could not read the name on its original connection and API"}
	case result.Resource.UID != "" && result.Resource.UID != uid:
		obs.State = "replaced"
	default:
		obs.State = "present"
	}
	return obs, nil
}
