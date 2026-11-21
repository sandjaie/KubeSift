/*
Purpose: Check native Helm worker ownership and duplicate-confirm safety.
Used by: The focused Helm execution test group.
Responsibilities: Hold locks until actual worker termination and retain truthful cancellation outcomes.
Important: The controllable worker makes no cluster changes.
*/
package helmmanager

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/helmreader"
)

func TestHelmExecution(t *testing.T) {
	warmRenderer(t)
	t.Run("SDK action cancellation", testActionCancellation)
	t.Run("stale heads and rollback records", func(t *testing.T) { testReviewFixture(t, true) })
	t.Run("background SDK HTTP is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		entered := make(chan struct{})
		stopped := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			<-r.Context().Done()
			close(stopped)
		}))
		defer server.Close()
		transport := &operationTransport{ctx: ctx, next: http.DefaultTransport}
		request, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		finished := make(chan error, 1)
		go func() { _, err := transport.RoundTrip(request); finished <- err }()
		<-entered
		cancel()
		select {
		case err := <-finished:
			if err == nil {
				t.Fatal("cancelled request succeeded")
			}
		case <-time.After(time.Second):
			t.Fatal("SDK background request survived cancellation")
		}
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("server still has a live request")
		}
	})
	t.Run("late transport loss is unknown", func(t *testing.T) {
		s := NewService(nil)
		defer s.Close()
		s.perform = func(context.Context, *reviewData) (*helmreader.ReleaseSummary, bool, error) {
			return nil, true, &net.OpError{Op: "read", Net: "tcp", Err: errors.New("synthetic connection lost")}
		}
		s.reviews["late"] = &reviewData{Review: Review{Token: "late", Action: "install", Options: Options{TimeoutSeconds: 60}, ExpiresAt: time.Now().Add(time.Minute)}}
		result, err := s.Execute("late")
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(time.Second)
		for {
			result, err = s.Execute("late")
			if err != nil {
				t.Fatal(err)
			}
			if result.State != "running" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("worker did not stop")
			}
			time.Sleep(time.Millisecond)
		}
		if result.State != "unknown" {
			t.Fatalf("late failure claimed %s", result.State)
		}
	})

	s := NewService(nil)
	defer s.Close()
	var calls atomic.Int32
	started := make(chan struct{}, 2)
	finish := make(chan struct{})
	s.perform = func(ctx context.Context, r *reviewData) (*helmreader.ReleaseSummary, bool, error) {
		calls.Add(1)
		started <- struct{}{}
		<-finish
		return nil, true, ctx.Err()
	}
	add := func(token, name string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.reviews[token] = &reviewData{Review: Review{Token: token, Target: Target{Namespace: "owned", Driver: "secret", ReleaseName: name}, Action: "install", Options: Options{TimeoutSeconds: 60}, ExpiresAt: time.Now().Add(time.Minute)}}
	}
	add("one", "demo")
	first, err := s.Execute("one")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	duplicate, err := s.Execute("one")
	if err != nil || first.ID != duplicate.ID || calls.Load() != 1 {
		t.Fatal("duplicate confirmation started another operation")
	}
	add("same", "demo")
	if _, err := s.Execute("same"); err == nil {
		t.Fatal("same release executed concurrently")
	}
	add("two", "other")
	second, err := s.Execute("two")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	add("three", "third")
	if _, err := s.Execute("three"); err == nil {
		t.Fatal("global worker cap ignored")
	}
	if err := s.Cancel(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute("same"); err == nil {
		t.Fatal("cancel released the lock before the worker terminated")
	}
	close(finish)
	deadline := time.Now().Add(time.Second)
	for {
		results := s.ListResults()
		finished := 0
		for _, result := range results {
			if result.State != "running" {
				finished++
			}
			if result.ID == first.ID && result.State != "running" && result.State != "unknown" {
				t.Fatalf("cancelled partial work claimed %s", result.State)
			}
			if result.ID == second.ID && result.State != "running" && result.State != "succeeded" {
				t.Fatalf("completed worker claimed %s", result.State)
			}
		}
		if finished == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("workers did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 2 {
		t.Fatal("worker ran more than once")
	}
}
