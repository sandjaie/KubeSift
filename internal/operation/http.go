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
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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
		Group  *string
		Kind   *string
		Name   *string
		UID    *string
		Causes []struct{ Reason, Message, Field string }
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
	return c.sendRequest(ctx, method, path, dry, raw, contentType, out, requestOptions{notFound: notFound})
}

type editFeedback struct {
	immutable         bool
	dryRunUnsupported bool
	strictUnsupported bool
}

func (c operationClient) editPatch(ctx context.Context, path string, dry bool, patch []byte, out *json.RawMessage, feedback *editFeedback) ([]string, error) {
	return c.sendRequest(ctx, http.MethodPatch, path, dry, patch, "application/json-patch+json", out, requestOptions{edit: feedback})
}

func (c operationClient) createPost(ctx context.Context, path string, dry bool, body []byte, out *json.RawMessage, feedback *editFeedback) ([]string, error) {
	return c.sendRequest(ctx, http.MethodPost, path, dry, body, "application/json", out, requestOptions{edit: feedback, create: true})
}

func (c operationClient) observeCreate(ctx context.Context, path string, metadataOnly bool, out *json.RawMessage, missing *operationStatus) ([]string, error) {
	return c.sendRequest(ctx, http.MethodGet, path, false, nil, "", out, requestOptions{notFound: missing, metadataOnly: metadataOnly})
}

const applyManager = "kubesift-apply"

type requestOptions struct {
	query                url.Values
	eviction             *evictionRejection
	notFound             *operationStatus
	edit                 *editFeedback
	create, metadataOnly bool
	apply, force         bool
	conflict             *operationStatus
}

func (c operationClient) applyPatch(ctx context.Context, path string, dry, force bool, raw []byte, out *json.RawMessage, feedback *editFeedback, conflict *operationStatus) ([]string, error) {
	return c.sendRequest(ctx, http.MethodPatch, path, dry, raw, "application/apply-patch+yaml", out,
		requestOptions{apply: true, force: force, edit: feedback, conflict: conflict})
}

func (c operationClient) sendRequest(ctx context.Context, method, path string, dry bool, raw []byte, contentType string, out any, options requestOptions) ([]string, error) {
	addr := *c.base
	addr.Path = strings.TrimRight(addr.Path, "/") + path
	addr.RawPath, addr.RawQuery, addr.Fragment = "", "", ""
	query := options.query
	if query == nil {
		query = url.Values{}
	}
	if dry {
		query.Set("dryRun", "All")
	}
	if options.edit != nil || options.create || options.apply {
		query.Set("fieldValidation", "Strict")
	}
	if options.create {
		query.Set("fieldManager", "kubesift-create")
	}
	if options.apply {
		query.Set("fieldManager", applyManager)
		if options.force {
			query.Set("force", "true")
		}
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
	if options.metadataOnly {
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
		(options.create && resp.StatusCode != http.StatusCreated) ||
		(options.apply && resp.StatusCode != http.StatusOK) ||
		(method == http.MethodDelete && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted) {
		if options.eviction != nil && resp.StatusCode == 429 {
			var status struct {
				Message string
				Details struct {
					RetryAfterSeconds int
					Causes            []struct{ Reason string }
				}
			}
			if json.NewDecoder(resp.Body).Decode(&status) == nil {
				options.eviction.budget = strings.Contains(strings.ToLower(status.Message), "disruption budget")
				options.eviction.delay = status.Details.RetryAfterSeconds
			}
			value := resp.Header.Get("Retry-After")
			delay, parseErr := strconv.Atoi(value)
			if parseErr != nil {
				if date, e := http.ParseTime(value); e == nil {
					delay = int(math.Ceil(time.Until(date).Seconds()))
				}
			}
			if delay > options.eviction.delay {
				options.eviction.delay = delay
			}
		}
		if notFound := options.notFound; notFound != nil && resp.StatusCode == http.StatusNotFound {
			response, err := io.ReadAll(resp.Body)
			var status operationStatus
			if err == nil && json.Unmarshal(response, &status) == nil {
				*notFound = status
			}
		}
		if edit := options.edit; edit != nil && (resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusBadRequest) {
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
		if options.conflict != nil && resp.StatusCode == http.StatusConflict {
			raw, err := io.ReadAll(io.LimitReader(resp.Body, (8<<10)+1))
			var status operationStatus
			if err == nil && len(raw) <= 8<<10 && json.Unmarshal(raw, &status) == nil {
				*options.conflict = status
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
