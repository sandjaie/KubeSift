/*
Purpose: Own retained product log sessions for an explicit Pod UID and container.
Used by: The explorer log bridge; legacy validation logs remain separate.
Responsibilities: Enforce eight reservations, bounded snapshots and periodic identity verification.
Important: Kubernetes logs are name-addressed; checks reduce but cannot eliminate UID races.
*/
package kube

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	"io"
	corev1 "k8s.io/api/core/v1"
	"time"
)

func (s *Service) validateLog(ctx context.Context, req LogRequest) error {
	if req.Resource.Kind != "Pod" {
		return model.Fail("invalid", "Choose a Pod for logs")
	}
	if err := target(model.Target{Namespace: req.Resource.Namespace, Name: req.Resource.Name, Container: req.Container}, true); err != nil {
		return err
	}
	client, err := s.boundedReadClient(ctx, req.Resource.Connection)
	if err != nil {
		return err
	}
	obj, err := checkedObject(ctx, client, req.Resource)
	if err != nil {
		return err
	}
	pod := obj.(*corev1.Pod)
	for _, c := range pod.Spec.Containers {
		if c.Name == req.Container {
			return nil
		}
	}
	for _, c := range pod.Spec.InitContainers {
		if c.Name == req.Container {
			return nil
		}
	}
	for _, c := range pod.Spec.EphemeralContainers {
		if c.Name == req.Container {
			return nil
		}
	}
	return model.Fail("invalid", "Container is not present in this Pod")
}
func (s *Service) OpenPodLogs(ctx context.Context, req LogRequest) (model.SessionRef, error) {
	if req.Previous && req.Follow {
		return model.SessionRef{}, model.Fail("invalid", "Previous logs cannot follow")
	}
	if req.TailLines < 0 || req.TailLines > 1000 {
		return model.SessionRef{}, model.Fail("invalid", "Tail must be between 1 and 1000 lines")
	}
	if req.TailLines == 0 {
		req.TailLines = 200
	}
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := s.validateLog(checkCtx, req)
	cancel()
	if err != nil {
		return model.SessionRef{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.productLogs) >= 8 {
		return model.SessionRef{}, model.Fail("capacity", "Close a log session before opening another (maximum eight)")
	}
	target := model.Target{Connection: req.Resource.Connection, Namespace: req.Resource.Namespace, Name: req.Resource.Name, Container: req.Container}
	var ref model.SessionRef
	ref, err = s.sessions.StartPending(ctx, target, "logs", func(runCtx context.Context, w *session.Writer) error {
		streamCtx, cancelStream := context.WithCancel(runCtx)
		if !req.Follow {
			cancelStream()
			streamCtx, cancelStream = context.WithTimeout(runCtx, 30*time.Second)
		}
		defer cancelStream()
		failedOpen := true
		defer func() {
			if failedOpen {
				s.mu.Lock()
				delete(s.productLogs, ref.ID)
				s.mu.Unlock()
			}
		}()
		if e := s.validateLog(streamCtx, req); e != nil {
			return e
		}
		client, e := s.streamClient(streamCtx, req.Resource.Connection)
		if e != nil {
			return e
		}
		opts := &corev1.PodLogOptions{Container: req.Container, Previous: req.Previous, Follow: req.Follow, Timestamps: true, TailLines: &req.TailLines}
		if !req.Follow {
			limit := int64(1 << 20)
			opts.LimitBytes = &limit
		}
		stream, e := client.CoreV1().Pods(req.Resource.Namespace).GetLogs(req.Resource.Name, opts).Stream(streamCtx)
		if e != nil {
			return safeError(streamCtx, e)
		}
		defer stream.Close()
		stopClose := context.AfterFunc(streamCtx, func() { stream.Close() })
		defer stopClose()
		if e = s.validateLog(streamCtx, req); e != nil {
			return e
		}
		failedOpen = false
		verification := make(chan error, 1)
		monitorDone := make(chan struct{})
		defer close(monitorDone)
		if req.Follow {
			go func() {
				ticker := time.NewTicker(15 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-monitorDone:
						return
					case <-streamCtx.Done():
						return
					case <-ticker.C:
						checkCtx, done := context.WithTimeout(streamCtx, 30*time.Second)
						e := s.validateLog(checkCtx, req)
						done()
						if e != nil {
							verification <- e
							cancelStream()
							return
						}
					}
				}
			}()
		}
		buffer := make([]byte, 32<<10)
		var total int64
		for {
			count, readErr := stream.Read(buffer)
			if count > 0 {
				total += int64(count)
				if e = w.Send("data", buffer[:count]); e != nil {
					return e
				}
			}
			select {
			case e := <-verification:
				return e
			default:
			}
			if readErr == io.EOF {
				if req.Follow {
					return w.Send("state", []byte("gap: Follow ended; Resume may overlap or omit logs"))
				}
				if total >= 1<<20 {
					if e = w.Send("state", []byte("loss: Snapshot reached byte cap; earlier logs may be omitted")); e != nil {
						return e
					}
				}
				return w.Send("state", []byte("complete: Snapshot complete; requested tail only"))
			}
			if readErr != nil {
				return safeError(streamCtx, readErr)
			}
		}
	})
	if err != nil {
		return model.SessionRef{}, err
	}
	s.productLogs[ref.ID] = ref
	return ref, nil
}
func (s *Service) ClosePodLogs(ref model.SessionRef) error {
	s.mu.Lock()
	old, ok := s.productLogs[ref.ID]
	if ok && old != ref {
		s.mu.Unlock()
		return model.Fail("stale_target", "Session connection does not match")
	}
	s.mu.Unlock()
	if err := s.sessions.Stop(ref); err != nil {
		return err
	}
	s.mu.Lock()
	if s.productLogs[ref.ID] == ref {
		delete(s.productLogs, ref.ID)
	}
	s.mu.Unlock()
	return nil
}
func (s *Service) closeConnection(ref model.ConnectionRef) {
	s.sessions.StopConnection(ref)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.discovery, ref)
	for id, entry := range s.productLogs {
		if entry.Connection == ref {
			delete(s.productLogs, id)
		}
	}
}
