//go:build integration

/*
Purpose: Verify actual Pod exec and fixed Pod/Service forwards in local kind.
Used by: Opt-in Phase 1D.6 verification on the two approved contexts.
Responsibilities: Exercise terminal input/resize, allocated TCP ports and cleanup.
Important: All workload fixtures are synthetic and removed by namespace UID.
*/
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
)

func TestInteractiveSessions(t *testing.T) {
	if os.Getenv("KUBESIFT_INTERACTIVE_INTEGRATION") != "1" {
		t.Skip("explicit interactive session opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			manager := connection.NewManager(nil)
			t.Cleanup(manager.CloseAll)
			ref, client := openLocalOperationFixture(t, context.Background(), manager, dir, suffix, "")
			ns := createInteractiveFixtures(t, ctx, client)
			t.Logf("kind-dev-local-%s fixture %s", suffix, ns)
			events := make(chan model.Event, 128)
			var sessions *session.Manager
			sessions = session.NewManager(func(event model.Event) {
				if event.Kind == "data" {
					_ = sessions.Acknowledge(event.Session, event.Sequence)
				}
				select {
				case events <- event:
				case <-ctx.Done():
				}
			})
			t.Cleanup(sessions.Close)
			s := kube.NewService(manager, sessions)
			const podName = "interactive-one"
			pod, err := client.CoreV1().Pods(ns).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			resource := kube.ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Pod", Namespace: ns, Name: podName, UID: string(pod.UID)}
			t.Run("terminal and resize", func(t *testing.T) {
				req := kube.TerminalRequest{Target: kube.PodSessionTarget{Resource: resource, Container: "shell"}, Command: []string{"/bin/sh"}}
				entry, err := s.OpenTerminal(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
				defer s.CloseInteractive(entry.Session)
				if err := s.Resize(entry.Session, 97, 31); err != nil {
					t.Fatal(err)
				}
				if err := sessions.Activate(entry.Session); err != nil {
					t.Fatal(err)
				}
				waitInteractiveState(t, ctx, events, entry.Session, "running")
				if err := s.Input(entry.Session, []byte("printf 'KS-PTY:%s:' \"$HOSTNAME\"; stty size\n")); err != nil {
					t.Fatal(err)
				}
				waitInteractiveText(t, ctx, events, entry.Session, "KS-PTY:interactive-one:31 97")
				if err := s.Input(entry.Session, []byte("exit\n")); err != nil {
					t.Fatal(err)
				}
				waitInteractiveState(t, ctx, events, entry.Session, "ended")
				req.Command = []string{"/kubesift-missing-executable"}
				missing, err := s.OpenTerminal(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
				defer s.CloseInteractive(missing.Session)
				if err := sessions.Activate(missing.Session); err != nil {
					t.Fatal(err)
				}
				if problem := waitInteractiveState(t, ctx, events, missing.Session, "failed").Problem; problem == nil {
					t.Fatal("missing command did not produce a safe failure")
				}
			})

			svc, err := client.CoreV1().Services(ns).Get(ctx, "interactive-web", metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			serviceRef := kube.ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Service", Namespace: ns, Name: svc.Name, UID: string(svc.UID)}
			selection := kube.ServiceForwardSelection{Resource: serviceRef, ResourceVersion: svc.ResourceVersion, PortName: "http", Port: 80}
			candidates, err := s.ReadForwardCandidates(ctx, kube.ForwardCandidateRequest{Service: serviceRef, ServiceVersion: svc.ResourceVersion, PortName: "http", Port: 80})
			if err != nil || !candidates.Complete || len(candidates.Items) != 2 {
				t.Fatalf("unexpected Service candidates: %+v %v", candidates, err)
			}
			var chosen kube.ForwardCandidate
			for _, candidate := range candidates.Items {
				if candidate.Pod.Name == "interactive-two" {
					chosen = candidate
				}
			}
			if chosen.RemotePort != 8081 {
				t.Fatalf("wrong named-port mapping %+v", chosen)
			}
			for _, fixture := range []struct {
				name, body string
				request    kube.ForwardRequest
			}{
				{"direct Pod", "kubesift-interactive-one", kube.ForwardRequest{Target: kube.PodSessionTarget{Resource: resource}, RemotePort: 8080}},
				{"fixed Service backend", "kubesift-interactive-two", kube.ForwardRequest{Target: kube.PodSessionTarget{Resource: chosen.Pod}, RemotePort: chosen.RemotePort, Service: &selection}},
			} {
				t.Run(fixture.name, func(t *testing.T) {
					entry, err := s.OpenForward(ctx, fixture.request)
					if err != nil || entry.LocalPort != 0 || entry.State != "connecting" {
						t.Fatalf("pending forward: %+v %v", entry, err)
					}
					defer s.CloseInteractive(entry.Session)
					if err := sessions.Activate(entry.Session); err != nil {
						t.Fatal(err)
					}
					ready := waitInteractiveState(t, ctx, events, entry.Session, "listening")
					if ready.LocalPort == 0 {
						t.Fatal("no allocated local port")
					}
					address := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(ready.LocalPort)))
					readForward(t, ctx, address, fixture.body)
					conflict := fixture.request
					conflict.LocalPort = ready.LocalPort
					duplicate, err := s.OpenForward(ctx, conflict)
					if err != nil {
						t.Fatal(err)
					}
					defer s.CloseInteractive(duplicate.Session)
					if err := sessions.Activate(duplicate.Session); err != nil {
						t.Fatal(err)
					}
					if problem := waitInteractiveState(t, ctx, events, duplicate.Session, "failed").Problem; problem == nil || problem.Code != "port_conflict" {
						t.Fatalf("occupied local port did not fail clearly: %+v", problem)
					}
					if fixture.request.Service == nil {
						if err := s.StopInteractive(entry.Session); err != nil {
							t.Fatal(err)
						}
					} else if err := s.CloseInteractive(entry.Session); err != nil {
						t.Fatal(err)
					}
					conn, err := net.DialTimeout("tcp4", address, time.Second)
					if err == nil {
						conn.Close()
						t.Fatal("forward still listening after cleanup")
					}
				})
			}
			if stats := sessions.Stats(); stats.Sessions != 0 || stats.QueuedBytes != 0 || stats.InFlightBytes != 0 {
				t.Fatalf("native session resources remain: %+v", stats)
			}
		})
	}
}

