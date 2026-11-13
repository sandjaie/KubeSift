/*
Purpose: Traverse Deployment controller ownership in bounded pages.
Used by: Deployment Pods investigation.
Responsibilities: Skip bounded intermediate owner pages and disclose partial permission coverage.
Important: Labels narrow requests; controller UIDs alone prove ownership.
*/
package kube

import (
	"context"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

type replicaSetIdentity struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

type relatedCursor struct {
	ReplicaSets string               `json:"replicaSets"`
	Pending     []replicaSetIdentity `json:"pending"`
	Pods        string               `json:"pods"`
}

func ownedBy(refs []metav1.OwnerReference, uid string) bool {
	for _, r := range refs {
		if r.Controller != nil && *r.Controller && r.UID == types.UID(uid) {
			return true
		}
	}
	return false
}
func (s *Service) ReadRelatedPods(ctx context.Context, req RelatedRequest) (RelatedPage, error) {
	// Owner discovery is not a user-facing Pod page. Skip empty intermediate
	// pages, but cap work per request even in namespaces with many ReplicaSets.
	for step := 0; ; step++ {
		page, err := s.readRelatedPodPage(ctx, req)
		if err != nil || page.Problem != nil || len(page.Items) > 0 || page.Complete || page.Page.NextCursor == "" || step == 7 {
			return page, err
		}
		req.Cursor = page.Page.NextCursor
	}
}

func (s *Service) readRelatedPodPage(ctx context.Context, req RelatedRequest) (RelatedPage, error) {
	result := RelatedPage{Items: []RelatedPod{}, Page: PageInfo{FetchedAt: time.Now().UTC()}}
	if req.Resource.Kind != "Deployment" && req.Resource.Kind != "ReplicaSet" {
		return result, model.Fail("invalid", "Deployment or ReplicaSet identity required")
	}
	client, err := s.boundedReadClient(ctx, req.Resource.Connection)
	if err != nil {
		return result, err
	}
	parent, err := checkedObject(ctx, client, req.Resource)
	if err != nil {
		return result, err
	}
	scope := pageCursor{Connection: req.Resource.Connection, Kind: req.Resource.Kind + "/Pods", Namespace: req.Resource.Namespace, Query: req.Resource.Name + "/" + req.Resource.UID}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return result, err
	}
	if rs, ok := parent.(*appsv1.ReplicaSet); ok {
		if rs.Spec.Selector == nil {
			return result, model.Fail("invalid", "ReplicaSet selector unavailable")
		}
		selector, selectorErr := metav1.LabelSelectorAsSelector(rs.Spec.Selector)
		if selectorErr != nil {
			return result, model.Fail("invalid", "ReplicaSet selector unavailable")
		}
		list, listErr := client.CoreV1().Pods(req.Resource.Namespace).List(ctx, metav1.ListOptions{Limit: 100, Continue: token, LabelSelector: selector.String()})
		if listErr != nil {
			if problem := ClassifyError(ctx, listErr); problem.Code == "forbidden" {
				result.Problem = problem
				return result, s.connections.Validate(req.Resource.Connection)
			}
			return result, readError(ctx, listErr)
		}
		if len(list.Items) > 100 {
			return result, model.Fail("capacity", "Server exceeded page size")
		}
		for _, pod := range list.Items {
			if ownedBy(pod.OwnerReferences, req.Resource.UID) {
				result.Items = append(result.Items, RelatedPod{Pod: podRow(req.Resource.Connection, pod), ReplicaSet: req.Resource})
			}
		}
		result.Page.ResourceVersion = list.ResourceVersion
		result.Complete = list.Continue == ""
		result.Page.NextCursor, err = encodeCursor(scope, list.Continue)
		if err != nil {
			return result, err
		}
		return result, s.connections.Validate(req.Resource.Connection)
	}
	cursor := relatedCursor{}
	if token != "" && json.Unmarshal([]byte(token), &cursor) != nil {
		return result, model.Fail("list_changed", "List changed; refresh")
	}
	if len(cursor.Pending) > 16 {
		return result, model.Fail("capacity", "Too many pending ReplicaSets")
	}
	partial := func(e error) (RelatedPage, error) {
		p := ClassifyError(ctx, e)
		if p.Code == "forbidden" {
			result.Problem = p
			return result, s.connections.Validate(req.Resource.Connection)
		}
		return result, readError(ctx, e)
	}
	if len(cursor.Pending) == 0 {
		// Keep the pending ownership queue within the bridge cursor bound.
		list, e := client.AppsV1().ReplicaSets(req.Resource.Namespace).List(ctx, metav1.ListOptions{Limit: 16, Continue: cursor.ReplicaSets})
		if e != nil {
			return partial(e)
		}
		if len(list.Items) > 16 {
			return result, model.Fail("capacity", "Server exceeded page size")
		}
		cursor.ReplicaSets = list.Continue
		result.Page.ResourceVersion = list.ResourceVersion
		for _, rs := range list.Items {
			if ownedBy(rs.OwnerReferences, req.Resource.UID) {
				cursor.Pending = append(cursor.Pending, replicaSetIdentity{Name: rs.Name, UID: string(rs.UID)})
			}
		}
	} else {
		identity := cursor.Pending[0]
		rsref := ResourceRef{Connection: req.Resource.Connection, APIVersion: "apps/v1", Kind: "ReplicaSet", Namespace: req.Resource.Namespace, Name: identity.Name, UID: identity.UID}
		if validResource(rsref) != nil {
			return result, model.Fail("list_changed", "List changed; refresh")
		}
		rs, e := client.AppsV1().ReplicaSets(req.Resource.Namespace).Get(ctx, rsref.Name, metav1.GetOptions{})
		if e != nil {
			return partial(e)
		}
		if string(rs.UID) != rsref.UID || !ownedBy(rs.OwnerReferences, req.Resource.UID) {
			return result, model.Fail("resource_replaced", "Relationship changed; refresh")
		}
		if rs.Spec.Selector == nil {
			return result, model.Fail("invalid", "ReplicaSet selector unavailable")
		}
		selector, e := metav1.LabelSelectorAsSelector(rs.Spec.Selector)
		if e != nil {
			return result, model.Fail("invalid", "ReplicaSet selector unavailable")
		}
		list, e := client.CoreV1().Pods(req.Resource.Namespace).List(ctx, metav1.ListOptions{Limit: 100, Continue: cursor.Pods, LabelSelector: selector.String()})
		if e != nil {
			return partial(e)
		}
		if len(list.Items) > 100 {
			return result, model.Fail("capacity", "Server exceeded page size")
		}
		result.Page.ResourceVersion = list.ResourceVersion
		for _, pod := range list.Items {
			if ownedBy(pod.OwnerReferences, rsref.UID) {
				result.Items = append(result.Items, RelatedPod{Pod: podRow(req.Resource.Connection, pod), ReplicaSet: rsref})
			}
		}
		cursor.Pods = list.Continue
		if cursor.Pods == "" {
			cursor.Pending = cursor.Pending[1:]
		}
	}
	result.Complete = len(cursor.Pending) == 0 && cursor.ReplicaSets == ""
	if !result.Complete {
		raw, _ := json.Marshal(cursor)
		result.Page.NextCursor, err = encodeCursor(scope, string(raw))
		if err != nil {
			return result, err
		}
	}
	return result, s.connections.Validate(req.Resource.Connection)
}

