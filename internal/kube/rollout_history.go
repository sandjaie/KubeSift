/*
Purpose: Read owned rollout history for the three supported workloads.
Used by: The Revisions view and rollback source validation.
Responsibilities: Validate controller ownership and project scoped, bounded revision rows.
Important: Duplicate revision numbers remain independently selectable by source UID.
*/
package kube

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
)

func RolloutSource(parentKind string) (kind, resource string) {
	switch parentKind {
	case "Deployment":
		return "ReplicaSet", "replicasets"
	case "StatefulSet", "DaemonSet":
		return "ControllerRevision", "controllerrevisions"
	default:
		return "", ""
	}
}

func rolloutNumber(value string) int64 {
	if len(value) == 0 || len(value) > 19 {
		return 0
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0
		}
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func rolloutRevision(raw []byte, parent ResourceRef, listed bool) (RolloutRevision, error) {
	var row RolloutRevision
	kind, plural := RolloutSource(parent.Kind)
	if kind == "" || len(raw) > detailLimit {
		return row, model.Fail("invalid", "Unsupported or oversized rollout history source")
	}
	api := apiResource{"apps", "v1", plural, kind, true}
	identity, err := objectIdentity(raw, api, parent.Namespace, listed)
	if err != nil {
		return row, err
	}
	metadata := identity.Metadata
	controllers := 0
	for _, owner := range metadata.OwnerReferences {
		if owner.Controller != nil && *owner.Controller {
			controllers++
		}
	}
	if controllers != 1 || !exactOwner(metadata.OwnerReferences, parent) {
		return row, nil
	}
	if metadata.ResourceVersion == "" || len(metadata.ResourceVersion) > 128 {
		return row, model.Fail("invalid_response", "Revision source has no usable resource version")
	}
	ref := resourceRef(parent.Connection, kind, "apps/v1", metadata)
	if err := validResource(ref); err != nil {
		return row, err
	}
	var revision int64
	var template core.PodTemplateSpec
	switch kind {
	case "ReplicaSet":
		var rs apps.ReplicaSet
		if json.Unmarshal(raw, &rs) != nil {
			return row, model.Fail("invalid_response", "Kubernetes returned invalid ReplicaSet history")
		}
		revision = rolloutNumber(metadata.Annotations["deployment.kubernetes.io/revision"])
		template = rs.Spec.Template
	case "ControllerRevision":
		var source apps.ControllerRevision
		if json.Unmarshal(raw, &source) != nil {
			return row, model.Fail("invalid_response", "Kubernetes returned invalid ControllerRevision history")
		}
		revision = source.Revision
		var data struct {
			Spec struct{ Template core.PodTemplateSpec }
		}
		if json.Unmarshal(source.Data.Raw, &data) == nil {
			template = data.Spec.Template
		}
	}
	if revision <= 0 {
		return row, nil
	}
	row = RolloutRevision{Resource: ref, ResourceVersion: metadata.ResourceVersion, Revision: strconv.FormatInt(revision, 10),
		CreatedAt: created(metadata), ChangeCause: limited(metadata.Annotations["kubernetes.io/change-cause"])}
	for _, c := range template.Spec.InitContainers {
		if c.Image != "" {
			row.Images = append(row.Images, limited(c.Image))
		}
	}
	for _, c := range template.Spec.Containers {
		if c.Image != "" {
			row.Images = append(row.Images, limited(c.Image))
		}
	}
	return row, nil
}

// CheckRolloutRevision verifies the exact GET source selected for a review.
func CheckRolloutRevision(raw []byte, parent, source ResourceRef, version, revision string) error {
	row, err := rolloutRevision(raw, parent, false)
	if err != nil {
		return err
	}
	if row.Resource != source || row.ResourceVersion != version || row.Revision != revision {
		return model.Fail("stale_target", "The selected revision changed or no longer belongs to this workload. Refresh Revisions and choose it again.")
	}
	return nil
}

func (s *Service) ReadRolloutHistory(ctx context.Context, req RolloutHistoryRequest) (RolloutHistoryPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ref := req.Resource
	kind, plural := RolloutSource(ref.Kind)
	if kind == "" || ref.APIVersion != "apps/v1" {
		return RolloutHistoryPage{}, model.Fail("invalid", "Select a Deployment, StatefulSet or DaemonSet")
	}
	client, err := s.boundedReadClient(ctx, ref.Connection)
	if err != nil {
		return RolloutHistoryPage{}, err
	}
	parent, err := checkedObject(ctx, client, ref)
	if err != nil {
		return RolloutHistoryPage{}, err
	}
	if parent.GetName() != ref.Name || parent.GetNamespace() != ref.Namespace || parent.GetResourceVersion() == "" ||
		len(parent.GetResourceVersion()) > 128 {
		return RolloutHistoryPage{}, model.Fail("stale_target", "The workload has no matching captured identity or version")
	}
	scope := pageCursor{Connection: ref.Connection, Namespace: ref.Namespace, Kind: ref.Kind + "/RolloutHistory",
		Query: ref.Name + "/" + ref.UID}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return RolloutHistoryPage{}, err
	}
	result := RolloutHistoryPage{Items: []RolloutRevision{}, SourceVersion: parent.GetResourceVersion(),
		Page: PageInfo{FetchedAt: time.Now().UTC()}}
	if set, ok := parent.(*apps.StatefulSet); ok {
		result.CurrentRevision, result.UpdateRevision = set.Status.CurrentRevision, set.Status.UpdateRevision
	}
	api := apiResource{"apps", "v1", plural, kind, true}
	for range 8 {
		list, err := listJSON(ctx, api.read(client, ref.Namespace, ""), api, token, true)
		if err != nil {
			if p := ClassifyError(ctx, err); p.Code == "forbidden" {
				result.Problem = p
				return result, s.finishAvailability(ctx, ref.Connection, result)
			}
			return RolloutHistoryPage{}, readError(ctx, err)
		}
		for _, raw := range list.Items {
			row, err := rolloutRevision(raw, ref, true)
			if err != nil {
				return RolloutHistoryPage{}, err
			}
			if row.Revision != "" {
				result.Items = append(result.Items, row)
			}
		}
		token, result.Page.ResourceVersion = list.Metadata.Continue, list.Metadata.ResourceVersion
		if len(result.Items) > 0 || token == "" {
			break
		}
	}
	slices.SortFunc(result.Items, func(a, b RolloutRevision) int {
		if order := cmp.Compare(len(b.Revision), len(a.Revision)); order != 0 {
			return order
		}
		if order := cmp.Compare(b.Revision, a.Revision); order != 0 {
			return order
		}
		return cmp.Compare(a.Resource.UID, b.Resource.UID)
	})
	result.Complete = token == ""
	result.Page.NextCursor, err = encodeCursor(scope, token)
	if err != nil {
		return RolloutHistoryPage{}, err
	}
	return result, s.finishAvailability(ctx, ref.Connection, result)
}
