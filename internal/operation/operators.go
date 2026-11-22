/*
Purpose: Review explicit OLM Classic install, approval and staged removal.
Used by: The desktop Operators workspace.
Responsibilities: Capture referenced identities, effective scope, plan steps and dependencies before each write.
Important: No step manifests are executed by the client and removal never cascades to CRDs or custom resources through direct writes.
*/
package operation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/apimachinery/pkg/util/validation"
	"net/http"
	"reflect"
	"slices"
)

type OperatorRequest struct {
	Action           string             `json:"action"`
	Target           kube.WorkflowQuery `json:"target"`
	Version          string             `json:"version"`
	CatalogNamespace string             `json:"catalogNamespace"`
	Catalog          string             `json:"catalog"`
	Package          string             `json:"package"`
	Channel          string             `json:"channel"`
	RemovalID        string             `json:"removalID"`
}
type operatorCommand struct {
	req                         OperatorRequest
	addr                        kube.ManifestAddress
	ref                         kube.ResourceRef
	method, media, verb, review string
	body                        []byte
	evidence                    [32]byte
	installed                   *kube.ResourceRef
}

func operatorAction(a string) bool {
	return a == "install-operator" || a == "approve-plan" || a == "remove-subscription" || a == "remove-csv"
}

type olmSnapshot struct {
	Identity kube.ResourceRef      `json:"identity"`
	Version  string                `json:"version"`
	Fields   kube.OperatorEvidence `json:"fields"`
}

