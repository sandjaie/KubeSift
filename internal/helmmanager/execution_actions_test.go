/*
Purpose: Exercise cancellation through the pinned Helm action implementations.
Used by: The Helm execution test group.
Responsibilities: Check context-aware and Run-only actions with a context-owned SDK waiter.
Important: Memory storage and a synthetic Kubernetes client avoid cluster mutation.
*/
package helmmanager

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/helmreader"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	helmkube "helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/kube/fake"
	stored "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
)

func TestUninstallLostDeleteResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/version":
			fmt.Fprint(w, `{"major":"1","minor":"37","gitVersion":"v1.37.0"}`)
		case "/api":
			fmt.Fprint(w, `{"kind":"APIVersions","versions":["v1"]}`)
		case "/apis":
			fmt.Fprint(w, `{"kind":"APIGroupList","groups":[]}`)
		case "/api/v1":
			fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true,"verbs":["get","delete"]}]}`)
		case "/api/v1/namespaces/owned/configmaps/demo":
			if req.Method == http.MethodDelete {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				conn.Close()
				return
			}
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"demo","namespace":"owned","labels":{"app.kubernetes.io/managed-by":"Helm"},"annotations":{"meta.helm.sh/release-name":"demo","meta.helm.sh/release-namespace":"owned"}}}`)
		default:
			t.Errorf("unexpected SDK request: %s %s", req.Method, req.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	ctx := context.Background()
	transport := &operationTransport{ctx: ctx, next: http.DefaultTransport}
	config := &rest.Config{Host: server.URL, Transport: transport}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	getter := &nativeGetter{config: config, namespace: "owned", discovery: memory.NewMemCacheClient(discoveryClient)}
	store := storage.Init(driver.NewMemory())
	ch, _ := loadChart(ctx, tinyChart(t))
	rel := &stored.Release{Name: "demo", Namespace: "owned", Version: 1, Info: &stored.Info{Status: "deployed"}, Chart: ch.(*chartv2.Chart), Manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\n"}
	if err := store.Create(rel); err != nil {
		t.Fatal(err)
	}
	cfg := &action.Configuration{KubeClient: helmkube.New(getter), RESTClientGetter: getter, Releases: store, Capabilities: common.DefaultCapabilities.Copy()}
	cfg.SetLogger(slog.DiscardHandler)
	r := &reviewData{Review: Review{Action: "uninstall", Target: Target{ReleaseName: "demo", Namespace: "owned"}, Options: Options{TimeoutSeconds: 60, DisableHooks: true}}}
	err = invokeAction(ctx, cfg, r)
	if err == nil || !transport.writes.Load() {
		t.Fatalf("did not exercise SDK delete: %v", err)
	}
	if !problemCode(transport.outcome(err), "unknown_outcome") {
		t.Fatalf("SDK-masked delete error lost uncertainty: %v", err)
	}
}

type contextWaiter struct {
	ctx     context.Context
	started chan struct{}
}

func (w contextWaiter) wait() error {
	select {
	case w.started <- struct{}{}:
	default:
	}
	<-w.ctx.Done()
	return w.ctx.Err()
}
func (w contextWaiter) Wait(helmkube.ResourceList, time.Duration) error { return w.wait() }
func (w contextWaiter) WaitWithJobs(helmkube.ResourceList, time.Duration) error {
	return w.wait()
}
func (w contextWaiter) WaitForDelete(helmkube.ResourceList, time.Duration) error {
	return w.wait()
}
func (w contextWaiter) WatchUntilReady(helmkube.ResourceList, time.Duration) error {
	return w.wait()
}

type contextKubeClient struct {
	fake.PrintingKubeClient
	waiter contextWaiter
}

func (c *contextKubeClient) GetWaiter(helmkube.WaitStrategy) (helmkube.Waiter, error) {
	return c.waiter, nil
}
func (c *contextKubeClient) GetWaiterWithOptions(helmkube.WaitStrategy, ...helmkube.WaitOption) (helmkube.Waiter, error) {
	return c.waiter, nil
}

func testActionCancellation(t *testing.T) {
	for _, verb := range []string{"install", "upgrade", "rollback", "uninstall"} {
		t.Run(verb, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{}, 1)
			raw := tinyChart(t)
			ch, err := loadChart(ctx, raw)
			if err != nil {
				t.Fatal(err)
			}
			memory := driver.NewMemory()
			for revision := 1; verb != "install" && revision <= 2; revision++ {
				rel := &stored.Release{Name: "demo", Namespace: "owned", Version: revision, Chart: ch.(*chartv2.Chart), Config: map[string]any{}, Manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\n", Info: &stored.Info{Status: "deployed"}}
				if err := storage.Init(memory).Create(rel); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &action.Configuration{KubeClient: &contextKubeClient{PrintingKubeClient: fake.PrintingKubeClient{Out: io.Discard}, waiter: contextWaiter{ctx, started}}, Releases: storage.Init(memory), Capabilities: common.DefaultCapabilities.Copy(), RESTClientGetter: &nativeGetter{config: &rest.Config{Host: "http://unused.invalid"}, namespace: "owned"}}
			cfg.SetLogger(slog.DiscardHandler)
			cfg.Releases.SetLogger(slog.DiscardHandler)
			r := &reviewData{Review: Review{Action: verb, Target: Target{Namespace: "owned", Driver: "secret", ReleaseName: "demo"}, Options: Options{TimeoutSeconds: 60}, Rollback: &helmreader.RecordRef{Revision: "1"}}, raw: raw}
			finished := make(chan error, 1)
			go func() { finished <- invokeAction(ctx, cfg, r) }()
			select {
			case <-started:
			case err := <-finished:
				t.Fatalf("action ended before its waiter: %v", err)
			// Trusted-code compilation is slower under -race; cancellation below must still join within one second.
			case <-time.After(5 * time.Minute):
				t.Fatal("action never entered its waiter")
			}
			cancel()
			select {
			case err := <-finished:
				if err == nil {
					t.Fatal("cancelled action reported success")
				}
			case <-time.After(time.Second):
				t.Fatal("SDK action did not join after cancellation")
			}
		})
	}
}
