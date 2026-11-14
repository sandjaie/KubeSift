/*
Purpose: Read one page or capture one object from an installed read-only API.
Used by: The generic resource browser.
Responsibilities: Resolve the descriptor, validate scope and response identity, and enforce evidence bounds.
Important: A list never exports object contents and a detail never converts numbers through float64.
*/
package kube

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode"

	"github.com/sandjaie/KubeSift/internal/model"
	"go.yaml.in/yaml/v3"
	"k8s.io/apimachinery/pkg/api/validate/content"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func genericAddress(api apiResource, ns string) error {
	if api.namespaced {
		return namespace(ns)
	}
	if ns != "" {
		return model.Fail("invalid", "This API requires cluster scope")
	}
	return nil
}
func genericName(name string) bool {
	return name != "" && len(name) <= 253 && len(content.IsPathSegmentName(name)) == 0 && !strings.ContainsAny(name, "\\") && strings.IndexFunc(name, unicode.IsControl) < 0
}
func genericRef(conn model.ConnectionRef, api apiResource, meta metav1.ObjectMeta) GenericResourceRef {
	return GenericResourceRef{Connection: conn, Group: api.group, Version: api.version, Resource: api.plural, Kind: api.kind, Namespace: meta.Namespace, Name: meta.Name, UID: string(meta.UID)}
}
func (s *Service) ReadGenericResources(ctx context.Context, req GenericPageRequest) (GenericResourcePage, error) {
	api, err := s.resolveDescriptor(req.Connection, req.Handle)
	if err != nil {
		return GenericResourcePage{}, err
	}
	if err := genericAddress(api, req.Namespace); err != nil {
		return GenericResourcePage{}, err
	}
	scope := pageCursor{Connection: req.Connection, Namespace: req.Namespace, Kind: "generic", Query: req.Handle}
	token, err := s.decodeGenericCursor(req.Cursor, scope)
	if err != nil {
		return GenericResourcePage{}, err
	}
	client, err := s.boundedReadClient(ctx, req.Connection)
	if err != nil {
		return GenericResourcePage{}, err
	}
	list, err := listJSON(ctx, api.read(client, req.Namespace, ""), api, token)
	if err != nil {
		return GenericResourcePage{}, resourceListError(ctx, err)
	}
	page, err := pageInfo(scope, list.Metadata)
	if err != nil {
		return GenericResourcePage{}, err
	}
	if page.NextCursor != "" {
		page.NextCursor += "." + s.signGenericCursor(page.NextCursor)
		if len(page.NextCursor) > cursorLimit {
			return GenericResourcePage{}, model.Fail("capacity", "Page cursor exceeds its size limit")
		}
	}
	out := GenericResourcePage{Items: make([]GenericResourceRow, 0, len(list.Items)), Page: page}
	for _, raw := range list.Items {
		item, err := objectIdentity(raw, api, req.Namespace, true)
		if err != nil {
			return GenericResourcePage{}, err
		}
		if !genericName(item.Metadata.Name) {
			return GenericResourcePage{}, model.Fail("invalid", "Server returned an invalid resource name")
		}
		out.Items = append(out.Items, GenericResourceRow{Resource: genericRef(req.Connection, api, item.Metadata), CreatedAt: created(item.Metadata)})
	}
	if err := checkPayload(out); err != nil {
		return GenericResourcePage{}, err
	}
	if ctx.Err() != nil {
		return GenericResourcePage{}, safeError(ctx, ctx.Err())
	}
	if _, err := s.resolveDescriptor(req.Connection, req.Handle); err != nil {
		return GenericResourcePage{}, err
	}
	return out, nil
}
func (s *Service) ReadGenericDetail(ctx context.Context, req GenericDetailRequest) (GenericResourceDetail, error) {
	api, err := s.resolveDescriptor(req.Connection, req.Handle)
	if err != nil {
		return GenericResourceDetail{}, err
	}
	if err := genericAddress(api, req.Namespace); err != nil {
		return GenericResourceDetail{}, err
	}
	if !genericName(req.Name) || len(req.UID) == 0 || len(req.UID) > 128 {
		return GenericResourceDetail{}, model.Fail("invalid", "Explicit valid resource name and UID required")
	}
	client, err := s.boundedReadClient(ctx, req.Connection)
	if err != nil {
		return GenericResourceDetail{}, err
	}
	raw, err := api.read(client, req.Namespace, req.Name).Do(ctx).Raw()
	if err != nil {
		return GenericResourceDetail{}, readError(ctx, err)
	}
	if len(raw) > detailLimit {
		return GenericResourceDetail{}, model.Fail("capacity", "Resource capture exceeds the 2 MiB size limit")
	}
	item, err := objectIdentity(raw, api, req.Namespace, false)
	if err != nil {
		return GenericResourceDetail{}, err
	}
	if item.Metadata.Name != req.Name {
		return GenericResourceDetail{}, model.Fail("invalid", "Server did not return the requested resource name")
	}
	if string(item.Metadata.UID) != req.UID {
		return GenericResourceDetail{}, model.Fail("resource_replaced", "Resource replaced; select again")
	}
	rendered, err := exactYAML(raw)
	if err != nil {
		return GenericResourceDetail{}, err
	}
	out := GenericResourceDetail{Resource: genericRef(req.Connection, api, item.Metadata), ResourceVersion: item.Metadata.ResourceVersion, FetchedAt: time.Now().UTC(), ObjectJSON: string(raw), YAML: rendered}
	if err := checkPayload(out); err != nil {
		return GenericResourceDetail{}, err
	}
	if ctx.Err() != nil {
		return GenericResourceDetail{}, safeError(ctx, ctx.Err())
	}
	if _, err := s.resolveDescriptor(req.Connection, req.Handle); err != nil {
		return GenericResourceDetail{}, err
	}
	return out, nil
}

