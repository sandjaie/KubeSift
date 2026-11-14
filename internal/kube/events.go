/*
Purpose: Read normalized event evidence for one captured resource UID.
Used by: Pod and Deployment Events sections.
Responsibilities: Bound pages, normalize optional times and use core events only for absent modern support.
Important: Forbidden and uncertain failures never trigger an alternate authorization path.
*/
package kube

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sort"
	"strings"
	"time"
)

func firstTime(values ...time.Time) *time.Time {
	for _, v := range values {
		if !v.IsZero() {
			copy := v
			return &copy
		}
	}
	return nil
}
func eventTime(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}
	return *v
}
func eventAPIAbsent(err error) bool {
	return apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err)
}
func (s *Service) ReadResourceEvents(ctx context.Context, req EventRequest) (EventPage, error) {
	result := EventPage{Items: []ResourceEvent{}}
	if isClusterScopedResource(req.Resource.Kind) || req.Resource.Kind == "Secret" || req.Resource.Kind == "Event" {
		return result, model.Fail("invalid", "Events are unavailable for this kind")
	}
	if err := validResource(req.Resource); err != nil {
		return result, err
	}
	client, err := s.boundedReadClient(ctx, req.Resource.Connection)
	if err != nil {
		return result, err
	}
	_, err = checkedObject(ctx, client, req.Resource)
	if err != nil {
		return result, err
	}
	scope := pageCursor{Connection: req.Resource.Connection, Kind: "events", Namespace: req.Resource.Namespace, Query: req.Resource.Kind + "/" + req.Resource.Name + "/" + req.Resource.UID}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return result, err
	}
	family := "modern"
	if token != "" {
		var ok bool
		family, token, ok = strings.Cut(token, ":")
		if !ok || (family != "modern" && family != "core") {
			return result, model.Fail("list_changed", "List changed; refresh")
		}
	}
	var meta metav1.ListMeta
	if family == "modern" {
		list, e := client.EventsV1().Events(req.Resource.Namespace).List(ctx, metav1.ListOptions{Limit: 100, Continue: token, FieldSelector: fields.OneTermEqualSelector("regarding.uid", req.Resource.UID).String()})
		if e != nil {
			if req.Cursor == "" && eventAPIAbsent(e) {
				family = "core"
			} else {
				return result, readError(ctx, e)
			}
		} else {
			if len(list.Items) > 100 {
				return result, model.Fail("capacity", "Server exceeded event page size")
			}
			meta = list.ListMeta
			for _, event := range list.Items {
				if string(event.Regarding.UID) != req.Resource.UID || event.UID == "" {
					continue
				}
				result.Items = append(result.Items, modernEvent(event))
			}
		}
	}
	if family == "core" {
		list, e := client.CoreV1().Events(req.Resource.Namespace).List(ctx, metav1.ListOptions{Limit: 100, Continue: token, FieldSelector: fields.OneTermEqualSelector("involvedObject.uid", req.Resource.UID).String()})
		if e != nil {
			return result, readError(ctx, e)
		}
		if len(list.Items) > 100 {
			return result, model.Fail("capacity", "Server exceeded event page size")
		}
		meta = list.ListMeta
		for _, event := range list.Items {
			if string(event.InvolvedObject.UID) != req.Resource.UID || event.UID == "" {
				continue
			}
			result.Items = append(result.Items, legacyEvent(event))
		}
	}
	unique := map[string]ResourceEvent{}
	for _, event := range result.Items {
		old, ok := unique[event.UID]
		if !ok || !eventTime(event.LastAt).Before(eventTime(old.LastAt)) {
			unique[event.UID] = event
		}
	}
	result.Items = []ResourceEvent{}
	for _, event := range unique {
		result.Items = append(result.Items, event)
	}
	sort.Slice(result.Items, func(i, j int) bool {
		a, b := result.Items[i], result.Items[j]
		if eventTime(a.LastAt).Equal(eventTime(b.LastAt)) {
			return a.UID < b.UID
		}
		return eventTime(a.LastAt).After(eventTime(b.LastAt))
	})
	next := ""
	if meta.Continue != "" {
		next = family + ":" + meta.Continue
	}
	result.Page.NextCursor, err = encodeCursor(scope, next)
	if err != nil {
		return result, err
	}
	result.Page.ResourceVersion = meta.ResourceVersion
	result.Page.FetchedAt = time.Now().UTC()
	if err = checkPayload(result); err != nil {
		return EventPage{}, err
	}
	return result, s.connections.Validate(req.Resource.Connection)
}
