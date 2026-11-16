/*
Purpose: Own one interactive container shell with explicit session identity.
Used by: Desktop terminal controls and local validation.
Responsibilities: Stream bytes, bound input, handle resize and release pipes when the session ends.
Important: The command is a fixed argument vector inside the selected container; this package does not expose a local command runner or reconnect commands.
*/
package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
	clientexec "k8s.io/client-go/util/exec"
)

type terminal struct {
	ref    model.SessionRef
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	input  chan []byte
	queued int
	size   chan remotecommand.TerminalSize
	reader *io.PipeReader
	writer *io.PipeWriter
}

func (t *terminal) close() { t.cancel(); _ = t.reader.Close(); _ = t.writer.Close() }

type terminalSizes struct {
	ctx     context.Context
	updates <-chan remotecommand.TerminalSize
}

func (q terminalSizes) Next() *remotecommand.TerminalSize {
	select {
	case value := <-q.updates:
		return &value
	case <-q.ctx.Done():
		return nil
	}
}

type terminalOutput struct {
	ctx    context.Context
	writer *session.Writer
}

func (w terminalOutput) Write(data []byte) (int, error) {
	if err := w.writer.SendContext(w.ctx, "data", data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func newTerminal(ctx context.Context) *terminal {
	owned, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	return &terminal{ctx: owned, cancel: cancel, input: make(chan []byte, 128), size: make(chan remotecommand.TerminalSize, 1), reader: reader, writer: writer}
}

func (s *Service) attachTerminal(ref model.SessionRef, term *terminal) error {
	term.ref = ref
	s.mu.Lock()
	s.terminals[ref.ID] = term
	s.mu.Unlock()
	cleanup := func() {
		term.close()
		s.mu.Lock()
		delete(s.terminals, ref.ID)
		s.mu.Unlock()
	}
	if err := s.sessions.OnStop(ref, cleanup); err != nil {
		cleanup()
		return err
	}
	return nil
}

func validCommand(command []string) bool {
	if len(command) == 0 || len(command) > 32 {
		return false
	}
	size := 0
	for _, arg := range command {
		if arg == "" || strings.ContainsRune(arg, 0) {
			return false
		}
		size += len(arg) + 1
		if size > 8<<10 {
			return false
		}
	}
	return true
}

func (s *Service) OpenTerminal(ctx context.Context, req TerminalRequest) (InteractiveEntry, error) {
	if !validCommand(req.Command) {
		return InteractiveEntry{}, model.Fail("invalid", "Use an executable and up to 32 non-empty command elements, totaling at most 8 KiB")
	}
	owned, err := s.connections.Lifetime(req.Target.Resource.Connection)
	if err != nil {
		return InteractiveEntry{}, err
	}
	term := newTerminal(owned)
	command := slices.Clone(req.Command)
	entry, err := s.reserveInteractive(ctx, InteractiveEntry{Kind: "terminal", Target: req.Target},
		func(ctx context.Context, w *session.Writer, ready func(uint16) error) error {
			return s.runTerminal(ctx, w, transportTarget(req.Target), term, command, ready)
		})
	if err != nil {
		term.close()
		return InteractiveEntry{}, err
	}
	if err = s.attachTerminal(entry.Session, term); err != nil || ctx.Err() != nil {
		_ = s.CloseInteractive(entry.Session)
		if err == nil {
			err = ctx.Err()
		}
		return InteractiveEntry{}, safeError(ctx, err)
	}
	return entry, nil
}

func (s *Service) Exec(ctx context.Context, t model.Target) (model.SessionRef, error) {
	if err := target(t, true); err != nil {
		return model.SessionRef{}, err
	}
	term := newTerminal(ctx)
	ref, err := s.sessions.StartPending(ctx, t, "terminal", func(runCtx context.Context, w *session.Writer) error {
		return s.runTerminal(runCtx, w, t, term, []string{"/bin/sh"}, nil)
	})
	if err != nil {
		term.close()
		return model.SessionRef{}, err
	}
	if err = s.attachTerminal(ref, term); err != nil {
		_ = s.sessions.Stop(ref)
		return model.SessionRef{}, err
	}
	return ref, nil
}

// interactiveUpgrade preserves permission status without returning an API error body.
type interactiveUpgrade struct {
	spdy.Upgrader
	ready func(uint16) error
}

func (u interactiveUpgrade) NewConnection(resp *http.Response) (httpstream.Connection, error) {
	switch resp.StatusCode {
	case http.StatusForbidden:
		resp.Body.Close()
		return nil, model.Fail("forbidden", "Kubernetes denied this Pod subresource")
	case http.StatusUnauthorized:
		resp.Body.Close()
		return nil, model.Fail("auth_required", "Kubernetes rejected the current credentials")
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, model.Fail("stale_target", "The selected Pod or subresource is unavailable")
	}
	// The SPDY library can read a failed upgrade body. Bound it before parsing.
	if resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.LimitReader(resp.Body, 8<<10), resp.Body}
	}
	conn, err := u.Upgrader.NewConnection(resp)
	if err != nil || u.ready == nil {
		return conn, err
	}
	if err := u.ready(0); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (s *Service) runTerminal(ctx context.Context, w *session.Writer, t model.Target, term *terminal, command []string, ready func(uint16) error) error {
	defer term.close()
	// The startup timer ends after upgrade/verification, not after the shell exits.
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	timeout := time.AfterFunc(20*time.Second, cancel)
	defer timeout.Stop()
	cfg, err := s.connections.StreamConfig(streamCtx, t.Connection)
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return model.Fail("invalid", "Cannot create terminal client")
	}
	request := client.CoreV1().RESTClient().Post().Namespace(t.Namespace).Resource("pods").Name(t.Name).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{Container: t.Container, Command: command, Stdin: true, Stdout: true, TTY: true}, scheme.ParameterCodec)
	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return model.Fail("disconnected", "Cannot create terminal transport")
	}
	upgraded := func(port uint16) error {
		if ready != nil {
			if err := ready(port); err != nil {
				return err
			}
		}
		timeout.Stop()
		return nil
	}
	executor, err := remotecommand.NewSPDYExecutorRejectRedirects(transport, interactiveUpgrade{Upgrader: upgrader, ready: upgraded}, http.MethodPost, request.URL())
	if err != nil {
		return model.Fail("disconnected", "Cannot create terminal transport")
	}
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		for {
			select {
			case <-streamCtx.Done():
				_ = term.writer.CloseWithError(streamCtx.Err())
				return
			case <-term.ctx.Done():
				return
			case data := <-term.input:
				_, err := term.writer.Write(data)
				term.mu.Lock()
				term.queued -= len(data)
				term.mu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()
	err = executor.StreamWithContext(streamCtx, remotecommand.StreamOptions{
		Stdin: term.reader, Stdout: terminalOutput{streamCtx, w}, Tty: true, TerminalSizeQueue: terminalSizes{streamCtx, term.size},
	})
	term.close()
	<-pumpDone
	if err == nil {
		return nil
	}
	var exit clientexec.ExitError
	if errors.As(err, &exit) {
		return model.Fail("exit_status", fmt.Sprintf("Container command exited with status %d", exit.ExitStatus()))
	}
	if streamCtx.Err() != nil && ctx.Err() == nil {
		return model.Fail("timeout", "Terminal startup timed out")
	}
	problem := ClassifyError(ctx, err)
	if problem.Code == "failed" {
		return model.Fail("exec_failed", "Could not start or continue the container command. Check that the executable is installed in the selected container.")
	}
	return problem
}

func (s *Service) terminal(ref model.SessionRef) (*terminal, error) {
	if err := s.connections.Validate(ref.Connection); err != nil {
		return nil, err
	}
	s.mu.Lock()
	term := s.terminals[ref.ID]
	s.mu.Unlock()
	if term == nil || term.ref != ref || term.ctx.Err() != nil {
		return nil, model.Fail("stale_target", "Terminal is unavailable")
	}
	return term, nil
}
func (s *Service) Input(ref model.SessionRef, data []byte) error {
	if len(data) > 64<<10 {
		return model.Fail("invalid", "Terminal input exceeds 64 KiB")
	}
	term, err := s.terminal(ref)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	term.mu.Lock()
	defer term.mu.Unlock()
	if term.queued+len(data) > 64<<10 {
		return model.Fail("invalid", "Terminal input buffer is full; input not accepted")
	}
	copy := append([]byte(nil), data...)
	select {
	case term.input <- copy:
		term.queued += len(copy)
		return nil
	default:
		return model.Fail("invalid", "Terminal input queue is full; input not accepted")
	}
}
func (s *Service) Resize(ref model.SessionRef, cols, rows uint16) error {
	if cols == 0 || rows == 0 || cols > 500 || rows > 500 {
		return model.Fail("invalid", "Terminal dimensions must be between 1 and 500")
	}
	term, err := s.terminal(ref)
	if err != nil {
		return err
	}
	value := remotecommand.TerminalSize{Width: cols, Height: rows}
	select {
	case term.size <- value:
	default:
		select {
		case <-term.size:
		default:
		}
		select {
		case term.size <- value:
		default:
		}
	}
	return nil
}
