/*
Purpose: Resolve only the captured availability relationships.
Used by: HPA targets, PDB selected Pods and revision navigation.
Responsibilities: Validate source version, target identity and current field or relationship.
Important: The result carries the same target snapshot used for validation.
*/
package kube

import (
	"context"

	"github.com/sandjaie/KubeSift/internal/model"
	autoscaling "k8s.io/api/autoscaling/v2"
	core "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func scaleTarget(kind, version string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "ReplicaSet":
		return version == "apps/v1"
	case "ReplicationController":
		return version == "v1"
	}
	return false
}

func availabilityRoute(source, target ResourceRef) bool {
	if source.Connection != target.Connection || source.Namespace != target.Namespace {
		return false
	}
	switch source.Kind {
	case "HorizontalPodAutoscaler":
		return scaleTarget(target.Kind, target.APIVersion)
	case "PodDisruptionBudget":
		return target.Kind == "Pod" && target.UID != ""
	case "StatefulSet", "DaemonSet":
		return target.Kind == "ControllerRevision" && target.UID != ""
	case "ControllerRevision":
		return revisionParent(target.Kind)
	}
	return false
}

func (s *Service) ResolveAvailabilityReference(ctx context.Context, req AvailabilityReferenceRequest) (ResolvedAvailabilityReference, error) {
	var empty ResolvedAvailabilityReference
	source, target := req.Source, req.Target
	if !availabilityRoute(source, target) || req.SourceVersion == "" || len(req.SourceVersion) > 256 {
		return empty, model.Fail("invalid", "Captured availability reference required")
	}
	if err := validResource(source); err != nil {
		return empty, err
	}
	if err := validResourceAddress(target); err != nil {
		return empty, err
	}
	client, err := s.boundedReadClient(ctx, source.Connection)
	if err != nil {
		return empty, err
	}
	from, err := checkedObject(ctx, client, source)
	if err != nil {
		return empty, err
	}
	if from.GetName() != source.Name || from.GetNamespace() != source.Namespace || from.GetResourceVersion() != req.SourceVersion {
		return empty, model.Fail("stale_target", "Availability source changed; refresh the section")
	}
	if err := checkPayload(from); err != nil {
		return empty, err
	}
	association, identity := "controller-owner", "uid-verified"
	var selector labels.Selector
	switch o := from.(type) {
	case *autoscaling.HorizontalPodAutoscaler:
		ref := o.Spec.ScaleTargetRef
		if ref.Name != target.Name || ref.Kind != target.Kind || ref.APIVersion != target.APIVersion {
			return empty, model.Fail("invalid", "Scale target is unavailable in the captured resource")
		}
		association, identity = "scale-target", "name-resolved"
	case *policy.PodDisruptionBudget:
		selector, err = pdbSelector(o)
		if err != nil {
			return empty, err
		}
		if selector == nil {
			return empty, model.Fail("invalid", "Captured selector selects no Pods")
		}
		association = "selector-match"
	default:
		if source.Kind == "ControllerRevision" {
			owner := controllerOwner(from.GetOwnerReferences())
			if owner != nil && target.UID == "" {
				target.UID = string(owner.UID)
			}
			if validResource(target) != nil || !exactOwner(from.GetOwnerReferences(), target) {
				return empty, model.Fail("invalid", "Controller owner is unavailable in the captured revision")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return empty, safeError(ctx, err)
	}
	if err := s.connections.Validate(source.Connection); err != nil {
		return empty, err
	}
	var object workloadObject
	var raw []byte
	if target.Kind == "ControllerRevision" {
		object, raw, err = getAvailability(ctx, client, target)
	} else {
		object, err = getResource(ctx, client, target)
	}
	if err != nil {
		return empty, err
	}
	target.UID = string(object.GetUID())
	if object.GetName() != target.Name || object.GetNamespace() != target.Namespace || validResource(target) != nil || object.GetResourceVersion() == "" {
		return empty, model.Fail("invalid", "Server did not return the requested target identity")
	}
	if selector != nil && !selector.Matches(labels.Set(object.(*core.Pod).Labels)) {
		return empty, model.Fail("stale_target", "Pod no longer matches the captured selector")
	}
	if target.Kind == "ControllerRevision" && !exactOwner(object.GetOwnerReferences(), source) {
		return empty, model.Fail("stale_target", "Revision no longer belongs to the captured controller")
	}
	detail, err := capturedDetail(target, object, raw)
	if err != nil {
		return empty, err
	}
	result := ResolvedAvailabilityReference{Detail: detail, Resource: target, SourceVersion: req.SourceVersion, TargetVersion: detail.ResourceVersion, IdentityEvidence: identity, Association: association}
	if err := s.finishAvailability(ctx, source.Connection, result); err != nil {
		return empty, err
	}
	return result, nil
}
