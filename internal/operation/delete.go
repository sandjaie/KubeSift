/*
Purpose: Prepare a conditional deletion of one supported workload.
Used by: The native operation flow and integration tests.
Responsibilities: Validate eligibility, project impact and verify a mandatory server dry-run.
Important: Only bounded impact and fixed DeleteOptions survive preparation.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

const deleteOptionsLimit = 16 << 10
const deleteGracePolicy = "Server default"

type DeleteService struct {
	connections *connection.Manager
	coordinator *Coordinator
	now         func() time.Time
}

func NewDeleteService(connections *connection.Manager, coordinator *Coordinator, now func() time.Time) *DeleteService {
	if now == nil {
		now = time.Now
	}
	return &DeleteService{connections: connections, coordinator: coordinator, now: now}
}

type deleteReview struct {
	version string
	options []byte
	impact  DeleteImpact
}

func deleteTarget(ref kube.ResourceRef) (string, string) {
	var api, plural string
	switch ref.Kind {
	case "Pod":
		api, plural = "v1", "pods"
	case "ReplicationController":
		api, plural = "v1", "replicationcontrollers"
	case "Deployment", "StatefulSet", "DaemonSet":
		return workloadTarget(ref)
	case "ReplicaSet":
		api, plural = "apps/v1", "replicasets"
	case "Job":
		api, plural = "batch/v1", "jobs"
	case "CronJob":
		api, plural = "batch/v1", "cronjobs"
	default:
		return "", ""
	}
	if ref.APIVersion != api || len(validation.IsDNS1123Label(ref.Namespace)) != 0 ||
		len(validation.IsDNS1123Subdomain(ref.Name)) != 0 || ref.UID == "" || len(ref.UID) > 128 {
		return "", ""
	}
	prefix := "/apis/"
	if api == "v1" {
		prefix = "/api/"
	}
	return prefix + api + "/namespaces/" + ref.Namespace + "/" + plural + "/" + ref.Name, plural
}

func deleteOptions(ref kube.ResourceRef, version string, dry bool) ([]byte, error) {
	uid, propagation := types.UID(ref.UID), metav1.DeletePropagationBackground
	options := metav1.DeleteOptions{
		TypeMeta:          metav1.TypeMeta{APIVersion: "v1", Kind: "DeleteOptions"},
		Preconditions:     &metav1.Preconditions{UID: &uid, ResourceVersion: &version},
		PropagationPolicy: &propagation,
	}
	if dry {
		options.DryRun = []string{metav1.DryRunAll}
	}
	raw, err := json.Marshal(options)
	if err != nil || len(raw) > deleteOptionsLimit {
		return nil, model.Fail("invalid", "Delete options exceed the request limit")
	}
	return raw, nil
}

func invalidDeleteEvidence() *model.Problem {
	return &model.Problem{Code: "invalid_response", Message: "Kubernetes returned malformed deletion evidence"}
}

func (c operationClient) inspectDelete(ctx context.Context, path string, ref kube.ResourceRef, version string) (DeleteImpact, error) {
	var raw json.RawMessage
	if _, err := c.call(ctx, http.MethodGet, path, false, nil, &raw); err != nil {
		return DeleteImpact{}, err
	}
	var parent scalePayload
	if json.Unmarshal(raw, &parent) != nil || parent.Kind != ref.Kind || parent.APIVersion != ref.APIVersion ||
		parent.Metadata.UID == "" || parent.Metadata.Version == "" {
		return DeleteImpact{}, invalidDeleteEvidence()
	}
	if !parent.matches(ref, version) || parent.Metadata.Deletion != nil {
		return DeleteImpact{}, model.Fail("stale_target", "Workload changed or is terminating. Refresh and review again.")
	}
	return projectDeleteImpact(raw, ref.Kind)
}

func validDeleteResponse(raw json.RawMessage, ref kube.ResourceRef, plural string) bool {
	var meta metav1.TypeMeta
	if json.Unmarshal(raw, &meta) != nil {
		return false
	}
	if meta.Kind != "Status" {
		var parent scalePayload
		return meta.Kind == ref.Kind && meta.APIVersion == ref.APIVersion &&
			json.Unmarshal(raw, &parent) == nil && parent.matches(ref, "")
	}
	var status operationStatus
	if meta.APIVersion != "v1" || json.Unmarshal(raw, &status) != nil || status.Status != "Success" || status.Reason != "" ||
		(status.Code != 0 && status.Code != http.StatusOK && status.Code != http.StatusAccepted) {
		return false
	}
	if status.Details == nil {
		return true
	}
	details := status.Details
	group, _, _ := strings.Cut(ref.APIVersion, "/")
	if ref.APIVersion == "v1" {
		group = ""
	}
	return (details.Name == nil || *details.Name == ref.Name) &&
		(details.UID == nil || *details.UID == ref.UID) &&
		(details.Group == nil || *details.Group == group) &&
		(details.Kind == nil || *details.Kind == plural)
}

func (s *DeleteService) PrepareDelete(ctx context.Context, req DeleteRequest) (DeletePreview, error) {
	path, plural := deleteTarget(req.Resource)
	if path == "" || req.SourceVersion == "" || len(req.SourceVersion) > 128 {
		return DeletePreview{}, model.Fail("invalid", "Select a supported namespaced workload with a captured UID and version")
	}
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return DeletePreview{}, err
	}
	defer release()
	client, err := newOperationClient(s.connections, req.Resource.Connection)
	if err != nil {
		return DeletePreview{}, deleteProblem(err, false)
	}
	impact, err := client.inspectDelete(ctx, path, req.Resource, req.SourceVersion)
	if err != nil {
		return DeletePreview{}, deleteProblem(err, false)
	}
	cluster, err := s.connections.ContextName(req.Resource.Connection)
	if err != nil {
		return DeletePreview{}, err
	}
	if len(cluster) > 1024 {
		return DeletePreview{}, model.Fail("invalid", "Cluster context name exceeds the display limit")
	}
	permission := permissionOf(ctx, client, req.Resource, plural, "delete", "")
	if permission == "denied" {
		return DeletePreview{}, model.Fail("forbidden", "Kubernetes reports that deleting this workload is denied")
	}
	options, err := deleteOptions(req.Resource, req.SourceVersion, false)
	if err != nil {
		return DeletePreview{}, err
	}
	dryOptions, err := deleteOptions(req.Resource, req.SourceVersion, true)
	if err != nil {
		return DeletePreview{}, err
	}
	var raw json.RawMessage
	warnings, err := client.send(ctx, http.MethodDelete, path, false, dryOptions, "application/json", &raw, nil)
	if err != nil {
		return DeletePreview{}, deleteProblem(err, true)
	}
	if !validDeleteResponse(raw, req.Resource, plural) {
		return DeletePreview{}, invalidDeleteEvidence()
	}
	preview := DeletePreview{Cluster: cluster, Resource: req.Resource, SourceVersion: req.SourceVersion, Permission: permission, Warnings: warnings, Impact: impact}
	preview.Token, preview.ExpiresAt, err = s.coordinator.save(ctx, pendingReview{
		result: OperationResult{Action: "delete", Resource: req.Resource, Cluster: cluster,
			Delete: &DeleteResultData{Propagation: impact.Propagation, GracePolicy: impact.GracePolicy}},
		payload:         deleteReview{version: req.SourceVersion, options: options, impact: impact},
		acknowledgement: req.Resource.Name,
	})
	if err != nil {
		return DeletePreview{}, err
	}
	return preview, nil
}

func deleteProblem(err error, dry bool) *model.Problem {
	var problem *model.Problem
	if errors.As(err, &problem) {
		return problem
	}
	var status operationHTTPStatus
	if errors.As(err, &status) {
		switch status {
		case 401, 403:
			return &model.Problem{Code: "forbidden", Message: "Kubernetes denied access to the workload or its mandatory deletion dry-run"}
		case 404, 409, 410:
			return &model.Problem{Code: "stale_target", Message: "Workload changed or disappeared. Refresh and review again."}
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &model.Problem{Code: "cancelled", Message: "Deletion preparation was interrupted"}
	}
	if dry {
		return &model.Problem{Code: "dry_run_failed", Message: "Mandatory deletion dry-run failed. No review was issued."}
	}
	return &model.Problem{Code: "unavailable", Message: "Cannot read deletion evidence from Kubernetes"}
}
