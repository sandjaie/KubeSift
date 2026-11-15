/*
Purpose: Prepare one reviewed controller restart.
Used by: The native operation flow and integration tests.
Responsibilities: Validate eligibility, build a conditional annotation patch and verify server dry-run.
Important: Only the fixed annotation patch and a spec hash survive preparation.
*/
package operation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

const restartAnnotation = "kubectl.kubernetes.io/restartedAt"
const annotationPath = "/spec/template/metadata/annotations"
const patchLimit = 16 << 10

type RestartService struct {
	connections *connection.Manager
	coordinator *Coordinator
	now         func() time.Time
}

func NewRestartService(connections *connection.Manager, coordinator *Coordinator, now func() time.Time) *RestartService {
	if now == nil {
		now = time.Now
	}
	return &RestartService{connections: connections, coordinator: coordinator, now: now}
}

type restartReview struct {
	version  string
	patch    []byte
	hash     [32]byte
	liveHash [32]byte
}

type restartObject struct {
	identity                      scalePayload
	spec, status                  map[string]any
	templateMetadata, annotations map[string]any
}

func invalidRestart() error {
	return model.Fail("invalid_response", "Kubernetes returned malformed restart evidence")
}

func parseRestartObject(raw json.RawMessage) (restartObject, error) {
	var response struct {
		Kind       string
		APIVersion string
		Metadata   json.RawMessage
		Spec       map[string]any
		Status     map[string]any
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return restartObject{}, invalidRestart()
	}
	var p restartObject
	if json.Unmarshal(response.Metadata, &p.identity.Metadata) != nil {
		return p, invalidRestart()
	}
	p.identity.Kind, p.identity.APIVersion = response.Kind, response.APIVersion
	p.spec, p.status = response.Spec, response.Status
	template, ok := p.spec["template"].(map[string]any)
	if !ok {
		return p, invalidRestart()
	}
	if _, ok := template["spec"].(map[string]any); !ok {
		return p, invalidRestart()
	}
	p.templateMetadata, ok = template["metadata"].(map[string]any)
	if !ok {
		return p, invalidRestart()
	}
	if raw := p.templateMetadata["annotations"]; raw != nil {
		p.annotations, ok = raw.(map[string]any)
		if !ok {
			return p, invalidRestart()
		}
		for _, v := range p.annotations {
			if _, ok := v.(string); !ok {
				return p, invalidRestart()
			}
		}
	}
	return p, nil
}

func (p restartObject) matches(ref kube.ResourceRef, version string) bool {
	return p.identity.Kind == ref.Kind && p.identity.APIVersion == ref.APIVersion && p.identity.matches(ref, version)
}

func (c operationClient) inspectRestart(ctx context.Context, path string, ref kube.ResourceRef, version string) (restartObject, RestartImpact, error) {
	var raw json.RawMessage
	if _, err := c.call(ctx, http.MethodGet, path, false, nil, &raw); err != nil {
		return restartObject{}, RestartImpact{}, err
	}
	parent, err := parseRestartObject(raw)
	if err != nil {
		return parent, RestartImpact{}, err
	}
	if !parent.matches(ref, version) || parent.identity.Metadata.Deletion != nil {
		return parent, RestartImpact{}, model.Fail("stale_target", "Workload changed or is terminating. Refresh and review again.")
	}
	impact, err := restartImpact(ref.Kind, parent.spec, parent.status)
	return parent, impact, err
}

type patchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

func (p restartObject) restartPatch(uid, version, at string) ([]byte, [32]byte, error) {
	change := patchOperation{Op: "add", Path: annotationPath + "/kubectl.kubernetes.io~1restartedAt", Value: at}
	if p.annotations == nil {
		p.annotations = map[string]any{}
		p.templateMetadata["annotations"] = p.annotations
		change.Path, change.Value = annotationPath, p.annotations
	}
	p.annotations[restartAnnotation] = at
	patch, err := json.Marshal([]patchOperation{{Op: "test", Path: "/metadata/uid", Value: uid}, {Op: "test", Path: "/metadata/resourceVersion", Value: version}, change})
	if err != nil || len(patch) > patchLimit {
		return nil, [32]byte{}, model.Fail("invalid", "Restart patch exceeds the size limit")
	}
	hash, err := specHash(p.spec)
	return patch, hash, err
}

