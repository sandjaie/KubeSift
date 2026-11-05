/*
Purpose: Own one interactive container shell with explicit session identity.
Used by: Desktop terminal controls and local validation.
Responsibilities: Stream bytes, bound input, handle resize and release pipes when the session ends.
Important: The command is a fixed container shell; this package does not expose a local command runner or reconnect commands.
*/
package kube

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"sync"
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

type terminalOutput struct{ writer *session.Writer }

func (w terminalOutput) Write(data []byte) (int, error) {
	if err := w.writer.Send("data", data); err != nil {
		return 0, err
	}
	return len(data), nil
}
func (s *Service) Exec(ctx context.Context, t model.Target) (model.SessionRef, error) {
	if err := target(t, true); err != nil {
		return model.SessionRef{}, err
	}
	owned, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	term := &terminal{ctx: owned, cancel: cancel, input: make(chan []byte, 128), size: make(chan remotecommand.TerminalSize, 1), reader: reader, writer: writer}
	var ref model.SessionRef
	var err error
	ref, err = s.sessions.StartPending(ctx, t, "terminal", func(runCtx context.Context, w *session.Writer) error {
		defer term.close()
		cfg, err := s.connections.StreamConfig(runCtx, t.Connection)
		if err != nil {
			return err
		}
		client, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return model.Fail("invalid", "Cannot create terminal client")
		}
		request := client.CoreV1().RESTClient().Post().Namespace(t.Namespace).Resource("pods").Name(t.Name).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: t.Container, Command: []string{"/bin/sh"}, Stdin: true, Stdout: true, Stderr: false, TTY: true}, scheme.ParameterCodec)
		executor, err := remotecommand.NewWebSocketExecutor(cfg, "GET", request.URL().String())
		if err != nil {
			return model.Fail("disconnected", "Cannot create terminal transport")
		}
		pumpDone := make(chan struct{})
		go func() {
			defer close(pumpDone)
			for {
				select {
				case <-runCtx.Done():
					_ = writer.CloseWithError(runCtx.Err())
					return
				case <-term.ctx.Done():
					return
				case data := <-term.input:
					_, e := writer.Write(data)
					term.mu.Lock()
					term.queued -= len(data)
					term.mu.Unlock()
					if e != nil {
						return
					}
				}
			}
		}()
		err = executor.StreamWithContext(runCtx, remotecommand.StreamOptions{Stdin: reader, Stdout: terminalOutput{w}, Tty: true, TerminalSizeQueue: terminalSizes{runCtx, term.size}})
		term.close()
		<-pumpDone
		return safeError(runCtx, err)
	})
	if err != nil {
		term.close()
		return model.SessionRef{}, err
	}
	term.ref = ref
	s.mu.Lock()
	s.terminals[ref.ID] = term
	s.mu.Unlock()
	if err = s.sessions.OnStop(ref, func() { term.close(); s.mu.Lock(); delete(s.terminals, ref.ID); s.mu.Unlock() }); err != nil {
		term.close()
		s.mu.Lock()
		delete(s.terminals, ref.ID)
		s.mu.Unlock()
		return model.SessionRef{}, err
	}
	return ref, nil
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
