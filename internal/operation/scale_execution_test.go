/*
Purpose: Test scale execution races and HTTP outcomes.
Used by: Native scaling regression tests.
Responsibilities: Count real requests and verify token revocation and uncertainty.
Important: The local HTTP server never targets a real workload.
*/
package operation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func scaleFixtureReply(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method + " " + r.URL.Path {
	case "GET " + scalePath:
		fmt.Fprint(w, `{"kind":"Deployment","apiVersion":"apps/v1","metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":"7"},"spec":{"replicas":1}}`)
	case "GET " + scalePath + "/scale":
		testScaleResponse(w, 1, "7")
	case "POST /apis/authorization.k8s.io/v1/selfsubjectaccessreviews":
		fmt.Fprint(w, `{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":true}}`)
	case "PUT " + scalePath + "/scale":
		if r.URL.Query().Get("dryRun") != "All" {
			return false
		}
		// For zero tests, the mandatory preview admits exactly zero.
		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(400)
			return true
		}
		testScaleResponse(w, 2, "7")
	default:
		return false
	}
	return true
}

func TestScaleWriteIsNeverRetried(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		want string
	}{
		{"conflict", 409, "stale"}, {"denied", 403, "rejected"}, {"rate-limited", 429, "rejected"},
		{"server-error", 503, "unknown"}, {"bad-response", 200, "unknown"}, {"disconnect", -1, "unknown"}, {"admitted-different", 0, "accepted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				if scaleFixtureReply(w, r) {
					return
				}
				if r.Method != "PUT" || r.URL.Path != scalePath+"/scale" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
					return
				}
				calls.Add(1)
				if tc.code == -1 {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					conn.Close()
					return
				}
				if tc.code == 0 {
					testScaleResponse(w, 3, "8")
					return
				}
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(tc.code)
			})
			ctx := context.Background()
			p, err := s.PrepareScale(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.ExecuteScale(ctx, p.Token, "")
			if err != nil || result.State != tc.want || calls.Load() != 1 {
				t.Fatalf("result %+v error %v PUTs %d", result, err, calls.Load())
			}
			if tc.code == 0 && (result.Reported == nil || *result.Reported != 3 || result.Requested != 2) {
				t.Fatalf("admitted count lost %+v", result)
			}
			duplicate, err := s.ExecuteScale(ctx, p.Token, "")
			if err != nil || duplicate.OperationID != result.OperationID || calls.Load() != 1 {
				t.Fatalf("duplicate %+v %v", duplicate, err)
			}
		})
	}
}

func TestScaleExecuteSingleUseAndRevocation(t *testing.T) {
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if scaleFixtureReply(w, r) {
			return
		}
		if r.Method != "PUT" || r.URL.Path != scalePath+"/scale" {
			t.Error("unexpected request")
			w.WriteHeader(404)
			return
		}
		if calls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		testScaleResponse(w, 2, "8")
	})
	ctx, cancel := context.WithCancel(context.Background())
	p, err := s.PrepareScale(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.PrepareScale(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := s.PrepareScale(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	s.RevokeScale(revoked.Token)
	if _, err := s.ExecuteScale(ctx, revoked.Token, ""); err == nil {
		t.Fatal("revoked review accepted")
	}
	result := make(chan ScaleResult, 1)
	go func() { v, _ := s.ExecuteScale(ctx, p.Token, ""); result <- v }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("did not dispatch")
	}
	cancel()
	s.RevokeScale(p.Token)
	duplicate, err := s.ExecuteScale(ctx, p.Token, "")
	if err != nil || duplicate.State != "executing" || calls.Load() != 1 {
		t.Fatalf("duplicate %+v %v", duplicate, err)
	}
	if _, err := s.ExecuteScale(ctx, other.Token, ""); err == nil {
		t.Fatal("second review bypassed target lock")
	}
	close(release)
	select {
	case v := <-result:
		if v.State != "accepted" {
			t.Fatalf("navigation context aborted operation: %+v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("operation did not finish")
	}
	if _, err := s.ExecuteScale(ctx, other.Token, ""); err == nil {
		t.Fatal("busy review still usable")
	}
}

func TestScaleExecuteRevalidates(t *testing.T) {
	var changed atomic.Bool
	var writes atomic.Int32
	s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if changed.Load() && r.Method == "GET" && r.URL.Path == scalePath {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"kind":"Deployment","apiVersion":"apps/v1","metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":"8"},"spec":{"replicas":1}}`)
			return
		}
		if scaleFixtureReply(w, r) {
			return
		}
		writes.Add(1)
		w.WriteHeader(500)
	})
	ctx := context.Background()
	p, err := s.PrepareScale(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	changed.Store(true)
	result, err := s.ExecuteScale(ctx, p.Token, "")
	if err != nil || result.State != "stale" || writes.Load() != 0 {
		t.Fatalf("stale %+v %v", result, err)
	}
	changed.Store(false)
	p, err = s.PrepareScale(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.connections.Close(req.Resource.Connection); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExecuteScale(ctx, p.Token, ""); err == nil || writes.Load() != 0 {
		t.Fatal("closed generation kept review")
	}
}

func TestScaleReviewExpiryAndResultsRetention(t *testing.T) {
	s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if scaleFixtureReply(w, r) {
			return
		}
		w.WriteHeader(403)
	})
	var seconds atomic.Int64
	start := time.Now()
	s.now = func() time.Time { return start.Add(time.Duration(seconds.Load()) * time.Second) }
	ctx := context.Background()
	expired, err := s.PrepareScale(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	seconds.Store(300)
	_, err = s.ExecuteScale(ctx, expired.Token, "")
	checkCode(t, err, "expired")
	var first string
	for i := 0; i < 101; i++ {
		p, err := s.PrepareScale(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		v, err := s.ExecuteScale(ctx, p.Token, "")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = v.OperationID
		}
	}
	if got := s.ListScaleResults(); len(got) != 100 {
		t.Fatalf("retained %d results", len(got))
	}
	if _, err := s.GetScaleResult(first); err == nil {
		t.Fatal("oldest result retained beyond the cap")
	}
}

func TestScaleDisconnectAfterDispatchLeavesUnknown(t *testing.T) {
	arrived := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if scaleFixtureReply(w, r) {
			return
		}
		if r.Method != "PUT" || r.URL.Path != scalePath+"/scale" {
			w.WriteHeader(404)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		if calls.Add(1) == 1 {
			close(arrived)
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	p, err := s.PrepareScale(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan ScaleResult, 1)
	go func() { result, _ := s.ExecuteScale(context.Background(), p.Token, ""); done <- result }()
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("write did not dispatch")
	}
	if err := s.connections.Close(req.Resource.Connection); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if result.State != "unknown" || calls.Load() != 1 {
			t.Fatalf("closed connection produced %+v, PUTs %d", result, calls.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("operation result missing")
	}
}
