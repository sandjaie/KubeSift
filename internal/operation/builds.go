/*
Purpose: Review and dispatch explicit Build start, clone and cancellation requests.
Used by: The OpenShift Builds workspace.
Responsibilities: Revalidate source identity and permissions and retain returned Build identity.
Important: Instantiate and clone are imperative and never invoked during preview.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/apimachinery/pkg/util/validation"
	"net/http"
	"strings"
)

type BuildActionRequest struct {
	Target  kube.WorkflowQuery `json:"target"`
	Version string             `json:"version"`
	Action  string             `json:"action"`
}

func buildAction(a string) bool { return a == "start-build" || a == "rebuild" || a == "cancel-build" }

type buildCommand struct {
	method, path, media string
	body                []byte
}

func (s *CreateService) buildCommand(ctx context.Context, c operationClient, r BuildActionRequest) (buildCommand, string, error) {
	var cmd buildCommand
	q := r.Target
	plural, verb, sub := "builds", "create", "clone"
	switch r.Action {
	case "start-build":
		plural, sub = "buildconfigs", "instantiate"
	case "rebuild":
	case "cancel-build":
		verb, sub = "patch", ""
	default:
		return cmd, "", model.Fail("invalid", "Invalid Build action")
	}
	if q.UID == "" || r.Version == "" || q.Name == "" || (q.Kind == "BuildConfig") != (r.Action == "start-build") || (q.Kind != "Build" && q.Kind != "BuildConfig") {
		return cmd, "", model.Fail("invalid", "Select the captured Build or BuildConfig")
	}
	a := kube.ManifestAddress{Connection: q.Connection, Group: "build.openshift.io", Version: "v1", Resource: plural, Kind: q.Kind, Namespace: q.Namespace, Name: q.Name}
	if permissionOf(ctx, c, a.Ref(), plural, verb, sub) != "allowed" {
		return cmd, "", model.Fail("forbidden", "Build action is unavailable for this identity")
	}
	page, err := s.reader.ReadWorkflow(ctx, q)
	if err != nil {
		return cmd, "", err
	}
	if len(page.Items) != 1 || page.Items[0].Version != r.Version {
		return cmd, "", model.Fail("stale_target", "Build source changed; review again")
	}
	var b kube.BuildEvidence
	if json.Unmarshal([]byte(page.Items[0].Fields), &b) != nil {
		return cmd, "", model.Fail("invalid", "Invalid Build evidence")
	}
	if sub != "" {
		if b.SourceType == "Binary" {
			return cmd, "", model.Fail("unsupported", "Binary builds need new input")
		}
		body := map[string]any{"kind": "BuildRequest", "apiVersion": a.Ref().APIVersion, "metadata": map[string]string{"name": q.Name, "namespace": q.Namespace}}
		if sub == "instantiate" {
			counter := int64(0)
			if b.LastVersion != nil {
				counter = *b.LastVersion
			}
			body["lastVersion"] = counter
		}
		cmd = buildCommand{method: http.MethodPost, path: a.Path() + "/" + sub, media: "application/json"}
		cmd.body, err = json.Marshal(body)
	} else {
		if b.Cancelled || (b.Phase != "New" && b.Phase != "Pending" && b.Phase != "Running") {
			return cmd, "", model.Fail("stale_target", "Only an active Build can be cancelled")
		}
		cmd = buildCommand{method: http.MethodPatch, path: a.Path(), media: "application/json-patch+json"}
		cmd.body, err = json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": q.UID}, {"op": "test", "path": "/metadata/resourceVersion", "value": r.Version}, {"op": "add", "path": "/status/cancelled", "value": true}})
	}
	return cmd, page.Items[0].Fields, err
}
func (s *CreateService) PrepareBuildAction(ctx context.Context, r BuildActionRequest, environment string) (WorkflowPreview, error) {
	var out WorkflowPreview
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return out, err
	}
	defer release()
	q := r.Target
	c, err := newOperationClient(s.connections, q.Connection)
	if err != nil {
		return out, err
	}
	command, before, err := s.buildCommand(ctx, c, r)
	if err != nil {
		return out, err
	}
	if command.method == http.MethodPatch {
		var raw json.RawMessage
		_, err = c.send(ctx, command.method, command.path, true, command.body, command.media, &raw, nil)
		if err != nil {
			return out, err
		}
		if !validBuildActionResponse(r, raw, nil) {
			return out, model.Fail("invalid", "Server did not confirm guarded Build cancellation")
		}
	}
	ref := kube.ResourceRef{Connection: q.Connection, APIVersion: "build.openshift.io/v1", Kind: q.Kind, Namespace: q.Namespace, Name: q.Name, UID: q.UID}
	out = WorkflowPreview{Name: q.Name, Namespace: q.Namespace, Before: before, Action: r.Action, Impact: "One name-addressed BuildRequest. A replacement between the final identity check and dispatch is possible. Mutable images or source may resolve differently."}
	if r.Action == "cancel-build" {
		out.Impact = "Request cancellation of this version. Completion may race with cancellation."
	}
	out.Cluster, err = s.connections.ContextName(q.Connection)
	if err != nil {
		return out, err
	}
	ack := ""
	if strings.EqualFold(environment, "prod") || strings.EqualFold(environment, "production") {
		ack = q.Name
		out.Typed = true
	}
	out.Token, out.ExpiresAt, err = s.coordinator.save(ctx, pendingReview{result: OperationResult{Action: r.Action, Resource: ref, Cluster: out.Cluster}, payload: r, acknowledgement: ack})
	return out, err
}
func validBuildActionResponse(r BuildActionRequest, raw []byte, result *OperationResult) bool {
	var b struct {
		Kind, APIVersion string
		Metadata         struct{ Name, Namespace, UID, ResourceVersion string }
		Status           struct{ Cancelled bool }
	}
	if json.Unmarshal(raw, &b) != nil || b.Kind != "Build" || b.APIVersion != "build.openshift.io/v1" || len(validation.IsDNS1123Subdomain(b.Metadata.Name)) != 0 || b.Metadata.Namespace != r.Target.Namespace || b.Metadata.UID == "" || len(b.Metadata.UID) > 128 || b.Metadata.ResourceVersion == "" {
		return false
	}
	if r.Action == "cancel-build" && (b.Metadata.UID != r.Target.UID || b.Metadata.Name != r.Target.Name || !b.Status.Cancelled) {
		return false
	}
	if result != nil {
		v := kube.ResourceRef{Connection: r.Target.Connection, APIVersion: b.APIVersion, Kind: b.Kind, Namespace: b.Metadata.Namespace, Name: b.Metadata.Name, UID: b.Metadata.UID}
		result.ReturnedBuild = &v
	}
	return true
}
func (s *CreateService) ExecuteBuildAction(action, token, ack string) (OperationResult, error) {
	if !buildAction(action) {
		return OperationResult{}, model.Fail("invalid", "Unknown Build action")
	}
	saved, result, err := s.coordinator.begin(action, token, ack)
	if saved == nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(s.coordinator.ctx, operationDeadline)
	defer cancel()
	r := saved.payload.(BuildActionRequest)
	c, err := newOperationClient(s.connections, r.Target.Connection)
	var cmd buildCommand
	if err == nil {
		cmd, _, err = s.buildCommand(ctx, c, r)
	}
	if err != nil {
		result.State = "cancelled-before-send"
		result.Problem = &model.Problem{Code: "unavailable", Message: "Build source or permission changed. Refresh the original source."}
		return s.coordinator.finish(result), nil
	}
	var raw json.RawMessage
	_, err = c.send(ctx, cmd.method, cmd.path, false, cmd.body, cmd.media, &raw, nil)
	result.State = "unknown"
	result.Problem = &model.Problem{Code: "uncertain", Message: "Inspect Builds in the original project before another action."}
	if err == nil && validBuildActionResponse(r, raw, &result) {
		result.State = "accepted"
		result.Problem = nil
	}
	var code operationHTTPStatus
	if errors.As(err, &code) && code >= 400 && code < 500 && code != 408 {
		result.State = "rejected"
		result.Problem = &model.Problem{Code: "rejected", Message: "Server rejected the Build action"}
	}
	return s.coordinator.finish(result), nil
}
func (s *CreateService) RevokeBuildAction(action, token string) {
	if buildAction(action) {
		s.coordinator.revoke(action, token)
	}
}
