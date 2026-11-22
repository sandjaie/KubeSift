/*
Purpose: Discover and inspect supported OpenShift APIs independently per connection.
Used by: The OpenShift workspace.
Responsibilities: Enforce exact groups, versions and namespace scope for bounded lists and named reads.
Important: Only allowlisted resource types and projected fields cross the bridge.
*/
package kube

import (
	"context"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	"slices"
	"time"
)

var workflowAPIs = []apiResource{
	{"route.openshift.io", "v1", "routes", "Route", true},
	{"build.openshift.io", "v1", "buildconfigs", "BuildConfig", true},
	{"build.openshift.io", "v1", "builds", "Build", true},
	{"operators.coreos.com", "v1alpha1", "catalogsources", "CatalogSource", true},
	{"packages.operators.coreos.com", "v1", "packagemanifests", "PackageManifest", true},
	{"operators.coreos.com", "v1", "operatorgroups", "OperatorGroup", true},
	{"operators.coreos.com", "v1alpha1", "subscriptions", "Subscription", true},
	{"operators.coreos.com", "v1alpha1", "installplans", "InstallPlan", true},
	{"operators.coreos.com", "v1alpha1", "clusterserviceversions", "ClusterServiceVersion", true},
	{"olm.operatorframework.io", "v1", "clustercatalogs", "ClusterCatalog", false},
	{"olm.operatorframework.io", "v1", "clusterextensions", "ClusterExtension", false},
}

type WorkflowDiscovery struct {
	Resources    []APIResourceDescriptor `json:"resources"`
	Problems     map[string]string       `json:"problems"`
	BuildActions map[string]bool         `json:"buildActions"`
}

func (s *Service) WorkflowDiscovery(ctx context.Context, ref model.ConnectionRef, refresh bool) (WorkflowDiscovery, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out := WorkflowDiscovery{Resources: []APIResourceDescriptor{}, Problems: map[string]string{}, BuildActions: map[string]bool{}}
	groups, err := s.ReadAPIGroups(ctx, ref, refresh)
	if err != nil {
		return out, err
	}
	if groups.APIsProblem != nil {
		return out, groups.APIsProblem
	}
	seen := map[string]bool{}
	for _, api := range workflowAPIs {
		gv := api.apiVersion()
		if seen[gv] {
			continue
		}
		seen[gv] = true
		if !slices.ContainsFunc(groups.Groups, func(g APIGroup) bool { return g.Group == api.group && slices.Contains(g.Versions, api.version) }) {
			continue
		}
		catalog, err := s.ReadAPIResources(ctx, ref, api.group, api.version, refresh)
		if err != nil {
			out.Problems[gv] = err.Error()
			continue
		}
		for _, d := range catalog.Resources {
			if slices.Contains(workflowAPIs, d.api()) {
				out.Resources = append(out.Resources, d)
			}
		}
		for action, ok := range catalog.buildActions {
			out.BuildActions[action] = ok
		}
	}
	return out, s.connections.Validate(ref)
}

type WorkflowQuery struct {
	Connection model.ConnectionRef `json:"connection"`
	Namespace  string              `json:"namespace"`
	ProjectUID string              `json:"projectUID"`
	Kind       string              `json:"kind"`
	Name       string              `json:"name"`
	UID        string              `json:"uid"`
	Cursor     string              `json:"cursor"`
}
type WorkflowItem struct {
	Links    []WorkflowLink `json:"links,omitempty"`
	Resource ResourceRef    `json:"resource"`
	Version  string         `json:"version"`
	Fields   string         `json:"fields"`
}
type WorkflowPage struct {
	Items []WorkflowItem `json:"items"`
	Next  string         `json:"next"`
}

