/*
Purpose: Own the native baseline and review for one existing object.
Used by: The desktop edit flow and API integration tests.
Responsibilities: Capture bounded editable YAML and share expiry with other operations.
Important: The baseline and patch stay in memory and never enter operation history.
*/
package operation

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

const editLimit = 2 << 20

type EditService struct {
	connections *connection.Manager
	reader      *kube.Service
	coordinator *Coordinator
}

type operationDraft struct {
	action        string
	connection    model.ConnectionRef
	baseline      kube.EditCapture
	cluster       string
	expires       time.Time
	revision      uint64
	applyConflict *[32]byte
}

func NewEditService(connections *connection.Manager, reader *kube.Service, coordinator *Coordinator) *EditService {
	return &EditService{connections, reader, coordinator}
}

func (s *EditService) OpenEdit(ctx context.Context, target kube.EditTarget) (EditDraft, error) {
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return EditDraft{}, err
	}
	defer release()
	baseline, err := s.reader.CaptureEdit(ctx, target)
	if err != nil {
		return EditDraft{}, err
	}
	projection, err := editProjection(baseline.ObjectJSON)
	if err != nil {
		return EditDraft{}, err
	}
	rendered, err := renderEdit(projection)
	if err != nil {
		return EditDraft{}, err
	}
	cluster, err := s.connections.ContextName(target.Connection)
	if err != nil {
		return EditDraft{}, err
	}
	draft := EditDraft{ID: model.ID(), Target: baseline.Target, ResourceVersion: baseline.ResourceVersion, Cluster: cluster, YAML: rendered}
	encoded, err := json.Marshal(draft)
	if err != nil || len(encoded) > editLimit {
		return EditDraft{}, model.Fail("capacity", "Editor content exceeds the 2 MiB limit")
	}
	if err := s.coordinator.storeDraft(ctx, draft.ID, operationDraft{action: "edit", connection: target.Connection, baseline: baseline, cluster: cluster}); err != nil {
		return EditDraft{}, err
	}
	return draft, nil
}

func (s *EditService) CloseEdit(id string) { s.coordinator.closeDraft("edit", id) }

var serverEditMetadata = []string{"creationTimestamp", "deletionTimestamp", "deletionGracePeriodSeconds", "generation", "managedFields", "selfLink"}

func editProjection(raw []byte) (map[string]any, error) {
	object, err := yamlValueProjection(raw)
	delete(object, "status")
	return object, err
}

func yamlValueProjection(raw []byte) (map[string]any, error) {
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, model.Fail("invalid_response", "Kubernetes returned an invalid resource")
	}
	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		return nil, model.Fail("invalid_response", "Kubernetes returned invalid resource metadata")
	}
	for _, key := range serverEditMetadata {
		delete(metadata, key)
	}
	return object, nil
}

func renderEdit(object map[string]any) (string, error) {
	raw, err := json.Marshal(object)
	if err != nil || len(raw) > editLimit {
		return "", model.Fail("capacity", "Edit document exceeds the 2 MiB limit")
	}
	return kube.RenderEditYAML(raw)
}

type editReview struct {
	version string
	patch   []byte
}

func (s *EditService) PrepareEdit(ctx context.Context, req EditRequest) (EditPreview, error) {
	c := s.coordinator
	ctx, release, err := c.prepare(ctx)
	if err != nil {
		return EditPreview{}, err
	}
	defer release()
	draft, err := c.reviseDraft("edit", req.DraftID)
	if err != nil {
		return EditPreview{}, err
	}
	patch, err := buildEditPatch(draft.baseline, req.YAML)
	if err != nil {
		return EditPreview{}, err
	}
	target := draft.baseline.Target
	client, err := newOperationClient(s.connections, target.Connection)
	if err != nil {
		return EditPreview{}, err
	}
	if err := checkEditVersion(ctx, client, target, draft.baseline.ResourceVersion); err != nil {
		return EditPreview{}, err
	}
	var raw json.RawMessage
	var feedback editFeedback
	warnings, err := client.editPatch(ctx, target.Path(), true, patch.body, &raw, &feedback)
	if err != nil {
		return EditPreview{}, editWriteProblem(ctx, client, target, draft.baseline.ResourceVersion, err, feedback, true)
	}
	if _, err := target.ReadVersion(raw); err != nil {
		return EditPreview{}, err
	}
	server, err := editProjection(raw)
	if err != nil {
		return EditPreview{}, err
	}
	preview := EditPreview{Target: target, Warnings: warnings}
	for _, item := range []struct {
		source map[string]any
		output *string
	}{{patch.before, &preview.BeforeYAML}, {patch.proposed, &preview.ProposedYAML}, {server, &preview.ServerYAML}} {
		*item.output, err = renderEdit(item.source)
		if err != nil {
			return EditPreview{}, err
		}
	}
	if preview.ServerYAML == preview.ProposedYAML {
		preview.ServerYAML = ""
	}
	if raw, err := json.Marshal(preview); err != nil || len(raw)+128 > editLimit {
		return EditPreview{}, model.Fail("capacity", "The combined edit review exceeds the 2 MiB limit")
	}
	preview.Token, preview.ExpiresAt, err = c.save(ctx, pendingReview{
		result:  OperationResult{Action: "edit", Cluster: draft.cluster, Resource: target.Ref(), Edit: &target},
		payload: editReview{version: draft.baseline.ResourceVersion, patch: patch.body},
		draftID: req.DraftID, revision: draft.revision,
	})
	if err != nil {
		return EditPreview{}, err
	}
	return preview, nil
}

func (s *EditService) RevokeEdit(token string) {
	s.coordinator.revoke("edit", token)
}
