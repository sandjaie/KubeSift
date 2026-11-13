/*
Purpose: Read the six allowlisted storage inventories and resource snapshots.
Used by: Storage workspace, PVC Events and storage reference resolution.
Responsibilities: Bind cursors to scope, check identity and project bounded inventory evidence.
Important: Capture JSON before typed decoding can canonicalize quantities or invent missing status.
*/
package kube

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type storageResource struct {
	version, plural string
	newObject       func() workloadObject
}

var storageResources = map[string]storageResource{
	"PersistentVolumeClaim": {"v1", "persistentvolumeclaims", func() workloadObject { return &core.PersistentVolumeClaim{} }},
	"PersistentVolume":      {"v1", "persistentvolumes", func() workloadObject { return &core.PersistentVolume{} }},
	"StorageClass":          {"storage.k8s.io/v1", "storageclasses", func() workloadObject { return &storage.StorageClass{} }},
	"VolumeAttachment":      {"storage.k8s.io/v1", "volumeattachments", func() workloadObject { return &storage.VolumeAttachment{} }},
	"CSIDriver":             {"storage.k8s.io/v1", "csidrivers", func() workloadObject { return &storage.CSIDriver{} }},
	"CSINode":               {"storage.k8s.io/v1", "csinodes", func() workloadObject { return &storage.CSINode{} }},
}

func storageVersion(kind string) string { return storageResources[kind].version }

func storageRead(client kubernetes.Interface, kind, namespace string) *rest.Request {
	resource := storageResources[kind]
	transport := client.CoreV1().RESTClient()
	if resource.version != "v1" {
		transport = client.StorageV1().RESTClient()
	}
	request := transport.Get().Resource(resource.plural).SetHeader("Accept", "application/json").MaxRetries(0)
	if kind == "PersistentVolumeClaim" {
		request.Namespace(namespace)
	}
	return request
}

func decodeStorage(kind string, raw []byte) (workloadObject, error) {
	resource := storageResources[kind]
	object := resource.newObject()
	if json.Unmarshal(raw, object) != nil {
		return nil, model.Fail("invalid", "Server returned invalid storage evidence")
	}
	expected := schema.FromAPIVersionAndKind(resource.version, kind)
	returned := object.GetObjectKind().GroupVersionKind()
	if !returned.Empty() && returned != expected {
		return nil, model.Fail("invalid", "Server returned an unexpected storage kind or API")
	}
	object.GetObjectKind().SetGroupVersionKind(expected)
	return object, nil
}

func getStorage(ctx context.Context, client kubernetes.Interface, ref ResourceRef) (workloadObject, []byte, error) {
	raw, err := storageRead(client, ref.Kind, ref.Namespace).Name(ref.Name).Do(ctx).Raw()
	if err != nil {
		return nil, nil, readError(ctx, err)
	}
	if len(raw) > detailLimit {
		return nil, nil, model.Fail("capacity", "Resource evidence exceeds the 2 MiB size limit")
	}
	object, err := decodeStorage(ref.Kind, raw)
	if err != nil {
		return nil, nil, err
	}
	captured := ref
	captured.UID = string(object.GetUID())
	if object.GetName() != ref.Name || object.GetNamespace() != ref.Namespace || validResource(captured) != nil {
		return nil, nil, model.Fail("invalid", "Server did not return the requested storage identity")
	}
	if ref.UID != "" && ref.UID != captured.UID {
		return nil, nil, model.Fail("resource_replaced", "Resource replaced; select again")
	}
	return object, raw, nil
}

type storageFields map[string]json.RawMessage

func (f storageFields) fields(key string) storageFields {
	var fields storageFields
	_ = json.Unmarshal(f[key], &fields)
	return fields
}

func (f storageFields) selectFields(keys ...string) storageFields {
	selected := make(storageFields, len(keys))
	for _, key := range keys {
		if value, exists := f[key]; exists {
			selected[key] = value
		}
	}
	return selected
}