func createInteractiveFixtures(t *testing.T, ctx context.Context, client kubernetes.Interface) string {
	t.Helper()
	namespaces := client.CoreV1().Namespaces()
	ns, err := namespaces.Create(ctx, &core.Namespace{ObjectMeta: metav1.ObjectMeta{
		GenerateName: "kubesift-interactive-check-", Labels: map[string]string{"kubesift.dev/fixture": "phase1d6-validation"},
	}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if err := namespaces.Delete(cleanup, ns.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}}); err != nil {
			t.Error(err)
			return
		}
		awaitDeletionFixture(t, cleanup, "interactive fixture namespace removal", func(ctx context.Context) (bool, error) {
			_, err := namespaces.Get(ctx, ns.Name, metav1.GetOptions{})
			return apierrors.IsNotFound(err), nil
		})
	})
	input, err := os.Open("../fixtures/explorer/phase-1d6.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	decoder := yaml.NewYAMLOrJSONDecoder(input, 4096)
	for {
		var raw runtime.RawExtension
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var typeMeta metav1.TypeMeta
		if err = json.Unmarshal(raw.Raw, &typeMeta); err != nil {
			t.Fatal(err)
		}
		switch typeMeta.Kind {
		case "Pod":
			var pod core.Pod
			if err = json.Unmarshal(raw.Raw, &pod); err != nil {
				t.Fatal(err)
			}
			_, err = client.CoreV1().Pods(ns.Name).Create(ctx, &pod, metav1.CreateOptions{})
		case "Service":
			var svc core.Service
			if err = json.Unmarshal(raw.Raw, &svc); err != nil {
				t.Fatal(err)
			}
			_, err = client.CoreV1().Services(ns.Name).Create(ctx, &svc, metav1.CreateOptions{})
		default:
			t.Fatal("unexpected fixture kind", typeMeta.Kind)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"interactive-one", "interactive-two"} {
		awaitDeletionFixture(t, ctx, "ready interactive Pod "+name, func(ctx context.Context) (bool, error) {
			pod, err := client.CoreV1().Pods(ns.Name).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			for _, condition := range pod.Status.Conditions {
				if condition.Type == core.PodReady && condition.Status == core.ConditionTrue {
					return true, nil
				}
			}
			return false, nil
		})
	}
	return ns.Name
}

type interactiveEvidence struct {
	State     string
	LocalPort uint16
	Problem   *model.Problem
}

func waitInteractiveState(t *testing.T, ctx context.Context, events <-chan model.Event, ref model.SessionRef, want string) interactiveEvidence {
	t.Helper()
	timer := time.NewTimer(25 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Session != ref || event.Kind != "state" {
				continue
			}
			var evidence interactiveEvidence
			if json.Unmarshal(event.Data, &evidence) != nil {
				continue
			}
			if evidence.State == want {
				return evidence
			}
			if evidence.State == "failed" || evidence.State == "ended" {
				t.Fatalf("wanted %s; got %+v", want, evidence)
			}
		case <-timer.C:
			t.Fatal("timed out waiting for " + want)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

func waitInteractiveText(t *testing.T, ctx context.Context, events <-chan model.Event, ref model.SessionRef, want string) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	text := ""
	for {
		select {
		case event := <-events:
			if event.Session == ref && event.Kind == "data" {
				text += string(event.Data)
				if strings.Contains(text, want) {
					return
				}
				if len(text) > 16<<10 {
					t.Fatal("fixture rejected expected terminal input/resize")
				}
			}
		case <-timer.C:
			t.Fatalf("terminal fixture interpreted input or resize incorrectly: %q", text)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

func readForward(t *testing.T, ctx context.Context, address, want string) {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s/", address), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil || resp.StatusCode != 200 || strings.TrimSpace(string(raw)) != want {
		t.Fatalf("forwarded response: %d, %q, %v", resp.StatusCode, raw, err)
	}
}
