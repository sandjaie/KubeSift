/*
Purpose: Forward a fixed Pod port through a loopback-only local listener.
Used by: Desktop forward controls and local integration.
Responsibilities: Bind an owned session, report its allocated port and close setup/network work on cancellation.
Important: Never bind a public address, proxy payload through React or retry an uncertain operation.
*/
package kube

import (
	"context"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	"io"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"net/http"
	"time"
)

type ForwardResult struct {
	Session   model.SessionRef `json:"session"`
	LocalPort uint16           `json:"localPort"`
}
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func (s *Service) Forward(ctx context.Context, t model.Target, local, remote uint16) (ForwardResult, error) {
	if err := target(t, false); err != nil {
		return ForwardResult{}, err
	}
	if remote == 0 {
		return ForwardResult{}, model.Fail("invalid", "Explicit remote port required")
	}
	ready := make(chan struct{})
	result := make(chan uint16, 1)
	failed := make(chan error, 1)
	ref, err := s.sessions.Start(ctx, t, "forward", func(runCtx context.Context, w *session.Writer) error {
		forwardCtx, cancel := context.WithCancel(runCtx)
		defer cancel()
		cfg, err := s.connections.StreamConfig(forwardCtx, t.Connection)
		if err != nil {
			failed <- err
			return err
		}
		client, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			safe := model.Fail("invalid", "Cannot create forward client")
			failed <- safe
			return safe
		}
		url := client.CoreV1().RESTClient().Post().Namespace(t.Namespace).Resource("pods").Name(t.Name).SubResource("portforward").URL()
		transport, upgrader, err := spdy.RoundTripperFor(cfg)
		if err != nil {
			safe := model.Fail("disconnected", "Cannot create forward transport")
			failed <- safe
			return safe
		}
		httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return transport.RoundTrip(request.Clone(forwardCtx))
		})}
		dialer := spdy.NewDialer(upgrader, httpClient, "POST", url)
		forward, err := portforward.NewOnAddresses(dialer, []string{"127.0.0.1"}, []string{fmt.Sprintf("%d:%d", local, remote)}, forwardCtx.Done(), ready, io.Discard, io.Discard)
		if err != nil {
			safe := model.Fail("invalid", "Cannot configure loopback forward")
			failed <- safe
			return safe
		}
		reportDone := make(chan struct{})
		go func() {
			defer close(reportDone)
			select {
			case <-ready:
				ports, e := forward.GetPorts()
				if e == nil && len(ports) > 0 {
					result <- ports[0].Local
					_ = w.Send("state", []byte(fmt.Sprintf("listening on 127.0.0.1:%d", ports[0].Local)))
				}
			case <-forwardCtx.Done():
			}
		}()
		err = forward.ForwardPorts()
		cancel()
		<-reportDone
		if err != nil && runCtx.Err() == nil {
			safe := model.Fail("disconnected", "Port forward failed; check local port conflict and remote availability")
			select {
			case failed <- safe:
			default:
			}
			return safe
		}
		return safeError(runCtx, err)
	})
	if err != nil {
		return ForwardResult{}, err
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	select {
	case port := <-result:
		return ForwardResult{ref, port}, nil
	case err := <-failed:
		_ = s.sessions.Stop(ref)
		return ForwardResult{}, err
	case <-ctx.Done():
		_ = s.sessions.Stop(ref)
		return ForwardResult{}, model.Fail("cancelled", "Forward start cancelled")
	case <-timer.C:
		_ = s.sessions.Stop(ref)
		return ForwardResult{}, model.Fail("disconnected", "Forward startup timed out")
	}
}
