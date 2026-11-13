/*
Purpose: Capture UID-checked workload overview and read-only YAML.
Used by: Investigation sections and other resource-scoped readers.
Responsibilities: Validate kind and identity, cap bridge payloads and serialize one captured version.
Important: Workload values stay local; referenced Secrets and ConfigMaps are never fetched.
*/
package kube

import (
	"context"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
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

func validResource(ref ResourceRef) error {
	if err := target(model.Target{Namespace: ref.Namespace, Name: ref.Name}, false); err != nil {
		return err
	}
	if ref.UID == "" || len(ref.UID) > 128 || !(ref.Kind == "Pod" && ref.APIVersion == "v1" || (ref.Kind == "Deployment" || ref.Kind == "ReplicaSet") && ref.APIVersion == "apps/v1") {
		return model.Fail("invalid", "Explicit workload identity required")
	}
	return nil
}
func checkedObject(ctx context.Context, client kubernetes.Interface, ref ResourceRef) (workloadObject, error) {
	if err := validResource(ref); err != nil {
		return nil, err
	}
	var object workloadObject
	var err error
	if ref.Kind == "Pod" {
		object, err = client.CoreV1().Pods(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	} else if ref.Kind == "ReplicaSet" {
		object, err = client.AppsV1().ReplicaSets(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	} else {
		object, err = client.AppsV1().Deployments(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, readError(ctx, err)
	}
	if string(object.GetUID()) != ref.UID {
		return nil, model.Fail("resource_replaced", "Resource replaced; select again")
	}
	object.GetObjectKind().SetGroupVersionKind(schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind))
	return object, nil
}
func checkPayload(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return model.Fail("failed", "Cannot encode workload evidence")
	}
	if len(raw) > detailLimit {
		return model.Fail("capacity", "Workload evidence exceeds the 2 MiB size limit")
	}
	return nil
}
func (s *Service) ReadResource(ctx context.Context, req DetailRequest) (ResourceDetail, error) {
	result := ResourceDetail{}
	client, err := s.boundedReadClient(ctx, req.Resource.Connection)
	if err != nil {
		return result, err
	}
	object, err := checkedObject(ctx, client, req.Resource)
	if err != nil {
		return result, err
	}
	if err = checkPayload(object); err != nil {
		return result, err
	}
	raw, err := json.Marshal(object)
	if err != nil {
		return result, model.Fail("failed", "Cannot encode workload")
	}
	result = ResourceDetail{Resource: req.Resource, ResourceVersion: object.GetResourceVersion(), FetchedAt: time.Now().UTC(), ObjectJSON: string(raw)}
	if err = checkPayload(result); err != nil {
		return ResourceDetail{}, err
	}
	return result, s.connections.Validate(req.Resource.Connection)
}
func (s *Service) ReadResourceYAML(ctx context.Context, req DetailRequest) (YAMLDocument, error) {
	result := YAMLDocument{}
	client, err := s.boundedReadClient(ctx, req.Resource.Connection)
	if err != nil {
		return result, err
	}
	object, err := checkedObject(ctx, client, req.Resource)
	if err != nil {
		return result, err
	}
	if err = checkPayload(object); err != nil {
		return result, err
	}
	raw, err := json.Marshal(object)
	if err != nil {
		return result, model.Fail("failed", "Cannot encode workload")
	}
	with, err := yaml.JSONToYAML(raw)
	if err != nil {
		return result, model.Fail("failed", "Cannot serialize workload YAML")
	}
	copy := object.DeepCopyObject().(workloadObject)
	copy.SetManagedFields(nil)
	raw, err = json.Marshal(copy)
	if err != nil {
		return result, model.Fail("failed", "Cannot encode workload")
	}
	without, err := yaml.JSONToYAML(raw)
	if err != nil {
		return result, model.Fail("failed", "Cannot serialize workload YAML")
	}
	result = YAMLDocument{Resource: req.Resource, ResourceVersion: object.GetResourceVersion(), FetchedAt: time.Now().UTC(), WithoutManagedFields: string(without), WithManagedFields: string(with)}
	if err = checkPayload(result); err != nil {
		return YAMLDocument{}, err
	}
	return result, s.connections.Validate(req.Resource.Connection)
}
