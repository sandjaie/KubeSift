/*
Purpose: Stream Pod changes and container logs for a fixed connection target.
Used by: The desktop session controls and local integration scenario.
Responsibilities: Relist watches after expiration or overflow, bound log reads and propagate cancellation.
Important: Only reads reconnect; terminal commands and mutations must never be replayed.
*/
package kube

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	"io"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"time"
)

func (s *Service) Logs(ctx context.Context, t model.Target, previous bool, tail int64) (model.SessionRef, error) {
	if err := target(t, true); err != nil {
		return model.SessionRef{}, err
	}
	if tail < 0 || tail > 1000 {
		return model.SessionRef{}, model.Fail("invalid", "Initial log tail must be between 0 and 1000 lines")
	}
	return s.sessions.StartPending(ctx, t, "logs", func(runCtx context.Context, w *session.Writer) error {
		client, err := s.streamClient(runCtx, t.Connection)
		if err != nil {
			return err
		}
		stream, err := client.CoreV1().Pods(t.Namespace).GetLogs(t.Name, &corev1.PodLogOptions{Container: t.Container, Previous: previous, Follow: !previous, TailLines: &tail}).Stream(runCtx)
		if err != nil {
			return safeError(runCtx, err)
		}
		defer stream.Close()
		buffer := make([]byte, 32<<10)
		for {
			count, err := stream.Read(buffer)
			if count > 0 {
				if sendErr := w.Send("data", buffer[:count]); sendErr != nil {
					return sendErr
				}
			}
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return safeError(runCtx, err)
			}
		}
	})
}
func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (s *Service) WatchPods(ctx context.Context, t model.Target) (model.SessionRef, error) {
	if err := namespace(t.Namespace); err != nil {
		return model.SessionRef{}, err
	}
	return s.sessions.StartPending(ctx, t, "watch", func(runCtx context.Context, w *session.Writer) error {
		backoff := 250 * time.Millisecond
		for {
			if runCtx.Err() != nil {
				return runCtx.Err()
			}
			client, err := s.streamClient(runCtx, t.Connection)
			if err != nil {
				return err
			}
			list, err := client.CoreV1().Pods(t.Namespace).List(runCtx, metav1.ListOptions{Limit: 100})
			if err != nil {
				safe := safeError(runCtx, err)
				var typed *model.Error
				if errors.As(safe, &typed) && typed.Code != "disconnected" {
					return safe
				}
				_ = w.Send("state", []byte("stale: reconnecting Pod watch"))
				if wait(runCtx, backoff) != nil {
					return runCtx.Err()
				}
				if backoff < 5*time.Second {
					backoff *= 2
				}
				continue
			}
			if list.Continue != "" {
				return model.Fail("invalid", "Validation watch is limited to 100 Pods")
			}
			views := make([]PodSummary, 0, len(list.Items))
			for _, pod := range list.Items {
				views = append(views, Summarize(pod))
			}
			payload, err := json.Marshal(views)
			if err != nil {
				return model.Fail("invalid", "Cannot encode Pod snapshot")
			}
			if err = w.Send("snapshot", payload); err != nil && !errors.Is(err, session.ErrResync) {
				return err
			}
			_ = w.Send("state", []byte("running"))
			timeout := int64(30)
			stream, err := client.CoreV1().Pods(t.Namespace).Watch(runCtx, metav1.ListOptions{ResourceVersion: list.ResourceVersion, TimeoutSeconds: &timeout, AllowWatchBookmarks: true})
			if err != nil {
				if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
					return safeError(runCtx, err)
				}
				_ = w.Send("state", []byte("stale: reconnecting Pod watch"))
				if wait(runCtx, backoff) != nil {
					return runCtx.Err()
				}
				continue
			}
			backoff = 250 * time.Millisecond
			resync := false
		watchLoop:
			for {
				select {
				case <-runCtx.Done():
					stream.Stop()
					return runCtx.Err()
				case event, open := <-stream.ResultChan():
					if !open {
						break watchLoop
					}
					if event.Type == watch.Bookmark {
						continue
					}
					if event.Type == watch.Error {
						status, ok := event.Object.(*metav1.Status)
						if ok && (status.Code == 410 || status.Reason == metav1.StatusReasonExpired) {
							resync = true
							break watchLoop
						}
						stream.Stop()
						return safeError(runCtx, apierrors.FromObject(event.Object))
					}
					pod, ok := event.Object.(*corev1.Pod)
					if !ok {
						continue
					}
					payload, err := json.Marshal(struct {
						Type string     `json:"type"`
						Pod  PodSummary `json:"pod"`
					}{string(event.Type), Summarize(*pod)})
					if err != nil {
						stream.Stop()
						return model.Fail("invalid", "Cannot encode Pod update")
					}
					if err = w.Send("update", payload); err != nil {
						if errors.Is(err, session.ErrResync) {
							resync = true
							break watchLoop
						}
						stream.Stop()
						return err
					}
				}
			}
			stream.Stop()
			message := "stale: watch reconnecting"
			if resync {
				message = "stale: watch expired or overflowed; resynchronizing"
			}
			_ = w.Send("state", []byte(message))
			if wait(runCtx, backoff) != nil {
				return runCtx.Err()
			}
		}
	})
}
