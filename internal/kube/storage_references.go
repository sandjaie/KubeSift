/*
Purpose: Resolve explicit storage links from a captured resource.
Used by: Storage and Pod Overview navigation.
Responsibilities: Check source version, field allowlist, target UID and binding consistency.
Important: A matching pair is evidence from separate reads, not a mount or availability check.
*/
package kube

import (
	"context"

	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"
)

func storageRoute(source, target string) bool {
	switch source {
	case "Pod":
		return target == "PersistentVolumeClaim"
	case "PersistentVolumeClaim":
		return target == "PersistentVolume" || target == "StorageClass"
	case "PersistentVolume":
		return target == "PersistentVolumeClaim" || target == "StorageClass" || target == "CSIDriver"
	case "VolumeAttachment":
		return target == "PersistentVolume" || target == "CSIDriver"
	case "CSINode":
		return target == "CSIDriver"
	}
	return false
}

func validClaimRef(ref *core.ObjectReference) bool {
	return ref != nil && ref.Name != "" && ref.Namespace != "" &&
		(ref.Kind == "" || ref.Kind == "PersistentVolumeClaim") &&
		(ref.APIVersion == "" || ref.APIVersion == "v1")
}

func explicitStorageReference(object workloadObject, target ResourceRef) (matches bool, authoritativeUID string) {
	switch source := object.(type) {
	case *core.Pod:
		for _, volume := range source.Spec.Volumes {
			if claim := volume.PersistentVolumeClaim; claim != nil && claim.ClaimName == target.Name {
				return true, ""
			}
		}
	case *core.PersistentVolumeClaim:
		if target.Kind == "PersistentVolume" {
			return source.Spec.VolumeName == target.Name, ""
		}
		return source.Spec.StorageClassName != nil && *source.Spec.StorageClassName == target.Name, ""
	case *core.PersistentVolume:
		switch target.Kind {
		case "StorageClass":
			return source.Spec.StorageClassName == target.Name, ""
		case "CSIDriver":
			return source.Spec.CSI != nil && source.Spec.CSI.Driver == target.Name, ""
		case "PersistentVolumeClaim":
			claim := source.Spec.ClaimRef
			if validClaimRef(claim) && claim.Name == target.Name && claim.Namespace == target.Namespace {
				return true, string(claim.UID)
			}
		}
	case *storage.VolumeAttachment:
		if target.Kind == "CSIDriver" {
			return source.Spec.Attacher == target.Name, ""
		}
		name := source.Spec.Source.PersistentVolumeName
		return source.Spec.Source.InlineVolumeSpec == nil && name != nil && *name == target.Name, ""
	case *storage.CSINode:
		for _, driver := range source.Spec.Drivers {
			if driver.Name == target.Name {
				return true, ""
			}
		}
	}
	return false, ""
}

func storageBinding(claim *core.PersistentVolumeClaim, volume *core.PersistentVolume) (string, error) {
	ref := volume.Spec.ClaimRef
	if (claim.Spec.VolumeName != "" && claim.Spec.VolumeName != volume.Name) ||
		(ref != nil && (!validClaimRef(ref) || ref.Name != claim.Name || ref.Namespace != claim.Namespace ||
			(ref.UID != "" && ref.UID != claim.UID))) {
		return "", model.Fail("reference_conflict", "Claim and volume binding references conflict")
	}
	if ref != nil && ref.UID != "" && claim.Spec.VolumeName == volume.Name {
		return "references-agree", nil
	}
	return "unconfirmed", nil
}

func (s *Service) ResolveStorageReference(ctx context.Context, req StorageReferenceRequest) (ResolvedStorageReference, error) {
	var result ResolvedStorageReference
	source, target := req.Source, req.Target
	if !storageRoute(source.Kind, target.Kind) || source.Connection != target.Connection ||
		(source.Kind == "Pod" && source.Namespace != target.Namespace) ||
		req.SourceVersion == "" || len(req.SourceVersion) > 256 {
		return result, model.Fail("invalid", "Captured storage reference required")
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
	captured, err := checkedObject(ctx, client, source)
	if err != nil {
		return result, err
	}
	if captured.GetName() != source.Name || captured.GetNamespace() != source.Namespace ||
		captured.GetResourceVersion() != req.SourceVersion {
		return result, model.Fail("stale_target", "Storage source changed; refresh the section")
	}
	if err := checkPayload(captured); err != nil {
		return result, err
	}
	matches, uid := explicitStorageReference(captured, target)
	if !matches {
		return result, model.Fail("invalid", "Reference is unavailable in the captured resource")
	}
	identity := "name-resolved"
	if uid != "" {
		if target.UID != "" && uid != target.UID {
			return result, model.Fail("reference_conflict", "Requested claim UID conflicts with the volume reference")
		}
		target.UID = uid
		if err := validResource(target); err != nil {
			return result, err
		}
		identity = "uid-verified"
	}
	if err := ctx.Err(); err != nil {
		return result, safeError(ctx, err)
	}
	if err := s.connections.Validate(source.Connection); err != nil {
		return result, err
	}
	resolved, _, err := getStorage(ctx, client, target)
	if err != nil {
		return result, err
	}
	binding := "not-applicable"
	switch from := captured.(type) {
	case *core.PersistentVolumeClaim:
		if volume, ok := resolved.(*core.PersistentVolume); ok {
			binding, err = storageBinding(from, volume)
		}
	case *core.PersistentVolume:
		if claim, ok := resolved.(*core.PersistentVolumeClaim); ok {
			binding, err = storageBinding(claim, from)
		}
	}
	if err != nil {
		return result, err
	}
	target.UID = string(resolved.GetUID())
	result = ResolvedStorageReference{
		Resource: target, SourceVersion: captured.GetResourceVersion(), TargetVersion: resolved.GetResourceVersion(),
		IdentityEvidence: identity, BindingEvidence: binding,
	}
	if err := checkPayload(result); err != nil {
		return ResolvedStorageReference{}, err
	}
	if err := ctx.Err(); err != nil {
		return ResolvedStorageReference{}, safeError(ctx, err)
	}
	return result, s.connections.Validate(source.Connection)
}
