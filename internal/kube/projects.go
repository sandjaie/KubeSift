/*
Purpose: Read visible OpenShift Projects and verify captured Project identities.
Used by: The Projects workspace and connection access probe.
Responsibilities: Discover exact API capabilities and project bounded, caller-visible fields.
Important: Namespace inventory is never used as evidence of Project membership.
*/
package kube

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	auth "k8s.io/api/authorization/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ProjectGroup = "project.openshift.io"
const ProjectVersion = ProjectGroup + "/v1"
const projectPath = "/apis/" + ProjectVersion + "/projects"

type ProjectCapabilities struct {
	Projects     bool `json:"projects"`
	List         bool `json:"list"`
	Get          bool `json:"get"`
	Delete       bool `json:"delete"`
	Request      bool `json:"request"`
	CheckRequest bool `json:"checkRequest"`
}

func (s *Service) ProjectCapabilities(ctx context.Context, ref model.ConnectionRef, refresh bool) (caps ProjectCapabilities, err error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	groups, err := s.ReadAPIGroups(ctx, ref, refresh)
	if err != nil {
		return caps, err
	}
	if groups.APIsProblem != nil {
		return caps, groups.APIsProblem
	}
	if !slices.ContainsFunc(groups.Groups, func(g APIGroup) bool { return g.Group == ProjectGroup && slices.Contains(g.Versions, "v1") }) {
		return caps, nil
	}
	catalog, err := s.ReadAPIResources(ctx, ref, ProjectGroup, "v1", refresh)
	if err != nil {
		return caps, err
	}
	for _, d := range catalog.Resources {
		if d.Namespaced {
			continue
		}
		if d.Kind == "Project" && d.Resource == "projects" {
			caps.Projects = true
			caps.List = d.List
			caps.Get = d.Get
			caps.Delete = d.Delete
		}
		if d.Kind == "ProjectRequest" && d.Resource == "projectrequests" {
			caps.Request = d.Create
			caps.CheckRequest = d.List
		}
	}
	return caps, nil
}

type Project struct {
	Resource    ResourceRef `json:"resource"`
	Version     string      `json:"version"`
	DisplayName string      `json:"displayName"`
	Description string      `json:"description"`
	Truncated   bool        `json:"truncated"`
	Phase       string      `json:"phase"`
	CreatedAt   string      `json:"createdAt"`
	DeletingAt  string      `json:"deletingAt"`
}
type ProjectPage struct {
	Items      []Project `json:"items"`
	Next       string    `json:"next"`
	Partial    bool      `json:"partial"`
	ObservedAt time.Time `json:"observedAt"`
}
type wireProject struct {
	meta.TypeMeta
	Metadata struct {
		Name, Namespace, UID, ResourceVersion, CreationTimestamp, DeletionTimestamp string
		Annotations                                                                 map[string]string
	}
	Status struct{ Phase string }
}

