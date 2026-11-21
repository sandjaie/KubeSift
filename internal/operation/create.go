/*
Purpose: Prepare one named Kubernetes resource creation.
Used by: The desktop YAML editor and native regression tests.
Responsibilities: Normalize a lossless draft, resolve its route and require a matching server dry-run.
Important: Only the reviewed proposed payload is retained for confirmation.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/apimachinery/pkg/util/validation"
)

type CreateService struct {
	connections *connection.Manager
	reader      *kube.Service
	coordinator *Coordinator
}

func NewCreateService(connections *connection.Manager, reader *kube.Service, coordinator *Coordinator) *CreateService {
	return &CreateService{connections, reader, coordinator}
}

func (s *CreateService) OpenCreate(ctx context.Context, ref model.ConnectionRef, namespace string) (CreateDraft, error) {
	if namespace != "" && len(validation.IsDNS1123Label(namespace)) != 0 {
		return CreateDraft{}, model.Fail("invalid", "Select a namespace or cluster scope")
	}
	cluster, err := s.connections.ContextName(ref)
	if err != nil {
		return CreateDraft{}, err
	}
	if len(cluster) > 1024 {
		return CreateDraft{}, model.Fail("capacity", "Cluster context name exceeds the display limit")
	}
	draft := CreateDraft{ID: model.ID(), Connection: ref, Cluster: cluster, Namespace: namespace}
	if err := s.coordinator.storeDraft(ctx, draft.ID, operationDraft{action: "create", connection: ref, cluster: cluster}); err != nil {
		return CreateDraft{}, err
	}
	return draft, nil
}

func (s *CreateService) CloseCreate(id string)     { s.coordinator.closeDraft("create", id) }
func (s *CreateService) RevokeCreate(token string) { s.coordinator.revoke("create", token) }

func parseCreateDocument(yaml, namespace string) (map[string]any, string, string, string, error) {
	object, err := parseEditYAML(yaml)
	if err != nil {
		return nil, "", "", "", err
	}
	apiVersion, _ := object["apiVersion"].(string)
	kind, _ := object["kind"].(string)
	if kind == "List" {
		return nil, "", "", "", model.Fail("invalid_yaml", "Enter one named Kubernetes resource, outside a List wrapper")
	}
	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		return nil, "", "", "", model.Fail("invalid_yaml", "Enter an object with apiVersion, kind and metadata.name")
	}
	if _, exists := object["status"]; exists {
		return nil, "", "", "", model.Fail("invalid_yaml", "Remove status from the create document")
	}
	for _, key := range append([]string{"generateName", "uid", "resourceVersion"}, serverEditMetadata...) {
		if _, exists := metadata[key]; exists {
			return nil, "", "", "", model.Fail("invalid_yaml", "Remove metadata."+key+" from the create document")
		}
	}
	name, _ := metadata["name"].(string)
	if name == "" || apiVersion == "" || kind == "" {
		return nil, "", "", "", model.Fail("invalid_yaml", "Enter apiVersion, kind and an explicit metadata.name")
	}
	if ns, exists := metadata["namespace"]; exists && (namespace == "" || ns != namespace) {
		return nil, "", "", "", model.Fail("namespace_mismatch", "Reconcile metadata.namespace with the selected scope before reviewing")
	}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	return object, apiVersion, kind, name, nil
}

func (s *CreateService) PrepareCreate(ctx context.Context, req CreateRequest) (CreatePreview, error) {
	c := s.coordinator
	ctx, release, err := c.prepare(ctx)
	if err != nil {
		return CreatePreview{}, err
	}
	defer release()
	draft, err := c.reviseDraft("create", req.DraftID)
	if err != nil {
		return CreatePreview{}, err
	}
	object, apiVersion, kind, name, err := parseCreateDocument(req.YAML, req.Namespace)
	if err != nil {
		return CreatePreview{}, err
	}
	target, err := s.reader.ResolveManifestTarget(ctx, draft.connection, apiVersion, kind, req.Resource, req.Namespace, name, "create")
	if err != nil {
		return CreatePreview{}, err
	}
	if err = normalizeSecretEdit(kube.EditTarget{Group: target.Group, Version: target.Version, Resource: target.Resource}, object); err != nil {
		return CreatePreview{}, err
	}
	body, err := json.Marshal(object)
	if err != nil || len(body) > editLimit {
		return CreatePreview{}, model.Fail("capacity", "The normalized create payload exceeds the 2 MiB limit")
	}
	client, err := newOperationClient(s.connections, target.Connection)
	if err != nil {
		return CreatePreview{}, err
	}
	var serverRaw json.RawMessage
	var feedback editFeedback
	warnings, err := client.createPost(ctx, target.CollectionPath(), true, body, &serverRaw, &feedback)
	if err != nil {
		return CreatePreview{}, createDryProblem(err, feedback)
	}
	if _, _, err = target.ReadIdentity(serverRaw); err != nil {
		return CreatePreview{}, err
	}
	if len(serverRaw) > editLimit {
		return CreatePreview{}, model.Fail("capacity", "Server preview exceeds the 2 MiB limit")
	}
	admitted, err := editProjection(serverRaw)
	if err != nil {
		return CreatePreview{}, err
	}
	metadata := admitted["metadata"].(map[string]any)
	delete(metadata, "uid")
	delete(metadata, "resourceVersion")
	preview := CreatePreview{Target: target, Warnings: warnings}
	if preview.ProposedYAML, err = renderEdit(object); err != nil {
		return CreatePreview{}, err
	}
	if preview.ServerYAML, err = renderEdit(admitted); err != nil {
		return CreatePreview{}, err
	}
	if data, err := json.Marshal(preview); err != nil || len(data)+128 > editLimit {
		return CreatePreview{}, model.Fail("capacity", "The combined create review exceeds the 2 MiB limit")
	}
	preview.Token, preview.ExpiresAt, err = c.save(ctx, pendingReview{
		result:  OperationResult{Action: "create", Cluster: draft.cluster, Resource: target.Ref(), Create: &CreateResultData{Target: target}},
		draftID: req.DraftID, revision: draft.revision, payload: body,
	})
	if err != nil {
		return CreatePreview{}, err
	}
	return preview, nil
}

func createDryProblem(err error, feedback editFeedback) error {
	var code operationHTTPStatus
	if errors.As(err, &code) {
		switch code {
		case http.StatusConflict:
			return model.Fail("already_exists", "A resource with this name already exists")
		case http.StatusUnauthorized, http.StatusForbidden:
			return model.Fail("forbidden", "Kubernetes denied the mandatory create dry-run")
		case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnsupportedMediaType:
			return model.Fail("unsupported", "The advertised API does not support this strict dry-run create")
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			if feedback.dryRunUnsupported || feedback.strictUnsupported {
				return model.Fail("unsupported", "Kubernetes does not support the required dry-run or strict validation")
			}
			return model.Fail("invalid_manifest", "Kubernetes rejected the manifest during strict dry-run. Check required fields and types.")
		}
	}
	var safe *model.Problem
	if errors.As(err, &safe) {
		return safe
	}
	return model.Fail("dry_run_failed", "Mandatory create dry-run did not complete. No review was issued.")
}
