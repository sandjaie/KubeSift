/*
Purpose: Send bounded guarded requests for reviewed workload operations.
Used by: Native workload operation preparation, execution and readback.
Responsibilities: Preserve non-replayable bodies, allowlisted media types and sanitized warnings.
Important: Raw error bodies never cross the operation boundary.
*/
package operation

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	utilnet "k8s.io/apimachinery/pkg/util/net"
)

type operationClient struct {
	client *http.Client
	base   *url.URL
}

func newOperationClient(connections *connection.Manager, ref model.ConnectionRef) (operationClient, error) {
	cfg, err := connections.RESTConfig(ref)
	if err != nil {
		return operationClient{}, err
	}
	base, err := url.Parse(cfg.Host)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return operationClient{}, model.Fail("invalid", "Invalid Kubernetes API endpoint")
	}
	client, err := kube.BoundedHTTPClient(cfg)
	return operationClient{client, base}, err
}

type operationHTTPStatus int

type operationStatus struct {
	APIVersion string
	Kind       string
	Status     string
	Reason     string
	Code       int
	Details    *struct {
		Group *string
		Kind  *string
		Name  *string
		UID   *string
	}
}

func (e operationHTTPStatus) Error() string { return http.StatusText(int(e)) }
func (c operationClient) call(ctx context.Context, method, path string, dry bool, input, out any) ([]string, error) {
	var raw []byte
	if input != nil {
		var err error
		raw, err = json.Marshal(input)
		if err != nil {
			return nil, model.Fail("invalid", "Cannot encode operation request")
		}
	}
	return c.send(ctx, method, path, dry, raw, "application/json", out, nil)
}

func (c operationClient) patch(ctx context.Context, path string, dry bool, patch []byte, out any) ([]string, error) {
	return c.send(ctx, http.MethodPatch, path, dry, patch, "application/json-patch+json", out, nil)
}

func (c operationClient) send(ctx context.Context, method, path string, dry bool, raw []byte, contentType string, out any, notFound *operationStatus) ([]string, error) {
	return c.sendRequest(ctx, method, path, dry, raw, contentType, out, notFound, nil, false, false)
}

type editFeedback struct {
	immutable         bool
	dryRunUnsupported bool
	strictUnsupported bool
}

func (c operationClient) editPatch(ctx context.Context, path string, dry bool, patch []byte, out *json.RawMessage, feedback *editFeedback) ([]string, error) {
	return c.sendRequest(ctx, http.MethodPatch, path, dry, patch, "application/json-patch+json", out, nil, feedback, false, false)
}

func (c operationClient) createPost(ctx context.Context, path string, dry bool, body []byte, out *json.RawMessage, feedback *editFeedback) ([]string, error) {
	return c.sendRequest(ctx, http.MethodPost, path, dry, body, "application/json", out, nil, feedback, true, false)
}

func (c operationClient) observeCreate(ctx context.Context, path string, metadataOnly bool, out *json.RawMessage, missing *operationStatus) ([]string, error) {
	return c.sendRequest(ctx, http.MethodGet, path, false, nil, "", out, missing, nil, false, metadataOnly)
}

func (c operationClient) sendRequest(ctx context.Context, method, path string, dry bool, raw []byte, contentType string, out any, notFound *operationStatus, edit *editFeedback, create, metadataOnly bool) ([]string, error) {
	addr := *c.base
	addr.Path = strings.TrimRight(addr.Path, "/") + path
	addr.RawPath, addr.RawQuery, addr.Fragment = "", "", ""
	query := url.Values{}
	if dry {
		query.Set("dryRun", "All")
	}
	if edit != nil || create {
		query.Set("fieldValidation", "Strict")
	}
	if create {
		query.Set("fieldManager", "kubesift-create")
	}
	addr.RawQuery = query.Encode()
	var body io.Reader
	if raw != nil {
		// Keep the body non-replayable even if the HTTP transport attempts a retry.
		body = io.NopCloser(bytes.NewReader(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, addr.String(), body)
	if err != nil {
		return nil, model.Fail("invalid", "Cannot build operation request")
	}
	accept := "application/json"
	if metadataOnly {
		accept = "application/json;as=PartialObjectMetadata;g=meta.k8s.io;v=v1"
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "KubeSift")
	if raw != nil {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 ||
		(create && resp.StatusCode != http.StatusCreated) ||
		(method == http.MethodDelete && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted) {
		if notFound != nil && resp.StatusCode == http.StatusNotFound {
			response, err := io.ReadAll(resp.Body)
			var status operationStatus
			if err == nil && json.Unmarshal(response, &status) == nil {
				*notFound = status
			}
		}
		if edit != nil && (resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusBadRequest) {
			var status struct {
				Message string
				Details struct {
					Causes []struct{ Message string }
				}
			}
			raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
			if err == nil && json.Unmarshal(raw, &status) == nil {
				message := strings.ToLower(status.Message)
				edit.immutable = strings.Contains(message, "immutable")
				edit.strictUnsupported = strings.Contains(message, "fieldvalidation") && (strings.Contains(message, "unsupported") || strings.Contains(message, "unknown parameter"))
				edit.dryRunUnsupported = strings.Contains(message, "dry run") || strings.Contains(message, "dry-run") || strings.Contains(message, "dryrun")
				for _, cause := range status.Details.Causes {
					edit.immutable = edit.immutable || strings.Contains(strings.ToLower(cause.Message), "immutable")
				}
			}
		}
		return nil, operationHTTPStatus(resp.StatusCode)
	}
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(response, out) != nil {
		return nil, model.Fail("invalid_response", "Kubernetes returned invalid operation data")
	}
	if !dry && method != http.MethodDelete {
		return nil, nil
	}
	return operationWarnings(resp.Header.Values("Warning")), nil
}

func operationWarnings(headers []string) []string {
	warnings := []string{}
	budget := 8192
	for _, header := range headers {
		if len(header) > budget {
			break
		}
		budget -= len(header)
		values, _ := utilnet.ParseWarningHeaders([]string{header})
		for _, value := range values {
			text := strings.TrimSpace(strings.Map(func(r rune) rune {
				if !unicode.IsPrint(r) {
					return ' '
				}
				return r
			}, value.Text))
			if text != "" && len(warnings) < 32 {
				warnings = append(warnings, text)
			}
		}
	}
	return warnings
}
