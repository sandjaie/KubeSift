/*
Purpose: Discover only advertised API paths on the selected connection.
Used by: Installed API inventory and generic resource reads.
Responsibilities: Limit discovery, guard dedicated APIs, and expire cache-bound handles.
Important: No server-supplied URLs or subresource paths are followed.
*/
package kube

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const catalogTTL = 5 * time.Minute

type versionKey struct{ group, version string }
type discoveryCache struct {
	groups   APIGroupCatalog
	versions map[versionKey]*discoveredVersion
}
type discoveredVersion struct {
	catalog APIResourceCatalog
	used    time.Time
}

var discoveryKind = regexp.MustCompile("^[A-Za-z][A-Za-z0-9]{0,127}$")

var dedicatedAPIs = func() map[[2]string]apiResource {
	out := map[[2]string]apiResource{}
	add := func(a apiResource) { out[[2]string{a.group, a.plural}] = a }
	for _, a := range clusterResources {
		add(a)
	}
	add(coreEvents)
	add(modernEvents)
	for _, item := range []struct {
		kind, plural string
		version      func(string) string
	}{
		{"Pod", "pods", workloadVersion}, {"ReplicationController", "replicationcontrollers", workloadVersion},
		{"Deployment", "deployments", workloadVersion}, {"ReplicaSet", "replicasets", workloadVersion},
		{"StatefulSet", "statefulsets", workloadVersion}, {"DaemonSet", "daemonsets", workloadVersion},
		{"Job", "jobs", workloadVersion}, {"CronJob", "cronjobs", workloadVersion},
		{"Service", "services", networkVersion}, {"Ingress", "ingresses", networkVersion},
		{"IngressClass", "ingressclasses", networkVersion}, {"EndpointSlice", "endpointslices", networkVersion},
		{"NetworkPolicy", "networkpolicies", networkVersion},
		{"PersistentVolume", "persistentvolumes", storageVersion}, {"PersistentVolumeClaim", "persistentvolumeclaims", storageVersion},
		{"StorageClass", "storageclasses", storageVersion}, {"VolumeAttachment", "volumeattachments", storageVersion},
		{"CSIDriver", "csidrivers", storageVersion}, {"CSINode", "csinodes", storageVersion},
		{"ServiceAccount", "serviceaccounts", accessControlVersion}, {"Role", "roles", accessControlVersion},
		{"RoleBinding", "rolebindings", accessControlVersion}, {"ClusterRole", "clusterroles", accessControlVersion},
		{"ClusterRoleBinding", "clusterrolebindings", accessControlVersion},
		{"HorizontalPodAutoscaler", "horizontalpodautoscalers", availabilityVersion},
		{"PodDisruptionBudget", "poddisruptionbudgets", availabilityVersion}, {"ControllerRevision", "controllerrevisions", availabilityVersion},
	} {
		group, version, hasGroup := strings.Cut(item.version(item.kind), "/")
		if !hasGroup {
			version, group = group, ""
		}
		add(apiResource{group, version, item.plural, item.kind, !isClusterScopedResource(item.kind)})
	}
	for kind, plural := range map[string]string{"Namespace": "namespaces", "ResourceQuota": "resourcequotas", "LimitRange": "limitranges", "ConfigMap": "configmaps", "Secret": "secrets"} {
		add(apiResource{"", "v1", plural, kind, !isClusterScopedResource(kind)})
	}
	return out
}()

func discoveryProblem(ctx context.Context, err error) *model.Problem {
	if err == nil {
		return nil
	}
	var problem *model.Problem
	if errors.As(err, &problem) {
		return problem
	}
	if errors.As(resourceListError(ctx, err), &problem) {
		return problem
	}
	return &model.Problem{Code: "failed", Message: "Cannot read Kubernetes API discovery"}
}
func invalidDiscovery() error {
	return model.Fail("invalid", "Server returned an invalid API discovery document")
}
func expiredDiscovery() error {
	return model.Fail("catalog_expired", "API catalog changed or expired; refresh resources")
}
func validVersion(v string) bool   { return len(validation.IsDNS1123Label(v)) == 0 }
func validGroup(group string) bool { return len(validation.IsDNS1123Subdomain(group)) == 0 }

