/*
Purpose: List namespaced ServiceAccounts and scoped RBAC resources.
Used by: The access control explorer and resource identity checks.
Responsibilities: Bound inventory, validate scope and project column evidence.
Important: No token, Secret or authorization-review endpoints are used.
*/
package kube

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/validate/content"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func accessControlVersion(kind string) string {
	switch kind {
	case "ServiceAccount":
		return "v1"
	case "Role", "RoleBinding", "ClusterRole", "ClusterRoleBinding":
		return rbac.SchemeGroupVersion.String()
	}
	return ""
}

func validResourceName(kind, name string) bool {
	if accessControlVersion(kind) == rbac.SchemeGroupVersion.String() {
		return name != "" && len(content.IsPathSegmentName(name)) == 0
	}
	return len(validation.IsDNS1123Subdomain(name)) == 0
}

func accessControlSummary(object workloadObject) any {
	switch o := object.(type) {
	case *core.ServiceAccount:
		return map[string]any{"automount": o.AutomountServiceAccountToken, "imagePullCount": len(o.ImagePullSecrets)}
	case *rbac.Role:
		return map[string]any{"ruleCount": len(o.Rules)}
	case *rbac.ClusterRole:
		return map[string]any{"ruleCount": len(o.Rules), "aggregation": o.AggregationRule != nil}
	case *rbac.RoleBinding:
		return map[string]any{"roleKind": o.RoleRef.Kind, "roleName": o.RoleRef.Name, "subjectCount": len(o.Subjects)}
	case *rbac.ClusterRoleBinding:
		return map[string]any{"roleName": o.RoleRef.Name, "subjectCount": len(o.Subjects)}
	}
	return nil
}

func accessControlRows[T any, P interface {
	*T
	workloadObject
}](req PageRequest, items []T) ([]AccessControlRow, error) {
	if len(items) > 100 {
		return nil, model.Fail("capacity", "Server exceeded page size")
	}
	rows := make([]AccessControlRow, 0, len(items))
	for i := range items {
		object := P(&items[i])
		ref := ResourceRef{Connection: req.Connection, Kind: req.Kind, APIVersion: accessControlVersion(req.Kind), Name: object.GetName(), Namespace: object.GetNamespace(), UID: string(object.GetUID())}
		if ref.Namespace != req.Namespace || validResource(ref) != nil {
			return nil, model.Fail("invalid", "Server returned an invalid access control identity")
		}
		raw, err := json.Marshal(accessControlSummary(object))
		if err != nil {
			return nil, model.Fail("failed", "Cannot encode access control inventory")
		}
		var createdAt *time.Time
		if stamp := object.GetCreationTimestamp(); !stamp.IsZero() {
			createdAt = &stamp.Time
		}
		rows = append(rows, AccessControlRow{Resource: ref, CreatedAt: createdAt, ObjectJSON: string(raw)})
	}
	return rows, nil
}

func (s *Service) ReadAccessControl(ctx context.Context, req PageRequest) (AccessControlPage, error) {
	var result AccessControlPage
	if accessControlVersion(req.Kind) == "" {
		return result, model.Fail("invalid", "Unsupported access control kind")
	}
	if err := resourceScope(req.Kind, req.Namespace); err != nil {
		return result, err
	}
	scope := pageCursor{Connection: req.Connection, Kind: req.Kind, Namespace: req.Namespace}
	cursor, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return result, err
	}
	client, err := s.boundedReadClient(ctx, req.Connection)
	if err != nil {
		return result, err
	}
	opts := metav1.ListOptions{Limit: 100, Continue: cursor}
	var rows []AccessControlRow
	var meta metav1.ListMeta
	switch req.Kind {
	case "ServiceAccount":
		list, e := client.CoreV1().ServiceAccounts(req.Namespace).List(ctx, opts)
		if e != nil {
			return result, accessControlListError(ctx, e)
		}
		meta = list.ListMeta
		rows, err = accessControlRows(req, list.Items)
	case "Role":
		list, e := client.RbacV1().Roles(req.Namespace).List(ctx, opts)
		if e != nil {
			return result, accessControlListError(ctx, e)
		}
		meta = list.ListMeta
		rows, err = accessControlRows(req, list.Items)
	case "RoleBinding":
		list, e := client.RbacV1().RoleBindings(req.Namespace).List(ctx, opts)
		if e != nil {
			return result, accessControlListError(ctx, e)
		}
		meta = list.ListMeta
		rows, err = accessControlRows(req, list.Items)
	case "ClusterRole":
		list, e := client.RbacV1().ClusterRoles().List(ctx, opts)
		if e != nil {
			return result, accessControlListError(ctx, e)
		}
		meta = list.ListMeta
		rows, err = accessControlRows(req, list.Items)
	case "ClusterRoleBinding":
		list, e := client.RbacV1().ClusterRoleBindings().List(ctx, opts)
		if e != nil {
			return result, accessControlListError(ctx, e)
		}
		meta = list.ListMeta
		rows, err = accessControlRows(req, list.Items)
	}
	if err != nil {
		return result, err
	}
	next, err := encodeCursor(scope, meta.Continue)
	if err != nil {
		return result, err
	}
	page := AccessControlPage{Items: rows, Page: PageInfo{NextCursor: next, ResourceVersion: meta.ResourceVersion, FetchedAt: time.Now().UTC()}}
	if err := checkPayload(page); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, safeError(ctx, err)
	}
	return page, s.connections.Validate(req.Connection)
}

func accessControlListError(ctx context.Context, err error) error {
	if apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) {
		return model.Fail("api_unavailable", "Access control API unavailable for this kind")
	}
	return readError(ctx, err)
}