func (s *Service) ReadWorkflow(ctx context.Context, q WorkflowQuery) (WorkflowPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var out WorkflowPage
	i := slices.IndexFunc(workflowAPIs, func(a apiResource) bool { return a.kind == q.Kind })
	if i < 0 {
		return out, model.Fail("invalid", "Unknown OpenShift resource")
	}
	api := workflowAPIs[i]
	if err := genericAddress(api, q.Namespace); err != nil {
		return out, err
	}
	verify := func() error {
		if q.ProjectUID != "" {
			p, err := s.ReadProject(ctx, q.Connection, q.Namespace, q.ProjectUID)
			if err != nil {
				return err
			}
			if p.Phase != "Active" || p.DeletingAt != "" {
				return model.Fail("project_unavailable", "Project is no longer active")
			}
		}
		return s.connections.Validate(q.Connection)
	}
	if err := verify(); err != nil {
		return out, err
	}
	if q.Name != "" && !genericName(q.Name) || q.Name == "" && q.UID != "" || q.Name != "" && q.Cursor != "" {
		return out, model.Fail("invalid", "Invalid OpenShift resource target")
	}
	client, err := s.boundedReadClient(ctx, q.Connection)
	if err != nil {
		return out, err
	}
	req := api.read(client, q.Namespace, q.Name)
	var rows []json.RawMessage
	if q.Name != "" {
		raw, err := req.Do(ctx).Raw()
		if err != nil {
			return out, readError(ctx, err)
		}
		rows = []json.RawMessage{raw}
	} else {
		scope := pageCursor{Connection: q.Connection, Kind: q.Kind, Namespace: q.Namespace, Query: q.ProjectUID}
		token, err := s.decodeGenericCursor(q.Cursor, scope)
		if err != nil {
			return out, err
		}
		list, err := listJSON(ctx, req, api, token, true)
		if err != nil {
			return out, resourceListError(ctx, err)
		}
		rows = list.Items
		if list.Metadata.Continue != "" {
			if token == list.Metadata.Continue {
				return out, model.Fail("list_changed", "Repeated page cursor")
			}
			next, err := encodeCursor(scope, list.Metadata.Continue)
			if err != nil {
				return out, err
			}
			out.Next = next + "." + s.signGenericCursor(next)
		}
	}
	out.Items = []WorkflowItem{}
	for _, raw := range rows {
		obj, err := objectIdentity(raw, api, q.Namespace, q.Name == "")
		if err != nil {
			return WorkflowPage{}, err
		}
		m := obj.Metadata
		if !genericName(m.Name) || m.ResourceVersion == "" || (q.Name != "" && m.Name != q.Name) {
			return WorkflowPage{}, model.Fail("invalid", "Invalid OpenShift object identity")
		}
		if q.UID != "" && q.UID != string(m.UID) {
			return WorkflowPage{}, model.Fail("resource_replaced", "Resource replaced; select again")
		}
		fields, err := workflowFields(api, raw)
		if err != nil {
			return WorkflowPage{}, err
		}
		encoded, err := json.MarshalIndent(fields, "", "  ")
		if err != nil {
			return WorkflowPage{}, model.Fail("invalid", "Invalid projected fields")
		}
		out.Items = append(out.Items, WorkflowItem{Resource: ResourceRef{Connection: q.Connection, Namespace: q.Namespace, Name: m.Name, UID: string(m.UID), Kind: api.kind, APIVersion: api.apiVersion()}, Version: m.ResourceVersion, Fields: string(encoded), Links: workflowLinks(api.kind, q.Namespace, encoded)})
	}
	if err := checkPayload(out); err != nil {
		return WorkflowPage{}, err
	}
	return out, verify()
}
func workflowFields(api apiResource, raw []byte) (any, error) {
	if api.kind == "Route" {
		return ProjectRoute(raw)
	}
	if api.kind == "Build" || api.kind == "BuildConfig" {
		return projectBuild(raw)
	}
	if api.group == "olm.operatorframework.io" {
		return projectOLMv1(raw)
	}
	return projectOperator(api.kind, raw)
}
