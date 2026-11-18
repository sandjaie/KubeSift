/*
Purpose: Describe scoped Helm changes and preview newly declared custom resources.
Used by: Native preparation before issuing a confirmation token.
Responsibilities: Defer only absent packaged CRD kinds and identify resources removed by changes.
Important: Deferred objects remain in review; execution always uses the normal SDK client.
*/
package helmmanager

import (
	"bytes"
	"io"
	"strings"

	"github.com/sandjaie/KubeSift/internal/model"
	"go.yaml.in/yaml/v3"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/kube"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func declaredCRDs(ch *chartv2.Chart) (map[schema.GroupVersionKind]bool, error) {
	out := map[schema.GroupVersionKind]bool{}
	for _, file := range ch.CRDObjects() {
		decoder := yaml.NewDecoder(bytes.NewReader(file.File.Data))
		for {
			var crd struct {
				Kind string `yaml:"kind"`
				Spec struct {
					Group string `yaml:"group"`
					Scope string `yaml:"scope"`
					Names struct {
						Kind string `yaml:"kind"`
					} `yaml:"names"`
					Versions []struct {
						Name   string `yaml:"name"`
						Served bool   `yaml:"served"`
					} `yaml:"versions"`
				} `yaml:"spec"`
			}
			if err := decoder.Decode(&crd); err == io.EOF {
				break
			} else if err != nil {
				return nil, model.Fail("invalid", "A packaged CRD could not be reviewed.")
			}
			if crd.Kind != "CustomResourceDefinition" || crd.Spec.Group == "" || crd.Spec.Names.Kind == "" || (crd.Spec.Scope != "Namespaced" && crd.Spec.Scope != "Cluster") {
				continue
			}
			for _, version := range crd.Spec.Versions {
				if version.Served && version.Name != "" {
					out[schema.GroupVersionKind{Group: crd.Spec.Group, Version: version.Name, Kind: crd.Spec.Names.Kind}] = crd.Spec.Scope == "Namespaced"
				}
			}
		}
	}
	return out, nil
}

type crdPreviewClient struct {
	kube.Interface
	mapper   meta.RESTMapper
	declared map[schema.GroupVersionKind]bool
	deferred map[schema.GroupVersionKind]bool
}

func (c *crdPreviewClient) Build(reader io.Reader, validate bool) (kube.ResourceList, error) {
	raw, err := readBounded(reader, renderedLimit)
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var filtered bytes.Buffer
	for {
		var node yaml.Node
		if err := decoder.Decode(&node); err == io.EOF {
			break
		} else if err != nil {
			return nil, model.Fail("invalid", "A rendered resource could not be reviewed.")
		}
		if err := boundedYAML(&node, 0); err != nil {
			return nil, err
		}
		var object map[string]any
		if err := node.Decode(&object); err != nil {
			return nil, model.Fail("invalid", "A rendered resource could not be reviewed.")
		}
		keep, err := c.filter(object)
		if err != nil {
			return nil, err
		}
		if keep {
			encoded, err := yaml.Marshal(object)
			if err != nil {
				return nil, model.Fail("invalid", "A rendered resource could not be reviewed.")
			}
			filtered.WriteString("---\n")
			filtered.Write(encoded)
		}
	}
	return c.Interface.Build(&filtered, validate)
}

func (c *crdPreviewClient) filter(object map[string]any) (bool, error) {
	kind, _ := object["kind"].(string)
	version, _ := object["apiVersion"].(string)
	if kind == "List" {
		items, ok := object["items"].([]any)
		if !ok {
			return true, nil
		}
		kept := make([]any, 0, len(items))
		for _, item := range items {
			child, ok := item.(map[string]any)
			if !ok {
				return true, nil
			}
			keep, err := c.filter(child)
			if err != nil {
				return false, err
			}
			if keep {
				kept = append(kept, child)
			}
		}
		object["items"] = kept
		return len(kept) > 0, nil
	}
	gvk := schema.FromAPIVersionAndKind(version, kind)
	if _, declared := c.declared[gvk]; !declared {
		return true, nil
	}
	_, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if meta.IsNoMatchError(err) {
		if c.deferred == nil {
			c.deferred = map[schema.GroupVersionKind]bool{}
		}
		c.deferred[gvk] = true
		return false, nil
	}
	return true, err
}

func (r *reviewData) scopeIdentity(item map[string]string) error {
	gvk := schema.FromAPIVersionAndKind(item["apiVersion"], item["kind"])
	if r.deferred[gvk] {
		item["verification"] = "unverified until packaged CRD installation"
	}
	namespaced, known := r.declared[gvk]
	if r.mapper != nil {
		mapping, err := r.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err == nil {
			namespaced, known = mapping.Scope.Name() == meta.RESTScopeNameNamespace, true
		} else if !meta.IsNoMatchError(err) {
			return err
		}
	}
	item["scope"] = "unresolved"
	if known {
		item["scope"] = "cluster"
		if namespaced {
			item["scope"] = "namespaced"
			if item["namespace"] == "" {
				item["namespace"] = r.Target.Namespace
			}
		} else {
			item["namespace"] = ""
		}
	}
	return nil
}

func identityKey(item map[string]string) string {
	gv := schema.FromAPIVersionAndKind(item["apiVersion"], item["kind"])
	return strings.Join([]string{gv.Group, gv.Kind, item["namespace"], item["name"]}, "/")
}