func (s *CreateService) operatorCommand(ctx context.Context, r OperatorRequest) (operatorCommand, error) {
	cmd := operatorCommand{req: r, media: "application/json"}
	q := r.Target
	if !operatorAction(r.Action) || len(validation.IsDNS1123Subdomain(q.Name)) != 0 || len(validation.IsDNS1123Label(q.Namespace)) != 0 {
		return cmd, model.Fail("invalid", "Select an explicit Classic Operator target")
	}
	if err := s.verifyWorkflowProject(ctx, q); err != nil {
		return cmd, err
	}
	var captures []olmSnapshot
	capture := func(item kube.WorkflowItem) (olmSnapshot, error) {
		v := olmSnapshot{Identity: item.Resource, Version: item.Version}
		if json.Unmarshal([]byte(item.Fields), &v.Fields) != nil {
			return v, model.Fail("invalid", "Invalid Operator projection")
		}
		captures = append(captures, v)
		return v, nil
	}
	query := func(kind, ns, name, uid string) (kube.WorkflowPage, error) {
		p := kube.WorkflowQuery{Connection: q.Connection, Namespace: ns, Kind: kind, Name: name, UID: uid}
		if ns == q.Namespace {
			p.ProjectUID = q.ProjectUID
		}
		return s.reader.ReadWorkflow(ctx, p)
	}
	named := func(kind, ns, name, uid string) (olmSnapshot, error) {
		if name == "" {
			return olmSnapshot{}, model.Fail("unresolved", "Required Operator reference is absent")
		}
		page, err := query(kind, ns, name, uid)
		if err != nil {
			return olmSnapshot{}, err
		}
		if len(page.Items) != 1 {
			return olmSnapshot{}, model.Fail("unresolved", "Incomplete named Operator evidence")
		}
		return capture(page.Items[0])
	}
	list := func(kind, ns string) ([]kube.WorkflowItem, error) {
		p, e := query(kind, ns, "", "")
		if e != nil {
			return nil, e
		}
		if p.Next != "" {
			return nil, model.Fail("incomplete", "Operator impact needs a complete scoped inventory")
		}
		return p.Items, nil
	}
	groups, err := list("OperatorGroup", q.Namespace)
	if err != nil {
		return cmd, err
	}
	if len(groups) != 1 {
		return cmd, model.Fail("unresolved", "Exactly one compatible OperatorGroup is required")
	}
	group, err := capture(groups[0])
	if err != nil {
		return cmd, err
	}
	if group.Fields.Targets == nil || len(*group.Fields.Targets) == 0 {
		return cmd, model.Fail("unresolved", "OperatorGroup effective target namespaces are unavailable")
	}
	targets := *group.Fields.Targets
	mode := "MultiNamespace"
	if len(targets) == 1 {
		switch targets[0] {
		case "":
			mode = "AllNamespaces"
		case q.Namespace:
			mode = "OwnNamespace"
		default:
			mode = "SingleNamespace"
		}
	}
	for _, ns := range targets {
		if ns == "" && mode != "AllNamespaces" || ns != "" && len(validation.IsDNS1123Label(ns)) != 0 {
			return cmd, model.Fail("invalid", "Invalid effective OperatorGroup scope")
		}
	}
	kind, plural := "Subscription", "subscriptions"
	name := q.Name
	var uid, rv string
	var steps []string
	switch r.Action {
	case "install-operator":
		if q.UID != "" || q.Kind != "Subscription" || r.Channel == "" {
			return cmd, model.Fail("invalid", "Select a new Subscription, package and channel")
		}
		if _, err = named("CatalogSource", r.CatalogNamespace, r.Catalog, ""); err != nil {
			return cmd, err
		}
		pkg, e := named("PackageManifest", r.CatalogNamespace, r.Package, "")
		if e != nil {
			return cmd, e
		}
		f := pkg.Fields
		if f.Package != r.Package || f.Catalog != r.Catalog || f.CatalogNamespace != r.CatalogNamespace {
			return cmd, model.Fail("unresolved", "Package does not match the selected catalog")
		}
		compatible := false
		for _, ch := range f.Channels {
			if ch.Name == r.Channel && ch.CurrentCSV != "" {
				for _, m := range ch.CurrentCSVDesc.InstallModes {
					if m.Type == mode && m.Supported {
						compatible = true
					}
				}
			}
		}
		if !compatible {
			return cmd, model.Fail("unsupported", "Channel or effective install mode is unavailable")
		}
		cmd.method, cmd.verb = http.MethodPost, "create"
		cmd.body, err = json.Marshal(map[string]any{"apiVersion": "operators.coreos.com/v1alpha1", "kind": kind, "metadata": map[string]string{"name": name, "namespace": q.Namespace}, "spec": map[string]string{"name": r.Package, "source": r.Catalog, "sourceNamespace": r.CatalogNamespace, "channel": r.Channel, "installPlanApproval": "Manual"}})
	case "approve-plan", "remove-subscription":
		if q.Kind != "Subscription" || q.UID == "" || r.Version == "" {
			return cmd, model.Fail("invalid", "Select a captured Subscription")
		}
		sub, e := named("Subscription", q.Namespace, q.Name, q.UID)
		if e != nil {
			return cmd, e
		}
		if sub.Version != r.Version {
			return cmd, model.Fail("stale_target", "Subscription changed")
		}
		if r.Action == "approve-plan" {
			f := sub.Fields
			if f.Approval != "Manual" || f.Plan == nil || f.Plan.Kind != "InstallPlan" || f.Plan.Namespace != q.Namespace || f.Plan.UID == "" {
				return cmd, model.Fail("unresolved", "A linked manual InstallPlan with UID is required")
			}
			p, e := named("InstallPlan", q.Namespace, f.Plan.Name, f.Plan.UID)
			if e != nil {
				return cmd, e
			}
			if p.Fields.Approval != "Manual" || p.Fields.Approved || p.Fields.Phase != "RequiresApproval" || len(p.Fields.Steps) == 0 || len(p.Fields.CSVs) == 0 {
				return cmd, model.Fail("unresolved", "A resolved unapproved manual plan is required")
			}
			resolved := map[string]bool{}
			for _, step := range p.Fields.Steps {
				a := step.Resource
				if step.ManifestDigest == "" || a.Version == "" || a.Kind == "" || a.Name == "" || step.Resolving == "" {
					return cmd, model.Fail("unresolved", "Plan step identity is incomplete")
				}
				catalog, e := s.reader.ReadAPIResources(ctx, q.Connection, a.Group, a.Version, false)
				if e != nil {
					return cmd, e
				}
				matches := 0
				scope := "cluster-wide"
				for _, d := range catalog.Resources {
					if d.Kind == a.Kind {
						matches++
						if d.Namespaced {
							scope = q.Namespace
							if step.Namespace != "" {
								scope = step.Namespace
							}
						}
					}
				}
				if matches != 1 || scope == "cluster-wide" && step.Namespace != "" {
					return cmd, model.Fail("unresolved", "Plan step scope is ambiguous or unavailable")
				}
				steps = append(steps, fmt.Sprintf("%s · %s/%s · %s %s · %s", step.Resolving, a.Group, a.Version, a.Kind, a.Name, scope))
				if a.Kind == "ClusterServiceVersion" && a.Group == "operators.coreos.com" {
					resolved[a.Name] = true
				}
			}
			for _, csv := range p.Fields.CSVs {
				if !resolved[csv] {
					return cmd, model.Fail("incomplete", "A CSV has no resolved plan step")
				}
			}
			kind, plural, name, uid, rv = "InstallPlan", "installplans", p.Identity.Name, p.Identity.UID, p.Version
			cmd.method, cmd.verb, cmd.media = http.MethodPatch, "patch", "application/json-patch+json"
			cmd.body, err = json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": uid}, {"op": "test", "path": "/metadata/resourceVersion", "value": rv}, {"op": "add", "path": "/spec/approved", "value": true}})
		} else {
			uid, rv = q.UID, sub.Version
			if sub.Fields.InstalledCSV != "" {
				csv, e := named("ClusterServiceVersion", q.Namespace, sub.Fields.InstalledCSV, "")
				if e != nil {
					return cmd, e
				}
				if !csv.Fields.Copied {
					cmd.installed = &csv.Identity
				}
			}
			cmd.method, cmd.verb = http.MethodDelete, "delete"
		}
	case "remove-csv":
		previous, e := s.coordinator.GetOperationResult(r.RemovalID)
		if e != nil || previous.Action != "remove-subscription" || previous.State != "accepted" || previous.InstalledCSV == nil || previous.InstalledCSV.Connection != q.Connection || previous.InstalledCSV.Namespace != q.Namespace || previous.InstalledCSV.Name != q.Name || previous.InstalledCSV.UID != q.UID || q.Kind != "ClusterServiceVersion" {
			return cmd, model.Fail("unverified", "Select the captured CSV of a confirmed Subscription removal")
		}
		// The absence of the Subscription is independent of the accepted DELETE.
		_, e = named("Subscription", q.Namespace, previous.Resource.Name, "")
		var missing *model.Problem
		if !errors.As(e, &missing) || missing.Code != "not_found" {
			return cmd, model.Fail("unverified", "Cannot verify that the original Subscription is absent")
		}
		csv, e := named("ClusterServiceVersion", q.Namespace, q.Name, q.UID)
		if e != nil {
			return cmd, e
		}
		if csv.Fields.Copied || csv.Version != r.Version {
			return cmd, model.Fail("stale_target", "CSV is copied or changed")
		}
		if mode == "AllNamespaces" {
			return cmd, model.Fail("incomplete", "Cannot establish complete dependent evidence across all namespaces")
		}
		scopes := append([]string{q.Namespace}, targets...)
		slices.Sort(scopes)
		for _, ns := range slices.Compact(scopes) {
			subs, e := list("Subscription", ns)
			if e != nil {
				return cmd, e
			}
			for _, item := range subs {
				v, e := capture(item)
				if e != nil {
					return cmd, e
				}
				if v.Fields.InstalledCSV == q.Name || v.Fields.CurrentCSV == q.Name {
					return cmd, model.Fail("shared", "A remaining Subscription references the CSV")
				}
			}
			csvs, e := list("ClusterServiceVersion", ns)
			if e != nil {
				return cmd, e
			}
			for _, item := range csvs {
				v, e := capture(item)
				if e != nil {
					return cmd, e
				}
				if v.Identity.UID == q.UID || v.Fields.Copied {
					continue
				}
				for _, required := range append(v.Fields.Required, v.Fields.RequiredServices...) {
					for _, owned := range append(csv.Fields.Owned, csv.Fields.OwnedServices...) {
						if owned.Name == required.Name {
							return cmd, model.Fail("shared", "Another CSV requires an API supplied by this CSV")
						}
					}
				}
			}
		}
		kind, plural, uid, rv = "ClusterServiceVersion", "clusterserviceversions", csv.Identity.UID, csv.Version
		cmd.method, cmd.verb = http.MethodDelete, "delete"
	}
	cmd.addr = kube.ManifestAddress{Connection: q.Connection, Group: "operators.coreos.com", Version: "v1alpha1", Kind: kind, Resource: plural, Name: name, Namespace: q.Namespace}
	cmd.ref = cmd.addr.Ref()
	cmd.ref.UID = uid
	if cmd.method == http.MethodDelete {
		cmd.body, err = deleteOptions(cmd.ref, rv, false)
	}
	if err != nil {
		return cmd, err
	}
	b, err := json.MarshalIndent(map[string]any{"effectiveMode": mode, "evidence": captures, "steps": steps}, "", "  ")
	if err != nil || len(b) > 256<<10 {
		return cmd, model.Fail("capacity", "Complete Operator impact exceeds the review limit")
	}
	cmd.review = string(b)
	cmd.evidence = sha256.Sum256(b)
	return cmd, nil
}
func (c operatorCommand) dispatch(ctx context.Context, client operationClient, dry bool) (json.RawMessage, error) {
	path := c.addr.Path()
	subject := c.ref
	if c.method == http.MethodPost {
		path = c.addr.CollectionPath()
		subject.Name = ""
	}
	if permissionOf(ctx, client, subject, c.addr.Resource, c.verb, "") != "allowed" {
		return nil, model.Fail("forbidden", "Operator action permission unavailable")
	}
	var raw json.RawMessage
	_, err := client.send(ctx, c.method, path, dry, c.body, c.media, &raw, nil)
	return raw, err
}
func (c operatorCommand) matches(raw []byte) bool {
	if c.method == http.MethodDelete {
		return validDeleteResponse(raw, c.ref, c.addr.Resource)
	}
	uid, rv, err := c.addr.ReadIdentity(raw)
	if err != nil || rv == "" || uid == "" || (c.ref.UID != "" && uid != c.ref.UID) {
		return false
	}
	var result struct {
		Spec struct {
			Approved                                                    bool
			InstallPlanApproval, Name, Source, SourceNamespace, Channel string
		}
	}
	if json.Unmarshal(raw, &result) != nil {
		return false
	}
	if c.method == http.MethodPatch {
		return result.Spec.Approved
	}
	return result.Spec.InstallPlanApproval == "Manual" && result.Spec.Name == c.req.Package && result.Spec.Source == c.req.Catalog && result.Spec.SourceNamespace == c.req.CatalogNamespace && result.Spec.Channel == c.req.Channel
}
func (s *CreateService) PrepareOperator(ctx context.Context, r OperatorRequest) (WorkflowPreview, error) {
	var p WorkflowPreview
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return p, err
	}
	defer release()
	c, err := s.operatorCommand(ctx, r)
	if err != nil {
		return p, err
	}
	client, err := newOperationClient(s.connections, r.Target.Connection)
	if err != nil {
		return p, err
	}
	raw, err := c.dispatch(ctx, client, true)
	if err != nil {
		return p, err
	}
	if !c.matches(raw) {
		return p, model.Fail("invalid", "Server dry-run returned unexpected Operator evidence")
	}
	p.Name, p.Namespace, p.Before, p.Typed, p.Action = c.ref.Name, c.ref.Namespace, c.review, true, r.Action
	p.Cluster, err = s.connections.ContextName(c.ref.Connection)
	if err != nil {
		return p, err
	}
	p.Impact = map[string]string{"install-operator": "Create a Manual Subscription. Resolution and dependency installation require a separate plan review.", "approve-plan": "Approve all listed steps, including cluster-wide dependencies. Controllers perform installation.", "remove-subscription": "Stop updates. The installed CSV and data remain. Removing an eligible CSV needs another review.", "remove-csv": "Stop reconciliation by the reviewed CSV. CRDs, custom resources and data may remain."}[r.Action]
	p.Token, p.ExpiresAt, err = s.coordinator.save(ctx, pendingReview{result: OperationResult{Action: r.Action, Resource: c.ref, Cluster: p.Cluster, InstalledCSV: c.installed}, acknowledgement: p.Name, payload: c})
	return p, err
}
func (s *CreateService) ExecuteOperator(action, token, ack string) (OperationResult, error) {
	if !operatorAction(action) {
		return OperationResult{}, model.Fail("invalid", "Unknown Operator action")
	}
	saved, result, err := s.coordinator.begin(action, token, ack)
	if saved == nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(s.coordinator.ctx, operationDeadline)
	defer cancel()
	old := saved.payload.(operatorCommand)
	c, err := s.operatorCommand(ctx, old.req)
	if err != nil || c.evidence != old.evidence || c.ref != old.ref || !reflect.DeepEqual(c.body, old.body) {
		result.State = "cancelled-before-send"
		result.Problem = &model.Problem{Code: "stale_target", Message: "Operator references, scope or steps changed or became unreadable."}
		return s.coordinator.finish(result), nil
	}
	client, err := newOperationClient(s.connections, c.ref.Connection)
	if err != nil {
		result.State = "cancelled-before-send"
		return s.coordinator.finish(result), nil
	}
	raw, err := c.dispatch(ctx, client, false)
	result.State = "unknown"
	result.Problem = &model.Problem{Code: "uncertain", Message: "Inspect the reviewed Operator resources before another action."}
	if err == nil && c.matches(raw) {
		result.State = "accepted"
		result.Problem = nil
	}
	var code operationHTTPStatus
	var deny *model.Problem
	if errors.As(err, &code) && code >= 400 && code < 500 && code != 408 || errors.As(err, &deny) && deny.Code == "forbidden" {
		result.State = "rejected"
		result.Problem = &model.Problem{Code: "rejected", Message: "Operator operation rejected."}
	}
	return s.coordinator.finish(result), nil
}
func (s *CreateService) RevokeOperator(action, token string) {
	if operatorAction(action) {
		s.coordinator.revoke(action, token)
	}
}
