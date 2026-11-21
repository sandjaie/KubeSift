/*
Purpose: Bind a historical metrics provider to an owned connection generation.
Used by: Pod and Node Overview historical usage.
Responsibilities: Validate explicit source changes, scope trust and retire active reads.
Important: The provider transport never inherits Kubernetes credentials or follows redirects.
*/
package kube

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"github.com/sandjaie/KubeSift/internal/model"
	"io"
	"k8s.io/apimachinery/pkg/util/validation"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type HistoryConfig struct {
	Origin        string `json:"origin"`
	Mode          string `json:"mode"`
	SingleCluster bool   `json:"singleCluster"`
	Cluster       string `json:"cluster"`
	CAFile        string `json:"caFile"`
	CAPEM         string `json:"caPEM"`
	Bearer        string `json:"bearer"`
}
type HistorySource struct {
	Revision      string `json:"revision"`
	Origin        string `json:"origin"`
	Mode          string `json:"mode"`
	Cluster       string `json:"cluster"`
	Authenticated bool   `json:"authenticated"`
}
type historySource struct {
	view   HistorySource
	bearer string
	client *http.Client
	ctx    context.Context
	cancel context.CancelFunc
}

func (h *historySource) close() {
	if h.cancel != nil {
		h.cancel()
		h.client.CloseIdleConnections()
	}
}
func historyOrigin(raw string) (string, error) {
	bad := model.Fail("invalid", "Enter an HTTPS origin with a host and optional port.")
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.User != nil || u.Host == "" || strings.ContainsAny(raw, "?#") || (u.EscapedPath() != "" && u.EscapedPath() != "/") {
		return "", bad
	}
	host := u.Hostname()
	if net.ParseIP(host) == nil && len(validation.IsDNS1123Subdomain(strings.ToLower(host))) != 0 {
		return "", bad
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", bad
	}
	if port := u.Port(); port != "" {
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil || n == 0 {
			return "", bad
		}
	}
	return "https://" + strings.ToLower(u.Host), nil
}
func historyTLS(path, certificate string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if path == "" && certificate == "" {
		return cfg, nil
	}
	bad := model.Fail("tls", "Choose one valid PEM CA certificate source of at most 1 MiB.")
	if len(certificate) > 1<<20 || (path != "" && certificate != "") {
		return nil, bad
	}
	b := []byte(certificate)
	if path != "" {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return nil, bad
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, bad
		}
		defer f.Close()
		b, err = io.ReadAll(io.LimitReader(f, (1<<20)+1))
		if err != nil || len(b) > 1<<20 {
			return nil, bad
		}
	}
	roots := x509.NewCertPool()
	for b = bytes.TrimSpace(b); len(b) > 0; b = bytes.TrimSpace(b) {
		if !bytes.HasPrefix(b, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, bad
		}
		block, rest := pem.Decode(b)
		// Decode can skip a malformed prefix before a valid certificate.
		consumed := b[:len(b)-len(rest)]
		if block == nil || block.Type != "CERTIFICATE" || bytes.Count(consumed, []byte("-----BEGIN ")) != 1 {
			return nil, bad
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !cert.IsCA {
			return nil, bad
		}
		roots.AddCert(cert)
		b = rest
	}
	if len(roots.Subjects()) == 0 {
		return nil, bad
	}
	cfg.RootCAs = roots
	return cfg, nil
}
func (s *Service) HistorySource(ref model.ConnectionRef) (HistorySource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.connections.Validate(ref); err != nil {
		return HistorySource{}, err
	}
	if h := s.history[ref]; h != nil {
		return h.view, nil
	}
	return HistorySource{}, nil
}
func (s *Service) SetHistorySource(ref model.ConnectionRef, revision string, c HistoryConfig) (HistorySource, error) {
	origin, err := historyOrigin(c.Origin)
	if err != nil {
		return HistorySource{}, err
	}
	if (c.Mode != "cluster" && c.Mode != "project") || c.SingleCluster != (c.Cluster == "") || len(c.Cluster) > 256 || strings.TrimSpace(c.Cluster) != c.Cluster || strings.IndexFunc(c.Cluster, unicode.IsControl) >= 0 || len(c.Bearer) > 16384 || strings.IndexFunc(c.Bearer, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return HistorySource{}, model.Fail("invalid", "Check source mode, cluster binding and bearer credential.")
	}
	cfg, err := historyTLS(c.CAFile, c.CAPEM)
	if err != nil {
		return HistorySource{}, err
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg, MaxResponseHeaderBytes: 1 << 16, IdleConnTimeout: 30 * time.Second}, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return s.replaceHistory(ref, revision, &historySource{view: HistorySource{Origin: origin, Mode: c.Mode, Cluster: c.Cluster, Authenticated: c.Bearer != ""}, bearer: c.Bearer, client: client})
}
func (s *Service) ClearHistorySource(ref model.ConnectionRef, revision string) (HistorySource, error) {
	return s.replaceHistory(ref, revision, &historySource{})
}
func (s *Service) replaceHistory(ref model.ConnectionRef, revision string, next *historySource) (HistorySource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	owned, err := s.connections.Lifetime(ref)
	if err != nil {
		return HistorySource{}, err
	}
	old := s.history[ref]
	if (old == nil && revision != "") || (old != nil && old.view.Revision != revision) {
		return HistorySource{}, model.Fail("stale_target", "Source changed. Reopen its settings.")
	}
	if old != nil {
		old.close()
	}
	if next.client != nil {
		next.ctx, next.cancel = context.WithCancel(owned)
	}
	next.view.Revision = model.ID()
	s.history[ref] = next
	return next.view, nil
}
func (s *Service) SuggestHistorySource(ctx context.Context, ref model.ConnectionRef) (string, error) {
	cfg, err := s.connections.RESTConfig(ref)
	if err != nil {
		return "", err
	}
	client, err := BoundedHTTPClient(cfg)
	if err != nil {
		return "", err
	}
	var route struct {
		Spec struct {
			Host string
			TLS  struct{ Termination string }
		}
		Status struct {
			Ingress []struct {
				Host       string
				Conditions []struct{ Type, Status string }
			}
		}
	}
	err = (metricsClient{client, cfg.Host}).get(ctx, "/apis/route.openshift.io/v1/namespaces/openshift-monitoring/routes/thanos-querier", 1<<20, &route)
	if err != nil {
		if errors.Is(err, metricsHTTP(http.StatusNotFound)) {
			return "", model.Fail("unavailable", "The monitoring Route was not found. Enter an HTTPS origin manually.")
		}
		return "", metricsProblem(ctx, err)
	}
	if len(validation.IsDNS1123Subdomain(route.Spec.Host)) == 0 && (route.Spec.TLS.Termination == "reencrypt" || route.Spec.TLS.Termination == "edge" || route.Spec.TLS.Termination == "passthrough") {
		for _, ingress := range route.Status.Ingress {
			if ingress.Host == route.Spec.Host {
				for _, cond := range ingress.Conditions {
					if cond.Type == "Admitted" && cond.Status == "True" {
						return historyOrigin("https://" + ingress.Host)
					}
				}
			}
		}
	}
	return "", model.Fail("unavailable", "No admitted TLS monitoring Route was found.")
}
