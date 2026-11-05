/*
Purpose: Check session cancellation and queue bounds under a log flood.
Used by: The Go unit suite and review gates.
Responsibilities: Stop workers during setup and activity, repeat stop, and verify visible truncation without retained sessions.
Important: Coordinate through channels; timeouts are failure limits rather than scheduling delays.
*/
package session

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/model"
	"sync"
	"testing"
	"time"
)

func TestSessionCancellationAndBounds(t *testing.T) {
	target := model.Target{Connection: model.ConnectionRef{ID: "fixture", Generation: 1}, Namespace: "payments", Name: "pod", Container: "fixture"}
	for _, phase := range []string{"setup", "active"} {
		t.Run(phase, func(t *testing.T) {
			manager := NewManager(func(model.Event) {})
			defer manager.Close()
			entered := make(chan struct{})
			exited := make(chan struct{})
			var writer *Writer
			ref, err := manager.Start(context.Background(), target, "logs", func(ctx context.Context, w *Writer) error {
				writer = w
				if phase == "active" {
					if err := w.Send("data", []byte("fixture")); err != nil {
						return err
					}
				}
				close(entered)
				defer close(exited)
				<-ctx.Done()
				return ctx.Err()
			})
			if err != nil {
				t.Fatal(err)
			}
			<-entered
			cleaned := make(chan struct{})
			if err = manager.OnStop(ref, func() { close(cleaned) }); err != nil {
				t.Fatal(err)
			}
			if err = manager.Stop(ref); err != nil {
				t.Fatal(err)
			}
			if err = manager.Stop(ref); err != nil {
				t.Fatal(err)
			}
			select {
			case <-cleaned:
			default:
				t.Fatal("owned resource cleanup did not finish")
			}
			select {
			case <-exited:
			default:
				t.Fatal("worker survived stop")
			}
			if manager.Stats().Sessions != 0 {
				t.Fatal("session retained after stop")
			}
			if writer.Send("data", []byte("late")) == nil {
				t.Fatal("cancelled writer accepted data")
			}
		})
	}
	t.Run("bounded-flood", func(t *testing.T) {
		blocked := make(chan struct{})
		firstSequence := make(chan uint64, 1)
		truncated := make(chan struct{})
		flooded := make(chan struct{})
		var blockOnce, truncateOnce sync.Once
		manager := NewManager(func(event model.Event) {
			if event.Kind == "data" {
				blockOnce.Do(func() { close(blocked); firstSequence <- event.Sequence })
			}
			if event.Kind == "truncated" {
				truncateOnce.Do(func() { close(truncated) })
			}
		})
		manager.RequireBridgeAcknowledgements()
		defer manager.Close()
		ref, err := manager.Start(context.Background(), target, "logs", func(ctx context.Context, w *Writer) error {
			chunk := make([]byte, 64<<10)
			if err := w.Send("data", chunk); err != nil {
				return err
			}
			<-blocked
			for i := 0; i < 40; i++ {
				if err := w.Send("data", chunk); err != nil {
					return err
				}
			}
			close(flooded)
			<-ctx.Done()
			return ctx.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
		<-flooded
		stats := manager.Stats()
		if stats.QueuedBytes > 1<<20 || stats.InFlightBytes > 64<<10 || stats.DroppedLogBytes == 0 {
			t.Fatalf("unbounded or silent flood: %+v", stats)
		}
		if err = manager.Acknowledge(ref, <-firstSequence); err != nil {
			t.Fatal(err)
		}
		select {
		case <-truncated:
		case <-time.After(2 * time.Second):
			t.Fatal("truncation was not visible")
		}
		if err = manager.Stop(ref); err != nil {
			t.Fatal(err)
		}
		if manager.Stats().Sessions != 0 {
			t.Fatal("flood worker leaked")
		}
	})
}
