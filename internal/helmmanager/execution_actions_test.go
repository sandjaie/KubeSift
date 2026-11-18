/*
Purpose: Exercise cancellation through the pinned Helm action implementations.
Used by: The Helm execution test group.
Responsibilities: Check context-aware and Run-only actions with a context-owned SDK waiter.
Important: Memory storage and a synthetic Kubernetes client avoid cluster mutation.
*/
package helmmanager

import (
	"context"
	"io"
	"log/slog"
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
	"k8s.io/client-go/rest"
)

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
			case <-time.After(3 * time.Second):
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
