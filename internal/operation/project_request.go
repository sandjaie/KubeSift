/*
Purpose: Review and submit an OpenShift self-service ProjectRequest.
Used by: The desktop Projects workspace.
Responsibilities: Bind the request to its cluster and capture the returned Project identity.
Important: Preparation never submits a ProjectRequest, including dry-run requests.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/apimachinery/pkg/util/validation"
)

type ProjectFields struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}
type ProjectRequestPreview struct {
	Cluster   string        `json:"cluster"`
	Typed     bool          `json:"typed"`
	Fields    ProjectFields `json:"fields"`
	Token     string        `json:"token"`
	ExpiresAt time.Time     `json:"expiresAt"`
}

const requestPath = "/apis/" + kube.ProjectVersion + "/projectrequests"

func (s *CreateService) PrepareProjectRequest(ctx context.Context, ref model.ConnectionRef, f ProjectFields, environment string) (ProjectRequestPreview, error) {
	if len(validation.IsDNS1123Label(f.Name)) != 0 || utf8.RuneCountInString(f.DisplayName) > 256 || utf8.RuneCountInString(f.Description) > 2048 {
		return ProjectRequestPreview{}, model.Fail("invalid", "Enter a valid Project name and bounded display name and description")
	}
	for _, name := range []string{"openshift", "kube", "kubernetes"} {
		if f.Name == name || strings.HasPrefix(f.Name, name+"-") {
			return ProjectRequestPreview{}, model.Fail("invalid", "OpenShift reserves this Project name")
		}
	}
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return ProjectRequestPreview{}, err
	}
	defer release()
	caps, err := s.reader.ProjectCapabilities(ctx, ref, false)
	if err != nil {
		return ProjectRequestPreview{}, err
	}
	if !caps.Request {
		return ProjectRequestPreview{}, model.Fail("unsupported", "Self-service Project requests are unavailable")
	}
	client, err := newOperationClient(s.connections, ref)
	if err != nil {
		return ProjectRequestPreview{}, err
	}
	target := kube.ManifestAddress{Connection: ref, Group: kube.ProjectGroup, Version: "v1", Resource: "projects", Kind: "Project", Name: f.Name}
	permission := "unknown"
	fallback := !caps.CheckRequest
	if caps.CheckRequest {
		var status operationStatus
		_, err := client.call(ctx, http.MethodGet, requestPath, false, nil, &status)
		switch {
		case err == nil && status.Kind == "Status" && status.APIVersion == "v1" && status.Status == "Success":
			permission = "allowed"
		case errors.Is(err, operationHTTPStatus(403)):
			permission = "denied"
		case errors.Is(err, operationHTTPStatus(404)), errors.Is(err, operationHTTPStatus(405)):
			fallback = true
		}
	}
	if fallback {
		subject := target.Ref()
		subject.Name = ""
		permission = permissionOf(ctx, client, subject, "projectrequests", "create", "")
	}
	if permission != "allowed" {
		return ProjectRequestPreview{}, model.Fail(permission, "Self-service Project request permission is "+permission)
	}
	if caps.Get {
		if _, err := s.reader.ReadProject(ctx, ref, f.Name, ""); err == nil {
			return ProjectRequestPreview{}, model.Fail("already_exists", "A Project with this name already exists")
		}
	}
	cluster, err := s.connections.ContextName(ref)
	if err != nil {
		return ProjectRequestPreview{}, err
	}
	typed := strings.EqualFold(environment, "prod") || strings.EqualFold(environment, "production")
	ack := ""
	if typed {
		ack = f.Name
	}
	p := ProjectRequestPreview{Cluster: cluster, Fields: f, Typed: typed}
	p.Token, p.ExpiresAt, err = s.coordinator.save(ctx, pendingReview{acknowledgement: ack, result: OperationResult{Action: "request-project", Resource: target.Ref(), Cluster: cluster, Create: &CreateResultData{Target: target}}, payload: f})
	return p, err
}
func (s *CreateService) ExecuteProjectRequest(token, ack string) (OperationResult, error) {
	review, result, err := s.coordinator.begin("request-project", token, ack)
	if review == nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(s.coordinator.ctx, operationDeadline)
	defer cancel()
	f := review.payload.(ProjectFields)
	client, err := newOperationClient(s.connections, result.Resource.Connection)
	if err != nil {
		result.State = "cancelled-before-send"
		result.Problem = &model.Problem{Code: "unavailable", Message: "Original connection is unavailable"}
		return s.coordinator.finish(result), nil
	}
	var raw json.RawMessage
	_, err = client.call(ctx, http.MethodPost, requestPath, false, map[string]any{"apiVersion": kube.ProjectVersion, "kind": "ProjectRequest", "metadata": map[string]string{"name": f.Name}, "displayName": f.DisplayName, "description": f.Description}, &raw)
	result.State = "unknown"
	result.Problem = &model.Problem{Code: "uncertain", Message: "Template processing may have left partial effects. Inspect the original Project name before taking another action."}
	if err == nil {
		uid, rv, e := result.Create.Target.ReadIdentity(raw)
		if e == nil && uid != "" && rv != "" && len(rv) <= 128 {
			result.State = "accepted"
			result.Resource.UID = uid
			result.Create.ResourceVersion = rv
			result.Problem = nil
		}
	}
	if errors.Is(err, operationHTTPStatus(409)) {
		result.State = "rejected"
		result.Problem = &model.Problem{Code: "already_exists", Message: "A Project with this name already exists"}
	}
	if errors.Is(err, operationHTTPStatus(403)) || errors.Is(err, operationHTTPStatus(401)) {
		result.State = "rejected"
		result.Problem = &model.Problem{Code: "forbidden", Message: "Project request denied"}
	}
	return s.coordinator.finish(result), nil
}
func (s *CreateService) RevokeProjectRequest(token string) {
	s.coordinator.revoke("request-project", token)
}
