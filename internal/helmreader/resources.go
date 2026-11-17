/*
Purpose: Extract resource links from stored Helm manifests.
Used by: Release Resources and the explicit Open current resource action.
Responsibilities: Resolve exact discovered scope, validate the source revision and read only the requested live identity.
Important: A current name match does not establish historical identity or Helm ownership.
*/
package helmreader

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"go.yaml.in/yaml/v3"
	stored "helm.sh/helm/v4/pkg/release/v1"
)

const referenceLimit = 500

type OpenResourceRequest struct {
	Record   RecordRef        `json:"record"`
	Resource ManifestResource `json:"resource"`
}

type CurrentResource struct {
	Address  kube.ManifestAddress `json:"address"`
	Resource kube.ResourceRef     `json:"resource"`
}

func (s *Service) OpenResource(ctx context.Context, req OpenResourceRequest) (CurrentResource, error) {
	ctx, client, done, err := s.start(ctx, req.Record.Scope)
	if err != nil {
		return CurrentResource{}, err
	}
	defer done()
	data, err := readRelease(ctx, client, req.Record, newBudget())
	if err != nil {
		return CurrentResource{}, err
	}
	release, ok := data.(*stored.Release)
	if !ok {
		return CurrentResource{}, model.Fail("unsupported", "Unknown Helm release record format.")
	}
	rows, _ := manifestResources(ctx, s.reader, req.Record, release.Manifest)
	for _, row := range rows {
		if !row.Resolvable || row != req.Resource {
			continue
		}
		addr, err := s.reader.ResolveManifestTarget(ctx, req.Record.Connection, row.APIVersion, row.Kind, "", row.Namespace, row.Name, "get")
		if err != nil {
			return CurrentResource{}, err
		}
		resource, err := s.reader.ReadCurrentManifestIdentity(ctx, addr)
		return CurrentResource{Address: addr, Resource: resource}, err
	}
	return CurrentResource{}, model.Fail("invalid", "This link does not identify a readable resource from the selected revision.")
}

func manifestResources(ctx context.Context, reader *kube.Service, ref RecordRef, manifest string) ([]ManifestResource, bool) {
	rows := []ManifestResource{}
	if manifest == "" {
		return rows, false
	}
	decoder := yaml.NewDecoder(strings.NewReader(manifest))
	var catalog kube.APIResourceCatalog
	var lastVersion string
	if reader != nil {
		_, _ = reader.ReadAPIGroups(ctx, ref.Connection, false)
	}
	for doc := 0; doc < referenceLimit*2; doc++ {
		if ctx.Err() != nil {
			return rows, true
		}
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return rows, false
		}
		if len(rows) >= referenceLimit {
			return rows, true
		}
		if err != nil {
			rows = append(rows, ManifestResource{Issue: "Malformed manifest document. The rest could not be parsed."})
			return rows, false
		}
		if len(document.Content) == 0 || document.Content[0].Tag == "!!null" {
			continue
		}
		node := document.Content[0]
		meta := mappingValue(node, "metadata")
		item := ManifestResource{APIVersion: literal(mappingValue(node, "apiVersion")), Kind: literal(mappingValue(node, "kind")),
			Name: literal(mappingValue(meta, "name")), Namespace: literal(mappingValue(meta, "namespace"))}
		if item.Name == "" || item.APIVersion == "" || item.Kind == "" {
			item.Issue = "The document needs a literal API version, kind and name."
		} else if reader == nil {
			item.Issue = "API discovery is unavailable."
		} else {
			if lastVersion != item.APIVersion {
				group, version, hasGroup := strings.Cut(item.APIVersion, "/")
				if !hasGroup {
					group, version = "", group
				}
				catalog, _ = reader.ReadAPIResources(ctx, ref.Connection, group, version, false)
				lastVersion = item.APIVersion
			}
			item = resolveResourceRow(ref.Namespace, catalog, item)
		}
		rows = append(rows, item)
	}
	return rows, true
}

func resolveResourceRow(releaseNS string, catalog kube.APIResourceCatalog, item ManifestResource) ManifestResource {
	var match *kube.APIResourceDescriptor
	for _, candidate := range catalog.Resources {
		if candidate.Kind != item.Kind {
			continue
		}
		if match != nil {
			item.Issue = "The kind has more than one route in this API version."
			return item
		}
		match = &candidate
	}
	if match == nil || !match.SupportsManifestRead() {
		item.Issue = "The recorded API version does not advertise a readable resource."
		return item
	}
	if match.Namespaced {
		if item.Namespace == "" {
			item.Namespace = releaseNS
		}
	} else {
		item.Namespace = ""
	}
	item.Resolvable = true
	return item
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	var value *yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		if literal(node.Content[i]) != key {
			continue
		}
		if value != nil {
			return nil
		}
		value = node.Content[i+1]
	}
	return value
}

func literal(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!str" || len(node.Value) > 253 {
		return ""
	}
	return node.Value
}
