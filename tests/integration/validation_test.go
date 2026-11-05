//go:build integration

/*
Purpose: Validate the Phase 1A adapters against only explicitly prepared local fixtures.
Used by: The opt-in integration command in the approved plan.
Responsibilities: Check distinct A/B markers, log routing, denied credentials, forwarding and owned helper authentication.
Important: Require the isolated kubeconfig and exact mapping; never fall back to ambient contexts or use provider credentials.
*/
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	"github.com/sandjaie/KubeSift/internal/trust"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLocalValidation(t *testing.T) {
	path := os.Getenv("KUBESIFT_TEST_KUBECONFIG")
	if path == "" || !filepath.IsAbs(path) {
		t.Fatal("explicit absolute KUBESIFT_TEST_KUBECONFIG required")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	type identity struct {
		Context   string `json:"context"`
		Namespace string `json:"namespace"`
		Server    string `json:"server"`
	}
	var mapping struct {
		Version int      `json:"version"`
		RunID   string   `json:"runId"`
		A       identity `json:"a"`
		B       identity `json:"b"`
	}
	data, err := os.ReadFile(path + ".targets.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&mapping); err != nil {
		t.Fatal(err)
	}
	if mapping.Version != 1 || mapping.RunID == "" || mapping.A.Context != "kind-dev-local" || mapping.B.Context != "kind-kubesift-b" || mapping.A.Namespace != "payments" || mapping.B.Namespace != "payments" || mapping.A.Server == mapping.B.Server {
		t.Fatal("unrecognized or aliased fixture mapping")
	}
	for _, target := range []identity{mapping.A, mapping.B} {
		endpoint, e := url.Parse(target.Server)
		if e != nil {
			t.Fatal(e)
		}
		ip := net.ParseIP(endpoint.Hostname())
		if ip == nil || !ip.IsLoopback() || endpoint.Scheme != "https" {
			t.Fatal("only explicit TLS loopback fixture endpoints allowed")
		}
	}
	snapshot, err := config.Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"kind-dev-local": true, "kind-kubesift-b": true, "phase1a-denied": true, "phase1a-helper": true}
	for _, summary := range snapshot.Contexts() {
		if !allowed[summary.Name] {
			t.Fatal("unrecognized context in isolated fixture source")
		}
	}
	store, err := trust.OpenStore(filepath.Join(t.TempDir(), "trust.json"))
	if err != nil {
		t.Fatal(err)
	}
	guard := trust.NewGuard(store)
	connections := connection.NewManager(guard)
	events := make(chan model.Event, 64)
	sessions := session.NewManager(func(event model.Event) {
		select {
		case events <- event:
		default:
		}
	})
	sessions.SetValidate(func(target model.Target) error { return connections.Validate(target.Connection) })
	connections.SetCleanup(sessions.StopConnection)
	defer func() { connections.CloseAll(); sessions.Close() }()
	service := kube.NewService(connections, sessions)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	refs := map[string]model.ConnectionRef{}
	for _, target := range []identity{mapping.A, mapping.B} {
		resolved, e := snapshot.Resolve(target.Context)
		if e != nil {
			t.Fatal(e)
		}
		if resolved.RESTConfig().Host != target.Server {
			t.Fatal("configuration disagrees with explicit mapping")
		}
		ref, e := connections.Open(ctx, resolved)
		if e != nil {
			t.Fatal(e)
		}
		refs[target.Context] = ref
	}
	t.Run("distinct-markers-and-logs", func(t *testing.T) {
		for _, target := range []struct{ context, marker string }{{mapping.A.Context, "cluster-a"}, {mapping.B.Context, "cluster-b"}} {
			ref := refs[target.context]
			pods, e := service.Pods(ctx, ref, "payments")
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, pod := range pods {
				if pod.Name == "ks-validation" {
					found = true
					if pod.Annotations["kubesift.dev/marker"] != target.marker || pod.Labels["kubesift.dev/run"] != mapping.RunID {
						t.Fatal("wrong cluster marker or fixture ownership")
					}
				}
			}
			if !found {
				t.Fatal("owned Pod absent")
			}
			logs, e := service.Logs(ctx, model.Target{Connection: ref, Namespace: "payments", Name: "ks-validation", Container: "fixture"}, false, 2)
			if e != nil {
				t.Fatal(e)
			}
			if e = sessions.Activate(logs); e != nil {
				t.Fatal(e)
			}
			matched := false
			deadline := time.NewTimer(10 * time.Second)
			for !matched {
				select {
				case event := <-events:
					if event.Session == logs && event.Kind == "data" && bytes.Contains(event.Data, []byte(target.marker)) {
						matched = true
					}
				case <-deadline.C:
					t.Fatal("expected marker missing from logs")
				}
			}
			deadline.Stop()
			if e = sessions.Stop(logs); e != nil {
				t.Fatal(e)
			}
		}
	})
	t.Run("denied-identity", func(t *testing.T) {
		resolved, e := snapshot.Resolve("phase1a-denied")
		if e != nil {
			t.Fatal(e)
		}
		ref, e := connections.Open(ctx, resolved)
		if e != nil {
			t.Fatal(e)
		}
		_, e = service.Pods(ctx, ref, "payments")
		var safe *model.Error
		if !errors.As(e, &safe) || safe.Code != "forbidden" {
			t.Fatalf("expected forbidden, got %v", e)
		}
	})
	t.Run("loopback-forward", func(t *testing.T) {
		forward, e := service.Forward(ctx, model.Target{Connection: refs[mapping.A.Context], Namespace: "payments", Name: "ks-validation", Container: "fixture"}, 0, 8080)
		if e != nil {
			t.Fatal(e)
		}
		address := net.JoinHostPort("127.0.0.1", fmtPort(forward.LocalPort))
		client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
		response, e := client.Get("http://" + address + "/marker")
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(io.LimitReader(response.Body, 1024))
		response.Body.Close()
		if e != nil || strings.TrimSpace(string(body)) != "cluster-a" {
			t.Fatal("forward returned wrong marker")
		}
		if e = sessions.Stop(forward.Session); e != nil {
			t.Fatal(e)
		}
		listener, e := net.DialTimeout("tcp", address, time.Second)
		if e == nil {
			listener.Close()
			t.Fatal("forward listener survived stop")
		}
	})

	t.Run("malformed-terminal-input", func(t *testing.T) {
		if e := service.Input(model.SessionRef{ID: "unknown", Connection: refs[mapping.A.Context]}, nil); e == nil {
			t.Fatal("unknown terminal accepted an empty input request")
		}
	})
	t.Run("terminal-roundtrip", func(t *testing.T) {
		terminal, e := service.Exec(ctx, model.Target{Connection: refs[mapping.A.Context], Namespace: "payments", Name: "ks-validation", Container: "fixture"})
		if e != nil {
			t.Fatal(e)
		}
		defer sessions.Stop(terminal)
		if e = sessions.Activate(terminal); e != nil {
			t.Fatal(e)
		}
		if e = service.Resize(terminal, 80, 24); e != nil {
			t.Fatal(e)
		}
		if e = service.Input(terminal, []byte("printf '__KS_%s__\\n' 'ready'\n")); e != nil {
			t.Fatal(e)
		}
		output := []byte{}
		deadline := time.NewTimer(10 * time.Second)
		defer deadline.Stop()
		for !bytes.Contains(output, []byte("__KS_ready__")) {
			select {
			case event := <-events:
				if event.Session == terminal && event.Kind == "data" {
					output = append(output, event.Data...)
					if len(output) > 16<<10 {
						t.Fatal("terminal roundtrip output exceeded validation bound")
					}
					if e = sessions.Acknowledge(terminal, event.Sequence); e != nil {
						t.Fatal(e)
					}
				}
				if event.Session == terminal && event.Kind == "error" {
					t.Fatalf("terminal failed: %s", event.Data)
				}
			case <-deadline.C:
				t.Fatal("terminal did not return the command marker")
			}
		}
	})
	t.Run("owned-helper", func(t *testing.T) {
		resolved, e := snapshot.Resolve("phase1a-helper")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = connections.Open(ctx, resolved); e == nil {
			t.Fatal("untrusted helper opened")
		}
		preview, e := guard.Review(resolved)
		if e != nil {
			t.Fatal(e)
		}
		if e = guard.ApprovePreview(preview.ID, resolved); e != nil {
			t.Fatal(e)
		}
		ref, e := connections.Open(ctx, resolved)
		if e != nil {
			t.Fatal(e)
		}
		pods, e := service.Pods(ctx, ref, "payments")
		if e != nil || len(pods) == 0 {
			t.Fatalf("owned helper authentication failed: %v", e)
		}
		if e = guard.Revoke(preview.Fingerprint); e != nil {
			t.Fatal(e)
		}
		if _, e = service.Pods(ctx, ref, "payments"); e == nil {
			t.Fatal("revoked helper executed")
		}
	})
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("isolated kubeconfig mutated")
	}
}
func fmtPort(port uint16) string { return strconv.Itoa(int(port)) }
