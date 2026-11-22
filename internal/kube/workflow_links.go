/*
Purpose: Follow current named references in OpenShift Build and Operator evidence.
Used by: Dedicated workspace details.
Responsibilities: Re-read the source relationship and fetch only the chosen destination and namespace.
Important: A name-based link does not prove historical ownership.
*/
package kube

import (
	"context"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	"slices"
	"strings"
	"time"
)

type WorkflowLink struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
}

func workflowLinks(kind, ns string, fields []byte) []WorkflowLink {
	links := []WorkflowLink{}
	add := func(kind, namespace, name, uid string) {
		if name == "" {
			return
		}
		if namespace == "" {
			namespace = ns
		}
		l := WorkflowLink{kind, namespace, name, uid}
		if !slices.Contains(links, l) {
			links = append(links, l)
		}
	}
	if kind == "Build" || kind == "BuildConfig" {
		var b BuildEvidence
		if json.Unmarshal(fields, &b) != nil {
			return nil
		}
		if b.Config != nil && b.Config.Kind == "BuildConfig" {
			add(b.Config.Kind, b.Config.Namespace, b.Config.Name, b.Config.UID)
		}
		for _, r := range []*BuildReference{b.Input, b.Output} {
			if r != nil && r.Kind == "ImageStreamTag" {
				add(r.Kind, r.Namespace, r.Name, r.UID)
			}
		}
	} else if kind == "Subscription" || kind == "PackageManifest" {
		var o OperatorEvidence
		if json.Unmarshal(fields, &o) != nil {
			return nil
		}
		add("CatalogSource", o.CatalogNamespace, o.Catalog, "")
		if kind == "Subscription" {
			add("PackageManifest", o.CatalogNamespace, o.Package, "")
			if o.Plan != nil && o.Plan.Kind == "InstallPlan" {
				add(o.Plan.Kind, o.Plan.Namespace, o.Plan.Name, o.Plan.UID)
			}
			add("ClusterServiceVersion", ns, o.InstalledCSV, "")
			add("ClusterServiceVersion", ns, o.CurrentCSV, "")
		}
	}
	return links
}
func (s *Service) ReadWorkflowLink(ctx context.Context, q WorkflowQuery, link WorkflowLink) (WorkflowItem, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if q.Name == "" || q.UID == "" || link.Name == "" {
		return WorkflowItem{}, model.Fail("invalid", "Select a captured reference")
	}
	verify := func() error {
		page, err := s.ReadWorkflow(ctx, q)
		if err != nil {
			return err
		}
		if len(page.Items) != 1 || !slices.Contains(page.Items[0].Links, link) {
			return model.Fail("stale_target", "Source no longer references this destination")
		}
		return nil
	}
	if err := verify(); err != nil {
		return WorkflowItem{}, err
	}
	var item WorkflowItem
	if link.Kind == "ImageStreamTag" {
		if len(strings.Split(link.Name, ":")) != 2 {
			return item, model.Fail("invalid", "Invalid ImageStreamTag name")
		}
		t, err := s.ResolveManifestTarget(ctx, q.Connection, "image.openshift.io/v1", link.Kind, "imagestreamtags", link.Namespace, link.Name, "get")
		if err != nil {
			return item, err
		}
		client, err := s.boundedReadClient(ctx, q.Connection)
		if err != nil {
			return item, err
		}
		raw, err := t.api().read(client, link.Namespace, link.Name).Do(ctx).Raw()
		if err != nil {
			return item, readError(ctx, err)
		}
		uid, rv, err := t.ReadIdentity(raw)
		if err != nil || uid == "" || rv == "" || link.UID != "" && uid != link.UID {
			return item, model.Fail("stale_target", "ImageStreamTag identity is unavailable or replaced")
		}
		r := t.Ref()
		r.UID = uid
		item = WorkflowItem{Resource: r, Version: rv, Fields: "{}"}
	} else {
		dest := WorkflowQuery{Connection: q.Connection, Kind: link.Kind, Namespace: link.Namespace, Name: link.Name, UID: link.UID}
		if dest.Namespace == q.Namespace {
			dest.ProjectUID = q.ProjectUID
		}
		p, e := s.ReadWorkflow(ctx, dest)
		if e != nil {
			return item, e
		}
		if len(p.Items) != 1 {
			return item, model.Fail("unavailable", "Destination unavailable")
		}
		item = p.Items[0]
	}
	return item, verify()
}

