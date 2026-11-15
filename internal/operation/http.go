/*
Purpose: Send bounded guarded requests for reviewed workload operations.
Used by: Native scale and restart preparation and execution.
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
	return c.send(ctx, method, path, dry, raw, "application/json", out)
}

func (c operationClient) patch(ctx context.Context, path string, dry bool, patch []byte, out any) ([]string, error) {
	return c.send(ctx, http.MethodPatch, path, dry, patch, "application/json-patch+json", out)
}

func (c operationClient) send(ctx context.Context, method, path string, dry bool, raw []byte, contentType string, out any) ([]string, error) {
	addr := *c.base
	addr.Path = strings.TrimRight(addr.Path, "/") + path
	addr.RawPath, addr.RawQuery, addr.Fragment = "", "", ""
	if dry {
		addr.RawQuery = "dryRun=All"
	}
	var body io.Reader
	if raw != nil {
		// Keep the body non-replayable even if the HTTP transport attempts a retry.
		body = io.NopCloser(bytes.NewReader(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, addr.String(), body)
	if err != nil {
		return nil, model.Fail("invalid", "Cannot build operation request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "KubeSift")
	if raw != nil {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, operationHTTPStatus(resp.StatusCode)
	}
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(response, out) != nil {
		return nil, model.Fail("invalid_response", "Kubernetes returned invalid operation data")
	}
	if !dry {
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
