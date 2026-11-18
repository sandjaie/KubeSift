/*
Purpose: Resolve bounded HTTPS and OCI chart sources without ambient Helm state.
Used by: Native Helm source settings and chart preparation.
Responsibilities: Scope credentials to origins, validate TLS and pin downloaded content.
Important: Redirects never receive credentials for another origin; HTTP downgrade is rejected.
*/
package helmmanager

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/preferences"
	"go.yaml.in/yaml/v3"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

const indexLimit = 10 << 20

func sourceURL(source Source) (*url.URL, error) {
	if err := preferences.ValidateHelmSource(source); err != nil {
		return nil, model.Fail("invalid", err.Error())
	}
	return url.Parse(source.URL)
}

func (s *Service) SaveSource(source Source) error {
	if _, err := sourceURL(source); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return model.Fail("cancelled", "Helm manager is closed.")
	}
	if _, ok := s.sources[source.ID]; !ok && len(s.sources) >= 32 {
		return model.Fail("capacity", "At most 32 Helm sources may be saved.")
	}
	s.sourceGeneration++
	s.invalidateSourceLocked(source.ID)
	ctx, cancel := context.WithCancel(s.ctx)
	s.sources[source.ID] = sourceData{source: source, generation: s.sourceGeneration, ctx: ctx, cancel: cancel}
	return nil
}
func (s *Service) invalidateSourceLocked(id string) {
	if old := s.sources[id]; old.cancel != nil {
		old.cancel()
	}
	delete(s.indexes, id)
	for token, r := range s.reviews {
		if r.source == id {
			if r.cancel != nil {
				r.cancel()
			}
			delete(s.reviews, token)
		}
	}
	for key, c := range s.charts {
		if c.source == id {
			delete(s.charts, key)
		}
	}
}
func (s *Service) DeleteSource(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidateSourceLocked(id)
	delete(s.sources, id)
	return nil
}
func (s *Service) SetCredentials(id string, credentials SourceCredentials) error {
	if len(credentials.Username) > 4096 || len(credentials.Password) > 65536 {
		return model.Fail("capacity", "Source credential is too large.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sources[id]
	if !ok {
		return model.Fail("invalid", "Choose a saved source.")
	}
	s.invalidateSourceLocked(id)
	s.sourceGeneration++
	v.generation = s.sourceGeneration
	v.ctx, v.cancel = context.WithCancel(s.ctx)
	v.credentials = credentials
	s.sources[id] = v
	return nil
}
func (s *Service) ForgetCredentials(id string) { _ = s.SetCredentials(id, SourceCredentials{}) }
func (s *Service) source(id string) (sourceData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sources[id]
	if !ok || s.closed {
		return v, model.Fail("invalid", "Choose an available chart source.")
	}
	return v, nil
}

type secureTransport struct {
	next        http.RoundTripper
	external    http.RoundTripper
	origin      string
	credentials SourceCredentials
	basic       bool
}

func (t secureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.User != nil {
		return nil, model.Fail("tls", "Chart sources require HTTPS without embedded credentials.")
	}
	r = r.Clone(r.Context())
	if r.URL.Host != t.origin {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			return nil, model.Fail("authentication", "Cross-origin credential exchange is not allowed.")
		}
		r.Header.Del("Authorization")
		r.Header.Del("Cookie")
		return t.external.RoundTrip(r)
	} else if t.basic && t.credentials.Password != "" {
		r.SetBasicAuth(t.credentials.Username, t.credentials.Password)
	}
	return t.next.RoundTrip(r)
}
func sourceClient(v sourceData, basic bool) (*http.Client, func(), error) {
	u, err := sourceURL(v.source)
	if err != nil {
		return nil, nil, err
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if v.source.CAFile != "" {
		raw, err := readTLSFile(v.source.CAFile)
		if err != nil || len(raw) > 1<<20 {
			return nil, nil, model.Fail("tls", "Cannot read the selected CA certificate.")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(raw) {
			return nil, nil, model.Fail("tls", "The selected CA certificate is invalid.")
		}
		config.RootCAs = roots
	}
	if v.source.CertFile != "" {
		certPEM, err := readTLSFile(v.source.CertFile)
		if err != nil {
			return nil, nil, model.Fail("tls", "Cannot read client certificate.")
		}
		keyPEM, err := readTLSFile(v.source.KeyFile)
		if err != nil {
			return nil, nil, model.Fail("tls", "Cannot read client key.")
		}
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, nil, model.Fail("tls", "Cannot load the selected client certificate/key.")
		}
		config.Certificates = []tls.Certificate{cert}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = config
	transport.MaxResponseHeaderBytes = 1 << 20
	external := transport.Clone()
	external.TLSClientConfig = config.Clone()
	external.TLSClientConfig.Certificates = nil
	client := &http.Client{Transport: secureTransport{next: transport, external: external, origin: u.Host, credentials: v.credentials, basic: basic}, Timeout: 120 * time.Second}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return model.Fail("unavailable", "Too many chart download redirects.")
		}
		if req.URL.Scheme != "https" || req.URL.User != nil {
			return model.Fail("tls", "HTTPS downgrade or embedded redirect credentials were rejected.")
		}
		return nil
	}
	return client, func() { transport.CloseIdleConnections(); external.CloseIdleConnections() }, nil
}
func download(ctx context.Context, client *http.Client, address string, limit int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, model.Fail("invalid", "Invalid chart download address.")
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, sourceError(ctx, err)
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return nil, model.Fail("authentication", "The chart source rejected authentication. Renew credentials externally and retry.")
	}
	if res.StatusCode != 200 {
		return nil, model.Fail("unavailable", "The requested chart or repository index is unavailable.")
	}
	if res.ContentLength > int64(limit) {
		return nil, model.Fail("capacity", "Chart source response exceeds its size limit.")
	}
	raw, err := readBounded(res.Body, limit)
	if err != nil {
		return nil, sourceError(ctx, err)
	}
	return raw, nil
}
func sourceError(ctx context.Context, err error) error {
	var p *model.Problem
	if errors.As(err, &p) {
		return p
	}
	if ctx.Err() != nil {
		return model.Fail("cancelled", "Chart resolution cancelled or timed out.")
	}
	var cert *tls.CertificateVerificationError
	if errors.As(err, &cert) {
		return model.Fail("tls", "Chart source certificate verification failed. Select the correct trusted CA.")
	}
	// SDK/HTTP errors can include URLs, credentials or response bodies.
	return model.Fail("unavailable", "Chart source request failed. Check its address, TLS configuration and credentials.")
}
func (s *Service) readIndex(ctx context.Context, v sourceData) (indexData, error) {
	client, done, err := sourceClient(v, true)
	if err != nil {
		return indexData{}, err
	}
	defer done()
	raw, err := download(ctx, client, strings.TrimRight(v.source.URL, "/")+"/index.yaml", indexLimit)
	if err != nil {
		return indexData{}, err
	}
	var index struct {
		Entries map[string][]ChartEntry `yaml:"entries"`
	}
	if err := yaml.Unmarshal(raw, &index); err != nil {
		return indexData{}, model.Fail("invalid", "The chart repository index is invalid.")
	}
	data := indexData{id: model.ID(), expires: time.Now().Add(reviewTTL), generation: v.generation}
	for name, versions := range index.Entries {
		for _, entry := range versions {
			if entry.Name != name || len(name) > 256 || len(entry.Version) > 256 || len(entry.Digest) > 128 || len(entry.URLs) > 16 {
				return data, model.Fail("invalid", "Repository entry metadata is invalid or oversized.")
			}
			for _, u := range entry.URLs {
				if len(u) > 4096 {
					return data, model.Fail("capacity", "Chart URL exceeds its limit.")
				}
			}
			data.entries = append(data.entries, entry)
			if len(data.entries) > 10000 {
				return data, model.Fail("capacity", "Repository exceeds 10,000 chart versions.")
			}
		}
	}
	sort.Slice(data.entries, func(i, j int) bool {
		a, b := data.entries[i], data.entries[j]
		if a.Name == b.Name {
			return a.Version > b.Version
		}
		return a.Name < b.Name
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.sources[v.source.ID].generation != v.generation {
		return data, model.Fail("stale_target", "Source changed; search again.")
	}
	s.indexes[v.source.ID] = data
	return data, nil
}
func (s *Service) ListCharts(ctx context.Context, id, filter, cursor string) (ChartPage, error) {
	ctx, done := s.owned(ctx, 120*time.Second)
	defer done()
	v, err := s.source(id)
	if err != nil {
		return ChartPage{}, err
	}
	ctx, sourceDone := sourceContext(ctx, v)
	defer sourceDone()
	if v.source.Kind != "https" || len(filter) > 256 {
		return ChartPage{}, model.Fail("invalid", "Search is available for HTTPS repositories; OCI uses an exact chart reference.")
	}
	var data indexData
	offset := 0
	if cursor == "" {
		data, err = s.readIndex(ctx, v)
	} else {
		parts := strings.Split(cursor, ":")
		if len(parts) != 2 {
			return ChartPage{}, model.Fail("invalid", "Invalid chart page cursor.")
		}
		offset, err = strconv.Atoi(parts[1])
		s.mu.Lock()
		data = s.indexes[id]
		s.mu.Unlock()
		if err != nil || data.id != parts[0] || time.Now().After(data.expires) || offset < 0 || offset%100 != 0 {
			return ChartPage{}, model.Fail("stale_target", "Chart search expired; search again.")
		}
	}
	if err != nil {
		return ChartPage{}, err
	}
	matches := []ChartEntry{}
	for _, e := range data.entries {
		if strings.Contains(strings.ToLower(e.Name), strings.ToLower(filter)) {
			matches = append(matches, e)
		}
	}
	if offset > len(matches) {
		return ChartPage{}, model.Fail("invalid", "Invalid chart page offset.")
	}
	end := min(offset+100, len(matches))
	out := ChartPage{Items: matches[offset:end]}
	if end < len(matches) {
		out.NextCursor = data.id + ":" + strconv.Itoa(end)
	}
	return out, nil
}
func (s *Service) ResolveChart(ctx context.Context, req ChartRequest) (ChartSnapshot, error) {
	ctx, done := s.owned(ctx, 120*time.Second)
	defer done()
	if req.LocalHandle != "" {
		if req.SourceID != "" || req.Chart != "" || req.Version != "" || req.Digest != "" {
			return ChartSnapshot{}, model.Fail("invalid", "Choose exactly one chart source.")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.expireLocked()
		c := s.charts[req.LocalHandle]
		if c == nil || len(c.raw) == 0 {
			return ChartSnapshot{}, model.Fail("stale_target", "Selected local chart expired. Select it again.")
		}
		return c.ChartSnapshot, nil
	}
	v, err := s.source(req.SourceID)
	if err != nil {
		return ChartSnapshot{}, err
	}
	ctx, sourceDone := sourceContext(ctx, v)
	defer sourceDone()
	if len(req.Version) > 256 || len(req.Digest) > 128 || req.Chart == "" || len(req.Chart) > 1024 || strings.ContainsAny(req.Chart, "?#@:\\") || strings.Contains(req.Chart, "..") || strings.HasPrefix(req.Chart, "/") || (req.Version == "") == (req.Digest == "") {
		return ChartSnapshot{}, model.Fail("invalid", "Choose a chart and exactly one version or digest.")
	}
	c, err := s.reserve(req.SourceID, v.generation)
	if err != nil {
		return ChartSnapshot{}, err
	}
	var raw []byte
	if v.source.Kind == "https" {
		if req.Digest != "" {
			s.Discard(c.ID)
			return ChartSnapshot{}, model.Fail("invalid", "HTTPS charts require an exact version.")
		}
		data, e := s.readIndex(ctx, v)
		err = e
		if err == nil {
			var entry *ChartEntry
			for _, candidate := range data.entries {
				if candidate.Name == req.Chart && candidate.Version == req.Version {
					copy := candidate
					entry = &copy
					break
				}
			}
			if entry == nil || len(entry.URLs) == 0 {
				err = model.Fail("not_found", "The exact chart version is not in this repository.")
			} else {
				base, _ := url.Parse(strings.TrimRight(v.source.URL, "/") + "/")
				location, e := url.Parse(entry.URLs[0])
				err = e
				if err == nil {
					client, close, e := sourceClient(v, true)
					err = e
					if err == nil {
						raw, err = download(ctx, client, base.ResolveReference(location).String(), archiveLimit)
						close()
					}
					if err == nil && entry.Digest != "" && strings.TrimPrefix(entry.Digest, "sha256:") != strings.TrimPrefix(chartHash(raw), "sha256:") {
						err = model.Fail("invalid", "Chart download does not match the repository digest.")
					}
				}
			}
		}
	} else {
		raw, err = pullOCI(ctx, v, req)
	}
	if err == nil {
		err = s.finishChart(ctx, c, raw)
	}
	if err == nil && (c.Name != req.Chart[strings.LastIndex(req.Chart, "/")+1:] || (req.Version != "" && c.Version != req.Version)) {
		err = model.Fail("invalid", "Downloaded chart identity differs from the requested name or version.")
	}
	if err != nil {
		s.Discard(c.ID)
		return ChartSnapshot{}, sourceError(ctx, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return c.ChartSnapshot, nil
}
func pullOCI(ctx context.Context, v sourceData, req ChartRequest) ([]byte, error) {
	u, _ := sourceURL(v.source)
	repository, err := remote.NewRepository(u.Host + strings.TrimRight(u.Path, "/") + "/" + req.Chart)
	if err != nil {
		return nil, model.Fail("invalid", "Invalid OCI chart reference.")
	}
	client, close, err := sourceClient(v, false)
	if err != nil {
		return nil, err
	}
	defer close()
	repository.Client = &auth.Client{Client: client, Cache: auth.NewCache(), Credential: func(_ context.Context, host string) (auth.Credential, error) {
		if host != u.Host {
			return auth.EmptyCredential, nil
		}
		return auth.Credential{Username: v.credentials.Username, Password: v.credentials.Password}, nil
	}}
	reference := req.Digest
	if reference == "" {
		reference = strings.ReplaceAll(req.Version, "+", "_")
	}
	desc, err := repository.Resolve(ctx, reference)
	if err != nil {
		return nil, sourceError(ctx, err)
	}
	fetch := func(d ocispec.Descriptor, limit int) ([]byte, error) {
		if d.Size < 0 || d.Size > int64(limit) {
			return nil, model.Fail("capacity", "OCI descriptor exceeds its size limit.")
		}
		body, err := repository.Fetch(ctx, d)
		if err != nil {
			return nil, sourceError(ctx, err)
		}
		defer body.Close()
		data, err := readBounded(body, limit)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) != d.Size || chartHash(data) != d.Digest.String() {
			return nil, model.Fail("invalid", "OCI content digest verification failed.")
		}
		return data, nil
	}
	manifestBytes, err := fetch(desc, 1<<20)
	if err != nil {
		return nil, err
	}
	var manifest ocispec.Manifest
	if json.Unmarshal(manifestBytes, &manifest) != nil || len(manifest.Layers) > 16 {
		return nil, model.Fail("invalid", "Invalid OCI chart manifest.")
	}
	for _, layer := range manifest.Layers {
		if layer.MediaType == "application/vnd.cncf.helm.chart.content.v1.tar+gzip" {
			return fetch(layer, archiveLimit)
		}
	}
	return nil, model.Fail("invalid", "OCI artifact contains no Helm chart layer.")
}

func readTLSFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readBounded(f, 1<<20)
}

func sourceContext(ctx context.Context, v sourceData) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(v.ctx, cancel)
	return ctx, func() { stop(); cancel() }
}
