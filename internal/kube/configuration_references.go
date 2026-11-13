/*
Purpose: Resolve an explicit configuration reference from a captured Pod.
Used by: Pod Overview configuration navigation.
Responsibilities: Revalidate source UID/version and check the field allowlist before target I/O.
Important: Secret targets use partial metadata only.
*/
package kube

import (
	"context"

	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
)

func hasConfigurationReference(pod *core.Pod, kind, name string) bool {
	if (kind != "Secret" && kind != "ConfigMap") || name == "" {
		return false
	}
	secret := kind == "Secret"
	containerHas := func(env []core.EnvVar, from []core.EnvFromSource) bool {
		for _, item := range env {
			ref := item.ValueFrom
			if ref == nil {
				continue
			}
			if secret && ref.SecretKeyRef != nil && ref.SecretKeyRef.Name == name {
				return true
			}
			if !secret && ref.ConfigMapKeyRef != nil && ref.ConfigMapKeyRef.Name == name {
				return true
			}
		}
		for _, ref := range from {
			if secret && ref.SecretRef != nil && ref.SecretRef.Name == name {
				return true
			}
			if !secret && ref.ConfigMapRef != nil && ref.ConfigMapRef.Name == name {
				return true
			}
		}
		return false
	}
	for _, containers := range [][]core.Container{pod.Spec.Containers, pod.Spec.InitContainers} {
		for _, c := range containers {
			if containerHas(c.Env, c.EnvFrom) {
				return true
			}
		}
	}
	for _, c := range pod.Spec.EphemeralContainers {
		if containerHas(c.Env, c.EnvFrom) {
			return true
		}
	}
	if secret {
		for _, ref := range pod.Spec.ImagePullSecrets {
			if ref.Name == name {
				return true
			}
		}
	}
	for _, vol := range pod.Spec.Volumes {
		if secret && vol.Secret != nil && vol.Secret.SecretName == name {
			return true
		}
		if !secret && vol.ConfigMap != nil && vol.ConfigMap.Name == name {
			return true
		}
		if vol.Projected == nil {
			continue
		}
		for _, ref := range vol.Projected.Sources {
			if secret && ref.Secret != nil && ref.Secret.Name == name {
				return true
			}
			if !secret && ref.ConfigMap != nil && ref.ConfigMap.Name == name {
				return true
			}
		}
	}
	return false
}

func (s *Service) ResolveConfigurationReference(ctx context.Context, req ConfigurationReferenceRequest) (ResolvedConfigurationReference, error) {
	result := ResolvedConfigurationReference{}
	source, target := req.Source, req.Target
	if source.Kind != "Pod" || (target.Kind != "ConfigMap" && target.Kind != "Secret") ||
		target.Connection != source.Connection || target.Namespace != source.Namespace || req.SourceVersion == "" || len(req.SourceVersion) > 256 {
		return result, model.Fail("invalid", "Captured Pod configuration reference required")
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
	if object.GetName() != source.Name || object.GetNamespace() != source.Namespace || object.GetResourceVersion() != req.SourceVersion {
		return result, model.Fail("stale_target", "Pod configuration changed; refresh the section")
	}
	if err = checkPayload(object); err != nil {
		return result, err
	}
	if !hasConfigurationReference(object.(*core.Pod), target.Kind, target.Name) {
		return result, model.Fail("invalid", "Reference is unavailable in the captured Pod")
	}
	if target.Kind == "Secret" {
		detail, e := s.readSecretMetadata(ctx, target)
		if e != nil {
			return result, e
		}
		target = detail.Resource
	} else {
		config, e := getResource(ctx, client, target)
		if e != nil {
			return result, e
		}
		if e = checkPayload(config); e != nil {
			return result, e
		}
		target.UID = string(config.GetUID())
	}
	if err = validResource(target); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, safeError(ctx, err)
	}
	result = ResolvedConfigurationReference{Resource: target, IdentityEvidence: "name-resolved"}
	if err = checkPayload(result); err != nil {
		return ResolvedConfigurationReference{}, err
	}
	return result, s.connections.Validate(source.Connection)
}
