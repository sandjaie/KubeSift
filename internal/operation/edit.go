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

type editCapture struct {
	baseline kube.EditCapture
	cluster  string
	expires  time.Time
	revision uint64
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
	c := s.coordinator
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || c.ctx.Err() != nil {
		return EditDraft{}, model.Fail("cancelled", "Opening the editor was cancelled")
	}
	if err := s.connections.Validate(target.Connection); err != nil {
		return EditDraft{}, err
	}
	now := c.now()
	c.prune(now)
	if len(c.pending)+len(c.captures) >= 128 {
		return EditDraft{}, model.Fail("busy", "Too many pending operations and editor captures")
	}
	c.captures[draft.ID] = editCapture{baseline: baseline, cluster: cluster, expires: now.Add(reviewTTL)}
	c.scheduleExpiry(now)
	return draft, nil
}

func (s *EditService) CloseEdit(id string) {
	c := s.coordinator
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropEdit(id)
	c.scheduleExpiry(c.now())
}

func (s *Coordinator) dropEdit(id string) {
	delete(s.captures, id)
	s.revokeEditDraft(id)
}

func (s *Coordinator) revokeEditDraft(id string) {
	for token, review := range s.pending {
		if review.draftID == id {
			delete(s.pending, token)
		}
	}
}

var serverEditMetadata = []string{"creationTimestamp", "deletionTimestamp", "deletionGracePeriodSeconds", "generation", "managedFields", "selfLink"}

func editProjection(raw []byte) (map[string]any, error) {
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
	delete(object, "status")
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
