/*
Purpose: Read bounded PDB label associations and controller revision history.
Used by: Selected Pods and Revisions investigation tabs.
Responsibilities: Validate selectors, source identity, ownership and continuation scope.
Important: A PDB selector match never establishes ownership or eviction eligibility.
*/
package kube

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	apps "k8s.io/api/apps/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func revisionParent(kind string) bool { return kind == "StatefulSet" || kind == "DaemonSet" }

func exactOwner(refs []metav1.OwnerReference, parent ResourceRef) bool {
	owner := controllerOwner(refs)
	return owner != nil && owner.Kind == parent.Kind && owner.APIVersion == parent.APIVersion && owner.Name == parent.Name && string(owner.UID) == parent.UID
}

func pdbSelector(pdb *policy.PodDisruptionBudget) (labels.Selector, error) {
	if pdb.Spec.Selector == nil {
		return nil, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
	if err != nil {
		return nil, model.Fail("invalid", "PodDisruptionBudget selector unavailable")
	}
	return selector, nil
}

func (s *Service) finishAvailability(ctx context.Context, ref model.ConnectionRef, result any) error {
	if err := ctx.Err(); err != nil {
		return safeError(ctx, err)
	}
	if err := checkPayload(result); err != nil {
		return err
	}
	return s.connections.Validate(ref)
}

func (s *Service) ReadPDBSelectedPods(ctx context.Context, req RelatedRequest) (SelectedPodPage, error) {
	ref := req.Resource
	var empty SelectedPodPage
	if ref.Kind != "PodDisruptionBudget" {
		return empty, model.Fail("invalid", "PodDisruptionBudget identity required")
	}
	if err := validResource(ref); err != nil {
		return empty, err
	}
	client, err := s.boundedReadClient(ctx, ref.Connection)
	if err != nil {
		return empty, err
	}
	object, _, err := getAvailability(ctx, client, ref)
	if err != nil {
		return empty, err
	}
	pdb := object.(*policy.PodDisruptionBudget)
	selector, err := pdbSelector(pdb)
	if err != nil {
		return empty, err
	}
	raw, err := json.Marshal(pdb.Spec.Selector)
	if err != nil {
		return empty, model.Fail("invalid", "Cannot encode selector evidence")
	}
	scope := pageCursor{Connection: ref.Connection, Namespace: ref.Namespace, Kind: "PodDisruptionBudget/Selected Pods", Query: ref.UID + "/null"}
	if selector != nil {
		scope.Query = ref.UID + "/selector/" + selector.String()
	}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return empty, err
	}
	result := SelectedPodPage{Items: []WorkloadRow{}, SourceVersion: pdb.ResourceVersion, SelectorJSON: string(raw), Complete: true, Page: PageInfo{FetchedAt: time.Now().UTC()}}
	if selector == nil {
		if token != "" {
			return empty, model.Fail("list_changed", "List changed; refresh")
		}
		return result, s.finishAvailability(ctx, ref.Connection, result)
	}
	list, err := client.CoreV1().Pods(ref.Namespace).List(ctx, metav1.ListOptions{Limit: 100, Continue: token, LabelSelector: selector.String()})
	if err != nil {
		if problem := ClassifyError(ctx, err); problem.Code == "forbidden" {
			result.Problem = problem
			result.Complete = false
			return result, s.finishAvailability(ctx, ref.Connection, result)
		}
		return empty, readError(ctx, err)
	}
	if len(list.Items) > 100 {
		return empty, model.Fail("capacity", "Server exceeded page size")
	}
	for _, pod := range list.Items {
		if pod.Namespace != ref.Namespace || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		row := podRow(ref.Connection, pod)
		if err := validResource(row.Resource); err != nil {
			return empty, model.Fail("invalid", "Server returned an invalid Pod identity")
		}
		result.Items = append(result.Items, row)
	}
	result.Page.ResourceVersion = list.ResourceVersion
	result.Complete = list.Continue == ""
	result.Page.NextCursor, err = encodeCursor(scope, list.Continue)
	if err != nil {
		return empty, err
	}
	if err := s.finishAvailability(ctx, ref.Connection, result); err != nil {
		return empty, err
	}
	return result, nil
}

func (s *Service) ReadControllerRevisions(ctx context.Context, req RelatedRequest) (ControllerRevisionPage, error) {
	ref := req.Resource
	var empty ControllerRevisionPage
	if !revisionParent(ref.Kind) {
		return empty, model.Fail("invalid", "Revision controller identity required")
	}
	if err := validResource(ref); err != nil {
		return empty, err
	}
	client, err := s.boundedReadClient(ctx, ref.Connection)
	if err != nil {
		return empty, err
	}
	parent, err := checkedObject(ctx, client, ref)
	if err != nil {
		return empty, err
	}
	if parent.GetName() != ref.Name || parent.GetNamespace() != ref.Namespace {
		return empty, model.Fail("invalid", "Invalid revision controller identity")
	}
	if err := checkPayload(parent); err != nil {
		return empty, err
	}
	scope := pageCursor{Connection: ref.Connection, Namespace: ref.Namespace, Kind: ref.Kind + "/Revisions", Query: ref.Name + "/" + ref.UID}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return empty, err
	}
	result := ControllerRevisionPage{Items: []AvailabilityRow{}, SourceVersion: parent.GetResourceVersion(), Page: PageInfo{FetchedAt: time.Now().UTC()}}
	if set, ok := parent.(*apps.StatefulSet); ok {
		result.CurrentRevision, result.UpdateRevision = set.Status.CurrentRevision, set.Status.UpdateRevision
	}
	for range 8 {
		rows, meta, e := availabilityList(ctx, client, PageRequest{Connection: ref.Connection, Namespace: ref.Namespace, Kind: "ControllerRevision"}, token,
			func(object workloadObject) bool { return exactOwner(object.GetOwnerReferences(), ref) })
		if e != nil {
			if p := ClassifyError(ctx, e); p.Code == "forbidden" {
				result.Problem = p
				return result, s.finishAvailability(ctx, ref.Connection, result)
			}
			return empty, e
		}
		token = meta.Continue
		result.Items, result.Page.ResourceVersion = rows, meta.ResourceVersion
		if len(rows) > 0 || token == "" {
			break
		}
	}
	result.Complete = token == ""
	result.Page.NextCursor, err = encodeCursor(scope, token)
	if err != nil {
		return empty, err
	}
	if err := s.finishAvailability(ctx, ref.Connection, result); err != nil {
		return empty, err
	}
	return result, nil
}
