/*
Purpose: Inventory Events within one namespace and follow supported historical targets.
Used by: Cluster Events and captured navigation.
Responsibilities: Keep one API family per page sequence and verify source version and target UID.
Important: Never falls back on denial or scans all namespaces.
*/
package kube

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	events "k8s.io/api/events/v1"
)

func modernEvent(event events.Event) ResourceEvent {
	first := firstTime(event.DeprecatedFirstTimestamp.Time, event.EventTime.Time, event.CreationTimestamp.Time)
	var series time.Time
	var count *int32
	if event.DeprecatedCount > 0 {
		count = &event.DeprecatedCount
	}
	if event.Series != nil {
		series = event.Series.LastObservedTime.Time
		count = &event.Series.Count
	}
	return ResourceEvent{UID: string(event.UID), Type: event.Type, Reason: event.Reason, Message: event.Note, Reporter: event.ReportingController, Count: count, FirstAt: first, LastAt: firstTime(series, event.DeprecatedLastTimestamp.Time, event.EventTime.Time, eventTime(first))}
}
func legacyEvent(event core.Event) ResourceEvent {
	first := firstTime(event.FirstTimestamp.Time, event.EventTime.Time, event.CreationTimestamp.Time)
	var series time.Time
	var count *int32
	if event.Count > 0 {
		count = &event.Count
	}
	if event.Series != nil {
		series = event.Series.LastObservedTime.Time
		count = &event.Series.Count
	}
	reporter := event.ReportingController
	if reporter == "" {
		reporter = event.Source.Component
	}
	return ResourceEvent{UID: string(event.UID), Type: event.Type, Reason: event.Reason, Message: event.Message, Reporter: reporter, Count: count, FirstAt: first, LastAt: firstTime(series, event.LastTimestamp.Time, event.EventTime.Time, eventTime(first))}
}

func eventEvidence(raw []byte, api apiResource) (ResourceEvent, core.ObjectReference, error) {
	if api == modernEvents {
		var e events.Event
		if json.Unmarshal(raw, &e) == nil {
			return modernEvent(e), e.Regarding, nil
		}
	} else {
		var e core.Event
		if json.Unmarshal(raw, &e) == nil {
			return legacyEvent(e), e.InvolvedObject, nil
		}
	}
	return ResourceEvent{}, core.ObjectReference{}, model.Fail("invalid", "Invalid Event evidence")
}
func supportedEventTarget(source ResourceRef, regarding core.ObjectReference) *ResourceRef {
	if regarding.Kind == "Secret" || regarding.Kind == "Event" || regarding.FieldPath != "" {
		return nil
	}
	if !isClusterScopedResource(regarding.Kind) && regarding.Namespace != source.Namespace {
		return nil
	}
	ref := ResourceRef{Connection: source.Connection, Kind: regarding.Kind, APIVersion: regarding.APIVersion, Name: regarding.Name, Namespace: regarding.Namespace, UID: string(regarding.UID)}
	if validResource(ref) != nil {
		return nil
	}
	return &ref
}
func (s *Service) ReadEventInventory(ctx context.Context, req PageRequest) (EventInventoryPage, error) {
	if req.Kind != "Event" {
		return EventInventoryPage{}, model.Fail("invalid", "Unsupported Event inventory")
	}
	if err := namespace(req.Namespace); err != nil {
		return EventInventoryPage{}, err
	}
	scope := pageCursor{Connection: req.Connection, Namespace: req.Namespace, Kind: "Event"}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return EventInventoryPage{}, err
	}
	api := modernEvents
	if token != "" {
		var family string
		var ok bool
		family, token, ok = strings.Cut(token, ":")
		if !ok || token == "" || (family != "modern" && family != "core") {
			return EventInventoryPage{}, model.Fail("list_changed", "List changed; refresh")
		}
		if family == "core" {
			api = coreEvents
		}
	}
	client, err := s.boundedReadClient(ctx, req.Connection)
	if err != nil {
		return EventInventoryPage{}, err
	}
	list, err := listJSON(ctx, api.read(client, req.Namespace, ""), api, token, true)
	if req.Cursor == "" && eventAPIAbsent(err) {
		api = coreEvents
		list, err = listJSON(ctx, api.read(client, req.Namespace, ""), api, "", true)
	}
	if err != nil {
		return EventInventoryPage{}, resourceListError(ctx, err)
	}
	result := EventInventoryPage{Items: make([]EventInventoryRow, 0, len(list.Items))}
	for _, raw := range list.Items {
		obj, err := objectIdentity(raw, api, req.Namespace, true)
		if err != nil {
			return EventInventoryPage{}, err
		}
		ref := resourceRef(req.Connection, "Event", api.apiVersion(), obj.Metadata)
		if err := validResource(ref); err != nil {
			return EventInventoryPage{}, err
		}
		e, target, err := eventEvidence(raw, api)
		if err != nil {
			return EventInventoryPage{}, err
		}
		result.Items = append(result.Items, EventInventoryRow{Resource: ref, Event: e, Regarding: strings.TrimSpace(target.Kind + " " + target.Name), Target: supportedEventTarget(ref, target)})
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		return eventTime(result.Items[i].Event.LastAt).After(eventTime(result.Items[j].Event.LastAt))
	})
	if list.Metadata.Continue != "" {
		family := "modern"
		if api == coreEvents {
			family = "core"
		}
		list.Metadata.Continue = family + ":" + list.Metadata.Continue
	}
	result.Page, err = pageInfo(scope, list.Metadata)
	if err != nil {
		return EventInventoryPage{}, err
	}
	if err = checkPayload(result); err != nil {
		return EventInventoryPage{}, err
	}
	return result, s.connections.Validate(req.Connection)
}
func (s *Service) ResolveEventTarget(ctx context.Context, req EventTargetRequest) (ResolvedEventTarget, error) {
	if req.Source.Kind != "Event" || req.SourceVersion == "" {
		return ResolvedEventTarget{}, model.Fail("invalid", "Explicit captured Event required")
	}
	if err := validResource(req.Source); err != nil {
		return ResolvedEventTarget{}, err
	}
	client, err := s.boundedReadClient(ctx, req.Source.Connection)
	if err != nil {
		return ResolvedEventTarget{}, err
	}
	source, raw, err := getClusterResource(ctx, client, req.Source)
	if err != nil {
		return ResolvedEventTarget{}, err
	}
	if source.GetResourceVersion() != req.SourceVersion {
		return ResolvedEventTarget{}, model.Fail("resource_changed", "Event changed; refresh the source")
	}
	_, regarding, err := eventEvidence(raw, clusterAPI("Event", req.Source.APIVersion))
	if err != nil {
		return ResolvedEventTarget{}, err
	}
	target := supportedEventTarget(req.Source, regarding)
	if target == nil {
		return ResolvedEventTarget{}, model.Fail("invalid", "Event has no supported target with a complete identity")
	}
	detail, err := s.readOverview(ctx, client, DetailRequest{Resource: *target})
	if err != nil {
		return ResolvedEventTarget{}, err
	}
	if ctx.Err() != nil {
		return ResolvedEventTarget{}, safeError(ctx, ctx.Err())
	}
	return ResolvedEventTarget{Detail: detail}, s.connections.Validate(req.Source.Connection)
}
