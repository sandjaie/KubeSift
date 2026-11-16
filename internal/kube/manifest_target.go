/*
Purpose: Resolve an exact manifest address using live Kubernetes discovery.
Used by: Native reviewed YAML creation and editing.
Responsibilities: Validate route components, advertised verbs and declared scope.
Important: No route is inferred from a plural, preferred version or caller URL.
*/
package kube

import (
	"context"
	"strings"

	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/client-go/kubernetes"
)

type ManifestAddress struct {
	Connection model.ConnectionRef `json:"connection"`
	Group      string              `json:"group"`
	Version    string              `json:"version"`
	Resource   string              `json:"resource"`
	Kind       string              `json:"kind"`
	Namespace  string              `json:"namespace"`
	Name       string              `json:"name"`
}

func (t ManifestAddress) api() apiResource {
	return apiResource{t.Group, t.Version, t.Resource, t.Kind, t.Namespace != ""}
}

func (t ManifestAddress) Ref() ResourceRef {
	return ResourceRef{Connection: t.Connection, APIVersion: t.api().apiVersion(), Kind: t.Kind, Namespace: t.Namespace, Name: t.Name}
}

func (t ManifestAddress) CollectionPath() string { return t.api().path(t.Namespace, "") }
func (t ManifestAddress) Path() string           { return t.api().path(t.Namespace, t.Name) }

func (t ManifestAddress) ReadIdentity(raw []byte) (string, string, error) {
	identity, err := objectIdentity(raw, t.api(), t.Namespace, false)
	if err != nil {
		return "", "", err
	}
	if identity.Metadata.Name != t.Name {
		return "", "", model.Fail("invalid_response", "Kubernetes returned a different resource name")
	}
	return string(identity.Metadata.UID), identity.Metadata.ResourceVersion, nil
}

func (t ManifestAddress) valid() bool {
	return (t.Group == "" || validGroup(t.Group)) && validVersion(t.Version) &&
		(t.Resource == "" || validVersion(t.Resource)) && discoveryKind.MatchString(t.Kind) && genericName(t.Name)
}

func (s *Service) ResolveManifestTarget(ctx context.Context, ref model.ConnectionRef, apiVersion, kind, resource, namespace, name, verb string) (ManifestAddress, error) {
	group, version, grouped := strings.Cut(apiVersion, "/")
	if !grouped {
		group, version = "", group
	}
	target := ManifestAddress{Connection: ref, Group: group, Version: version, Resource: resource, Kind: kind, Namespace: namespace, Name: name}
	if (grouped && group == "") || !target.valid() || (verb != "create" && verb != "get" && verb != "patch") {
		return ManifestAddress{}, model.Fail("invalid", "Enter a named resource with a valid API version, kind and resource")
	}
	client, err := s.boundedReadClient(ctx, ref)
	if err != nil {
		return ManifestAddress{}, err
	}
	return resolveManifestRoute(ctx, client, target, verb)
}

func resolveManifestRoute(ctx context.Context, client kubernetes.Interface, target ManifestAddress, verb string) (ManifestAddress, error) {
	raw, err := fixedRead(client, apiVersionPath(target.Group, target.Version)).Do(ctx).Raw()
	if err != nil {
		return ManifestAddress{}, readError(ctx, err)
	}
	catalog, err := projectDiscovery(raw, versionKey{target.Group, target.Version})
	if err != nil {
		return ManifestAddress{}, err
	}
	var match *APIResourceDescriptor
	for _, entry := range catalog.Resources {
		if entry.Kind != target.Kind || (target.Resource != "" && entry.Resource != target.Resource) {
			continue
		}
		if match != nil {
			return ManifestAddress{}, model.Fail("ambiguous_route", "This kind has multiple advertised resources. Enter the exact API resource from discovery.")
		}
		match = &entry
	}
	if verb == "create" && (match == nil || !match.Create) {
		return ManifestAddress{}, model.Fail("unsupported", "This API version does not advertise Create for the requested kind and resource")
	}
	if verb == "get" && (match == nil || !match.Get) {
		return ManifestAddress{}, model.Fail("unsupported", "This API version does not advertise Get for the selected resource")
	}
	if verb == "patch" && (match == nil || !match.Get || !match.Patch) {
		return ManifestAddress{}, model.Fail("unsupported", "This API does not advertise both Get and Patch for the selected resource")
	}
	if err := genericAddress(match.api(), target.Namespace); err != nil {
		return ManifestAddress{}, err
	}
	target.Resource = match.Resource
	return target, nil
}
