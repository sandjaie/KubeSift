/*
Purpose: Read bounded typed networking inventories.
Used by: The explorer bridge and Service association reader.
Responsibilities: Validate resource scope and project only approved inventory fields.
Important: IngressClass is cluster scoped; namespaced kinds require a namespace.
*/
package kube

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	networking "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func networkVersion(kind string) string {
	switch kind {
	case "Service":
		return "v1"
	case "EndpointSlice":
		return "discovery.k8s.io/v1"
	case "Ingress", "IngressClass", "NetworkPolicy":
		return "networking.k8s.io/v1"
	default:
		return ""
	}
}

func isClusterScopedResource(kind string) bool {
	return kind == "IngressClass" || kind == "Namespace" ||
		(storageVersion(kind) != "" && kind != "PersistentVolumeClaim")
}

func resourceScope(kind, ns string) error {
	if isClusterScopedResource(kind) {
		if ns != "" {
			return model.Fail("invalid", "Selected kind requires cluster scope")
		}
		return nil
	}
	return namespace(ns)
}

func networkRow(connection model.ConnectionRef, object workloadObject) NetworkRow {
	var meta metav1.ObjectMeta
	var kind string
	var projected map[string]any
	switch o := object.(type) {
	case *core.Service:
		meta, kind = o.ObjectMeta, "Service"
		projected = map[string]any{"spec": map[string]any{"type": o.Spec.Type, "clusterIP": o.Spec.ClusterIP, "clusterIPs": o.Spec.ClusterIPs, "externalName": o.Spec.ExternalName, "externalIPs": o.Spec.ExternalIPs, "ports": o.Spec.Ports}, "status": map[string]any{"loadBalancer": o.Status.LoadBalancer}}
	case *discovery.EndpointSlice:
		meta, kind = o.ObjectMeta, "EndpointSlice"
		projected = map[string]any{"metadata": map[string]any{"labels": o.Labels}, "addressType": o.AddressType, "endpointCount": len(o.Endpoints)}
	case *networking.Ingress:
		meta, kind = o.ObjectMeta, "Ingress"
		hosts := make([]string, 0, len(o.Spec.Rules))
		for _, r := range o.Spec.Rules {
			hosts = append(hosts, r.Host)
		}
		projected = map[string]any{"spec": map[string]any{"ingressClassName": o.Spec.IngressClassName, "hosts": hosts}, "status": map[string]any{"loadBalancer": o.Status.LoadBalancer}}
	case *networking.IngressClass:
		meta, kind = o.ObjectMeta, "IngressClass"
		var annotation *string
		if value, ok := o.Annotations["ingressclass.kubernetes.io/is-default-class"]; ok {
			annotation = &value
		}
		projected = map[string]any{"spec": map[string]any{"controller": o.Spec.Controller, "parameters": o.Spec.Parameters}, "defaultClass": annotation}
	case *networking.NetworkPolicy:
		meta, kind = o.ObjectMeta, "NetworkPolicy"
		projected = map[string]any{"spec": map[string]any{"podSelector": o.Spec.PodSelector, "policyTypes": o.Spec.PolicyTypes}, "ingressRuleCount": len(o.Spec.Ingress), "egressRuleCount": len(o.Spec.Egress)}
	}
	raw, _ := json.Marshal(projected) // Only typed Kubernetes values and JSON primitives.
	return NetworkRow{Resource: resourceRef(connection, kind, networkVersion(kind), meta), CreatedAt: created(meta), ObjectJSON: string(raw)}
}

func networkingListError(ctx context.Context, err error) error {
	if apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) {
		return model.Fail("api_unavailable", "Networking API unavailable for this kind")
	}
	return readError(ctx, err)
}

func projectNetworkList[T any, P interface {
	*T
	workloadObject
}](ctx context.Context, req PageRequest, meta metav1.ListMeta, items []T) (NetworkPage, error) {
	if len(items) > 100 {
		return NetworkPage{}, model.Fail("capacity", "Server exceeded page size")
	}
	scope := pageCursor{Connection: req.Connection, Kind: req.Kind, Namespace: req.Namespace}
	next, err := encodeCursor(scope, meta.Continue)
	if err != nil {
		return NetworkPage{}, err
	}
	page := NetworkPage{Items: make([]NetworkRow, 0, len(items)), Page: PageInfo{NextCursor: next, ResourceVersion: meta.ResourceVersion, FetchedAt: time.Now().UTC()}}
	for i := range items {
		object := P(&items[i])
		if object.GetNamespace() != req.Namespace {
			return NetworkPage{}, model.Fail("invalid", "Server returned an object outside the requested scope")
		}
		page.Items = append(page.Items, networkRow(req.Connection, object))
	}
	if err = checkPayload(page); err != nil {
		return NetworkPage{}, err
	}
	return page, safeError(ctx, ctx.Err())
}

func (s *Service) ReadNetworking(ctx context.Context, req PageRequest) (NetworkPage, error) {
	if networkVersion(req.Kind) == "" {
		return NetworkPage{}, model.Fail("invalid", "Unsupported networking kind")
	}
	if err := resourceScope(req.Kind, req.Namespace); err != nil {
		return NetworkPage{}, err
	}
	scope := pageCursor{Connection: req.Connection, Kind: req.Kind, Namespace: req.Namespace}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return NetworkPage{}, err
	}
	client, err := s.boundedReadClient(ctx, req.Connection)
	if err != nil {
		return NetworkPage{}, err
	}
	opts := metav1.ListOptions{Limit: 100, Continue: token}
	var page NetworkPage
	switch req.Kind {
	case "Service":
		list, e := client.CoreV1().Services(req.Namespace).List(ctx, opts)
		if e != nil {
			return NetworkPage{}, networkingListError(ctx, e)
		}
		page, err = projectNetworkList(ctx, req, list.ListMeta, list.Items)
	case "EndpointSlice":
		list, e := client.DiscoveryV1().EndpointSlices(req.Namespace).List(ctx, opts)
		if e != nil {
			return NetworkPage{}, networkingListError(ctx, e)
		}
		page, err = projectNetworkList(ctx, req, list.ListMeta, list.Items)
	case "Ingress":
		list, e := client.NetworkingV1().Ingresses(req.Namespace).List(ctx, opts)
		if e != nil {
			return NetworkPage{}, networkingListError(ctx, e)
		}
		page, err = projectNetworkList(ctx, req, list.ListMeta, list.Items)
	case "IngressClass":
		list, e := client.NetworkingV1().IngressClasses().List(ctx, opts)
		if e != nil {
			return NetworkPage{}, networkingListError(ctx, e)
		}
		page, err = projectNetworkList(ctx, req, list.ListMeta, list.Items)
	case "NetworkPolicy":
		list, e := client.NetworkingV1().NetworkPolicies(req.Namespace).List(ctx, opts)
		if e != nil {
			return NetworkPage{}, networkingListError(ctx, e)
		}
		page, err = projectNetworkList(ctx, req, list.ListMeta, list.Items)
	}
	if err != nil {
		return NetworkPage{}, err
	}
	return page, s.connections.Validate(req.Connection)
}
