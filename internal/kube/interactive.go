/*
Purpose: Own product interactive sessions bound to a Pod UID.
Used by: Pod exec and Pod/Service forwarding adapters.
Responsibilities: Reserve pending entries, verify identities and stop or close sessions.
Important: Name-addressed subresources do not support an atomic UID precondition.
*/
package kube

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	core "k8s.io/api/core/v1"
)

func transportTarget(pod PodSessionTarget) model.Target {
	r := pod.Resource
	return model.Target{Connection: r.Connection, Namespace: r.Namespace, Name: r.Name, Container: pod.Container}
}

func (s *Service) interactivePod(ctx context.Context, pod PodSessionTarget, terminal bool) (*core.Pod, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ref := pod.Resource
	if ref.Kind != "Pod" || ref.APIVersion != "v1" || (!terminal && pod.Container != "") {
		return nil, model.Fail("invalid", "Select an explicit Pod session target")
	}
	if err := target(transportTarget(pod), terminal); err != nil {
		return nil, err
	}
	client, err := s.boundedReadClient(ctx, ref.Connection)
	if err != nil {
		return nil, err
	}
	object, err := checkedObject(ctx, client, ref)
	if err != nil {
		return nil, err
	}
	live := object.(*core.Pod)
	if live.Namespace != ref.Namespace || live.Name != ref.Name || live.DeletionTimestamp != nil {
		return nil, model.Fail("stale_target", "The selected Pod changed or is terminating")
	}
	if terminal {
		for _, group := range [][]core.ContainerStatus{live.Status.ContainerStatuses, live.Status.InitContainerStatuses, live.Status.EphemeralContainerStatuses} {
			for _, c := range group {
				if c.Name == pod.Container && c.State.Running != nil {
					return live, nil
				}
			}
		}
		return nil, model.Fail("unavailable_container", "The selected container is not running")
	}
	if live.Status.Phase != core.PodRunning {
		return nil, model.Fail("invalid", "The selected Pod is not running")
	}
	return live, nil
}

type interactiveRun func(context.Context, *session.Writer, func(uint16) error) error

func (s *Service) reserveInteractive(ctx context.Context, entry InteractiveEntry, run interactiveRun) (InteractiveEntry, error) {
	isTerminal := entry.Kind == "terminal"
	if _, err := s.interactivePod(ctx, entry.Target, isTerminal); err != nil {
		return InteractiveEntry{}, err
	}
	owned, err := s.connections.Lifetime(entry.Target.Resource.Connection)
	if err != nil {
		return InteractiveEntry{}, err
	}
	s.mu.Lock()
	if len(s.interactive) >= 8 {
		s.mu.Unlock()
		return InteractiveEntry{}, model.Fail("capacity", "Close a terminal or forward before opening another (maximum eight)")
	}
	ref, err := s.sessions.StartPending(owned, transportTarget(entry.Target), entry.Kind, func(runCtx context.Context, w *session.Writer) error {
		s.runInteractive(runCtx, w, entry.Target, isTerminal, run)
		return nil
	})
	if err == nil {
		s.interactive[ref.ID] = ref
	}
	s.mu.Unlock()
	if err != nil {
		return InteractiveEntry{}, err
	}
	if ctx.Err() != nil || owned.Err() != nil {
		_ = s.CloseInteractive(ref)
		return InteractiveEntry{}, model.Fail("cancelled", "Session start cancelled")
	}
	entry.Session, entry.State = ref, "connecting"
	return entry, nil
}

func (s *Service) runInteractive(ctx context.Context, w *session.Writer, pod PodSessionTarget, terminal bool, run interactiveRun) {
	send := func(value interactiveState) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return w.Send("state", data)
	}
	err := func() error {
		if _, err := s.interactivePod(ctx, pod, terminal); err != nil {
			return err
		}
		stream, cancel := context.WithCancel(ctx)
		defer cancel()
		verified := make(chan struct{})
		monitor := make(chan error, 1)
		go func() {
			monitor <- s.monitorInteractive(stream, pod, terminal, verified, cancel)
		}()
		err := run(stream, w, func(port uint16) error {
			if _, err := s.interactivePod(stream, pod, terminal); err != nil {
				return err
			}
			state := "running"
			if !terminal {
				state = "listening"
			}
			close(verified)
			return send(interactiveState{State: state, LocalPort: port})
		})
		cancel()
		if changed := <-monitor; changed != nil {
			return changed
		}
		return err
	}()
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		_ = send(interactiveState{State: "failed", Problem: ClassifyError(ctx, err)})
	} else {
		_ = send(interactiveState{State: "ended"})
	}
}

func (s *Service) monitorInteractive(ctx context.Context, pod PodSessionTarget, terminal bool, verified <-chan struct{}, cancel context.CancelFunc) error {
	select {
	case <-ctx.Done():
		return nil
	case <-verified:
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Exec ends naturally when its process exits; only recheck the active Pod identity.
			if terminal {
				pod.Container = ""
			}
			if _, err := s.interactivePod(ctx, pod, false); err != nil && ctx.Err() == nil {
				cancel()
				return model.Fail("target_unverified", "The original Pod could no longer be verified. The session ended.")
			}
		}
	}
}

func (s *Service) StopInteractive(ref model.SessionRef) error {
	s.mu.Lock()
	actual, ok := s.interactive[ref.ID]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	if actual != ref {
		return model.Fail("stale_target", "Session connection does not match")
	}
	return s.sessions.Stop(ref)
}

func (s *Service) CloseInteractive(ref model.SessionRef) error {
	if err := s.StopInteractive(ref); err != nil {
		return err
	}
	s.mu.Lock()
	if s.interactive[ref.ID] == ref {
		delete(s.interactive, ref.ID)
	}
	s.mu.Unlock()
	return nil
}
