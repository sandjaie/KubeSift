/*
Purpose: Traverse Deployment controller ownership in bounded pages.
Used by: Deployment Pods investigation.
Responsibilities: Advance one ReplicaSet or Pod list page and disclose partial permission coverage.
Important: Labels narrow requests; controller UIDs alone prove ownership.
*/
package kube

import (
	"context"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

type relatedCursor struct {
	ReplicaSets string        `json:"replicaSets"`
	Pending     []ResourceRef `json:"pending"`
	Pods        string        `json:"pods"`
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
	result := RelatedPage{Items: []RelatedPod{}, Page: PageInfo{FetchedAt: time.Now().UTC()}}
	if req.Resource.Kind != "Deployment" {
		return result, model.Fail("invalid", "Deployment identity required")
	}
	client, err := s.boundedReadClient(ctx, req.Resource.Connection)
	if err != nil {
		return result, err
	}
	_, err = checkedObject(ctx, client, req.Resource)
	if err != nil {
		return result, err
	}
	scope := pageCursor{Connection: req.Resource.Connection, Kind: "related", Namespace: req.Resource.Namespace, Query: req.Resource.Name + "/" + req.Resource.UID}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return result, err
	}
	cursor := relatedCursor{}
	if token != "" && json.Unmarshal([]byte(token), &cursor) != nil {
		return result, model.Fail("list_changed", "List changed; refresh")
	}
	if len(cursor.Pending) > 100 {
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
		list, e := client.AppsV1().ReplicaSets(req.Resource.Namespace).List(ctx, metav1.ListOptions{Limit: 100, Continue: cursor.ReplicaSets})
		if e != nil {
			return partial(e)
		}
		if len(list.Items) > 100 {
			return result, model.Fail("capacity", "Server exceeded page size")
		}
		cursor.ReplicaSets = list.Continue
		result.Page.ResourceVersion = list.ResourceVersion
		for _, rs := range list.Items {
			if ownedBy(rs.OwnerReferences, req.Resource.UID) {
				cursor.Pending = append(cursor.Pending, resourceRef(req.Resource.Connection, "ReplicaSet", "apps/v1", rs.ObjectMeta))
			}
		}
	} else {
		rsref := cursor.Pending[0]
		if rsref.Connection != req.Resource.Connection || rsref.Namespace != req.Resource.Namespace || rsref.Kind != "ReplicaSet" {
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