func projectText(value string, limit int) (string, bool) {
	for i := range value {
		if limit == 0 {
			return value[:i], true
		}
		limit--
	}
	return value, false
}
func (w wireProject) project(ref model.ConnectionRef) (Project, error) {
	m := w.Metadata
	if namespace(m.Name) != nil || m.Namespace != "" || m.UID == "" || len(m.UID) > 128 || m.ResourceVersion == "" || len(m.ResourceVersion) > 128 || (w.Kind != "" && w.Kind != "Project") || (w.APIVersion != "" && w.APIVersion != ProjectVersion) {
		return Project{}, model.Fail("invalid_response", "Invalid Project identity")
	}
	display, dt := projectText(m.Annotations["openshift.io/display-name"], 256)
	desc, tt := projectText(m.Annotations["openshift.io/description"], 2048)
	phase := w.Status.Phase
	if phase != "Active" && phase != "Terminating" {
		phase = "Unknown"
	}
	return Project{Resource: ResourceRef{Connection: ref, Kind: "Project", APIVersion: ProjectVersion, Name: m.Name, UID: m.UID}, Version: m.ResourceVersion, DisplayName: display, Description: desc, Truncated: dt || tt, Phase: phase, CreatedAt: m.CreationTimestamp, DeletingAt: m.DeletionTimestamp}, nil
}
func (s *Service) ListProjects(ctx context.Context, ref model.ConnectionRef, cursor string) (ProjectPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	scope := pageCursor{Connection: ref, Kind: "Project"}
	token, err := decodeCursor(cursor, scope)
	if err != nil {
		return ProjectPage{}, err
	}
	client, err := s.boundedReadClient(ctx, ref)
	if err != nil {
		return ProjectPage{}, err
	}
	req := fixedRead(client, projectPath).Param("limit", "100")
	if token != "" {
		req.Param("continue", token)
	}
	raw, err := req.Do(ctx).Raw()
	if err != nil {
		return ProjectPage{}, resourceListError(ctx, err)
	}
	var list struct {
		meta.TypeMeta
		Metadata meta.ListMeta
		Items    []wireProject
	}
	if json.Unmarshal(raw, &list) != nil || list.Kind != "ProjectList" || list.APIVersion != ProjectVersion || list.Items == nil {
		return ProjectPage{}, model.Fail("invalid_response", "Invalid Project list")
	}
	p := ProjectPage{Items: []Project{}, ObservedAt: time.Now().UTC()}
	for _, w := range list.Items {
		if len(p.Items) == 500 {
			p.Partial = true
			break
		}
		item, err := w.project(ref)
		if err != nil {
			return ProjectPage{}, err
		}
		p.Items = append(p.Items, item)
	}
	if list.Metadata.Continue != "" {
		p.Partial = true
		if list.Metadata.Continue != token && len(p.Items) < 500 {
			p.Next, err = encodeCursor(scope, list.Metadata.Continue)
			if err != nil {
				return ProjectPage{}, err
			}
		}
	}
	if ctx.Err() != nil {
		return ProjectPage{}, safeError(ctx, ctx.Err())
	}
	return p, s.connections.Validate(ref)
}
func (s *Service) ReadProject(ctx context.Context, ref model.ConnectionRef, name, uid string) (Project, error) {
	if err := namespace(name); err != nil {
		return Project{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client, err := s.boundedReadClient(ctx, ref)
	if err != nil {
		return Project{}, err
	}
	raw, err := fixedRead(client, projectPath+"/"+name).Do(ctx).Raw()
	if err != nil {
		return Project{}, readError(ctx, err)
	}
	var w wireProject
	if json.Unmarshal(raw, &w) != nil || w.Kind != "Project" || w.APIVersion != ProjectVersion {
		return Project{}, model.Fail("invalid_response", "Invalid Project")
	}
	p, err := w.project(ref)
	if err != nil {
		return Project{}, err
	}
	if p.Resource.Name != name || (uid != "" && p.Resource.UID != uid) {
		return Project{}, model.Fail("stale_target", "Project identity changed")
	}
	if ctx.Err() != nil {
		return Project{}, safeError(ctx, ctx.Err())
	}
	return p, s.connections.Validate(ref)
}
func (s *Service) verifyProject(ctx context.Context, r ResourceRef) error {
	if r.Kind != "Project" || r.APIVersion != ProjectVersion || r.Namespace != "" || r.UID == "" {
		return model.Fail("invalid", "Select a captured Project")
	}
	_, err := s.ReadProject(ctx, r.Connection, r.Name, r.UID)
	if err != nil {
		return model.Fail("project_unavailable", "The selected Project could not be verified. Look it up again.")
	}
	return nil
}

type ProjectAccess struct {
	Pods       string    `json:"pods"`
	Quotas     string    `json:"quotas"`
	Delete     string    `json:"delete"`
	ObservedAt time.Time `json:"observedAt"`
}

func (s *Service) MyProjectAccess(ctx context.Context, r ResourceRef) (ProjectAccess, error) {
	if err := s.verifyProject(ctx, r); err != nil {
		return ProjectAccess{}, err
	}
	client, err := s.boundedReadClient(ctx, r.Connection)
	if err != nil {
		return ProjectAccess{}, err
	}
	check := func(attrs auth.ResourceAttributes) string {
		var review auth.SelfSubjectAccessReview
		body, err := json.Marshal(auth.SelfSubjectAccessReview{TypeMeta: meta.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SelfSubjectAccessReview"}, Spec: auth.SelfSubjectAccessReviewSpec{ResourceAttributes: &attrs}})
		if err != nil {
			return "unknown"
		}
		raw, err := client.AuthorizationV1().RESTClient().Post().Resource("selfsubjectaccessreviews").SetHeader("Content-Type", "application/json").SetHeader("Accept", "application/json").MaxRetries(0).Body(body).Do(ctx).Raw()
		if err != nil || json.Unmarshal(raw, &review) != nil || review.Kind != "SelfSubjectAccessReview" || review.APIVersion != "authorization.k8s.io/v1" {
			return "unknown"
		}
		if review.Status.EvaluationError != "" || review.Status.Allowed == review.Status.Denied {
			return "unknown"
		}
		if review.Status.Allowed {
			return "allowed"
		}
		return "denied"
	}
	a := ProjectAccess{Pods: check(auth.ResourceAttributes{Verb: "list", Resource: "pods", Namespace: r.Name}), Quotas: check(auth.ResourceAttributes{Verb: "list", Resource: "resourcequotas", Namespace: r.Name}), Delete: check(auth.ResourceAttributes{Verb: "delete", Group: ProjectGroup, Resource: "projects", Name: r.Name}), ObservedAt: time.Now().UTC()}
	if err := s.verifyProject(ctx, r); err != nil {
		return ProjectAccess{}, err
	}
	return a, nil
}

type ProjectQuota struct {
	Name string            `json:"name"`
	Hard map[string]string `json:"hard"`
	Used map[string]string `json:"used"`
}
type ProjectQuotaPage struct {
	Items []ProjectQuota `json:"items"`
	Next  string         `json:"next"`
}

func (s *Service) ProjectQuotas(ctx context.Context, r ResourceRef, cursor string) (ProjectQuotaPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.verifyProject(ctx, r); err != nil {
		return ProjectQuotaPage{}, err
	}
	scope := pageCursor{Connection: r.Connection, Kind: "ProjectQuotas", Namespace: r.Name, Query: r.UID}
	token, err := decodeCursor(cursor, scope)
	if err != nil {
		return ProjectQuotaPage{}, err
	}
	client, err := s.boundedReadClient(ctx, r.Connection)
	if err != nil {
		return ProjectQuotaPage{}, err
	}
	req := fixedRead(client, "/api/v1/namespaces/"+r.Name+"/resourcequotas").Param("limit", "100")
	if token != "" {
		req.Param("continue", token)
	}
	raw, err := req.Do(ctx).Raw()
	if err != nil {
		return ProjectQuotaPage{}, configurationListError(ctx, err)
	}
	var list struct {
		meta.TypeMeta
		Metadata meta.ListMeta
		Items    []struct {
			Metadata struct{ Name, Namespace, UID string }
			Status   struct{ Hard, Used map[string]string }
		}
	}
	if json.Unmarshal(raw, &list) != nil || list.Kind != "ResourceQuotaList" || list.APIVersion != "v1" || list.Items == nil || len(list.Items) > 100 {
		return ProjectQuotaPage{}, model.Fail("invalid_response", "Invalid namespace quota page")
	}
	p := ProjectQuotaPage{Items: []ProjectQuota{}}
	for _, row := range list.Items {
		if row.Metadata.Namespace != r.Name || !genericName(row.Metadata.Name) || row.Metadata.UID == "" {
			return ProjectQuotaPage{}, model.Fail("invalid_response", "Invalid namespace quota identity")
		}
		p.Items = append(p.Items, ProjectQuota{row.Metadata.Name, row.Status.Hard, row.Status.Used})
	}
	p.Next, err = encodeCursor(scope, list.Metadata.Continue)
	if err != nil {
		return ProjectQuotaPage{}, err
	}
	if err := checkPayload(p); err != nil {
		return ProjectQuotaPage{}, err
	}
	if err := s.verifyProject(ctx, r); err != nil {
		return ProjectQuotaPage{}, err
	}
	return p, nil
}
