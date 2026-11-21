/*
Purpose: Verify connection-owned historical source configuration.
Used by: Native historical metrics tests.
Responsibilities: Exercise revisions, TLS scope and explicit Route discovery.
Important: All provider data and credentials are synthetic.
*/
package kube

import (
	"context"
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
)

func TestHistorySourceRevision(t *testing.T) {
	s, ref := workloadServer(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected probe") })
	initial, err := s.HistorySource(ref)
	if err != nil {
		t.Fatal(err)
	}
	cfg := HistoryConfig{Origin: "https://monitor.example", Mode: "cluster", SingleCluster: true, Bearer: "test-only"}
	set, err := s.SetHistorySource(ref, initial.Revision, cfg)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(set)
	if strings.Contains(string(raw), "test-only") || !set.Authenticated {
		t.Fatal("credential projection")
	}
	for _, url := range []string{"http://host", "https://user@host", "https://host/path", "https://host?", "https://host#", "https://host:70000"} {
		bad := cfg
		bad.Origin = url
		if _, err := s.SetHistorySource(ref, set.Revision, bad); err == nil {
			t.Errorf("accepted invalid origin %s", url)
		}
	}
	empty, err := s.ClearHistorySource(ref, set.Revision)
	if err != nil || empty.Revision == set.Revision || empty.Origin != "" {
		t.Fatal(empty, err)
	}
	if _, err = s.SetHistorySource(ref, initial.Revision, cfg); err == nil {
		t.Fatal("accepted stale revision")
	}
}
func TestHistorySourceRoute(t *testing.T) {
	for _, admitted := range []string{"True", "False"} {
		s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/apis/route.openshift.io/v1/namespaces/openshift-monitoring/routes/thanos-querier" {
				t.Error("unexpected route read")
			}
			fmt.Fprintf(w, `{"spec":{"host":"thanos.example","tls":{"termination":"reencrypt"}},"status":{"ingress":[{"host":"thanos.example","conditions":[{"type":"Admitted","status":%q}]}]}}`, admitted)
		})
		origin, err := s.SuggestHistorySource(context.Background(), ref)
		if admitted == "True" && (origin != "https://thanos.example" || err != nil) || admitted == "False" && err == nil {
			t.Fatal(origin, err)
		}
	}
}

func TestHistorySourceTransport(t *testing.T) {
	s, ref := workloadServer(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected core read") })
	var forwarded, calls atomic.Int32
	other := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded.Add(1) }))
	defer other.Close()
	first := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Redirect(w, r, other.URL, 302) }))
	defer first.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: first.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := HistoryConfig{Origin: first.URL, CAFile: ca, Bearer: "fixture", Mode: "project", Cluster: "synthetic"}
	view, err := s.SetHistorySource(ref, "", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("probed before read")
	}
	h := s.history[ref]
	request, _ := http.NewRequest("GET", view.Origin, nil)
	request.Header.Set("Authorization", "Bearer "+h.bearer)
	response, err := h.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 302 || forwarded.Load() != 0 {
		t.Fatal("redirect followed")
	}
	transport := h.client.Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.ServerName = "unrelated.invalid"
	mismatch := &http.Client{Transport: transport}
	if r, err := mismatch.Get(first.URL); err == nil {
		r.Body.Close()
		t.Fatal("accepted wrong hostname")
	}
	bad := cfg
	bad.Cluster = ""
	if _, err = s.SetHistorySource(ref, view.Revision, bad); err == nil {
		t.Fatal("empty shared label")
	}
	if err = os.WriteFile(ca, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetHistorySource(ref, view.Revision, cfg); err == nil {
		t.Fatal("invalid CA")
	}
	cfg.CAFile = ""
	cfg.Bearer = ""
	cfg.Origin = other.URL
	wrong := ref
	wrong.Generation++
	if _, err = s.SetHistorySource(wrong, "", cfg); err == nil {
		t.Fatal("unknown generation")
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := s.SetHistorySource(ref, view.Revision, cfg); results <- err }()
	}
	if (<-results == nil) == (<-results == nil) {
		t.Fatal("expected one successful replacement")
	}
	if h.ctx.Err() == nil || s.history[ref].bearer != "" {
		t.Fatal("old source still active")
	}
	s.connections.CloseAll()
	if len(s.history) != 0 {
		t.Fatal("source retained after disconnect")
	}
}

func TestHistorySourceRouteUnavailable(t *testing.T) {
	for _, code := range []int{403, 404, 200} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			s, ref := workloadServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
				fmt.Fprint(w, `{"spec":{"host":"bad/host","tls":{"termination":"edge"}},"status":{"ingress":[{"host":"bad/host","conditions":[{"type":"Admitted","status":"True"}]}]}}`)
			})
			if origin, err := s.SuggestHistorySource(context.Background(), ref); err == nil || origin != "" {
				t.Fatal("suggested unusable Route")
			} else if code == 404 && !strings.Contains(err.Error(), "Route") {
				t.Fatal("wrong discovery diagnosis", err)
			}
		})
	}
}