func (s *Service) ReadRelatedReplicaSets(ctx context.Context, req RelatedRequest) (RelatedReplicaSetPage, error) {
	result := RelatedReplicaSetPage{Items: []WorkloadRow{}, Page: PageInfo{FetchedAt: time.Now().UTC()}}
	parent := req.Resource
	if parent.Kind != "Deployment" {
		return result, model.Fail("invalid", "Deployment identity required")
	}
	client, err := s.boundedReadClient(ctx, parent.Connection)
	if err != nil {
		return result, err
	}
	if _, err = checkedObject(ctx, client, parent); err != nil {
		return result, err
	}
	scope := pageCursor{Connection: parent.Connection, Kind: "Deployment/ReplicaSets", Namespace: parent.Namespace, Query: parent.Name + "/" + parent.UID}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return result, err
	}
	for range 8 {
		list, e := client.AppsV1().ReplicaSets(parent.Namespace).List(ctx, metav1.ListOptions{Limit: 100, Continue: token})
		if e != nil {
			if problem := ClassifyError(ctx, e); problem.Code == "forbidden" {
				result.Problem = problem
				return result, s.connections.Validate(parent.Connection)
			}
			return result, readError(ctx, e)
		}
		if len(list.Items) > 100 {
			return result, model.Fail("capacity", "Server exceeded page size")
		}
		for _, rs := range list.Items {
			if ownedBy(rs.OwnerReferences, parent.UID) {
				result.Items = append(result.Items, replicaSetRow(parent.Connection, rs))
			}
		}
		token = list.Continue
		result.Page.ResourceVersion = list.ResourceVersion
		if len(result.Items) != 0 || token == "" {
			break
		}
	}
	result.Complete = token == ""
	result.Page.NextCursor, err = encodeCursor(scope, token)
	if err != nil {
		return result, err
	}
	return result, s.connections.Validate(parent.Connection)
}
