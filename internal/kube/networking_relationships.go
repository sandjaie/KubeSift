/*
Purpose: Read Service associations and resolve explicit networking references.
Used by: Networking investigation navigation.
Responsibilities: Check captured source identity, allowlisted fields and referenced UIDs before navigation.
Important: No selector expansion, arbitrary kinds or cross-namespace target reads.
*/
package kube

import (
	"context"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	discovery "k8s.io/api/discovery/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const serviceNameLabel = "kubernetes.io/service-name"

func sliceAssociation(slice *discovery.EndpointSlice, service ResourceRef) string {
	association := "label-only"
	for _, owner := range slice.OwnerReferences {
		if owner.Kind != "Service" || owner.APIVersion != "v1" || owner.UID == "" {
			continue
		}
		if owner.Name != service.Name || string(owner.UID) != service.UID {
			return "owner-conflict"
		}
		association = "owner-confirmed"
	}
	return association
}

func (s *Service) ReadServiceEndpointSlices(ctx context.Context, req RelatedRequest) (ServiceSlicePage, error) {
	result := ServiceSlicePage{Items: []ServiceSliceRow{}, Page: PageInfo{FetchedAt: time.Now().UTC()}}
	parent := req.Resource
	if parent.Kind != "Service" {
		return result, model.Fail("invalid", "Service identity required")
	}
	if err := validResource(parent); err != nil {
		return result, err
	}
	scope := pageCursor{Connection: parent.Connection, Kind: "Service/EndpointSlices", Namespace: parent.Namespace, Query: parent.Name + "/" + parent.UID}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return result, err
	}
	client, err := s.boundedReadClient(ctx, parent.Connection)
	if err != nil {
		return result, err
	}
	if _, err = checkedObject(ctx, client, parent); err != nil {
		return result, err
	}
	selector := labels.Set{serviceNameLabel: parent.Name}.AsSelector().String()
	for range 8 {
		list, e := client.DiscoveryV1().EndpointSlices(parent.Namespace).List(ctx, metav1.ListOptions{Limit: 100, Continue: token, LabelSelector: selector})
		if e != nil {
			if problem := ClassifyError(ctx, e); problem.Code == "forbidden" {
				result.Problem = problem
				return result, s.connections.Validate(parent.Connection)
			}
			return result, networkingListError(ctx, e)
		}
		if len(list.Items) > 100 {
			return result, model.Fail("capacity", "Server exceeded page size")
		}
		for i := range list.Items {
			slice := &list.Items[i]
			if slice.Namespace != parent.Namespace || slice.Labels[serviceNameLabel] != parent.Name {
				continue
			}
			result.Items = append(result.Items, ServiceSliceRow{Slice: networkRow(parent.Connection, slice), Association: sliceAssociation(slice, parent)})
		}
		token = list.Continue
		result.Page.ResourceVersion = list.ResourceVersion
		if len(result.Items) > 0 || token == "" {
			break
		}
	}
	result.Complete = token == ""
	result.Page.NextCursor, err = encodeCursor(scope, token)
	if err != nil {
		return ServiceSlicePage{}, err
	}
	if err = checkPayload(result); err != nil {
		return ServiceSlicePage{}, err
	}
	if err = ctx.Err(); err != nil {
		return ServiceSlicePage{}, safeError(ctx, err)
	}
	return result, s.connections.Validate(parent.Connection)
}

func validIngressService(backend *networking.IngressBackend, name string) bool {
	if backend == nil || backend.Resource != nil || backend.Service == nil || backend.Service.Name != name {
		return false
	}
	port := backend.Service.Port
	return (port.Name != "" && port.Number == 0) || (port.Name == "" && port.Number > 0 && port.Number <= 65535)
}

func referencePresent(source workloadObject, target ResourceRef) bool {
	switch o := source.(type) {
	case *networking.Ingress:
		if target.UID != "" {
			return false
		}
		if target.Kind == "IngressClass" {
			return o.Spec.IngressClassName != nil && *o.Spec.IngressClassName == target.Name
		}
		if target.Kind != "Service" || target.Namespace != o.Namespace {
			return false
		}
		if validIngressService(o.Spec.DefaultBackend, target.Name) {
			return true
		}
		for _, rule := range o.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if validIngressService(&path.Backend, target.Name) {
					return true
				}
			}
		}
	case *discovery.EndpointSlice:
		if target.Namespace != o.Namespace {
			return false
		}
		if target.Kind == "Service" && o.Labels[serviceNameLabel] == target.Name {
			// An existing owner UID cannot be downgraded to a name-only link.
			confirmed := sliceAssociation(o, target)
			return confirmed == "owner-confirmed" || (confirmed == "label-only" && target.UID == "")
		}
		if target.Kind != "Pod" {
			return false
		}
		for _, endpoint := range o.Endpoints {
			ref := endpoint.TargetRef
			if ref == nil || ref.Kind != "Pod" || ref.APIVersion != "v1" || ref.Name != target.Name || string(ref.UID) != target.UID {
				continue
			}
			if ref.Namespace == "" || ref.Namespace == o.Namespace {
				return true
			}
		}
	}
	return false
}

func (s *Service) ResolveNetworkReference(ctx context.Context, req NetworkReferenceRequest) (ResolvedNetworkReference, error) {
	result := ResolvedNetworkReference{}
	source, target := req.Source, req.Target
	if (source.Kind != "Ingress" && source.Kind != "EndpointSlice") || source.Connection != target.Connection || req.SourceVersion == "" || len(req.SourceVersion) > 256 {
		return result, model.Fail("invalid", "Captured networking reference required")
	}
	if err := validResource(source); err != nil {
		return result, err
	}
	if err := validResourceAddress(target); err != nil {
		return result, err
	}
	client, err := s.boundedReadClient(ctx, source.Connection)
	if err != nil {
		return result, err
	}
	object, err := checkedObject(ctx, client, source)
	if err != nil {
		return result, err
	}
	if object.GetResourceVersion() != req.SourceVersion {
		return result, model.Fail("stale_target", "Source configuration changed; refresh the section")
	}
	if err = checkPayload(object); err != nil {
		return result, err
	}
	if !referencePresent(object, target) {
		return result, model.Fail("invalid", "Reference is unavailable or conflicts with captured evidence")
	}
	resolved, err := getResource(ctx, client, target)
	if err != nil {
		return result, err
	}
	if resolved.GetName() != target.Name || resolved.GetNamespace() != target.Namespace || resolved.GetUID() == "" {
		return result, model.Fail("invalid", "Server did not return the referenced identity")
	}
	if err = ctx.Err(); err != nil {
		return result, safeError(ctx, err)
	}
	evidence := "uid-verified"
	if target.UID == "" {
		evidence = "name-resolved"
	}
	target.UID = string(resolved.GetUID())
	if err = validResource(target); err != nil {
		return result, err
	}
	return ResolvedNetworkReference{Resource: target, IdentityEvidence: evidence}, s.connections.Validate(source.Connection)
}