func exactYAML(raw []byte) (string, error) {
	// Decode into syntax nodes, so numbers retain their lexical form.
	var document yaml.Node
	if yaml.Unmarshal(raw, &document) != nil {
		return "", model.Fail("invalid", "Cannot render captured YAML")
	}
	type item struct {
		node  *yaml.Node
		depth int
	}
	queue := []item{{&document, 0}}
	size := len(raw)
	for len(queue) > 0 {
		last := len(queue) - 1
		current := queue[last]
		queue = queue[:last]
		current.node.Style &^= yaml.FlowStyle
		size += 4*current.depth + 2
		if size > detailLimit {
			return "", model.Fail("capacity", "Captured YAML exceeds the 2 MiB size limit")
		}
		depth := current.depth
		if current.node.Kind == yaml.SequenceNode || current.node.Kind == yaml.MappingNode {
			depth++
		}
		for _, child := range current.node.Content {
			queue = append(queue, item{child, depth})
		}
	}
	encoded, err := yaml.Marshal(&document)
	if err != nil {
		return "", model.Fail("invalid", "Cannot render captured YAML")
	}
	if len(encoded) > detailLimit {
		return "", model.Fail("capacity", "Captured YAML exceeds the 2 MiB size limit")
	}
	return string(encoded), nil
}

func (s *Service) signGenericCursor(cursor string) string {
	mac := hmac.New(sha256.New, []byte(s.cursorKey))
	mac.Write([]byte(cursor))
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Service) decodeGenericCursor(cursor string, scope pageCursor) (string, error) {
	if cursor == "" {
		return "", nil
	}
	if len(cursor) > cursorLimit {
		return "", model.Fail("list_changed", "List changed; refresh")
	}
	encoded, signature, ok := strings.Cut(cursor, ".")
	if !ok || !hmac.Equal([]byte(signature), []byte(s.signGenericCursor(encoded))) {
		return "", model.Fail("list_changed", "List changed; refresh")
	}
	return decodeCursor(encoded, scope)
}