func readCoreGroups(raw []byte) ([]APIGroup, error) {
	var core metav1.APIVersions
	if json.Unmarshal(raw, &core) != nil || core.Kind != "APIVersions" || core.Versions == nil {
		return nil, invalidDiscovery()
	}
	if len(core.Versions) > 32 {
		return nil, model.Fail("capacity", "API group exceeds 32 versions")
	}
	if len(core.Versions) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	for _, v := range core.Versions {
		if !validVersion(v) || seen[v] {
			return nil, invalidDiscovery()
		}
		seen[v] = true
	}
	preferred := ""
	if len(core.Versions) == 1 {
		preferred = core.Versions[0]
	}
	return []APIGroup{{Group: "", Versions: core.Versions, Preferred: preferred}}, nil
}
func readNamedGroups(raw []byte) ([]APIGroup, error) {
	var root metav1.APIGroupList
	if json.Unmarshal(raw, &root) != nil || root.Kind != "APIGroupList" || root.Groups == nil {
		return nil, invalidDiscovery()
	}
	if len(root.Groups) > 512 {
		return nil, model.Fail("capacity", "API catalog exceeds 512 groups")
	}
	groups := make([]APIGroup, 0, len(root.Groups))
	seen := map[string]bool{}
	for _, g := range root.Groups {
		if !validGroup(g.Name) || seen[g.Name] {
			return nil, invalidDiscovery()
		}
		seen[g.Name] = true
		if len(g.Versions) > 32 {
			return nil, model.Fail("capacity", "API group exceeds 32 versions")
		}
		row := APIGroup{Group: g.Name, Versions: []string{}}
		for _, v := range g.Versions {
			if !validVersion(v.Version) || v.GroupVersion != g.Name+"/"+v.Version || slices.Contains(row.Versions, v.Version) {
				return nil, invalidDiscovery()
			}
			row.Versions = append(row.Versions, v.Version)
		}
		if p := g.PreferredVersion; p.Version != "" || p.GroupVersion != "" {
			if !slices.Contains(row.Versions, p.Version) || p.GroupVersion != g.Name+"/"+p.Version {
				return nil, invalidDiscovery()
			}
			row.Preferred = p.Version
		}
		groups = append(groups, row)
	}
	return groups, nil
}
func (s *Service) ReadAPIGroups(ctx context.Context, ref model.ConnectionRef, refresh bool) (APIGroupCatalog, error) {
	if err := s.connections.Validate(ref); err != nil {
		return APIGroupCatalog{}, err
	}
	s.mu.Lock()
	if err := s.connections.Validate(ref); err != nil {
		s.mu.Unlock()
		return APIGroupCatalog{}, err
	}
	cache := s.discovery[ref]
	if cache != nil && !refresh && !cache.groups.FetchedAt.IsZero() && time.Since(cache.groups.FetchedAt) < catalogTTL {
		result := cache.groups
		s.mu.Unlock()
		return result, nil
	}
	next := &discoveryCache{versions: map[versionKey]*discoveredVersion{}}
	s.discovery[ref] = next
	s.mu.Unlock()
	client, err := s.boundedReadClient(ctx, ref)
	if err != nil {
		return APIGroupCatalog{}, err
	}
	catalog := APIGroupCatalog{Groups: []APIGroup{}, Epoch: model.ID()}
	for _, root := range []struct {
		path    string
		decode  func([]byte) ([]APIGroup, error)
		problem **model.Problem
	}{
		{"/api", readCoreGroups, &catalog.CoreProblem}, {"/apis", readNamedGroups, &catalog.APIsProblem},
	} {
		raw, err := fixedRead(client, root.path).Do(ctx).Raw()
		if err == nil {
			var groups []APIGroup
			groups, err = root.decode(raw)
			catalog.Groups = append(catalog.Groups, groups...)
		}
		*root.problem = discoveryProblem(ctx, err)
	}
	if len(catalog.Groups) > 512 {
		catalog.Groups = append([]APIGroup{}, catalog.Groups[:1]...)
		catalog.APIsProblem = &model.Problem{Code: "capacity", Message: "API catalog exceeds 512 groups"}
	}
	catalog.FetchedAt = time.Now().UTC()
	if err := checkPayload(catalog); err != nil {
		return APIGroupCatalog{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.discoveryCurrent(ctx, ref, next); err != nil {
		return APIGroupCatalog{}, err
	}
	next.groups = catalog
	return catalog, nil
}
func (s *Service) discoveryCurrent(ctx context.Context, ref model.ConnectionRef, cache *discoveryCache) error {
	if ctx.Err() != nil {
		return safeError(ctx, ctx.Err())
	}
	if err := s.connections.Validate(ref); err != nil {
		return err
	}
	if s.discovery[ref] != cache {
		return expiredDiscovery()
	}
	return nil
}
func (s *Service) ReadAPIResources(ctx context.Context, ref model.ConnectionRef, group, version string, refresh bool) (APIResourceCatalog, error) {
	if (group != "" && !validGroup(group)) || !validVersion(version) {
		return APIResourceCatalog{}, model.Fail("invalid", "Select an advertised API group and version")
	}
	key := versionKey{group, version}
	if err := s.connections.Validate(ref); err != nil {
		return APIResourceCatalog{}, err
	}
	s.mu.Lock()
	cache := s.discovery[ref]
	advertised := cache != nil && time.Since(cache.groups.FetchedAt) < catalogTTL && slices.ContainsFunc(cache.groups.Groups, func(g APIGroup) bool { return g.Group == group && slices.Contains(g.Versions, version) })
	if !advertised {
		s.mu.Unlock()
		return APIResourceCatalog{}, expiredDiscovery()
	}
	prior := cache.versions[key]
	if prior != nil && !refresh && time.Since(prior.catalog.FetchedAt) < catalogTTL {
		prior.used = time.Now()
		result := prior.catalog
		s.mu.Unlock()
		return result, nil
	}
	candidate := &discoveredVersion{used: time.Now()}
	if len(cache.versions) >= 8 && prior == nil {
		var oldest versionKey
		var stamp time.Time
		for key, v := range cache.versions {
			if stamp.IsZero() || v.used.Before(stamp) {
				oldest, stamp = key, v.used
			}
		}
		delete(cache.versions, oldest)
	}
	cache.versions[key] = candidate
	s.mu.Unlock()
	client, err := s.boundedReadClient(ctx, ref)
	if err != nil {
		return APIResourceCatalog{}, err
	}
	raw, err := fixedRead(client, apiVersionPath(group, version)).Do(ctx).Raw()
	if err != nil {
		return APIResourceCatalog{}, discoveryProblem(ctx, err)
	}
	catalog, err := projectDiscovery(raw, key)
	if err != nil {
		return APIResourceCatalog{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = s.discoveryCurrent(ctx, ref, cache); err != nil {
		return APIResourceCatalog{}, err
	}
	if cache.versions[key] != candidate || time.Since(cache.groups.FetchedAt) >= catalogTTL {
		return APIResourceCatalog{}, expiredDiscovery()
	}
	candidate.catalog = catalog
	candidate.used = time.Now()
	return catalog, nil
}

func projectDiscovery(raw []byte, key versionKey) (APIResourceCatalog, error) {
	// Pointer scope distinguishes an explicit cluster-scoped resource from a malformed descriptor.
	var document struct {
		metav1.TypeMeta
		GroupVersion string `json:"groupVersion"`
		Resources    []struct {
			metav1.APIResource
			Namespaced *bool `json:"namespaced"`
		} `json:"resources"`
	}
	expected := apiResource{group: key.group, version: key.version}.apiVersion()
	if json.Unmarshal(raw, &document) != nil || document.Kind != "APIResourceList" || document.GroupVersion != expected || document.Resources == nil {
		return APIResourceCatalog{}, invalidDiscovery()
	}
	if len(document.Resources) > 1024 {
		return APIResourceCatalog{}, model.Fail("capacity", "API version exceeds 1,024 resource descriptors")
	}
	out := APIResourceCatalog{Resources: []APIResourceDescriptor{}, FetchedAt: time.Now().UTC(), Epoch: model.ID()}
	seen := map[string]bool{}
	for _, r := range document.Resources {
		if strings.Contains(r.Name, "/") {
			continue
		}
		if !validVersion(r.Name) || !discoveryKind.MatchString(r.Kind) || r.Namespaced == nil || seen[r.Name] ||
			(r.Group != "" && r.Group != key.group) || (r.Version != "" && r.Version != key.version) {
			return APIResourceCatalog{}, invalidDiscovery()
		}
		seen[r.Name] = true
		for _, verb := range r.Verbs {
			if !validVersion(verb) {
				return APIResourceCatalog{}, invalidDiscovery()
			}
		}
		d := APIResourceDescriptor{Group: key.group, Version: key.version, Resource: r.Name, Kind: r.Kind, Namespaced: *r.Namespaced, List: slices.Contains(r.Verbs, "list"), Get: slices.Contains(r.Verbs, "get")}
		known, protected := dedicatedAPIs[[2]string{d.Group, d.Resource}]
		switch {
		case protected && known == d.api():
			d.Mode = "dedicated"
			d.Reason = "Open the dedicated resource view"
		case protected:
			d.Mode = "unavailable"
			d.Reason = "This version or scope has no supported dedicated reader"
		case !d.List || !d.Get || slices.Contains(r.Verbs, "connect") || d.Resource == "proxy" || d.Resource == "connect":
			d.Mode = "unavailable"
			d.Reason = "This API does not advertise supported inventory and detail reads"
		default:
			d.Mode = "generic"
			d.Handle = model.ID()
		}
		out.Resources = append(out.Resources, d)
	}
	if err := checkPayload(out); err != nil {
		return APIResourceCatalog{}, err
	}
	return out, nil
}
func (s *Service) resolveDescriptor(ref model.ConnectionRef, handle string) (apiResource, error) {
	if len(handle) != 32 {
		return apiResource{}, expiredDiscovery()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.connections.Validate(ref); err != nil {
		return apiResource{}, err
	}
	cache := s.discovery[ref]
	if cache != nil && time.Since(cache.groups.FetchedAt) < catalogTTL {
		for _, version := range cache.versions {
			if time.Since(version.catalog.FetchedAt) >= catalogTTL {
				continue
			}
			for _, d := range version.catalog.Resources {
				if d.Handle == handle && d.Mode == "generic" {
					if _, protected := dedicatedAPIs[[2]string{d.Group, d.Resource}]; protected {
						return apiResource{}, model.Fail("invalid", "Dedicated API cannot be read generically")
					}
					version.used = time.Now()
					return d.api(), nil
				}
			}
		}
	}
	return apiResource{}, expiredDiscovery()
}
