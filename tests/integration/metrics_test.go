//go:build integration

/*
Purpose: Compare current Pod/Node metrics with their exact captured live API samples.
Used by: Opt-in Phase 1E.3 validation on the two approved local kind contexts.
Responsibilities: Verify explicit identities, exact quantities, provider absence and restricted RBAC.
Important: The recording proxy forwards only named read paths; fixtures are provisioned separately.
*/
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/session"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCurrentMetrics(t *testing.T) {
	if os.Getenv("KUBESIFT_METRICS_INTEGRATION") != "1" {
		t.Skip("explicit metrics opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	manager := connection.NewManager(nil)
	defer manager.CloseAll()
	sessions := session.NewManager(nil)
	defer sessions.Close()
	service := kube.NewService(manager, sessions)
	var firstUID string
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			ref, client := openLocalOperationFixture(t, ctx, manager, dir, suffix, "")
			ns, err := client.CoreV1().Namespaces().Get(ctx, "kubesift-metrics-check", metav1.GetOptions{})
			if err != nil || ns.Labels["kubesift.dev/fixture"] != "phase1e3" {
				t.Fatal("fixture ownership unavailable", err)
			}
			pod, err := client.CoreV1().Pods(ns.Name).Get(ctx, "current-usage", metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if string(pod.UID) == firstUID {
				t.Fatal("same-name fixture identities unexpectedly match")
			}
			firstUID = string(pod.UID)
			target := kube.ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Pod", Namespace: ns.Name, Name: pod.Name, UID: string(pod.UID)}
			if suffix == "b" {
				batch, err := service.ReadCurrentMetrics(ctx, kube.MetricsRequest{Connection: ref, Targets: []kube.ResourceRef{target}})
				if err != nil || batch.Problem == nil || batch.Problem.Code != "unsupported" {
					t.Fatalf("expected absent API: %+v %v", batch, err)
				}
				t.Log("B: independent same-name UID; no Metrics API; ordinary Pod read succeeds")
				return
			}
			nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
			if err != nil || len(nodes.Items) != 1 {
				t.Fatal("node fixture unavailable", err)
			}
			node := nodes.Items[0]
			nodeTarget := kube.ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Node", Name: node.Name, UID: string(node.UID)}
			cfg, err := manager.RESTConfig(ref)
			if err != nil {
				t.Fatal(err)
			}
			upstream, err := kube.BoundedHTTPClient(cfg)
			if err != nil {
				t.Fatal(err)
			}
			allowed := map[string]bool{"/apis/metrics.k8s.io": true, "/apis/metrics.k8s.io/v1beta1": true, "/apis/metrics.k8s.io/v1": true,
				"/api/v1/namespaces/" + ns.Name + "/pods/" + pod.Name: true, "/api/v1/nodes/" + node.Name: true}
			for _, v := range []string{"v1", "v1beta1"} {
				allowed["/apis/metrics.k8s.io/"+v+"/namespaces/"+ns.Name+"/pods/"+pod.Name] = true
				allowed["/apis/metrics.k8s.io/"+v+"/nodes/"+node.Name] = true
			}
			var mu sync.Mutex
			captures := map[string][]byte{}
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || !allowed[r.URL.Path] {
					w.WriteHeader(403)
					return
				}
				req, e := http.NewRequestWithContext(r.Context(), "GET", cfg.Host+r.URL.Path, nil)
				if e != nil {
					t.Error(e)
					return
				}
				req.Header.Set("Accept", "application/json")
				response, e := upstream.Do(req)
				if e != nil {
					w.WriteHeader(502)
					return
				}
				defer response.Body.Close()
				raw, e := io.ReadAll(io.LimitReader(response.Body, 16<<20))
				if e != nil {
					w.WriteHeader(502)
					return
				}
				mu.Lock()
				captures[r.URL.Path] = raw
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(response.StatusCode)
				_, _ = w.Write(raw)
			}))
			defer proxy.Close()
			rawConfig := fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"recording","cluster":{"server":%q}}],"contexts":[{"name":"recording-a","context":{"cluster":"recording","user":"recording"}}],"users":[{"name":"recording","user":{}}]}`, proxy.URL)
			path := filepath.Join(t.TempDir(), "recording.kubeconfig")
			if err = os.WriteFile(path, []byte(rawConfig), 0600); err != nil {
				t.Fatal(err)
			}
			snapshot, err := config.Load([]string{path})
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := snapshot.Resolve("recording-a")
			if err != nil {
				t.Fatal(err)
			}
			recordedRef, err := manager.Open(ctx, resolved)
			if err != nil {
				t.Fatal(err)
			}
			target.Connection, nodeTarget.Connection = recordedRef, recordedRef
			batch, err := service.ReadCurrentMetrics(ctx, kube.MetricsRequest{Connection: recordedRef, Targets: []kube.ResourceRef{target, nodeTarget}, Detail: &target})
			if err != nil || len(batch.Items) != 2 {
				t.Fatalf("metrics read: %+v %v", batch, err)
			}
			for _, item := range batch.Items {
				if item.Problem != nil || item.State != "available" {
					t.Fatalf("live sample unavailable: %+v", item)
				}
				path := "/apis/" + item.APIVersion + "/nodes/" + item.Resource.Name
				if item.Resource.Kind == "Pod" {
					path = "/apis/" + item.APIVersion + "/namespaces/" + ns.Name + "/pods/" + pod.Name
					if len(item.Containers) != 2 {
						t.Fatal("container breakdown missing")
					}
				}
				mu.Lock()
				raw := bytes.Clone(captures[path])
				mu.Unlock()
				var sample struct {
					Timestamp  time.Time         `json:"timestamp"`
					Usage      map[string]string `json:"usage"`
					Containers []struct {
						Usage map[string]string `json:"usage"`
					} `json:"containers"`
				}
				if json.Unmarshal(raw, &sample) != nil || !sample.Timestamp.Equal(*item.SampledAt) {
					t.Fatal("captured sample timestamp differs")
				}
				for key, got := range map[string]*string{"cpu": item.CPU.Amount, "memory": item.Memory.Amount} {
					total := new(big.Rat)
					add := func(text string) {
						q, e := resource.ParseQuantity(text)
						if e != nil {
							t.Fatal(e)
						}
						v, ok := new(big.Rat).SetString(q.AsDec().String())
						if !ok {
							t.Fatal("invalid raw quantity")
						}
						total.Add(total, v)
					}
					if item.Resource.Kind == "Node" {
						add(sample.Usage[key])
					} else {
						for _, c := range sample.Containers {
							add(c.Usage[key])
						}
					}
					if key == "cpu" {
						total.Mul(total, big.NewRat(1000, 1))
					}
					if got == nil {
						t.Fatal("missing live quantity")
					}
					actual, ok := new(big.Rat).SetString(*got)
					if !ok || total.Cmp(actual) != 0 {
						t.Fatalf("%s %s exact mismatch %s vs %s", item.Resource.Kind, key, total, actual)
					}
				}
				t.Logf("A %s: CPU=%sm memory=%sB sample=%s API=%s", item.Resource.Kind, *item.CPU.Amount, *item.Memory.Amount, item.SampledAt.Format(time.RFC3339), item.APIVersion)
			}
			deniedRef, deniedClient := openLocalOperationFixture(t, ctx, manager, dir, suffix, "-denied")
			if _, err = deniedClient.CoreV1().Pods(ns.Name).Get(ctx, pod.Name, metav1.GetOptions{}); err != nil {
				t.Fatal("restricted core read should work", err)
			}
			target.Connection = deniedRef
			denied, err := service.ReadCurrentMetrics(ctx, kube.MetricsRequest{Connection: deniedRef, Targets: []kube.ResourceRef{target}})
			if err != nil || len(denied.Items) != 1 || denied.Items[0].Problem == nil || denied.Items[0].Problem.Code != "forbidden" {
				t.Fatalf("expected metrics Forbidden: %+v %v", denied, err)
			}
			t.Log("A restricted identity: core Pod read allowed; metrics Forbidden")
			if _, err = service.ReadCurrentMetrics(ctx, kube.MetricsRequest{Connection: ref, Targets: []kube.ResourceRef{target}}); err == nil {
				t.Fatal("accepted mixed connection identity")
			}
		})
	}
}
