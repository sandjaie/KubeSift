/*
Purpose: Read namespaced availability inventories and snapshots.
Used by: Availability views and relationship readers.
Responsibilities: Validate identities, retain raw evidence and bound list and detail responses.
Important: No live metrics or scaling endpoints are involved.
*/
package kube

import (
	"context"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	apps "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2"
	policy "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"strconv"
	"time"
)

type availabilityResource struct {
	version, plural string
	newObject       func() workloadObject
}

var availabilityResources = map[string]availabilityResource{
	"HorizontalPodAutoscaler": {"autoscaling/v2", "horizontalpodautoscalers", func() workloadObject { return &autoscaling.HorizontalPodAutoscaler{} }},
	"PodDisruptionBudget":     {"policy/v1", "poddisruptionbudgets", func() workloadObject { return &policy.PodDisruptionBudget{} }},
	"ControllerRevision":      {"apps/v1", "controllerrevisions", func() workloadObject { return &apps.ControllerRevision{} }},
}

func availabilityVersion(kind string) string { return availabilityResources[kind].version }
func availabilityRead(client kubernetes.Interface, kind, ns string) *rest.Request {
	resource := availabilityResources[kind]
	transport := client.AppsV1().RESTClient()
	switch kind {
	case "HorizontalPodAutoscaler":
		transport = client.AutoscalingV2().RESTClient()
	case "PodDisruptionBudget":
		transport = client.PolicyV1().RESTClient()
	}
	return transport.Get().Namespace(ns).Resource(resource.plural).SetHeader("Accept", "application/json").MaxRetries(0)
}
func decodeAvailability(kind string, raw []byte) (workloadObject, error) {
	resource := availabilityResources[kind]
	object := resource.newObject()
	if json.Unmarshal(raw, object) != nil {
		return nil, model.Fail("invalid", "Server returned invalid availability evidence")
	}
	expected := schema.FromAPIVersionAndKind(resource.version, kind)
	returned := object.GetObjectKind().GroupVersionKind()
	if !returned.Empty() && returned != expected {
		return nil, model.Fail("invalid", "Server returned an unexpected availability kind or API")
	}
	object.GetObjectKind().SetGroupVersionKind(expected)
	return object, nil
}

func getAvailability(ctx context.Context, client kubernetes.Interface, ref ResourceRef) (workloadObject, []byte, error) {
	raw, err := availabilityRead(client, ref.Kind, ref.Namespace).Name(ref.Name).Do(ctx).Raw()
	if err != nil {
		return nil, nil, readError(ctx, err)
	}
	if len(raw) > detailLimit {
		return nil, nil, model.Fail("capacity", "Resource evidence exceeds the 2 MiB size limit")
	}
	object, err := decodeAvailability(ref.Kind, raw)
	if err != nil {
		return nil, nil, err
	}
	captured := ref
	captured.UID = string(object.GetUID())
	if object.GetName() != ref.Name || object.GetNamespace() != ref.Namespace || validResource(captured) != nil {
		return nil, nil, model.Fail("invalid", "Server did not return the requested availability identity")
	}
	if ref.UID != "" && ref.UID != captured.UID {
		return nil, nil, model.Fail("resource_replaced", "Resource replaced; select again")
	}
	return object, raw, nil
}

func (s *Service) ReadAvailability(ctx context.Context, req PageRequest) (AvailabilityPage, error) {
	version := availabilityVersion(req.Kind)
	if version == "" {
		return AvailabilityPage{}, model.Fail("invalid", "Unsupported availability kind")
	}
	if err := resourceScope(req.Kind, req.Namespace); err != nil {
		return AvailabilityPage{}, err
	}
	scope := pageCursor{Connection: req.Connection, Kind: req.Kind, Namespace: req.Namespace}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return AvailabilityPage{}, err
	}
	client, err := s.boundedReadClient(ctx, req.Connection)
	if err != nil {
		return AvailabilityPage{}, err
	}
	const pageSize = 100
	rows, meta, err := availabilityList(ctx, client, req, token, nil)
	if err != nil {
		return AvailabilityPage{}, err
	}
	next, err := encodeCursor(scope, meta.Continue)
	if err != nil {
		return AvailabilityPage{}, err
	}
	result := AvailabilityPage{Items: rows, Page: PageInfo{NextCursor: next, ResourceVersion: meta.ResourceVersion, FetchedAt: time.Now().UTC()}}
	if err := checkPayload(result); err != nil {
		return AvailabilityPage{}, err
	}
	if err := ctx.Err(); err != nil {
		return AvailabilityPage{}, safeError(ctx, err)
	}
	return result, s.connections.Validate(req.Connection)
}