func escapedPrevious(value string) (string, bool) {
	var text strings.Builder
	for _, r := range value {
		quoted := strconv.QuoteToGraphic(string(r))
		fragment := quoted[1 : len(quoted)-1]
		if text.Len()+len(fragment) > 1024 {
			return text.String(), true
		}
		text.WriteString(fragment)
	}
	return text.String(), false
}

func (s *RestartService) PrepareRestart(ctx context.Context, req RestartRequest) (RestartPreview, error) {
	path, plural := workloadTarget(req.Resource)
	if path == "" || req.SourceVersion == "" || len(req.SourceVersion) > 128 {
		return RestartPreview{}, model.Fail("invalid", "Select a captured apps/v1 Deployment, StatefulSet or DaemonSet with a UID and version")
	}
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return RestartPreview{}, err
	}
	defer release()
	client, err := newOperationClient(s.connections, req.Resource.Connection)
	if err != nil {
		return RestartPreview{}, restartProblem(err, false)
	}
	parent, impact, err := client.inspectRestart(ctx, path, req.Resource, req.SourceVersion)
	if err != nil {
		return RestartPreview{}, restartProblem(err, false)
	}
	cluster, err := s.connections.ContextName(req.Resource.Connection)
	if err != nil {
		return RestartPreview{}, err
	}
	if len(cluster) > 1024 {
		return RestartPreview{}, model.Fail("invalid", "Cluster context name exceeds the display limit")
	}
	permission := permissionOf(ctx, client, req.Resource, plural, "patch", "")
	if permission == "denied" {
		return RestartPreview{}, model.Fail("forbidden", "Kubernetes reports that restarting this workload is denied")
	}
	previous, previousSet := parent.annotations[restartAnnotation].(string)
	now := s.now().UTC()
	at := now.Format(time.RFC3339Nano)
	if at == previous {
		at = now.Add(time.Nanosecond).Format(time.RFC3339Nano)
	}
	shown, truncated := escapedPrevious(previous)
	liveHash, err := specHash(parent.spec)
	if err != nil {
		return RestartPreview{}, err
	}
	patch, hash, err := parent.restartPatch(req.Resource.UID, req.SourceVersion, at)
	if err != nil {
		return RestartPreview{}, err
	}
	var raw json.RawMessage
	warnings, err := client.patch(ctx, path, true, patch, &raw)
	if err != nil {
		return RestartPreview{}, restartProblem(err, true)
	}
	dry, err := parseRestartObject(raw)
	if err != nil || !dry.matches(req.Resource, "") || dry.annotations[restartAnnotation] != at {
		return RestartPreview{}, model.Fail("invalid_response", "Dry-run did not return the selected identity and requested annotation")
	}
	dryHash, err := specHash(dry.spec)
	if err != nil || dryHash != hash {
		return RestartPreview{}, model.Fail("dry_run_changed_spec", "The dry-run returned other changes to the workload spec. No review was issued.")
	}
	p := RestartPreview{Cluster: cluster, Resource: req.Resource, SourceVersion: req.SourceVersion, Permission: permission, Warnings: warnings,
		RestartAt: at, PreviousValue: shown, PreviousSet: previousSet, PreviousTruncated: truncated, Impact: impact}
	pending := pendingReview{result: OperationResult{Action: "restart", Resource: req.Resource, Cluster: cluster, Restart: &RestartResultData{RequestedAt: at}},
		payload: restartReview{version: req.SourceVersion, patch: patch, hash: hash, liveHash: liveHash}}
	if impact.Recreate {
		pending.acknowledgement = req.Resource.Name
	}
	p.Token, p.ExpiresAt, err = s.coordinator.save(ctx, pending)
	if err != nil {
		return RestartPreview{}, err
	}
	return p, nil
}

func restartProblem(err error, dry bool) *model.Problem {
	var problem *model.Problem
	if errors.As(err, &problem) {
		return problem
	}
	var status operationHTTPStatus
	if errors.As(err, &status) {
		switch status {
		case 401, 403:
			return &model.Problem{Code: "forbidden", Message: "Kubernetes denied access to the workload or its mandatory restart dry-run"}
		case 404, 409, 410:
			return &model.Problem{Code: "stale_target", Message: "Workload changed or disappeared. Refresh and review again."}
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &model.Problem{Code: "cancelled", Message: "Restart preparation was interrupted"}
	}
	if dry {
		return &model.Problem{Code: "dry_run_failed", Message: "Mandatory restart dry-run failed. No review was issued."}
	}
	return &model.Problem{Code: "unavailable", Message: "Cannot read restart evidence from Kubernetes"}
}