// ReadOperatorInstance verifies one explicitly named declared API instance and its ownership evidence.
func (s *Service) ReadOperatorInstance(ctx context.Context, q WorkflowQuery, declared OperatorAPI, ns, name string) (WorkflowItem, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var out WorkflowItem
	if q.Kind != "ClusterServiceVersion" || q.Name == "" || q.UID == "" || !genericName(name) {
		return out, model.Fail("invalid", "Select a CSV and a valid instance name")
	}
	verify := func() error {
		p, e := s.ReadWorkflow(ctx, q)
		if e != nil {
			return e
		}
		var f OperatorEvidence
		if len(p.Items) != 1 || json.Unmarshal([]byte(p.Items[0].Fields), &f) != nil || f.Copied || !slices.Contains(f.Owned, declared) {
			return model.Fail("unverified", "CSV does not declare this owned API")
		}
		return nil
	}
	if err := verify(); err != nil {
		return out, err
	}
	plural, group, ok := strings.Cut(declared.Name, ".")
	if !ok {
		return out, model.Fail("unresolved", "Declared API group is missing")
	}
	t, err := s.ResolveManifestTarget(ctx, q.Connection, group+"/"+declared.Version, declared.Kind, plural, ns, name, "get")
	if err != nil {
		return out, err
	}
	if ns != "" && ns != q.Namespace {
		groups, err := s.ReadWorkflow(ctx, WorkflowQuery{Connection: q.Connection, Namespace: q.Namespace, ProjectUID: q.ProjectUID, Kind: "OperatorGroup"})
		if err != nil {
			return out, err
		}
		var g OperatorEvidence
		if len(groups.Items) != 1 || groups.Next != "" || json.Unmarshal([]byte(groups.Items[0].Fields), &g) != nil || g.Targets == nil || (!slices.Contains(*g.Targets, ns) && !slices.Equal(*g.Targets, []string{""})) {
			return out, model.Fail("unverified", "Instance namespace is outside verified OperatorGroup scope")
		}
	}
	client, err := s.boundedReadClient(ctx, q.Connection)
	if err != nil {
		return out, err
	}
	raw, err := t.api().read(client, ns, name).Do(ctx).Raw()
	if err != nil {
		return out, readError(ctx, err)
	}
	id, err := objectIdentity(raw, t.api(), ns, false)
	if err != nil {
		return out, err
	}
	m := id.Metadata
	if m.Name != name || m.ResourceVersion == "" {
		return out, model.Fail("invalid", "Unexpected instance identity")
	}
	basis := ""
	for _, r := range m.OwnerReferences {
		if ns == q.Namespace && r.Kind == q.Kind && r.APIVersion == "operators.coreos.com/v1alpha1" && r.Name == q.Name && string(r.UID) == q.UID {
			basis = "Matching CSV owner reference"
		}
	}
	if basis == "" && m.Labels["olm.owner"] == q.Name && m.Labels["olm.owner.kind"] == q.Kind && m.Labels["olm.owner.namespace"] == q.Namespace {
		basis = "Current OLM owner labels (name-based association)"
	}
	if basis == "" {
		return out, model.Fail("unverified", "Declared API alone does not establish ownership of this instance")
	}
	out.Resource = t.Ref()
	out.Resource.UID = string(m.UID)
	out.Version = m.ResourceVersion
	b, _ := json.Marshal(map[string]string{"relationship": basis})
	out.Fields = string(b)
	return out, verify()
}
