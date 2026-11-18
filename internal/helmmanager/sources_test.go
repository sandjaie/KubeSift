/*
Purpose: Exercise chart acquisition and its credential and allocation boundaries.
Used by: Go tests for the native Helm manager.
Responsibilities: Check packaged charts, authenticated repositories and hostile archives with synthetic data.
Important: No real credentials, external registry or cluster is used.
*/
package helmmanager

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func tinyChart(t *testing.T) []byte {
	return archive(t, map[string]string{"demo/Chart.yaml": "apiVersion: v2\nname: demo\nversion: 1.0.0\n", "demo/values.yaml": "message: hello\n", "demo/templates/config.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}\ndata:\n  message: {{ .Values.message | quote }}\n"})
}
func TestChartSources(t *testing.T) {
	raw := tinyChart(t)
	t.Run("local immutable handle and bounds", func(t *testing.T) {
		s := NewService(nil)
		defer s.Close()
		path := filepath.Join(t.TempDir(), "demo.tgz")
		os.WriteFile(path, raw, 0600)
		handle, err := s.ImportChart(path)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(path, []byte("changed"), 0600)
		got, err := s.ResolveChart(context.Background(), ChartRequest{LocalHandle: handle})
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if got.Name != "demo" || got.Version != "1.0.0" || got.Digest != "sha256:"+hex.EncodeToString(sum[:]) {
			t.Fatalf("bad snapshot: %+v", got)
		}
		if _, err = s.ResolveChart(context.Background(), ChartRequest{LocalHandle: handle, SourceID: "other"}); err == nil {
			t.Fatal("ambiguous source accepted")
		}
	})
	t.Run("archive rejection", func(t *testing.T) {
		for _, name := range []string{"../Chart.yaml", "demo/../../escape", "/absolute", "demo/..\\escape"} {
			if _, err := loadChart(context.Background(), archive(t, map[string]string{name: "x"})); err == nil {
				t.Fatalf("accepted %q", name)
			}
		}
		if _, err := loadChart(context.Background(), bytes.Repeat([]byte{'x'}, archiveLimit+1)); err == nil {
			t.Fatal("archive cap missing")
		}
		nested := archive(t, map[string]string{"nested/escape": "x"})
		if _, err := loadChart(context.Background(), archive(t, map[string]string{"demo/Chart.yaml": "apiVersion: v2\nname: demo\nversion: 1.0.0\ndependencies:\n- name: absent\n  version: 1.0.0\n  repository: https://unused.invalid\n", "demo/charts/nested.tgz": string(nested)})); err == nil {
			t.Fatal("missing dependency accepted")
		}
	})
	t.Run("private HTTPS digest and redirect isolation", func(t *testing.T) {
		var leaked atomic.Bool
		other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			leaked.Store(r.Header.Get("Authorization") != "")
			w.Write(raw)
		}))
		defer other.Close()
		sum := sha256.Sum256(raw)
		repo := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok || u != "test" || p != "secret" {
				w.WriteHeader(401)
				return
			}
			switch r.URL.Path {
			case "/index.yaml":
				fmt.Fprintf(w, "apiVersion: v1\nentries:\n  demo:\n  - name: demo\n    version: 1.0.0\n    digest: %x\n    urls: [chart.tgz]\n", sum)
			case "/chart.tgz":
				http.Redirect(w, r, other.URL+"/chart.tgz", http.StatusFound)
			}
		}))
		defer repo.Close()
		ca := filepath.Join(t.TempDir(), "ca.pem")
		writeTestCA(t, ca, repo.Certificate(), other.Certificate())
		s := NewService(nil)
		defer s.Close()
		if err := s.SaveSource(Source{ID: "repo", Name: "test", Kind: "https", URL: repo.URL, CAFile: ca}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ListCharts(context.Background(), "repo", "", ""); err == nil {
			t.Fatal("anonymous private index accepted")
		}
		s.SetCredentials("repo", SourceCredentials{Username: "test", Password: "secret"})
		page, err := s.ListCharts(context.Background(), "repo", "demo", "")
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("index: %+v %v", page, err)
		}
		chart, err := s.ResolveChart(context.Background(), ChartRequest{SourceID: "repo", Chart: "demo", Version: "1.0.0"})
		if err != nil {
			t.Fatal(err)
		}
		if leaked.Load() || chart.Name != "demo" {
			t.Fatal("credential leak or wrong chart")
		}
		s.ForgetCredentials("repo")
		if _, err = s.ResolveChart(context.Background(), ChartRequest{SourceID: "repo", Chart: "demo", Version: "1.0.0"}); err == nil {
			t.Fatal("forgotten credential still used")
		}
	})
	t.Run("OCI exact digest without ambient credentials", func(t *testing.T) {
		t.Setenv("HELM_REGISTRY_CONFIG", filepath.Join(t.TempDir(), "must-not-read"))
		manifest, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json", "config": map[string]any{"mediaType": "application/vnd.cncf.helm.config.v1+json", "digest": chartHash([]byte("{}")), "size": 2}, "layers": []map[string]any{{"mediaType": "application/vnd.cncf.helm.chart.content.v1.tar+gzip", "digest": chartHash(raw), "size": len(raw)}}})
		var expired atomic.Bool
		repo := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, _ := r.BasicAuth()
			if expired.Load() || user != "test" || pass != "secret" {
				w.Header().Set("WWW-Authenticate", `Basic realm="chart"`)
				w.WriteHeader(401)
				return
			}
			if strings.Contains(r.URL.Path, "/manifests/") {
				w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
				w.Header().Set("Docker-Content-Digest", chartHash(manifest))
				w.Header().Set("Content-Length", fmt.Sprint(len(manifest)))
				if r.Method != "HEAD" {
					w.Write(manifest)
				}
			} else if strings.Contains(r.URL.Path, "/blobs/") {
				w.Write(raw)
			} else {
				w.WriteHeader(404)
			}
		}))
		defer repo.Close()
		ca := filepath.Join(t.TempDir(), "ca.pem")
		writeTestCA(t, ca, repo.Certificate())
		s := NewService(nil)
		defer s.Close()
		if err := s.SaveSource(Source{ID: "oci", Name: "OCI", Kind: "oci", URL: strings.Replace(repo.URL, "https:", "oci:", 1), CAFile: ca}); err != nil {
			t.Fatal(err)
		}
		s.SetCredentials("oci", SourceCredentials{Username: "test", Password: "secret"})
		req := ChartRequest{SourceID: "oci", Chart: "demo", Digest: chartHash(manifest)}
		got, err := s.ResolveChart(context.Background(), req)
		if err != nil || got.Digest != chartHash(raw) {
			t.Fatalf("OCI: %+v %v", got, err)
		}
		expired.Store(true)
		if _, err := s.ResolveChart(context.Background(), req); err == nil {
			t.Fatal("expired credential accepted")
		}
	})
	t.Run("downgrade and idle expiry", func(t *testing.T) {
		var touched atomic.Bool
		insecure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { touched.Store(true) }))
		defer insecure.Close()
		repo := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, insecure.URL, 302) }))
		defer repo.Close()
		ca := filepath.Join(t.TempDir(), "ca.pem")
		writeTestCA(t, ca, repo.Certificate())
		s := NewService(nil)
		defer s.Close()
		s.SaveSource(Source{ID: "repo", Name: "repo", Kind: "https", URL: repo.URL, CAFile: ca})
		if _, err := s.ListCharts(context.Background(), "repo", "", ""); err == nil || touched.Load() {
			t.Fatal("HTTP downgrade followed")
		}
		c, _ := s.reserve("", 0)
		s.mu.Lock()
		c.ExpiresAt = time.Now().Add(-time.Second)
		s.expireLocked()
		s.mu.Unlock()
		if _, err := s.ResolveChart(context.Background(), ChartRequest{LocalHandle: c.ID}); err == nil {
			t.Fatal("expired handle accepted")
		}
	})
	t.Run("symlinks nested expansion and entry limits", func(t *testing.T) {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: "demo/link", Typeflag: tar.TypeSymlink, Linkname: "/tmp/elsewhere"})
		tw.Close()
		gz.Close()
		if _, err := loadChart(context.Background(), b.Bytes()); err == nil {
			t.Fatal("symlink accepted")
		}
		nested := archive(t, map[string]string{"nested/large": strings.Repeat("x", 1<<20)})
		outer := archive(t, map[string]string{"demo/charts/nested.tgz": string(nested)})
		if err := inspectArchive(context.Background(), outer, 0, &archiveBudget{bytes: 1 << 20}); err == nil {
			t.Fatal("embedded expansion ignored")
		}
		if err := inspectArchive(context.Background(), raw, 0, &archiveBudget{bytes: chartExpandedLimit, entries: 4096}); err == nil {
			t.Fatal("entry limit ignored")
		}
	})

	t.Run("client certificate and index limits", func(t *testing.T) {
		var mode atomic.Int32
		repo := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(r.TLS.PeerCertificates) == 0 {
				w.WriteHeader(401)
				return
			}
			switch mode.Load() {
			case 1:
				w.Header().Set("Content-Length", fmt.Sprint(indexLimit+1))
			case 2:
				fmt.Fprint(w, "entries:\n  demo:\n")
				for i := 0; i <= 10000; i++ {
					fmt.Fprintf(w, "  - name: demo\n    version: 1.0.%d\n    urls: [chart.tgz]\n", i)
				}
			default:
				fmt.Fprint(w, "entries:\n  demo:\n  - name: demo\n    version: 1.0.0\n    urls: [chart.tgz]\n")
			}
		}))
		repo.TLS = &tls.Config{ClientAuth: tls.RequireAnyClientCert, MinVersion: tls.VersionTLS12}
		repo.StartTLS()
		defer repo.Close()
		dir := t.TempDir()
		ca, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
		writeTestCA(t, ca, repo.Certificate())
		key, err := x509.MarshalPKCS8PrivateKey(repo.TLS.Certificates[0].PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
			t.Fatal(err)
		}
		s := NewService(nil)
		defer s.Close()
		if err := s.SaveSource(Source{ID: "mtls", Name: "mtls", Kind: "https", URL: repo.URL, CAFile: ca, CertFile: ca, KeyFile: keyPath}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ListCharts(context.Background(), "mtls", "", ""); err != nil {
			t.Fatal(err)
		}
		for _, value := range []int32{1, 2} {
			mode.Store(value)
			if _, err := s.ListCharts(context.Background(), "mtls", "", ""); err == nil {
				t.Fatalf("index limit %d ignored", value)
			}
		}
	})

}

func writeTestCA(t *testing.T, path string, certs ...*x509.Certificate) {
	t.Helper()
	var raw []byte
	for _, cert := range certs {
		raw = append(raw, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})...)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
