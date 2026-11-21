/*
Purpose: Capture the full object for an explicit YAML edit.
Used by: The reviewed edit service.
Responsibilities: Resolve advertised routes and check scope, version, and UID on bounded reads.
Important: This path runs only after a deliberate open and can read an authorized Secret.
*/
package kube

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
)

type EditTarget GenericResourceRef

type EditCapture struct {
	Target          EditTarget
	ResourceVersion string
	ObjectJSON      json.RawMessage
}

func (t EditTarget) api() apiResource {
	return apiResource{t.Group, t.Version, t.Resource, t.Kind, t.Namespace != ""}
}

func (t EditTarget) Ref() ResourceRef {
	return ResourceRef{Connection: t.Connection, APIVersion: t.api().apiVersion(), Kind: t.Kind, Namespace: t.Namespace, Name: t.Name, UID: t.UID}
}

// Path uses the identity returned by CaptureEdit, never a caller-supplied URL.
func (t EditTarget) Path() string {
	return t.api().path(t.Namespace, t.Name)
}

func (t EditTarget) ReadVersion(raw []byte) (string, error) {
	if len(raw) > detailLimit {
		return "", model.Fail("capacity", "Resource capture exceeds the 2 MiB size limit")
	}
	obj, err := objectIdentity(raw, t.api(), t.Namespace, false)
	if err != nil {
		return "", err
	}
	if obj.Metadata.Name != t.Name || string(obj.Metadata.UID) != t.UID {
		return "", model.Fail("resource_replaced", "Resource replaced; select it again")
	}
	if obj.Metadata.ResourceVersion == "" || len(obj.Metadata.ResourceVersion) > 128 {
		return "", model.Fail("invalid_response", "Resource has no usable version")
	}
	return obj.Metadata.ResourceVersion, nil
}

func (s *Service) CaptureEdit(ctx context.Context, target EditTarget) (EditCapture, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	address := ManifestAddress{target.Connection, target.Group, target.Version, target.Resource, target.Kind, target.Namespace, target.Name}
	if !address.valid() || target.UID == "" || len(target.UID) > 128 {
		return EditCapture{}, model.Fail("invalid", "Select an existing resource with a canonical API identity")
	}
	client, err := s.boundedReadClient(ctx, target.Connection)
	if err != nil {
		return EditCapture{}, err
	}
	address, err = resolveManifestRoute(ctx, client, address, "patch")
	if err != nil {
		return EditCapture{}, err
	}
	target.Resource = address.Resource
	raw, err := address.api().read(client, target.Namespace, target.Name).Do(ctx).Raw()
	if err != nil {
		return EditCapture{}, readError(ctx, err)
	}
	version, err := target.ReadVersion(raw)
	if err != nil {
		return EditCapture{}, err
	}
	return EditCapture{Target: target, ResourceVersion: version, ObjectJSON: raw}, nil
}

func RenderEditYAML(raw []byte) (string, error) {
	return exactYAML(raw)
}
