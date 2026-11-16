/*
Purpose: Own the cancellable HTTP upgrade used by native Kubernetes sessions.
Used by: The client-go SPDY terminal and port-forward adapters.
Responsibilities: Preserve the configured TLS, proxy and credential wrappers, and hand off the upgraded socket once.
Important: A pending upgrade must stop even when the API has not returned headers.
*/
package kube

import (
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/apimachinery/pkg/util/httpstream"
	httpstreamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport/spdy"
)

type upgradeSocket struct {
	net.Conn
	body io.ReadWriteCloser
}

func (c upgradeSocket) Read(p []byte) (int, error)  { return c.body.Read(p) }
func (c upgradeSocket) Write(p []byte) (int, error) { return c.body.Write(p) }
func (c upgradeSocket) Close() error                { return c.body.Close() }

type streamUpgrade struct {
	http   *http.Transport
	ready  func(uint16) error
	socket *upgradeSocket
}

func newStreamUpgrade(cfg *rest.Config, ready func(uint16) error) (http.RoundTripper, spdy.Upgrader, error) {
	tlsConfig, err := rest.TLSConfigFor(cfg)
	if err != nil {
		return nil, nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	// Kubernetes SPDY uses an HTTP/1.1 upgrade, independent of the negotiated stream protocol.
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	transport.DisableKeepAlives = true
	if cfg.Proxy != nil {
		transport.Proxy = cfg.Proxy
	}
	if cfg.Dial != nil {
		transport.DialContext = cfg.Dial
	}
	upgrade := &streamUpgrade{http: transport, ready: ready}
	wrapped, err := rest.HTTPWrappersForConfig(cfg, upgrade)
	return wrapped, upgrade, err
}

func (u *streamUpgrade) RoundTrip(req *http.Request) (*http.Response, error) {
	var underlying net.Conn
	ctx := httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { underlying = info.Conn },
	})
	req = req.Clone(ctx)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", httpstreamspdy.HeaderSpdy31)
	resp, err := u.http.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		return resp, err
	}
	body, ok := resp.Body.(io.ReadWriteCloser)
	if !ok || underlying == nil {
		resp.Body.Close()
		return nil, model.Fail("disconnected", "Kubernetes did not provide an upgraded socket")
	}
	u.socket = &upgradeSocket{Conn: underlying, body: body}
	// Negotiate closes the HTTP response body after the upgrader takes ownership.
	resp.Body = http.NoBody
	return resp, nil
}

func (u *streamUpgrade) NewConnection(resp *http.Response) (httpstream.Connection, error) {
	switch resp.StatusCode {
	case http.StatusForbidden:
		return nil, model.Fail("forbidden", "Kubernetes denied this Pod subresource")
	case http.StatusUnauthorized:
		return nil, model.Fail("auth_required", "Kubernetes rejected the current credentials")
	case http.StatusNotFound:
		return nil, model.Fail("stale_target", "The selected Pod or subresource is unavailable")
	}
	socket := u.socket
	u.socket = nil
	if socket == nil {
		return nil, model.Fail("upgrade_failed", "Kubernetes did not accept the interactive connection")
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), httpstreamspdy.HeaderSpdy31) ||
		!strings.Contains(strings.ToLower(resp.Header.Get("Connection")), "upgrade") {
		socket.Close()
		return nil, model.Fail("upgrade_failed", "Kubernetes returned an unsupported stream upgrade")
	}
	conn, err := httpstreamspdy.NewClientConnectionWithPings(socket, 5*time.Second)
	if err != nil {
		socket.Close()
		return nil, err
	}
	if u.ready != nil {
		if err = u.ready(0); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}
