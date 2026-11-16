/*
Purpose: Forward a fixed Pod port through a loopback-only local listener.
Used by: Product interactive sessions and legacy validation callers.
Responsibilities: Own one cancellable upgrade, local listener and ready notification.
Important: Forwarded traffic never crosses the bridge and an upgrade is never retried.
*/
package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/klog/v2"
)

type ForwardResult struct {
	Session   model.SessionRef `json:"session"`
	LocalPort uint16           `json:"localPort"`
}

type forwardDialFunc func(...string) (httpstream.Connection, string, error)

func (f forwardDialFunc) Dial(protocols ...string) (httpstream.Connection, string, error) {
	return f(protocols...)
}

type bindProblem struct{ occupied bool }

func (p *bindProblem) Write(data []byte) (int, error) {
	// The only errOut message comes from the local listener. Retain no text.
	p.occupied = strings.Contains(string(data), "address already in use")
	return len(data), nil
}

func forwardListener(ctx context.Context, dial httpstream.Dialer, local, remote uint16, report func(uint16) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stream, cancel := context.WithCancel(ctx)
	defer cancel()
	ready := make(chan struct{})
	finished := make(chan error, 1)
	var dialErr error
	guarded := forwardDialFunc(func(protocols ...string) (httpstream.Connection, string, error) {
		conn, protocol, err := dial.Dial(protocols...)
		if err == nil && stream.Err() != nil {
			conn.Close()
			err = stream.Err()
			conn = nil
		}
		dialErr = err
		return conn, protocol, err
	})
	bind := &bindProblem{}
	forward, err := portforward.NewOnAddressesWithContext(klog.NewContext(stream, klog.Logger{}), guarded, []string{"127.0.0.1"},
		[]string{fmt.Sprintf("%d:%d", local, remote)}, ready, io.Discard, bind)
	if err != nil {
		return model.Fail("invalid", "Cannot configure loopback forwarding")
	}
	go func() { finished <- forward.ForwardPorts() }()
	select {
	case <-ready:
		ports, readErr := forward.GetPorts()
		if readErr != nil || len(ports) != 1 || ports[0].Local == 0 {
			readErr = model.Fail("invalid_response", "Forward did not bind the requested loopback mapping")
		}
		if readErr == nil {
			readErr = stream.Err()
		}
		if readErr == nil {
			readErr = report(ports[0].Local)
		}
		if readErr != nil {
			cancel()
			<-finished
			return readErr
		}
		err = <-finished
	case err = <-finished:
	case <-stream.Done():
		<-finished
		return stream.Err()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if dialErr != nil {
		return dialErr
	}
	if bind.occupied {
		return model.Fail("port_conflict", "The requested local TCP port is already in use")
	}
	if errors.Is(err, portforward.ErrLostConnectionToPod) {
		return model.Fail("disconnected", "The forward lost its connection to the selected Pod")
	}
	if err != nil {
		return model.Fail("forward_failed", "Could not establish the loopback forward")
	}
	return nil
}

func (s *Service) runForward(ctx context.Context, t model.Target, local, remote uint16, verify, report func(uint16) error) error {
	stream, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := time.AfterFunc(20*time.Second, cancel)
	defer timer.Stop()
	cfg, err := s.connections.StreamConfig(stream, t.Connection)
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return model.Fail("invalid", "Cannot create forward client")
	}
	url := client.CoreV1().RESTClient().Post().Namespace(t.Namespace).Resource("pods").Name(t.Name).SubResource("portforward").URL()
	transport, upgrader, err := newStreamUpgrade(cfg, verify)
	if err != nil {
		return model.Fail("disconnected", "Cannot create forward transport")
	}
	httpClient := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	dial := forwardDialFunc(func(protocols ...string) (httpstream.Connection, string, error) {
		request, err := http.NewRequestWithContext(stream, http.MethodPost, url.String(), nil)
		if err != nil {
			return nil, "", err
		}
		return spdy.Negotiate(upgrader, httpClient, request, protocols...)
	})
	err = forwardListener(stream, dial, local, remote, func(port uint16) error {
		if err := report(port); err != nil {
			return err
		}
		timer.Stop()
		return nil
	})
	if stream.Err() != nil && ctx.Err() == nil {
		return model.Fail("timeout", "Port forward startup timed out")
	}
	return err
}

func (s *Service) Forward(ctx context.Context, t model.Target, local, remote uint16) (ForwardResult, error) {
	if err := target(t, false); err != nil {
		return ForwardResult{}, err
	}
	if remote == 0 {
		return ForwardResult{}, model.Fail("invalid", "Explicit remote port required")
	}
	result := make(chan uint16, 1)
	failed := make(chan error, 1)
	ref, err := s.sessions.Start(ctx, t, "forward", func(runCtx context.Context, w *session.Writer) error {
		err := s.runForward(runCtx, t, local, remote, nil, func(port uint16) error {
			result <- port
			return w.Send("state", []byte(fmt.Sprintf("listening on 127.0.0.1:%d", port)))
		})
		failed <- err
		return safeError(runCtx, err)
	})
	if err != nil {
		return ForwardResult{}, err
	}
	select {
	case port := <-result:
		return ForwardResult{Session: ref, LocalPort: port}, nil
	case err := <-failed:
		_ = s.sessions.Stop(ref)
		return ForwardResult{}, err
	case <-ctx.Done():
		_ = s.sessions.Stop(ref)
		return ForwardResult{}, model.Fail("cancelled", "Forward start cancelled")
	}
}
