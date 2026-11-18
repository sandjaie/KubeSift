/*
Purpose: Build Helm SDK clients from one captured native connection.
Used by: Review preparation and operation workers.
Responsibilities: Own request cancellation, block preview writes and avoid ambient kubeconfig.
Important: Every SDK client, including discovery and storage, shares the guarded transport.
*/
package helmmanager

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync/atomic"

	appkube "github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"helm.sh/helm/v4/pkg/action"
	helmkube "helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/storage"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientapi "k8s.io/client-go/tools/clientcmd/api"
)

type operationTransport struct {
	ctx       context.Context
	next      http.RoundTripper
	readOnly  bool
	origin    string
	writes    atomic.Bool
	ambiguous atomic.Bool
}

type ownedBody struct {
	io.ReadCloser
	done      func()
	ambiguous *atomic.Bool
}

func (t *operationTransport) outcome(err error) error {
	if t.ambiguous.Load() {
		return model.Fail("unknown_outcome", "A Kubernetes write response was lost. Refresh release history before any recovery.")
	}
	return err
}

func (b *ownedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && err != io.EOF && b.ambiguous != nil {
		b.ambiguous.Store(true)
	}
	return n, err
}

func (b *ownedBody) Close() error {
	err := b.ReadCloser.Close()
	b.done()
	return err
}

func (t *operationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.origin != "" && req.URL.Scheme+"://"+req.URL.Host != t.origin {
		return nil, model.Fail("invalid", "Kubernetes redirect left the captured server origin.")
	}
	mutating := req.Method != http.MethodGet && req.Method != http.MethodHead
	if t.readOnly && mutating {
		return nil, model.Fail("unsupported", "Helm preview attempted a mutation and was stopped.")
	}
	if err := t.ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	done := func() { stop(); cancel() }
	if mutating {
		t.writes.Store(true)
	}
	res, err := t.next.RoundTrip(req.Clone(ctx))
	if err != nil {
		if mutating {
			t.ambiguous.Store(true)
		}
		done()
		return nil, err
	}
	body := &ownedBody{ReadCloser: res.Body, done: done}
	if mutating {
		body.ambiguous = &t.ambiguous
	}
	res.Body = body
	return res, nil
}

type nativeGetter struct {
	config    *rest.Config
	namespace string
	discovery discovery.CachedDiscoveryInterface
}

func (g *nativeGetter) ToRESTConfig() (*rest.Config, error) {
	return rest.CopyConfig(g.config), nil
}
func (g *nativeGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return g.discovery, nil
}
func (g *nativeGetter) ToRESTMapper() (meta.RESTMapper, error) {
	if g.discovery == nil {
		client, err := discovery.NewDiscoveryClientForConfig(g.config)
		if err != nil {
			return nil, err
		}
		g.discovery = memory.NewMemCacheClient(client)
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(g.discovery), nil
}
func (g *nativeGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	config := clientapi.Config{CurrentContext: "owned", Contexts: map[string]*clientapi.Context{"owned": {Namespace: g.namespace}}}
	return clientcmd.NewNonInteractiveClientConfig(config, "owned", &clientcmd.ConfigOverrides{Context: clientapi.Context{Namespace: g.namespace}}, nil)
}

type configuration struct {
	sdk       *action.Configuration
	client    kubernetes.Interface
	transport *operationTransport
}

func (s *Service) configuration(ctx context.Context, target Target, readOnly bool) (*configuration, error) {
	if s.configure != nil {
		return s.configure(ctx, target, readOnly)
	}
	if s.connections == nil {
		return nil, model.Fail("invalid", "Choose a connected cluster.")
	}
	config, err := s.connections.RESTConfig(target.Connection)
	if err != nil {
		return nil, err
	}
	bounded, err := appkube.BoundedHTTPClient(config)
	if err != nil {
		return nil, err
	}
	server, err := url.Parse(config.Host)
	if err != nil {
		return nil, model.Fail("invalid", "Invalid Kubernetes server address.")
	}
	transport := &operationTransport{ctx: ctx, next: bounded.Transport, readOnly: readOnly, origin: server.Scheme + "://" + server.Host}
	// The transport already owns credentials and the connection guard. Never wrap it twice.
	config = &rest.Config{Host: config.Host, APIPath: config.APIPath, ContentConfig: config.ContentConfig, Transport: transport, Timeout: config.Timeout, QPS: config.QPS, Burst: config.Burst}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, model.Fail("invalid", "Cannot create Helm Kubernetes clients.")
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, model.Fail("invalid", "Cannot create Helm discovery client.")
	}
	getter := &nativeGetter{config: config, namespace: target.Namespace, discovery: memory.NewMemCacheClient(discoveryClient)}
	kubeClient := helmkube.New(getter)
	kubeClient.WaitContext = ctx
	kubeClient.SetLogger(slog.DiscardHandler)
	cfg := &action.Configuration{RESTClientGetter: getter, KubeClient: kubeClient, HookOutputFunc: func(string, string, string) io.Writer { return io.Discard }}
	cfg.SetLogger(slog.DiscardHandler)
	cfg.Releases = storage.Init(newStorageDriver(ctx, client, target, readOnly))
	cfg.Releases.SetLogger(slog.DiscardHandler)
	cfg.Releases.MaxHistory = 0
	return &configuration{sdk: cfg, client: client, transport: transport}, nil
}
