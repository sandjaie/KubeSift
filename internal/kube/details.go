/*
Purpose: Capture UID-checked resource overview and read-only YAML.
Used by: Investigation sections and other resource-scoped readers.
Responsibilities: Validate kind and identity, cap bridge payloads and serialize one captured version.
Important: Secret identity uses a separate partial metadata reader; generic reads never fetch a full Secret.
*/
package kube

import (
	"context"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
	"time"
)

type workloadObject interface {
	metav1.Object
	runtime.Object
}

func workloadVersion(kind string) string {
	switch kind {
	case "Pod", "ReplicationController":
		return "v1"
	case "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet":
		return "apps/v1"
	case "Job", "CronJob":
		return "batch/v1"
	default:
		return ""
	}
}

func validResource(ref ResourceRef) error {
	if ref.UID == "" {
		return model.Fail("invalid", "Explicit resource UID required")
	}
	return validResourceAddress(ref)
}
func validResourceAddress(ref ResourceRef) error {
	if err := resourceScope(ref.Kind, ref.Namespace); err != nil {
		return err
	}
	version := workloadVersion(ref.Kind)
	if version == "" {
		version = networkVersion(ref.Kind)
	}
	if version == "" {
		version = storageVersion(ref.Kind)
	}
	if version == "" {
		version = accessControlVersion(ref.Kind)
	}
	if version == "" {
		version = availabilityVersion(ref.Kind)
	}
	if version == "" {
		version = clusterAPI(ref.Kind, ref.APIVersion).apiVersion()
	}
	if version == "" && configurationKind(ref.Kind) {
		version = "v1"
	}
	if !validResourceName(ref.Kind, ref.Name) || len(ref.UID) > 128 || version == "" || version != ref.APIVersion {
		return model.Fail("invalid", "Explicit resource identity required")
	}
	return nil
}
func checkedObject(ctx context.Context, client kubernetes.Interface, ref ResourceRef) (workloadObject, error) {
	if err := validResource(ref); err != nil {
		return nil, err
	}
	return getResource(ctx, client, ref)
}
func getResource(ctx context.Context, client kubernetes.Interface, ref ResourceRef) (workloadObject, error) {
	if clusterAPI(ref.Kind, ref.APIVersion).kind != "" {
		object, _, err := getClusterResource(ctx, client, ref)
		return object, err
	}
	if storageVersion(ref.Kind) != "" {
		object, _, err := getStorage(ctx, client, ref)
		return object, err
	}

	if availabilityVersion(ref.Kind) != "" {
		object, _, err := getAvailability(ctx, client, ref)
		return object, err
	}
	var object workloadObject
	var err error
	switch ref.Kind {
	case "ServiceAccount":
		object, err = client.CoreV1().ServiceAccounts(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "Role":
		object, err = client.RbacV1().Roles(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "RoleBinding":
		object, err = client.RbacV1().RoleBindings(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "ClusterRole":
		object, err = client.RbacV1().ClusterRoles().Get(ctx, ref.Name, metav1.GetOptions{})
	case "ClusterRoleBinding":
		object, err = client.RbacV1().ClusterRoleBindings().Get(ctx, ref.Name, metav1.GetOptions{})
	case "ReplicationController":
		object, err = client.CoreV1().ReplicationControllers(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "Pod":
		object, err = client.CoreV1().Pods(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "Deployment":
		object, err = client.AppsV1().Deployments(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "ReplicaSet":
		object, err = client.AppsV1().ReplicaSets(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "StatefulSet":
		object, err = client.AppsV1().StatefulSets(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "DaemonSet":
		object, err = client.AppsV1().DaemonSets(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "Job":
		object, err = client.BatchV1().Jobs(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "CronJob":
		object, err = client.BatchV1().CronJobs(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "NetworkPolicy":
		object, err = client.NetworkingV1().NetworkPolicies(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "IngressClass":
		object, err = client.NetworkingV1().IngressClasses().Get(ctx, ref.Name, metav1.GetOptions{})
	case "Ingress":
		object, err = client.NetworkingV1().Ingresses(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "EndpointSlice":
		object, err = client.DiscoveryV1().EndpointSlices(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "Namespace":
		object, err = client.CoreV1().Namespaces().Get(ctx, ref.Name, metav1.GetOptions{})
	case "ResourceQuota":
		object, err = client.CoreV1().ResourceQuotas(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "LimitRange":
		object, err = client.CoreV1().LimitRanges(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "ConfigMap":
		object, err = client.CoreV1().ConfigMaps(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "Service":
		object, err = client.CoreV1().Services(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	default:
		return nil, model.Fail("invalid", "Generic detail is unavailable for this kind")
	}

	if err != nil {
		return nil, readError(ctx, err)
	}
	if accessControlVersion(ref.Kind) != "" {
		expected := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)
		if actual := object.GetObjectKind().GroupVersionKind(); !actual.Empty() && actual != expected {
			return nil, model.Fail("invalid", "Server returned an unexpected access control kind or API")
		}
		captured := ref
		captured.UID = string(object.GetUID())
		if validResource(captured) != nil || object.GetName() != ref.Name || object.GetNamespace() != ref.Namespace {
			return nil, model.Fail("invalid", "Server did not return the requested access control identity")
		}
	}
	if (networkVersion(ref.Kind) != "" || configurationKind(ref.Kind) || ref.Kind == "ReplicationController") && (object.GetName() != ref.Name || object.GetNamespace() != ref.Namespace || object.GetUID() == "") {
		return nil, model.Fail("invalid", "Server did not return the requested resource identity")
	}
	if ref.UID != "" && string(object.GetUID()) != ref.UID {
		return nil, model.Fail("resource_replaced", "Resource replaced; select again")
	}
	object.GetObjectKind().SetGroupVersionKind(schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind))
	return object, nil
}
func checkPayload(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return model.Fail("failed", "Cannot encode resource evidence")
	}
	if len(raw) > detailLimit {
		return model.Fail("capacity", "Resource evidence exceeds the 2 MiB size limit")
	}
	return nil
}
func (s *Service) ReadResource(ctx context.Context, req DetailRequest) (ResourceDetail, error) {
	result := ResourceDetail{}
	if req.Resource.Kind == "Secret" {
		if err := validResource(req.Resource); err != nil {
			return result, err
		}
		return s.readSecretMetadata(ctx, req.Resource)
	}
	if err := validResource(req.Resource); err != nil {
		return result, err
	}
	client, err := s.boundedReadClient(ctx, req.Resource.Connection)
	if err != nil {
		return result, err
	}
	result, err = s.readOverview(ctx, client, req)
	if err != nil {
		return ResourceDetail{}, err
	}
	if ctx.Err() != nil {
		return ResourceDetail{}, safeError(ctx, ctx.Err())
	}
	return result, s.connections.Validate(req.Resource.Connection)
}

func (s *Service) readOverview(ctx context.Context, client kubernetes.Interface, req DetailRequest) (ResourceDetail, error) {
	result := ResourceDetail{}
	if req.Resource.Kind == "ResourceQuota" || req.Resource.Kind == "LimitRange" {
		return s.readPolicyOverview(ctx, client, req.Resource)
	}
	object, captured, err := detailObject(ctx, client, req.Resource)
	if err != nil {
		return result, err
	}
	if object.GetName() != req.Resource.Name || object.GetNamespace() != req.Resource.Namespace {
		return result, model.Fail("invalid", "Server did not return the requested resource identity")
	}
	if err = checkPayload(object); err != nil {
		return result, err
	}
	if configMap, ok := object.(*core.ConfigMap); ok && !req.IncludeConfigMapData {
		raw, e := json.Marshal(map[string]any{
			"metadata": configMap.ObjectMeta, "immutable": configMap.Immutable,
			"textKeys": len(configMap.Data), "binaryKeys": len(configMap.BinaryData),
		})
		if e != nil {
			return result, model.Fail("failed", "Cannot encode ConfigMap overview")
		}
		result = ResourceDetail{Resource: req.Resource, ResourceVersion: object.GetResourceVersion(), FetchedAt: time.Now().UTC(), ObjectJSON: string(raw)}
		if e = checkPayload(result); e != nil {
			return ResourceDetail{}, e
		}
		return result, nil
	}
	result, err = capturedDetail(req.Resource, object, captured)
	if err != nil {
		return ResourceDetail{}, err
	}
	return result, nil
}

func capturedDetail(ref ResourceRef, object workloadObject, raw []byte) (ResourceDetail, error) {
	if raw == nil {
		var err error
		raw, err = json.Marshal(object)
		if err != nil {
			return ResourceDetail{}, model.Fail("failed", "Cannot encode resource")
		}
	}
	result := ResourceDetail{Resource: ref, ResourceVersion: object.GetResourceVersion(), FetchedAt: time.Now().UTC(), ObjectJSON: string(raw)}
	if job, ok := object.(*batch.Job); ok {
		result.Job = summarizeJob(job)
	}
	if availabilityVersion(ref.Kind) != "" {
		var body storageFields
		if json.Unmarshal(raw, &body) != nil {
			return ResourceDetail{}, model.Fail("invalid", "Invalid availability evidence")
		}
		summary := availabilitySummary(ref.Kind, body)
		result.Availability = &summary
	}
	if ref.Kind == "Event" {
		_, regarding, err := eventEvidence(raw, clusterAPI("Event", ref.APIVersion))
		if err != nil {
			return ResourceDetail{}, err
		}
		result.EventTarget = supportedEventTarget(ref, regarding)
	}
	if clusterAPI(ref.Kind, ref.APIVersion).kind != "" {
		doc, err := resourceYAMLDocument(ref, object, raw)
		if err != nil {
			return ResourceDetail{}, err
		}
		doc.FetchedAt = result.FetchedAt
		result.CapturedYAML = &doc
	}
	if err := checkPayload(result); err != nil {
		return ResourceDetail{}, err
	}
	return result, nil
}

func detailYAMLJSON(ref ResourceRef, object workloadObject, captured []byte, managed bool) ([]byte, error) {
	if availabilityVersion(ref.Kind) == "" && clusterAPI(ref.Kind, ref.APIVersion).kind == "" {
		return storageYAMLJSON(object, captured)
	}
	if managed {
		return captured, nil
	}
	var body storageFields
	if err := json.Unmarshal(captured, &body); err != nil {
		return nil, err
	}
	meta := body.fields("metadata")
	delete(meta, "managedFields")
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	body["metadata"] = raw
	return json.Marshal(body)
}

func (s *Service) ReadResourceYAML(ctx context.Context, req DetailRequest) (YAMLDocument, error) {
	result := YAMLDocument{}
	if req.Resource.Kind == "Secret" {
		return result, model.Fail("invalid", "Secret YAML is unavailable")
	}
	if err := validResource(req.Resource); err != nil {
		return result, err
	}
	client, err := s.boundedReadClient(ctx, req.Resource.Connection)
	if err != nil {
		return result, err
	}
	object, captured, err := detailObject(ctx, client, req.Resource)
	if err != nil {
		return result, err
	}
	if err = checkPayload(object); err != nil {
		return result, err
	}
	result, err = resourceYAMLDocument(req.Resource, object, captured)
	if err != nil {
		return YAMLDocument{}, err
	}
	return result, s.connections.Validate(req.Resource.Connection)
}

func resourceYAMLDocument(ref ResourceRef, object workloadObject, captured []byte) (YAMLDocument, error) {
	raw, err := detailYAMLJSON(ref, object, captured, true)
	if err != nil {
		return YAMLDocument{}, model.Fail("failed", "Cannot encode resource")
	}
	render := func(raw []byte) (string, error) {
		if clusterAPI(ref.Kind, ref.APIVersion).kind != "" {
			return exactYAML(raw)
		}
		encoded, err := yaml.JSONToYAML(raw)
		if err != nil {
			return "", model.Fail("failed", "Cannot serialize resource YAML")
		}
		return string(encoded), nil
	}
	with, err := render(raw)
	if err != nil {
		return YAMLDocument{}, err
	}
	copy := object.DeepCopyObject().(workloadObject)
	copy.SetManagedFields(nil)
	raw, err = detailYAMLJSON(ref, copy, captured, false)
	if err != nil {
		return YAMLDocument{}, model.Fail("failed", "Cannot encode resource")
	}
	without, err := render(raw)
	if err != nil {
		return YAMLDocument{}, err
	}
	result := YAMLDocument{Resource: ref, ResourceVersion: object.GetResourceVersion(), FetchedAt: time.Now().UTC(), WithoutManagedFields: without, WithManagedFields: with}
	if err := checkPayload(result); err != nil {
		return YAMLDocument{}, err
	}
	return result, nil
}

func detailObject(ctx context.Context, client kubernetes.Interface, ref ResourceRef) (workloadObject, []byte, error) {
	if clusterAPI(ref.Kind, ref.APIVersion).kind != "" {
		return getClusterResource(ctx, client, ref)
	}
	if storageVersion(ref.Kind) != "" {
		return getStorage(ctx, client, ref)
	}
	if availabilityVersion(ref.Kind) != "" {
		return getAvailability(ctx, client, ref)
	}
	object, err := checkedObject(ctx, client, ref)
	return object, nil, err
}
