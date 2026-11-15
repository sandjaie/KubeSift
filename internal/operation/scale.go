/*
Purpose: Prepare an immutable, reviewed Deployment or StatefulSet scale.
Used by: The scaling bridge and API server integration tests.
Responsibilities: Validate the target, enforce bounded reads and require a matching server dry-run.
Important: A permission hint is advisory and only the original live version is used for writing.
*/
package operation

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	auth "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

type ScaleService struct {
	connections *connection.Manager
	coordinator *Coordinator
}

func NewScaleService(connections *connection.Manager, coordinator *Coordinator) *ScaleService {
	if coordinator == nil {
		coordinator = NewCoordinator(connections, nil)
	}
	return &ScaleService{connections: connections, coordinator: coordinator}
}

func scaleTarget(ref kube.ResourceRef) (string, string) {
	if ref.Kind != "Deployment" && ref.Kind != "StatefulSet" {
		return "", ""
	}
	return workloadTarget(ref)
}
func workloadTarget(ref kube.ResourceRef) (string, string) {
	plural := ""
	switch ref.Kind {
	case "Deployment":
		plural = "deployments"
	case "StatefulSet":
		plural = "statefulsets"
	case "DaemonSet":
		plural = "daemonsets"
	}
	if plural == "" || ref.APIVersion != "apps/v1" || len(validation.IsDNS1123Label(ref.Namespace)) != 0 ||
		len(validation.IsDNS1123Subdomain(ref.Name)) != 0 || ref.UID == "" || len(ref.UID) > 128 {
		return "", ""
	}
	return "/apis/apps/v1/namespaces/" + ref.Namespace + "/" + plural + "/" + ref.Name, plural
}
func parseReplicas(input string) (int32, error) {
	invalid := model.Fail("invalid", "Enter a whole replica count from 0 through 2147483647")
	if len(input) == 0 || len(input) > 128 {
		return 0, invalid
	}
	for _, c := range input {
		if c < '0' || c > '9' {
			return 0, invalid
		}
	}
	n, err := strconv.ParseInt(input, 10, 32)
	if err != nil {
		return 0, invalid
	}
	return int32(n), nil
}

type scalePayload struct {
	metav1.TypeMeta
	Metadata struct {
		Name      string       `json:"name"`
		Namespace string       `json:"namespace"`
		UID       string       `json:"uid"`
		Version   string       `json:"resourceVersion"`
		Deletion  *metav1.Time `json:"deletionTimestamp,omitempty"`
	} `json:"metadata"`
	Spec *scaleSpec `json:"spec"`
}

type scaleSpec struct {
	Replicas *int32 `json:"replicas"`
	Policy   *struct {
		WhenScaled  string `json:"whenScaled"`
		WhenDeleted string `json:"whenDeleted"`
	} `json:"persistentVolumeClaimRetentionPolicy,omitempty"`
}

func (s *scaleSpec) desired() int32 {
	// autoscaling/v1 Scale omits its zero count in JSON.
	if s.Replicas == nil {
		return 0
	}
	return *s.Replicas
}

func (p scalePayload) matches(ref kube.ResourceRef, version string) bool {
	return p.Metadata.UID == ref.UID && p.Metadata.Name == ref.Name && p.Metadata.Namespace == ref.Namespace &&
		p.Metadata.Version != "" && len(p.Metadata.Version) <= 128 && (version == "" || p.Metadata.Version == version)
}

func (c operationClient) inspect(ctx context.Context, path string, ref kube.ResourceRef, version string) (int32, ScaleImpact, error) {
	var parent, scale scalePayload
	stale := model.Fail("stale_target", "Workload changed or is terminating. Refresh and review again.")
	if _, err := c.call(ctx, http.MethodGet, path, false, nil, &parent); err != nil {
		return 0, ScaleImpact{}, err
	}
	if parent.Kind != ref.Kind || parent.APIVersion != ref.APIVersion || !parent.matches(ref, version) || parent.Metadata.Deletion != nil {
		return 0, ScaleImpact{}, stale
	}
	if _, err := c.call(ctx, http.MethodGet, path+"/scale", false, nil, &scale); err != nil {
		return 0, ScaleImpact{}, err
	}
	if scale.Kind != "Scale" || scale.APIVersion != "autoscaling/v1" || !scale.matches(ref, version) || scale.Spec == nil ||
		parent.Spec == nil || parent.Spec.Replicas == nil || scale.Spec.desired() < 0 || scale.Spec.desired() != *parent.Spec.Replicas {
		return 0, ScaleImpact{}, stale
	}
	impact := ScaleImpact{}
	if ref.Kind == "StatefulSet" {
		impact.WhenScaled, impact.WhenDeleted = "Retain", "Retain"
		policy := parent.Spec.Policy
		impact.PolicyDefault = policy == nil || policy.WhenScaled == "" || policy.WhenDeleted == ""
		if policy != nil {
			if policy.WhenScaled != "" {
				impact.WhenScaled = policy.WhenScaled
			}
			if policy.WhenDeleted != "" {
				impact.WhenDeleted = policy.WhenDeleted
			}
		}
		if (impact.WhenScaled != "Retain" && impact.WhenScaled != "Delete") || (impact.WhenDeleted != "Retain" && impact.WhenDeleted != "Delete") {
			return 0, ScaleImpact{}, model.Fail("invalid_response", "StatefulSet returned an unrecognized claim retention policy")
		}
	}
	return scale.Spec.desired(), impact, nil
}
func permissionOf(ctx context.Context, c operationClient, ref kube.ResourceRef, plural, verb, subresource string) string {
	attrs := &auth.ResourceAttributes{Namespace: ref.Namespace, Name: ref.Name, Group: "apps", Version: "v1", Resource: plural, Subresource: subresource, Verb: verb}
	input := auth.SelfSubjectAccessReview{TypeMeta: metav1.TypeMeta{Kind: "SelfSubjectAccessReview", APIVersion: "authorization.k8s.io/v1"}, Spec: auth.SelfSubjectAccessReviewSpec{ResourceAttributes: attrs}}
	var review auth.SelfSubjectAccessReview
	_, err := c.call(ctx, http.MethodPost, "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews", false, input, &review)
	status := review.Status
	if err != nil || review.Kind != input.Kind || review.APIVersion != input.APIVersion || status.EvaluationError != "" || status.Allowed == status.Denied {
		return "unknown"
	}
	if status.Allowed {
		return "allowed"
	}
	return "denied"
}

