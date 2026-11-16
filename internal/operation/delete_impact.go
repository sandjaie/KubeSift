/*
Purpose: Project deletion evidence from a captured workload.
Used by: Native deletion preparation and final eligibility checks.
Responsibilities: Validate storage policy and bound direct owner, finalizer and count evidence.
Important: No dependent scans, arbitrary annotations or workload configuration cross the bridge.
*/
package operation

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/sandjaie/KubeSift/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const deleteEvidenceEntries = 100
const deleteEvidenceBytes = 8 << 10

func projectDeleteImpact(raw json.RawMessage, kind string) (DeleteImpact, error) {
	var p struct {
		Metadata struct {
			Annotations     map[string]json.RawMessage
			OwnerReferences json.RawMessage
			Finalizers      []string
		}
		Spec struct {
			PVCPolicy        json.RawMessage `json:"persistentVolumeClaimRetentionPolicy"`
			TerminationGrace *int64          `json:"terminationGracePeriodSeconds"`
			Replicas         *int32
		}
		Status struct {
			Replicas               *int32
			Ready                  *int32
			ReadyReplicas          *int32
			DesiredNumberScheduled *int32
			CurrentNumberScheduled *int32
			NumberReady            *int32
			Active                 json.RawMessage
		}
	}
	if json.Unmarshal(raw, &p) != nil {
		return DeleteImpact{}, invalidDeleteEvidence()
	}
	impact := DeleteImpact{
		Propagation: string(metav1.DeletePropagationBackground), GracePolicy: deleteGracePolicy,
		Finalizers: []string{}, FinalizerCount: len(p.Metadata.Finalizers),
	}
	switch kind {
	case "Pod":
		if _, mirror := p.Metadata.Annotations["kubernetes.io/config.mirror"]; mirror {
			return DeleteImpact{}, model.Fail("mirror_pod", "Mirror Pods are managed by the node and cannot be deleted here")
		}
		grace := int64(30)
		impact.PodGrace, impact.PodGraceDefault = p.Spec.TerminationGrace, p.Spec.TerminationGrace == nil
		if impact.PodGraceDefault {
			impact.PodGrace = &grace
		}
		if *impact.PodGrace < 0 {
			return DeleteImpact{}, invalidDeleteEvidence()
		}
	case "StatefulSet":
		var policy map[string]json.RawMessage
		if len(p.Spec.PVCPolicy) != 0 && (json.Unmarshal(p.Spec.PVCPolicy, &policy) != nil || policy == nil) {
			return DeleteImpact{}, model.Fail("unsafe_retention", "StatefulSet claim retention on deletion could not be verified")
		}
		if value, supplied := policy["whenScaled"]; supplied {
			var retention string
			if json.Unmarshal(value, &retention) != nil || (retention != "Retain" && retention != "Delete") {
				return DeleteImpact{}, model.Fail("unsafe_retention", "StatefulSet claim retention on scaling could not be verified")
			}
		}
		impact.WhenDeleted = "Retain"
		value, supplied := policy["whenDeleted"]
		impact.RetentionDefault = !supplied
		if supplied {
			var retention *string
			if json.Unmarshal(value, &retention) != nil || retention == nil || *retention != "Retain" {
				return DeleteImpact{}, model.Fail("unsafe_retention", "StatefulSet deletion requires a verified Retain policy for claims")
			}
		}
	}
	switch kind {
	case "Deployment", "StatefulSet", "ReplicaSet", "ReplicationController":
		impact.Desired, impact.Current, impact.Ready = p.Spec.Replicas, p.Status.Replicas, p.Status.ReadyReplicas
	case "DaemonSet":
		impact.Desired, impact.Current, impact.Ready = p.Status.DesiredNumberScheduled, p.Status.CurrentNumberScheduled, p.Status.NumberReady
	case "Job":
		impact.Ready = p.Status.Ready
		if len(p.Status.Active) != 0 && json.Unmarshal(p.Status.Active, &impact.Active) != nil {
			return DeleteImpact{}, invalidDeleteEvidence()
		}
	case "CronJob":
		if len(p.Status.Active) != 0 && string(p.Status.Active) != "null" {
			var active []json.RawMessage
			if json.Unmarshal(p.Status.Active, &active) != nil {
				return DeleteImpact{}, invalidDeleteEvidence()
			}
			count := int32(len(active))
			impact.Active = &count
		}
	}
	for _, count := range []*int32{impact.Desired, impact.Current, impact.Ready, impact.Active} {
		if count != nil && *count < 0 {
			return DeleteImpact{}, invalidDeleteEvidence()
		}
	}
	impact.Owner, impact.OwnershipUnknown = deleteOwner(p.Metadata.OwnerReferences)
	budget := deleteEvidenceBytes - 2
	maxFinalizers := deleteEvidenceEntries
	if owner := impact.Owner; owner != nil {
		encoded, _ := json.Marshal(owner)
		budget -= len(encoded)
		maxFinalizers--
	}
	for _, name := range p.Metadata.Finalizers {
		if len(validation.IsQualifiedName(name)) != 0 {
			return DeleteImpact{}, invalidDeleteEvidence()
		}
		encoded, _ := json.Marshal(name)
		if len(impact.Finalizers) == maxFinalizers || len(encoded)+1 > budget {
			impact.FinalizersPartial = true
			continue
		}
		budget -= len(encoded) + 1
		impact.Finalizers = append(impact.Finalizers, name)
	}
	return impact, nil
}

func deleteOwner(raw json.RawMessage) (*DeleteOwner, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var refs []metav1.OwnerReference
	if len(raw) > deleteEvidenceBytes || json.Unmarshal(raw, &refs) != nil || len(refs) > deleteEvidenceEntries {
		return nil, true
	}
	var owner *DeleteOwner
	for _, ref := range refs {
		group, version, grouped := strings.Cut(ref.APIVersion, "/")
		if !grouped {
			version = group
		}
		if (grouped && len(validation.IsDNS1123Subdomain(group)) != 0) || len(validation.IsDNS1035Label(version)) != 0 ||
			len(validation.IsQualifiedName(ref.Kind)) != 0 || len(validation.IsDNS1123Subdomain(ref.Name)) != 0 ||
			len(ref.UID) == 0 || len(ref.UID) > 128 || strings.IndexFunc(string(ref.UID), func(r rune) bool { return !unicode.IsGraphic(r) || unicode.IsSpace(r) }) >= 0 {
			return nil, true
		}
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		if owner != nil {
			return nil, true
		}
		owner = &DeleteOwner{APIVersion: ref.APIVersion, Kind: ref.Kind, Name: ref.Name, UID: string(ref.UID)}
	}
	return owner, false
}
