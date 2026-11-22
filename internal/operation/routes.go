/*
Purpose: Prepare and confirm field-limited OpenShift Route changes.
Used by: The desktop OpenShift operation dialogs and Session Results.
Responsibilities: Keep the request and preconditions native, dry-run admission and consume each review once.
Important: Only allowlisted Route projections can reach the bridge.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/apimachinery/pkg/util/validation"
	"net/http"
	"reflect"
	"strings"
	"time"
)

type RouteRequest struct {
	Target  kube.WorkflowQuery `json:"target"`
	Version string             `json:"version"`
	Action  string             `json:"action"`
	Fields  kube.RouteFields   `json:"fields"`
}
type WorkflowPreview struct {
	Cluster   string    `json:"cluster"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Action    string    `json:"action"`
	Before    string    `json:"before"`
	Admitted  string    `json:"admitted"`
	Impact    string    `json:"impact"`
	Typed     bool      `json:"typed"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type routeReview struct {
	Request             RouteRequest
	Address             kube.ManifestAddress
	Method, Path, Media string
	Body                []byte
}

func validateRouteFields(f kube.RouteFields, create bool) error {
	invalid := model.Fail("invalid", "Enter compatible Route fields, Service names and weights from 0 to 256")
	if len(f.AlternateBackends) > 3 || len(f.Path) > 2048 || (f.Path != "" && !strings.HasPrefix(f.Path, "/")) {
		return invalid
	}
	for _, b := range append(append([]kube.RouteBackend{}, f.To), f.AlternateBackends...) {
		if !(b.Kind == "" || b.Kind == "Service") || len(validation.IsDNS1035Label(b.Name)) != 0 || (b.Weight != nil && (*b.Weight < 0 || *b.Weight > 256)) {
			return invalid
		}
	}
	if f.Host != "" && len(validation.IsDNS1123Subdomain(f.Host)) != 0 {
		return invalid
	}
	if f.Port != nil {
		p := f.Port.TargetPort
		if p.Type == 0 && (p.IntVal < 1 || p.IntVal > 65535) || p.Type != 0 && len(validation.IsValidPortName(p.StrVal)) > 0 {
			return invalid
		}
	}
	if f.TLS != nil {
		if create && (f.TLS.ExternalCertificate != nil || f.TLS.Termination != "edge" && f.TLS.Termination != "passthrough") {
			return invalid
		}
		if f.TLS.Termination == "passthrough" && (f.Path != "" || f.TLS.Policy == "Allow") {
			return invalid
		}
	}
	return nil
}
func routePatch(before, after kube.RouteFields, uid, version string) ([]byte, error) {
	if before.Host != after.Host || !reflect.DeepEqual(before.TLS, after.TLS) {
		return nil, model.Fail("unsupported", "Host and TLS settings are read-only on an existing Route")
	}
	toMap := func(v any) map[string]any {
		b, _ := json.Marshal(v)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m
	}
	ops := []map[string]any{{"op": "test", "path": "/metadata/uid", "value": uid}, {"op": "test", "path": "/metadata/resourceVersion", "value": version}}
	add := func(op, path string, v any) error {
		entry := map[string]any{"op": op, "path": path}
		if op != "remove" {
			entry["value"] = v
		}
		ops = append(ops, entry)
		return nil
	}
	old, newFields := toMap(before), toMap(after)
	// Patch existing entries in place so future backend fields survive edits.
	if len(before.AlternateBackends) > 0 && len(after.AlternateBackends) > 0 {
		delete(old, "alternateBackends")
		delete(newFields, "alternateBackends")
		for i, b := range after.AlternateBackends {
			path := fmt.Sprintf("/spec/alternateBackends/%d", i)
			if i < len(before.AlternateBackends) {
				_ = diffEditMaps(path, toMap(before.AlternateBackends[i]), toMap(b), add)
			} else {
				_ = add("add", path, b)
			}
		}
		for i := len(before.AlternateBackends) - 1; i >= len(after.AlternateBackends); i-- {
			_ = add("remove", fmt.Sprintf("/spec/alternateBackends/%d", i), nil)
		}
	}
	if err := diffEditMaps("/spec", old, newFields, add); err != nil {
		return nil, err
	}
	if len(ops) == 2 {
		return nil, model.Fail("no_changes", "No Route fields changed")
	}
	return json.Marshal(ops)
}
func (s *CreateService) verifyWorkflowProject(ctx context.Context, q kube.WorkflowQuery) error {
	if q.ProjectUID == "" {
		return s.connections.Validate(q.Connection)
	}
	p, err := s.reader.ReadProject(ctx, q.Connection, q.Namespace, q.ProjectUID)
	if err != nil {
		return err
	}
	if p.Phase != "Active" || p.DeletingAt != "" {
		return model.Fail("project_unavailable", "Project is no longer active")
	}
	return nil
}
func (s *CreateService) PrepareRoute(ctx context.Context, r RouteRequest, environment string) (WorkflowPreview, error) {
	var out WorkflowPreview
	q := r.Target
	if q.Kind != "Route" || len(validation.IsDNS1123Subdomain(q.Name)) != 0 || len(validation.IsDNS1123Label(q.Namespace)) != 0 {
		return out, model.Fail("invalid", "Explicit Route name and namespace required")
	}
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return out, err
	}
	defer release()
	if err := s.verifyWorkflowProject(ctx, q); err != nil {
		return out, err
	}
	t := kube.ManifestAddress{Connection: q.Connection, Group: "route.openshift.io", Version: "v1", Resource: "routes", Kind: "Route", Namespace: q.Namespace, Name: q.Name}
	ref := t.Ref()
	ref.UID = q.UID
	c, err := newOperationClient(s.connections, q.Connection)
	if err != nil {
		return out, err
	}
	review := routeReview{Request: r, Address: t, Path: t.Path(), Media: "application/json"}
	out.Name, out.Namespace, out.Action = q.Name, q.Namespace, r.Action
	verb := ""
	switch r.Action {
	case "create-route":
		if q.UID != "" || r.Version != "" {
			return out, model.Fail("invalid", "A new Route has no UID or version")
		}
		verb = "create"
		review.Path = t.CollectionPath()
		review.Method = http.MethodPost
		if err = validateRouteFields(r.Fields, true); err != nil {
			return out, err
		}
		review.Body, err = json.Marshal(map[string]any{"apiVersion": ref.APIVersion, "kind": "Route", "metadata": map[string]string{"name": q.Name, "namespace": q.Namespace}, "spec": r.Fields})
		out.Impact = "Create an ingress Route to the reviewed Services. Router admission is observed separately."
	case "edit-route", "delete-route":
		if q.UID == "" || r.Version == "" {
			return out, model.Fail("invalid", "Select a captured Route version")
		}
		page, e := s.reader.ReadWorkflow(ctx, q)
		if e != nil {
			return out, e
		}
		if len(page.Items) != 1 || page.Items[0].Version != r.Version {
			return out, model.Fail("stale_target", "Route changed; refresh and review again")
		}
		out.Before = page.Items[0].Fields
		if r.Action == "delete-route" {
			verb = "delete"
			review.Method = http.MethodDelete
			review.Body, err = deleteOptions(ref, r.Version, false)
			out.Typed = true
			out.Impact = "Remove this ingress Route. The Service remains."
		} else {
			verb = "patch"
			review.Method = http.MethodPatch
			review.Media = "application/json-patch+json"
			if err = validateRouteFields(r.Fields, false); err != nil {
				return out, err
			}
			var before struct{ Spec kube.RouteFields }
			if json.Unmarshal([]byte(out.Before), &before) != nil {
				return out, model.Fail("invalid", "Invalid Route projection")
			}
			review.Body, err = routePatch(before.Spec, r.Fields, q.UID, r.Version)
			out.Impact = "Update only the reviewed Route fields."
		}
	default:
		return out, model.Fail("unsupported", "Unsupported Route action")
	}
	if err != nil {
		return out, err
	}
	permRef := ref
	if verb == "create" {
		permRef.Name = ""
	}
	if permissionOf(ctx, c, permRef, "routes", verb, "") != "allowed" {
		return out, model.Fail("forbidden", "Route permission unavailable")
	}
	var raw json.RawMessage
	dryBody := review.Body
	if review.Method == http.MethodDelete {
		dryBody, err = deleteOptions(ref, r.Version, true)
		if err != nil {
			return out, err
		}
	}
	_, err = c.send(ctx, review.Method, review.Path, true, dryBody, review.Media, &raw, nil)
	if err != nil {
		return out, err
	}
	if r.Action == "delete-route" {
		if !validDeleteResponse(raw, ref, "routes") {
			return out, invalidDeleteEvidence()
		}
	} else {
		uid, _, err := t.ReadIdentity(raw)
		if err != nil || uid == "" || q.UID != "" && q.UID != uid {
			return out, model.Fail("invalid", "Invalid admitted Route identity")
		}
		projected, e := kube.ProjectRoute(raw)
		if e != nil {
			return out, e
		}
		b, _ := json.MarshalIndent(projected, "", "  ")
		out.Admitted = string(b)
	}
	out.Cluster, err = s.connections.ContextName(q.Connection)
	if err != nil {
		return out, err
	}
	out.Typed = out.Typed || strings.EqualFold(environment, "production") || strings.EqualFold(environment, "prod")
	ack := ""
	if out.Typed {
		ack = q.Name
	}
	out.Token, out.ExpiresAt, err = s.coordinator.save(ctx, pendingReview{result: OperationResult{Action: r.Action, Resource: ref, Cluster: out.Cluster}, payload: review, acknowledgement: ack})
	return out, err
}
func (s *CreateService) ExecuteRoute(action, token, ack string) (OperationResult, error) {
	if !routeAction(action) {
		return OperationResult{}, model.Fail("invalid", "Invalid Route action")
	}
	saved, result, err := s.coordinator.begin(action, token, ack)
	if saved == nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(s.coordinator.ctx, operationDeadline)
	defer cancel()
	r := saved.payload.(routeReview)
	q := r.Request.Target
	c, err := newOperationClient(s.connections, q.Connection)
	if err == nil {
		err = s.verifyWorkflowProject(ctx, q)
	}
	if err != nil {
		result.State = "cancelled-before-send"
		result.Problem = &model.Problem{Code: "unavailable", Message: "Original connection or Project unavailable"}
		return s.coordinator.finish(result), nil
	}
	verb := map[string]string{http.MethodPost: "create", http.MethodPatch: "patch", http.MethodDelete: "delete"}[r.Method]
	permRef := r.Address.Ref()
	if r.Method == http.MethodPost {
		permRef.Name = ""
	}
	if permissionOf(ctx, c, permRef, "routes", verb, "") != "allowed" {
		result.State = "cancelled-before-send"
		result.Problem = &model.Problem{Code: "forbidden", Message: "Route permission changed. Refresh before reviewing again."}
		return s.coordinator.finish(result), nil
	}
	var raw json.RawMessage
	_, err = c.send(ctx, r.Method, r.Path, false, r.Body, r.Media, &raw, nil)
	result.State = "unknown"
	result.Problem = &model.Problem{Code: "uncertain", Message: "Inspect the original Route before another action."}
	var code operationHTTPStatus
	if errors.As(err, &code) && code >= 400 && code < 500 && code != 408 {
		result.State = "rejected"
		result.Problem = &model.Problem{Code: "rejected", Message: "Server rejected the reviewed Route operation."}
	}
	if err == nil {
		uid, _, e := r.Address.ReadIdentity(raw)
		if action == "delete-route" && validDeleteResponse(raw, result.Resource, "routes") || action != "delete-route" && e == nil && uid != "" && (q.UID == "" || q.UID == uid) {
			result.State = "accepted"
			result.Problem = nil
			if uid != "" {
				result.Resource.UID = uid
			}
		}
	}
	return s.coordinator.finish(result), nil
}
func routeAction(a string) bool {
	return a == "create-route" || a == "edit-route" || a == "delete-route"
}
func (s *CreateService) RevokeRoute(action, token string) {
	if routeAction(action) {
		s.coordinator.revoke(action, token)
	}
}
