/*
Purpose: Open explicit Pod and binding references from captured access control evidence.
Used by: Pod and RBAC Overview navigation.
Responsibilities: Check source version, allowed fields, target namespace and identity.
Important: A named lookup does not establish access, membership or historical identity.
*/
package kube

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	rbac "k8s.io/api/rbac/v1"
)

func accessControlRoute(source, target ResourceRef) bool {
	switch source.Kind {
	case "Pod":
		return target.Kind == "ServiceAccount" && source.Namespace == target.Namespace
	case "RoleBinding":
		return target.Kind == "ServiceAccount" || target.Kind == "ClusterRole" ||
			(target.Kind == "Role" && source.Namespace == target.Namespace)
	case "ClusterRoleBinding":
		return target.Kind == "ServiceAccount" || target.Kind == "ClusterRole"
	}
	return false
}

func bindingContains(role rbac.RoleRef, subjects []rbac.Subject, bindingNamespace string, target ResourceRef) bool {
	if target.Kind != "ServiceAccount" {
		return role.APIGroup == rbac.GroupName && role.Kind == target.Kind && role.Name == target.Name
	}
	for _, subject := range subjects {
		ns := subject.Namespace
		if ns == "" {
			ns = bindingNamespace
		}
		if subject.Kind == "ServiceAccount" && subject.APIGroup == "" && subject.Name == target.Name && ns == target.Namespace {
			return true
		}
	}
	return false
}

func (s *Service) ResolveAccessControlReference(ctx context.Context, req AccessControlReferenceRequest) (ResolvedAccessControlReference, error) {
	var result ResolvedAccessControlReference
	source, target := req.Source, req.Target
	if !accessControlRoute(source, target) || source.Connection != target.Connection ||
		req.SourceVersion == "" || len(req.SourceVersion) > 256 {
		return result, model.Fail("invalid", "Captured access control reference required")
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

	var matches bool
	if source.Kind == "Pod" {
		// Decode the exact field rather than an API conversion which can alias serviceAccount.
		raw, err := client.CoreV1().RESTClient().Get().Namespace(source.Namespace).Resource("pods").Name(source.Name).
			SetHeader("Accept", "application/json").MaxRetries(0).Do(ctx).Raw()
		if err != nil {
			return result, readError(ctx, err)
		}
		if len(raw) > detailLimit {
			return result, model.Fail("capacity", "Resource evidence exceeds the 2 MiB size limit")
		}
		var pod struct {
			Kind       string `json:"kind"`
			APIVersion string `json:"apiVersion"`
			Metadata   struct {
				Name            string `json:"name"`
				Namespace       string `json:"namespace"`
				UID             string `json:"uid"`
				ResourceVersion string `json:"resourceVersion"`
			} `json:"metadata"`
			Spec struct {
				ServiceAccountName string `json:"serviceAccountName"`
			} `json:"spec"`
		}
		if json.Unmarshal(raw, &pod) != nil || pod.Kind != "Pod" || pod.APIVersion != "v1" ||
			pod.Metadata.Name != source.Name || pod.Metadata.Namespace != source.Namespace {
			return result, model.Fail("invalid", "Server did not return the requested Pod identity")
		}
		if pod.Metadata.UID != source.UID {
			return result, model.Fail("resource_replaced", "Resource replaced; select again")
		}
		if pod.Metadata.ResourceVersion != req.SourceVersion {
			return result, model.Fail("stale_target", "Pod access configuration changed; refresh the section")
		}
		matches = pod.Spec.ServiceAccountName == target.Name
	} else {
		object, err := checkedObject(ctx, client, source)
		if err != nil {
			return result, err
		}
		if object.GetResourceVersion() != req.SourceVersion {
			return result, model.Fail("stale_target", "Binding changed; refresh the section")
		}
		if err := checkPayload(object); err != nil {
			return result, err
		}
		switch b := object.(type) {
		case *rbac.RoleBinding:
			matches = bindingContains(b.RoleRef, b.Subjects, b.Namespace, target)
		case *rbac.ClusterRoleBinding:
			matches = bindingContains(b.RoleRef, b.Subjects, "", target)
		}
	}
	if !matches {
		return result, model.Fail("invalid", "Reference is unavailable in the captured resource")
	}
	if err := ctx.Err(); err != nil {
		return result, safeError(ctx, err)
	}
	if err := s.connections.Validate(source.Connection); err != nil {
		return result, err
	}
	object, err := getResource(ctx, client, target)
	if err != nil {
		return result, err
	}
	raw, err := json.Marshal(object)
	if err != nil {
		return result, model.Fail("failed", "Cannot encode access control evidence")
	}
	target.UID = string(object.GetUID())
	version := object.GetResourceVersion()
	if version == "" {
		return result, model.Fail("invalid", "Server did not return a target resource version")
	}
	result = ResolvedAccessControlReference{
		Detail:   ResourceDetail{Resource: target, ResourceVersion: version, FetchedAt: time.Now().UTC(), ObjectJSON: string(raw)},
		Resource: target, SourceVersion: req.SourceVersion, TargetVersion: version, IdentityEvidence: "name-resolved",
	}
	if err := checkPayload(result); err != nil {
		return ResolvedAccessControlReference{}, err
	}
	if err := ctx.Err(); err != nil {
		return ResolvedAccessControlReference{}, safeError(ctx, err)
	}
	return result, s.connections.Validate(source.Connection)
}