func projectStorage(kind string, object workloadObject, raw []byte) ([]byte, error) {
	var body storageFields
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	spec, status := body.fields("spec"), body.fields("status")
	var summary map[string]any
	switch kind {
	case "PersistentVolumeClaim":
		summary = map[string]any{
			"phase": status["phase"], "requested": spec.fields("resources").fields("requests")["storage"],
			"capacity": status.fields("capacity")["storage"], "class": spec["storageClassName"], "volume": spec["volumeName"],
		}
		// Keep the class field absent when there is no supplied value.
		if spec["storageClassName"] == nil {
			delete(summary, "class")
		}
	case "PersistentVolume":
		claim := spec.fields("claimRef")
		summary = map[string]any{
			"phase": status["phase"], "capacity": spec.fields("capacity")["storage"], "accessModes": spec["accessModes"],
			"reclaimPolicy": spec["persistentVolumeReclaimPolicy"], "class": spec["storageClassName"], "claim": claim.selectFields("namespace", "name"),
		}
	case "StorageClass":
		annotations := body.fields("metadata").fields("annotations")
		summary = map[string]any{
			"provisioner": body["provisioner"], "defaultClass": annotations["storageclass.kubernetes.io/is-default-class"],
			"legacyDefault": annotations["storageclass.beta.kubernetes.io/is-default-class"], "bindingMode": body["volumeBindingMode"],
			"reclaimPolicy": body["reclaimPolicy"], "expansion": body["allowVolumeExpansion"],
		}
	case "VolumeAttachment":
		source := object.(*storage.VolumeAttachment).Spec.Source
		fields := map[string]any{"attacher": spec["attacher"], "node": spec["nodeName"], "volume": source.PersistentVolumeName, "inline": source.InlineVolumeSpec != nil}
		if attached := status["attached"]; attached != nil {
			fields["attached"] = attached
		}
		summary = fields
	case "CSIDriver":
		summary = map[string]any{"attachRequired": spec["attachRequired"], "volumeLifecycleModes": spec["volumeLifecycleModes"]}
	case "CSINode":
		summary = map[string]any{"drivers": len(object.(*storage.CSINode).Spec.Drivers)}
	}
	return json.Marshal(summary)
}

func (s *Service) ReadStorage(ctx context.Context, req PageRequest) (StoragePage, error) {
	version := storageVersion(req.Kind)
	if version == "" {
		return StoragePage{}, model.Fail("invalid", "Unsupported storage kind")
	}
	if err := resourceScope(req.Kind, req.Namespace); err != nil {
		return StoragePage{}, err
	}
	scope := pageCursor{Connection: req.Connection, Kind: req.Kind, Namespace: req.Namespace}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return StoragePage{}, err
	}
	client, err := s.boundedReadClient(ctx, req.Connection)
	if err != nil {
		return StoragePage{}, err
	}
	const pageSize = 100
	request := storageRead(client, req.Kind, req.Namespace).Param("limit", strconv.Itoa(pageSize))
	if token != "" {
		request.Param("continue", token)
	}
	raw, err := request.Do(ctx).Raw()
	if apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) {
		return StoragePage{}, model.Fail("api_unavailable", "Storage API unavailable for this kind")
	}
	if err != nil {
		return StoragePage{}, readError(ctx, err)
	}
	var list struct {
		metav1.TypeMeta
		Metadata metav1.ListMeta   `json:"metadata"`
		Items    []json.RawMessage `json:"items"`
	}
	if json.Unmarshal(raw, &list) != nil ||
		(list.Kind != "" && list.Kind != req.Kind+"List") || (list.APIVersion != "" && list.APIVersion != version) {
		return StoragePage{}, model.Fail("invalid", "Server returned an invalid storage list")
	}
	if len(list.Items) > pageSize {
		return StoragePage{}, model.Fail("capacity", "Server exceeded page size")
	}
	rows := make([]StorageRow, 0, len(list.Items))
	for _, item := range list.Items {
		object, err := decodeStorage(req.Kind, item)
		if err != nil {
			return StoragePage{}, err
		}
		ref := ResourceRef{Connection: req.Connection, Kind: req.Kind, APIVersion: version, Name: object.GetName(), Namespace: object.GetNamespace(), UID: string(object.GetUID())}
		if ref.Namespace != req.Namespace || validResource(ref) != nil {
			return StoragePage{}, model.Fail("invalid", "Server returned an invalid storage identity")
		}
		projection, err := projectStorage(req.Kind, object, item)
		if err != nil {
			return StoragePage{}, model.Fail("failed", "Cannot encode storage inventory")
		}
		var created *time.Time
		if stamp := object.GetCreationTimestamp(); !stamp.IsZero() {
			created = &stamp.Time
		}
		rows = append(rows, StorageRow{Resource: ref, CreatedAt: created, ObjectJSON: string(projection)})
	}
	next, err := encodeCursor(scope, list.Metadata.Continue)
	if err != nil {
		return StoragePage{}, err
	}
	result := StoragePage{Items: rows, Page: PageInfo{NextCursor: next, ResourceVersion: list.Metadata.ResourceVersion, FetchedAt: time.Now().UTC()}}
	if err := checkPayload(result); err != nil {
		return StoragePage{}, err
	}
	if err := ctx.Err(); err != nil {
		return StoragePage{}, safeError(ctx, err)
	}
	return result, s.connections.Validate(req.Connection)
}

// Typed YAML can omit unknown fields, but must not invent attachment state.
func storageYAMLJSON(object workloadObject, captured []byte) ([]byte, error) {
	typed, err := json.Marshal(object)
	if err != nil || object.GetObjectKind().GroupVersionKind().Kind != "VolumeAttachment" {
		return typed, err
	}
	var source, output storageFields
	if err := json.Unmarshal(captured, &source); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(typed, &output); err != nil {
		return nil, err
	}
	if status := source.fields("status"); status == nil {
		delete(output, "status")
	} else if status["attached"] == nil || string(status["attached"]) == "null" {
		fields := output.fields("status")
		delete(fields, "attached")
		output["status"], err = json.Marshal(fields)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(output)
}
