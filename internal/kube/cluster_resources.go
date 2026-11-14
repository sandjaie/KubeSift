/*
Purpose: Read fixed infrastructure APIs independently of discovery.
Used by: Cluster inventory, detail and Event target navigation.
Responsibilities: Validate scope and identity and project bounded inventory evidence.
Important: CRD schemas and APIService TLS settings are captured only in detail.
*/
package kube

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type apiResource struct {
	group, version, plural, kind string
	namespaced                   bool
}

func (a apiResource) apiVersion() string {
	if a.group == "" {
		return a.version
	}
	return a.group + "/" + a.version
}
func apiVersionPath(group, version string) string {
	if group == "" {
		return "/api/" + version
	}
	return "/apis/" + group + "/" + version
}
func (a apiResource) read(client kubernetes.Interface, ns, name string) *rest.Request {
	path := apiVersionPath(a.group, a.version)
	if a.namespaced {
		path += "/namespaces/" + ns
	}
	path += "/" + a.plural
	if name != "" {
		path += "/" + name
	}
	return fixedRead(client, path)
}
func fixedRead(client kubernetes.Interface, path string) *rest.Request {
	return client.CoreV1().RESTClient().Get().AbsPath(path).SetHeader("Accept", "application/json").MaxRetries(0)
}

var clusterResources = map[string]apiResource{
	"Node":                     {"", "v1", "nodes", "Node", false},
	"PriorityClass":            {"scheduling.k8s.io", "v1", "priorityclasses", "PriorityClass", false},
	"RuntimeClass":             {"node.k8s.io", "v1", "runtimeclasses", "RuntimeClass", false},
	"Lease":                    {"coordination.k8s.io", "v1", "leases", "Lease", true},
	"CustomResourceDefinition": {"apiextensions.k8s.io", "v1", "customresourcedefinitions", "CustomResourceDefinition", false},
	"APIService":               {"apiregistration.k8s.io", "v1", "apiservices", "APIService", false},
}
var modernEvents = apiResource{"events.k8s.io", "v1", "events", "Event", true}
var coreEvents = apiResource{"", "v1", "events", "Event", true}

func clusterAPI(kind, version string) apiResource {
	if kind == "Event" {
		if version == coreEvents.apiVersion() {
			return coreEvents
		}
		if version == modernEvents.apiVersion() {
			return modernEvents
		}
	}
	return clusterResources[kind]
}

type rawResource struct {
	metav1.TypeMeta
	Metadata metav1.ObjectMeta `json:"metadata"`
}
type rawList struct {
	metav1.TypeMeta
	Metadata metav1.ListMeta   `json:"metadata"`
	Items    []json.RawMessage `json:"items"`
}