func scaleInput(p ScalePreview) scalePayload {
	var payload scalePayload
	payload.Kind, payload.APIVersion = "Scale", "autoscaling/v1"
	payload.Metadata.Name, payload.Metadata.Namespace = p.Resource.Name, p.Resource.Namespace
	payload.Metadata.UID, payload.Metadata.Version = p.Resource.UID, p.SourceVersion
	payload.Spec = &scaleSpec{Replicas: &p.After}
	return payload
}
func matchingScale(result scalePayload, ref kube.ResourceRef) bool {
	return result.Kind == "Scale" && result.APIVersion == "autoscaling/v1" && result.matches(ref, "") && result.Spec != nil && result.Spec.desired() >= 0
}

func (s *ScaleService) PrepareScale(ctx context.Context, req ScaleRequest) (ScalePreview, error) {
	path, plural := scaleTarget(req.Resource)
	if path == "" || req.SourceVersion == "" || len(req.SourceVersion) > 128 {
		return ScalePreview{}, model.Fail("invalid", "Select a captured apps/v1 Deployment or StatefulSet with a UID and version")
	}
	after, err := parseReplicas(req.ReplicasText)
	if err != nil {
		return ScalePreview{}, err
	}
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return ScalePreview{}, err
	}
	defer release()
	client, err := newOperationClient(s.connections, req.Resource.Connection)
	if err != nil {
		return ScalePreview{}, scaleReadProblem(err)
	}
	before, impact, err := client.inspect(ctx, path, req.Resource, req.SourceVersion)
	if err != nil {
		return ScalePreview{}, scaleReadProblem(err)
	}
	cluster, err := s.connections.ContextName(req.Resource.Connection)
	if err != nil {
		return ScalePreview{}, err
	}
	if len(cluster) > 1024 {
		return ScalePreview{}, model.Fail("invalid", "Cluster context name exceeds the display limit")
	}
	p := ScalePreview{Cluster: cluster, Resource: req.Resource, SourceVersion: req.SourceVersion, Before: before, After: after, Impact: impact, Warnings: []string{}}
	if before == after {
		p.NoChange = true
		return p, nil
	}
	p.Impact.Direction = scaleDirection(before, after)
	p.Permission = permissionOf(ctx, client, req.Resource, plural, "update", "scale")
	if p.Permission == "denied" {
		return ScalePreview{}, model.Fail("forbidden", "Kubernetes reports that scaling this workload is denied")
	}
	var dry scalePayload
	p.Warnings, err = client.call(ctx, http.MethodPut, path+"/scale", true, scaleInput(p), &dry)
	if err != nil {
		return ScalePreview{}, scaleDryProblem(err)
	}
	if !matchingScale(dry, req.Resource) || dry.Spec.desired() != after {
		return ScalePreview{}, model.Fail("invalid_response", "Dry-run did not preserve the selected identity and replica count")
	}
	result := OperationResult{Action: "scale", Resource: p.Resource, Cluster: p.Cluster, Scale: &ScaleResultData{Before: p.Before, Requested: p.After}}
	pending := pendingReview{result: result, payload: p}
	if p.After == 0 {
		pending.acknowledgement = p.Resource.Name
	}
	p.Token, p.ExpiresAt, err = s.coordinator.save(ctx, pending)
	if err != nil {
		return ScalePreview{}, err
	}
	return p, nil
}

func scaleDirection(before, after int32) string {
	if after == 0 {
		return "zero"
	}
	if after < before {
		return "down"
	}
	return "up"
}
func scaleReadProblem(err error) *model.Problem {
	var problem *model.Problem
	if errors.As(err, &problem) {
		return problem
	}
	var code operationHTTPStatus
	if errors.As(err, &code) {
		switch code {
		case 401, 403:
			return &model.Problem{Code: "forbidden", Message: "Kubernetes denied access to the selected workload or its Scale"}
		case 404, 409:
			return &model.Problem{Code: "stale_target", Message: "Workload changed or disappeared. Refresh and review again."}
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &model.Problem{Code: "cancelled", Message: "Scaling inspection was interrupted"}
	}
	return &model.Problem{Code: "unavailable", Message: "Cannot read scaling data from Kubernetes"}
}
func scaleDryProblem(err error) *model.Problem {
	var code operationHTTPStatus
	if errors.As(err, &code) {
		switch code {
		case 401, 403:
			return &model.Problem{Code: "forbidden", Message: "Kubernetes denied the mandatory scale dry-run"}
		case 404, 409:
			return &model.Problem{Code: "stale_target", Message: "Workload changed or disappeared during dry-run. Refresh and review again."}
		}
	}
	return &model.Problem{Code: "dry_run_failed", Message: "Mandatory scale dry-run failed. No review was issued."}
}