func availabilitySummary(kind string, body storageFields) AvailabilitySummary {
	if kind == "ControllerRevision" {
		var revision *int64
		_ = json.Unmarshal(body["revision"], &revision)
		if revision != nil {
			return AvailabilitySummary{Revision: strconv.FormatInt(*revision, 10)}
		}
		return AvailabilitySummary{}
	}
	var generation, observed *int64
	_ = json.Unmarshal(body.fields("metadata")["generation"], &generation)
	_ = json.Unmarshal(body.fields("status")["observedGeneration"], &observed)
	observation := "unknown"
	if generation != nil && observed != nil {
		observation = "stale"
		if *generation == *observed {
			observation = "current"
		}
	}
	return AvailabilitySummary{Observation: observation}
}

func availabilityList(ctx context.Context, client kubernetes.Interface, req PageRequest, token string, include func(workloadObject) bool) ([]AvailabilityRow, metav1.ListMeta, error) {
	request := availabilityRead(client, req.Kind, req.Namespace).Param("limit", "100")
	if token != "" {
		request.Param("continue", token)
	}
	raw, err := request.Do(ctx).Raw()
	if err != nil {
		return nil, metav1.ListMeta{}, availabilityListError(ctx, err)
	}
	var list struct {
		metav1.TypeMeta
		Metadata metav1.ListMeta   `json:"metadata"`
		Items    []json.RawMessage `json:"items"`
	}
	version := availabilityVersion(req.Kind)
	if json.Unmarshal(raw, &list) != nil || (list.Kind != "" && list.Kind != req.Kind+"List") ||
		(list.APIVersion != "" && list.APIVersion != version) {
		return nil, metav1.ListMeta{}, model.Fail("invalid", "Server returned invalid availability inventory")
	}
	if len(list.Items) > 100 {
		return nil, metav1.ListMeta{}, model.Fail("capacity", "Server exceeded page size")
	}
	rows := make([]AvailabilityRow, 0, len(list.Items))
	for _, raw := range list.Items {
		object, err := decodeAvailability(req.Kind, raw)
		if err != nil {
			return nil, metav1.ListMeta{}, err
		}
		ref := ResourceRef{Connection: req.Connection, Kind: req.Kind, APIVersion: version, Namespace: object.GetNamespace(), Name: object.GetName(), UID: string(object.GetUID())}
		if ref.Namespace != req.Namespace || validResource(ref) != nil {
			return nil, metav1.ListMeta{}, model.Fail("invalid", "Server returned an invalid availability identity")
		}
		if include != nil && !include(object) {
			continue
		}
		var body storageFields
		if json.Unmarshal(raw, &body) != nil {
			return nil, metav1.ListMeta{}, model.Fail("invalid", "Invalid availability evidence")
		}
		spec, status := body.fields("spec"), body.fields("status")
		var projection any
		switch o := object.(type) {
		case *autoscaling.HorizontalPodAutoscaler:
			projection = map[string]any{"target": o.Spec.ScaleTargetRef, "min": spec["minReplicas"], "max": spec["maxReplicas"], "current": status["currentReplicas"], "desired": status["desiredReplicas"]}
		case *policy.PodDisruptionBudget:
			projection = map[string]any{"min": spec["minAvailable"], "max": spec["maxUnavailable"], "allowed": status["disruptionsAllowed"]}
		case *apps.ControllerRevision:
			projection = map[string]any{"owner": controllerOwner(o.OwnerReferences)}
		}
		payload, err := json.Marshal(projection)
		if err != nil {
			return nil, metav1.ListMeta{}, model.Fail("failed", "Cannot encode availability inventory")
		}
		var stamp *time.Time
		if created := object.GetCreationTimestamp(); !created.IsZero() {
			stamp = &created.Time
		}
		rows = append(rows, AvailabilityRow{Resource: ref, CreatedAt: stamp, ObjectJSON: string(payload), Summary: availabilitySummary(req.Kind, body)})
	}
	return rows, list.Metadata, nil
}

func controllerOwner(refs []metav1.OwnerReference) *metav1.OwnerReference {
	var owner *metav1.OwnerReference
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			if owner != nil {
				return nil
			}
			owner = &refs[i]
		}
	}
	return owner
}

func availabilityListError(ctx context.Context, err error) error {
	if apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) {
		return model.Fail("api_unavailable", "Availability API unavailable for this kind")
	}
	return readError(ctx, err)
}
