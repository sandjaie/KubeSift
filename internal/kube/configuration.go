/*
Purpose: Read a bounded inventory of namespace policy and configuration.
Used by: Explorer configuration views.
Responsibilities: Route typed lists, project approved fields and bind continuations to scope.
Important: ConfigMap values do not enter inventory DTOs; Secret reads use separate metadata transport.
*/
package kube

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func configurationKind(kind string) bool {
	switch kind {
	case "Namespace", "ResourceQuota", "LimitRange", "ConfigMap", "Secret":
		return true
	}
	return false
}

func configurationProjection(object workloadObject) any {
	switch o := object.(type) {
	case *core.Namespace:
		return map[string]any{"phase": o.Status.Phase}
	case *core.ResourceQuota:
		return map[string]any{"resources": len(o.Spec.Hard), "scopes": o.Spec.Scopes, "scopeSelector": o.Spec.ScopeSelector}
	case *core.LimitRange:
		types := make([]core.LimitType, 0, len(o.Spec.Limits))
		for _, item := range o.Spec.Limits {
			types = append(types, item.Type)
		}
		return map[string]any{"types": types}
	case *core.ConfigMap:
		return map[string]any{"textKeys": len(o.Data), "binaryKeys": len(o.BinaryData), "immutable": o.Immutable}
	}
	return struct{}{}
}

func configurationRows[T any, P interface {
	*T
	workloadObject
}](req PageRequest, items []T) ([]ConfigurationRow, error) {
	if len(items) > 100 {
		return nil, model.Fail("capacity", "Server exceeded page size")
	}
	rows := make([]ConfigurationRow, 0, len(items))
	for i := range items {
		object := P(&items[i])
		ref := ResourceRef{Connection: req.Connection, Kind: req.Kind, APIVersion: "v1", Name: object.GetName(), Namespace: object.GetNamespace(), UID: string(object.GetUID())}
		if ref.Namespace != req.Namespace || validResource(ref) != nil {
			return nil, model.Fail("invalid", "Server returned an invalid configuration identity")
		}
		raw, err := json.Marshal(configurationProjection(object))
		if err != nil {
			return nil, model.Fail("failed", "Cannot encode configuration inventory")
		}
		var stamp *time.Time
		if at := object.GetCreationTimestamp(); !at.IsZero() {
			stamp = &at.Time
		}
		rows = append(rows, ConfigurationRow{Resource: ref, CreatedAt: stamp, ObjectJSON: string(raw)})
	}
	return rows, nil
}

func (s *Service) configurationPage(ctx context.Context, scope pageCursor, meta metav1.ListMeta, rows []ConfigurationRow) (ConfigurationPage, error) {
	next, err := encodeCursor(scope, meta.Continue)
	if err != nil {
		return ConfigurationPage{}, err
	}
	page := ConfigurationPage{Items: rows, Page: PageInfo{NextCursor: next, ResourceVersion: meta.ResourceVersion, FetchedAt: time.Now().UTC()}}
	if err = checkPayload(page); err != nil {
		return ConfigurationPage{}, err
	}
	if err = ctx.Err(); err != nil {
		return ConfigurationPage{}, safeError(ctx, err)
	}
	return page, s.connections.Validate(scope.Connection)
}

func (s *Service) ReadConfiguration(ctx context.Context, req PageRequest) (ConfigurationPage, error) {
	if !configurationKind(req.Kind) {
		return ConfigurationPage{}, model.Fail("invalid", "Unsupported configuration kind")
	}
	if err := resourceScope(req.Kind, req.Namespace); err != nil {
		return ConfigurationPage{}, err
	}
	scope := pageCursor{Connection: req.Connection, Kind: req.Kind, Namespace: req.Namespace}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return ConfigurationPage{}, err
	}
	if req.Kind == "Secret" {
		return s.listSecretMetadata(ctx, req, scope, token)
	}
	client, err := s.boundedReadClient(ctx, req.Connection)
	if err != nil {
		return ConfigurationPage{}, err
	}
	opts := metav1.ListOptions{Limit: 100, Continue: token}
	var rows []ConfigurationRow
	var meta metav1.ListMeta
	switch req.Kind {
	case "Namespace":
		list, e := client.CoreV1().Namespaces().List(ctx, opts)
		if e != nil {
			return ConfigurationPage{}, configurationListError(ctx, e)
		}
		meta = list.ListMeta
		rows, err = configurationRows(req, list.Items)
	case "ResourceQuota":
		list, e := client.CoreV1().ResourceQuotas(req.Namespace).List(ctx, opts)
		if e != nil {
			return ConfigurationPage{}, configurationListError(ctx, e)
		}
		meta = list.ListMeta
		rows, err = configurationRows(req, list.Items)
	case "LimitRange":
		list, e := client.CoreV1().LimitRanges(req.Namespace).List(ctx, opts)
		if e != nil {
			return ConfigurationPage{}, configurationListError(ctx, e)
		}
		meta = list.ListMeta
		rows, err = configurationRows(req, list.Items)
	case "ConfigMap":
		list, e := client.CoreV1().ConfigMaps(req.Namespace).List(ctx, opts)
		if e != nil {
			return ConfigurationPage{}, configurationListError(ctx, e)
		}
		meta = list.ListMeta
		rows, err = configurationRows(req, list.Items)
	}
	if err != nil {
		return ConfigurationPage{}, err
	}
	return s.configurationPage(ctx, scope, meta, rows)
}

func configurationListError(ctx context.Context, err error) error {
	if apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) {
		return model.Fail("api_unavailable", "Configuration API unavailable for this kind")
	}
	return readError(ctx, err)
}

// Preserve server quantity spelling; decoding through resource.Quantity may canonicalize it.
func (s *Service) readPolicyOverview(ctx context.Context, client kubernetes.Interface, ref ResourceRef) (ResourceDetail, error) {
	resource := "resourcequotas"
	if ref.Kind == "LimitRange" {
		resource = "limitranges"
	}
	raw, err := client.CoreV1().RESTClient().Get().Namespace(ref.Namespace).Resource(resource).Name(ref.Name).
		SetHeader("Accept", "application/json").MaxRetries(0).Do(ctx).Raw()
	if err != nil {
		return ResourceDetail{}, readError(ctx, err)
	}
	if len(raw) > detailLimit {
		return ResourceDetail{}, model.Fail("capacity", "Resource evidence exceeds the 2 MiB size limit")
	}
	var object struct {
		metav1.TypeMeta
		Metadata metav1.ObjectMeta `json:"metadata"`
	}
	if json.Unmarshal(raw, &object) != nil || object.Kind != ref.Kind || object.APIVersion != ref.APIVersion ||
		object.Metadata.Name != ref.Name || object.Metadata.Namespace != ref.Namespace {
		return ResourceDetail{}, model.Fail("invalid", "Server returned an invalid policy identity")
	}
	if string(object.Metadata.UID) != ref.UID {
		return ResourceDetail{}, model.Fail("resource_replaced", "Resource replaced; select again")
	}
	result := ResourceDetail{Resource: ref, ResourceVersion: object.Metadata.ResourceVersion, FetchedAt: time.Now().UTC(), ObjectJSON: string(raw)}
	if err = checkPayload(result); err != nil {
		return ResourceDetail{}, err
	}
	if err = ctx.Err(); err != nil {
		return ResourceDetail{}, safeError(ctx, err)
	}
	return result, s.connections.Validate(ref.Connection)
}
