/*
Purpose: Read Secret identity through strict partial metadata negotiation.
Used by: Secret inventory, Overview and explicit Pod references.
Responsibilities: Check response representation, discard arbitrary metadata and sanitize errors.
Important: Requests never fall back to a full Secret representation.
*/
package kube

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type secretIdentity struct {
	Name            string     `json:"name"`
	Namespace       string     `json:"namespace"`
	UID             string     `json:"uid"`
	ResourceVersion string     `json:"resourceVersion"`
	CreatedAt       *time.Time `json:"creationTimestamp"`
}

type secretMetadataObject struct {
	metav1.TypeMeta
	Metadata secretIdentity `json:"metadata"`
}
type secretMetadataList struct {
	metav1.TypeMeta
	Metadata metav1.ListMeta        `json:"metadata"`
	Items    []secretMetadataObject `json:"items"`
}

func unsupportedSecretMetadata() error {
	return model.Fail("metadata_unsupported", "Server did not provide Secret metadata in the required partial format")
}

func (s *Service) requestSecretMetadata(ctx context.Context, ref model.ConnectionRef, namespace, name string, query url.Values, kind string, out any) error {
	cfg, err := s.boundedReadConfig(ctx, ref)
	if err != nil {
		return err
	}
	client, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return model.Fail("invalid", "Cannot create metadata reader")
	}
	// A redirect must never replay credentials or broaden the explicitly selected target.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	base, _, err := rest.DefaultServerURL(cfg.Host, cfg.APIPath, metav1.SchemeGroupVersion, cfg.TLSClientConfig.Insecure || len(cfg.CAData) > 0 || cfg.CAFile != "" || cfg.CertFile != "" || len(cfg.CertData) > 0)
	if err != nil {
		return model.Fail("invalid", "Cannot resolve metadata API")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v1/namespaces/" + namespace + "/secrets"
	base.RawPath = ""
	if name != "" {
		base.Path += "/" + name
	}
	base.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return model.Fail("invalid", "Cannot create metadata request")
	}
	req.Header.Set("Accept", "application/json;as="+kind+";g=meta.k8s.io;v=v1")
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return safeError(ctx, ctx.Err())
		}
		return model.Fail("unavailable", "Secret metadata request failed")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden:
		return model.Fail("forbidden", "Access denied for Secret metadata")
	case http.StatusUnauthorized:
		return model.Fail("auth_required", "Authentication was rejected")
	case http.StatusNotFound:
		if name != "" {
			return model.Fail("not_found", "Secret metadata not found")
		}
		return unsupportedSecretMetadata()
	case http.StatusGone:
		return model.Fail("list_changed", "List changed; refresh")
	case http.StatusNotAcceptable:
		return unsupportedSecretMetadata()
	default:
		return model.Fail("failed", "Secret metadata request failed")
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return unsupportedSecretMetadata()
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		if ctx.Err() != nil {
			return safeError(ctx, ctx.Err())
		}
		var problem *model.Error
		if errors.As(err, &problem) && problem.Code == "capacity" {
			return problem
		}
		return model.Fail("failed", "Cannot read Secret metadata")
	}
	var meta metav1.TypeMeta
	if json.Unmarshal(raw, &meta) != nil || meta.APIVersion != "meta.k8s.io/v1" || meta.Kind != kind || json.Unmarshal(raw, out) != nil {
		return unsupportedSecretMetadata()
	}
	if err = ctx.Err(); err != nil {
		return safeError(ctx, err)
	}
	return s.connections.Validate(ref)
}

func secretResource(connection model.ConnectionRef, meta secretIdentity) ResourceRef {
	return ResourceRef{Connection: connection, Kind: "Secret", APIVersion: "v1", Namespace: meta.Namespace, Name: meta.Name, UID: meta.UID}
}

func (s *Service) readSecretMetadata(ctx context.Context, ref ResourceRef) (ResourceDetail, error) {
	if ref.Kind != "Secret" {
		return ResourceDetail{}, model.Fail("invalid", "Secret identity required")
	}
	if err := validResourceAddress(ref); err != nil {
		return ResourceDetail{}, err
	}
	var object secretMetadataObject
	if err := s.requestSecretMetadata(ctx, ref.Connection, ref.Namespace, ref.Name, nil, "PartialObjectMetadata", &object); err != nil {
		return ResourceDetail{}, err
	}
	actual := secretResource(ref.Connection, object.Metadata)
	if validResource(actual) != nil || actual.Name != ref.Name || actual.Namespace != ref.Namespace {
		return ResourceDetail{}, model.Fail("invalid", "Server returned an invalid Secret metadata identity")
	}
	if ref.UID != "" && actual.UID != ref.UID {
		return ResourceDetail{}, model.Fail("resource_replaced", "Resource replaced; select again")
	}
	raw, _ := json.Marshal(object.Metadata)
	result := ResourceDetail{Resource: actual, ResourceVersion: object.Metadata.ResourceVersion, FetchedAt: time.Now().UTC(), ObjectJSON: string(raw)}
	if err := checkPayload(result); err != nil {
		return ResourceDetail{}, err
	}
	return result, nil
}

func (s *Service) listSecretMetadata(ctx context.Context, req PageRequest, scope pageCursor, token string) (ConfigurationPage, error) {
	query := url.Values{"limit": {"100"}}
	if token != "" {
		query.Set("continue", token)
	}
	var list secretMetadataList
	if err := s.requestSecretMetadata(ctx, req.Connection, req.Namespace, "", query, "PartialObjectMetadataList", &list); err != nil {
		return ConfigurationPage{}, err
	}
	if len(list.Items) > 100 {
		return ConfigurationPage{}, model.Fail("capacity", "Server exceeded page size")
	}
	rows := make([]ConfigurationRow, 0, len(list.Items))
	for _, item := range list.Items {
		if (item.Kind != "" && item.Kind != "PartialObjectMetadata") || (item.APIVersion != "" && item.APIVersion != "meta.k8s.io/v1") {
			return ConfigurationPage{}, unsupportedSecretMetadata()
		}
		ref := secretResource(req.Connection, item.Metadata)
		if ref.Namespace != req.Namespace || validResource(ref) != nil {
			return ConfigurationPage{}, model.Fail("invalid", "Server returned an invalid Secret metadata identity")
		}
		rows = append(rows, ConfigurationRow{Resource: ref, CreatedAt: item.Metadata.CreatedAt, ObjectJSON: "{}"})
	}
	return s.configurationPage(ctx, scope, list.Metadata, rows)
}
