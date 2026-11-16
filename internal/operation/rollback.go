/*
Purpose: Prepare a reviewed rollback to an exact retained revision.
Used by: The desktop bridge and the native operation coordinator.
Responsibilities: Validate both objects, preview a template-only patch and describe the live strategy.
Important: Kubernetes decides whether the patch can run; no local permission gate is added.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strconv"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/apimachinery/pkg/util/validation"
)

type RollbackService struct {
	connections *connection.Manager
	coordinator *Coordinator
}

func NewRollbackService(connections *connection.Manager, coordinator *Coordinator) *RollbackService {
	return &RollbackService{connections: connections, coordinator: coordinator}
}

type rollbackReview struct {
	request RollbackRequest
	patch   []byte
}

func rollbackPaths(req RollbackRequest) (parent, source string) {
	parent, _ = workloadTarget(req.Resource)
	kind, plural := kube.RolloutSource(req.Resource.Kind)
	ref := req.Source
	number, err := strconv.ParseInt(req.Revision, 10, 64)
	if parent == "" || ref.Connection != req.Resource.Connection || ref.APIVersion != "apps/v1" || ref.Kind != kind ||
		ref.Namespace != req.Resource.Namespace || len(validation.IsDNS1123Subdomain(ref.Name)) != 0 ||
		ref.UID == "" || len(ref.UID) > 128 || req.ResourceVersion == "" || len(req.ResourceVersion) > 128 ||
		req.SourceVersion == "" || len(req.SourceVersion) > 128 || err != nil || number <= 0 || strconv.FormatInt(number, 10) != req.Revision {
		return "", ""
	}
	return parent, "/apis/apps/v1/namespaces/" + ref.Namespace + "/" + plural + "/" + ref.Name
}

func readRollbackObject(ctx context.Context, client operationClient, path string) (json.RawMessage, error) {
	var raw json.RawMessage
	if _, err := client.call(ctx, http.MethodGet, path, false, nil, &raw); err != nil {
		return nil, err
	}
	if len(raw) > editLimit {
		return nil, model.Fail("capacity", "Rollback evidence exceeds the 2 MiB limit")
	}
	return raw, nil
}

func checkRollbackParent(raw json.RawMessage, ref kube.ResourceRef, version string) error {
	var identity scalePayload
	if len(raw) > editLimit || json.Unmarshal(raw, &identity) != nil {
		return invalidRollbackResponse()
	}
	if identity.Kind != ref.Kind || identity.APIVersion != ref.APIVersion || !identity.matches(ref, version) ||
		identity.Metadata.Deletion != nil {
		return staleEdit()
	}
	return nil
}

func readRollbackSource(ctx context.Context, client operationClient, path string, req RollbackRequest) (json.RawMessage, error) {
	raw, err := readRollbackObject(ctx, client, path)
	if err != nil {
		return nil, err
	}
	if err := kube.CheckRolloutRevision(raw, req.Resource, req.Source, req.SourceVersion, req.Revision); err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *RollbackService) PrepareRollback(ctx context.Context, req RollbackRequest) (RollbackPreview, error) {
	path, sourcePath := rollbackPaths(req)
	if path == "" {
		return RollbackPreview{}, model.Fail("invalid", "Select a captured workload and one of its retained revisions")
	}
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return RollbackPreview{}, err
	}
	defer release()
	client, err := newOperationClient(s.connections, req.Resource.Connection)
	if err != nil {
		return RollbackPreview{}, err
	}
	parent, err := readRollbackObject(ctx, client, path)
	if err != nil {
		return RollbackPreview{}, rollbackProblem(err, false)
	}
	if err := checkRollbackParent(parent, req.Resource, req.ResourceVersion); err != nil {
		return RollbackPreview{}, err
	}
	source, err := readRollbackSource(ctx, client, sourcePath, req)
	if err != nil {
		return RollbackPreview{}, rollbackProblem(err, false)
	}
	template, err := rollbackTemplate(req.Resource, source)
	if err != nil {
		return RollbackPreview{}, err
	}
	patch, err := rollbackPatch(parent, template)
	if err != nil {
		return RollbackPreview{}, err
	}
	cluster, err := s.connections.ContextName(req.Resource.Connection)
	if err != nil {
		return RollbackPreview{}, err
	}
	var raw json.RawMessage
	warnings, err := client.patch(ctx, path, true, patch, &raw)
	if err != nil {
		return RollbackPreview{}, rollbackWriteProblem(ctx, client, path, req, err, true)
	}
	if err := checkRollbackParent(raw, req.Resource, ""); err != nil {
		return RollbackPreview{}, invalidRollbackResponse()
	}
	before, err := editProjection(parent)
	if err != nil {
		return RollbackPreview{}, err
	}
	server, err := editProjection(raw)
	if err != nil {
		return RollbackPreview{}, err
	}
	beforeSpec, serverSpec := rollbackField(before, "spec"), rollbackField(server, "spec")
	beforeTemplate, serverTemplate := rollbackField(beforeSpec, "template"), rollbackField(serverSpec, "template")
	if serverTemplate == nil {
		return RollbackPreview{}, invalidRollbackResponse()
	}
	notices := rollbackNotices(req.Resource.Kind, beforeSpec, rollbackField(rollbackObject(parent), "status"))
	// Ignore generated fields but reject admission changes outside the reviewed template.
	for _, obj := range []map[string]any{before, server} {
		metadata := rollbackField(obj, "metadata")
		delete(metadata, "resourceVersion")
		if req.Resource.Kind == "DaemonSet" {
			// The API increments this legacy annotation when a DaemonSet template changes.
			annotations := rollbackField(metadata, "annotations")
			if _, exists := annotations["deprecated.daemonset.template.generation"]; exists {
				delete(annotations, "deprecated.daemonset.template.generation")
				if len(annotations) == 0 {
					delete(metadata, "annotations")
				}
			}
		}
		delete(rollbackField(obj, "spec"), "template")
	}
	if !reflect.DeepEqual(before, server) {
		return RollbackPreview{}, model.Fail("dry_run_changed_fields", "Dry-run changed fields outside the Pod template. No rollback review was issued.")
	}
	if reflect.DeepEqual(beforeTemplate, serverTemplate) {
		return RollbackPreview{}, model.Fail("no_changes", "The server preview leaves the current Pod template unchanged")
	}
	p := RollbackPreview{Request: req, Cluster: cluster, Notices: notices, Warnings: warnings}
	p.BeforeYAML, err = renderEdit(beforeTemplate)
	if err != nil {
		return RollbackPreview{}, err
	}
	proposed := rollbackObject(template)
	p.ProposedYAML, err = renderEdit(proposed)
	if err != nil {
		return RollbackPreview{}, err
	}
	if !reflect.DeepEqual(proposed, serverTemplate) {
		p.ServerYAML, err = renderEdit(serverTemplate)
		if err != nil {
			return RollbackPreview{}, err
		}
	}
	if raw, err := json.Marshal(p); err != nil || len(raw)+128 > editLimit {
		return RollbackPreview{}, model.Fail("capacity", "The combined rollback review exceeds the 2 MiB limit")
	}
	result := OperationResult{Action: "rollback", Resource: req.Resource, Cluster: cluster,
		Rollback: &RollbackResultData{SourceName: req.Source.Name, SourceUID: req.Source.UID, Revision: req.Revision}}
	p.Token, p.ExpiresAt, err = s.coordinator.save(ctx, pendingReview{result: result, payload: rollbackReview{request: req, patch: patch}})
	if err != nil {
		return RollbackPreview{}, err
	}
	return p, nil
}

func rollbackNotices(kind string, spec, status map[string]any) []string {
	notices := []string{}
	strategy := rollbackField(spec, "updateStrategy")
	if kind == "Deployment" {
		strategy = rollbackField(spec, "strategy")
	}
	switch strategy["type"] {
	case "OnDelete":
		notices = append(notices, "OnDelete: existing Pods will not be replaced automatically. KubeSift does not delete Pods.")
	case "RollingUpdate":
		notices = append(notices, "RollingUpdate: the controller decides when to replace Pods.")
	case "Recreate":
		notices = append(notices, "Recreate: the Deployment controller can stop existing Pods before creating replacements.")
	}
	if kind == "StatefulSet" {
		partition, ok := rollbackField(strategy, "rollingUpdate")["partition"].(json.Number)
		if n, err := partition.Int64(); ok && err == nil && n > 0 {
			notices = append(notices, "RollingUpdate partition "+partition.String()+": Pods below the partition may keep their current template.")
		}
		if strategy["type"] == "RollingUpdate" {
			notices = append(notices, "A StatefulSet stuck on an unready Pod may require separate manual recovery after restoring the template.")
		}
	}
	if spec["paused"] == true && kind == "Deployment" {
		notices = append(notices, "The Deployment is paused. Rollback keeps it paused.")
	}
	if spec["replicas"] == json.Number("0") {
		notices = append(notices, "There are zero desired replicas. Rollback does not change this count.")
	}
	if kind == "DaemonSet" && (status["desiredNumberScheduled"] == json.Number("0") || status["desiredNumberScheduled"] == nil) && status != nil {
		notices = append(notices, "The DaemonSet currently reports zero desired Pods.")
	}
	return notices
}

func rollbackProblem(err error, dry bool) *model.Problem {
	var p *model.Problem
	if errors.As(err, &p) {
		return p
	}
	var status operationHTTPStatus
	if errors.As(err, &status) {
		switch status {
		case 401, 403:
			return &model.Problem{Code: "forbidden", Message: "Kubernetes denied access to the workload, its history or the rollback patch."}
		case 404, 409, 410:
			return &model.Problem{Code: "stale_target", Message: "The workload or selected revision changed. Refresh Revisions and review again."}
		case 405, 415, 501:
			if dry {
				return &model.Problem{Code: "unsupported", Message: "The API did not support the required rollback dry-run."}
			}
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &model.Problem{Code: "cancelled", Message: "Rollback was interrupted."}
	}
	if dry {
		return &model.Problem{Code: "dry_run_failed", Message: "Kubernetes did not accept the rollback dry-run. No review was issued."}
	}
	return &model.Problem{Code: "unavailable", Message: "Could not read workload rollback evidence from Kubernetes."}
}

func rollbackWriteProblem(ctx context.Context, client operationClient, path string, req RollbackRequest, err error, dry bool) *model.Problem {
	var status operationHTTPStatus
	if errors.As(err, &status) && status == 422 {
		// Some API servers also return 422 for a failed JSON Patch test.
		if parent, readErr := readRollbackObject(ctx, client, path); readErr == nil {
			if err := checkRollbackParent(parent, req.Resource, req.ResourceVersion); isStaleEdit(err) {
				return staleEdit()
			}
		} else if problem := rollbackProblem(readErr, false); problem.Code == "stale_target" {
			return problem
		}
	}
	return rollbackProblem(err, dry)
}