func listJSON(ctx context.Context, request *rest.Request, api apiResource, token string, fixedKind bool) (rawList, error) {
	request.Param("limit", "100")
	if token != "" {
		request.Param("continue", token)
	}
	raw, err := request.Do(ctx).Raw()
	if err != nil {
		return rawList{}, err
	}
	var list rawList
	if json.Unmarshal(raw, &list) != nil || list.APIVersion != api.apiVersion() || (!discoveryKind.MatchString(list.Kind) || (fixedKind && list.Kind != api.kind+"List")) || list.Items == nil {
		return rawList{}, model.Fail("invalid", "Server returned an invalid resource inventory")
	}
	if len(list.Items) > 100 {
		return rawList{}, model.Fail("capacity", "Server exceeded page size")
	}
	return list, nil
}
func objectIdentity(raw []byte, api apiResource, ns string, listItem bool) (rawResource, error) {
	var obj rawResource
	if json.Unmarshal(raw, &obj) != nil || ((!listItem || obj.Kind != "") && obj.Kind != api.kind) || ((!listItem || obj.APIVersion != "") && obj.APIVersion != api.apiVersion()) ||
		obj.Metadata.Namespace != ns || len(obj.Metadata.UID) == 0 || len(obj.Metadata.UID) > 128 {
		return rawResource{}, model.Fail("invalid", "Server returned an invalid resource identity")
	}
	return obj, nil
}
func getClusterResource(ctx context.Context, client kubernetes.Interface, ref ResourceRef) (workloadObject, []byte, error) {
	api := clusterAPI(ref.Kind, ref.APIVersion)
	raw, err := api.read(client, ref.Namespace, ref.Name).Do(ctx).Raw()
	if err != nil {
		return nil, nil, readError(ctx, err)
	}
	if len(raw) > detailLimit {
		return nil, nil, model.Fail("capacity", "Resource evidence exceeds the 2 MiB size limit")
	}
	obj, err := objectIdentity(raw, api, ref.Namespace, false)
	if err != nil {
		return nil, nil, err
	}
	if obj.Metadata.Name != ref.Name {
		return nil, nil, model.Fail("invalid", "Server did not return the requested resource name")
	}
	if string(obj.Metadata.UID) != ref.UID {
		return nil, nil, model.Fail("resource_replaced", "Resource replaced; select again")
	}
	// Only the capture is serialized; this object provides metadata to the detail pipeline.
	return &metav1.PartialObjectMetadata{TypeMeta: obj.TypeMeta, ObjectMeta: obj.Metadata}, raw, nil
}
func pageInfo(scope pageCursor, meta metav1.ListMeta) (PageInfo, error) {
	next, err := encodeCursor(scope, meta.Continue)
	return PageInfo{NextCursor: next, ResourceVersion: meta.ResourceVersion, FetchedAt: time.Now().UTC()}, err
}
func (s *Service) ReadClusterResources(ctx context.Context, req PageRequest) (ClusterResourcePage, error) {
	api, ok := clusterResources[req.Kind]
	if !ok {
		return ClusterResourcePage{}, model.Fail("invalid", "Unsupported cluster resource kind")
	}
	if err := resourceScope(req.Kind, req.Namespace); err != nil {
		return ClusterResourcePage{}, err
	}
	scope := pageCursor{Connection: req.Connection, Kind: req.Kind, Namespace: req.Namespace}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return ClusterResourcePage{}, err
	}
	client, err := s.boundedReadClient(ctx, req.Connection)
	if err != nil {
		return ClusterResourcePage{}, err
	}
	list, err := listJSON(ctx, api.read(client, req.Namespace, ""), api, token, true)
	if err != nil {
		return ClusterResourcePage{}, resourceListError(ctx, err)
	}
	page, err := pageInfo(scope, list.Metadata)
	if err != nil {
		return ClusterResourcePage{}, err
	}
	result := ClusterResourcePage{Items: make([]ClusterResourceRow, 0, len(list.Items)), Page: page}
	for _, raw := range list.Items {
		obj, err := objectIdentity(raw, api, req.Namespace, true)
		if err != nil {
			return ClusterResourcePage{}, err
		}
		ref := resourceRef(req.Connection, req.Kind, api.apiVersion(), obj.Metadata)
		if err := validResource(ref); err != nil {
			return ClusterResourcePage{}, err
		}
		evidence, err := clusterSummary(req.Kind, raw)
		if err != nil {
			return ClusterResourcePage{}, err
		}
		result.Items = append(result.Items, ClusterResourceRow{Resource: ref, CreatedAt: created(obj.Metadata), ObjectJSON: string(evidence)})
	}
	if err := checkPayload(result); err != nil {
		return ClusterResourcePage{}, err
	}
	return result, s.connections.Validate(req.Connection)
}

func clusterSummary(kind string, raw []byte) ([]byte, error) {
	var body storageFields
	if json.Unmarshal(raw, &body) != nil {
		return nil, model.Fail("invalid", "Invalid cluster evidence")
	}
	spec, status := body.fields("spec"), body.fields("status")
	selectFields := func(source storageFields, keys ...string) map[string]any {
		out := map[string]any{}
		for _, key := range keys {
			if value, ok := source[key]; ok {
				out[key] = value
			}
		}
		return out
	}
	var evidence map[string]any
	switch kind {
	case "Node":
		evidence = selectFields(spec, "unschedulable")
		evidence["kubeletVersion"] = status.fields("nodeInfo")["kubeletVersion"]
		evidence["ready"] = conditionStatus(status, "Ready")
	case "PriorityClass":
		evidence = selectFields(body, "globalDefault", "preemptionPolicy")
		var value *int32
		if field, ok := body["value"]; ok {
			if json.Unmarshal(field, &value) != nil {
				return nil, model.Fail("invalid", "Invalid priority value")
			}
			if value != nil {
				evidence["value"] = strconv.FormatInt(int64(*value), 10)
			}
		}
	case "RuntimeClass":
		evidence = selectFields(body, "handler")
	case "Lease":
		evidence = selectFields(spec, "holderIdentity", "renewTime", "leaseDurationSeconds")
	case "CustomResourceDefinition":
		evidence = selectFields(spec, "group", "scope")
		evidence["kind"] = spec.fields("names")["kind"]
		evidence["established"] = conditionStatus(status, "Established")
	case "APIService":
		evidence = selectFields(spec, "group", "version")
		evidence["service"] = selectFields(spec.fields("service"), "name", "namespace", "port")
		evidence["available"] = conditionStatus(status, "Available")
	}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return nil, model.Fail("invalid", "Invalid cluster evidence")
	}
	return encoded, nil
}
func conditionStatus(fields storageFields, name string) string {
	var conditions []struct{ Type, Status string }
	if json.Unmarshal(fields["conditions"], &conditions) == nil {
		for _, condition := range conditions {
			if condition.Type == name && (condition.Status == "True" || condition.Status == "False" || condition.Status == "Unknown") {
				return condition.Status
			}
		}
	}
	return "Unknown"
}

func resourceListError(ctx context.Context, err error) error {
	if apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) {
		return model.Fail("api_unavailable", "The selected Kubernetes API is unavailable")
	}
	return readError(ctx, err)
}
