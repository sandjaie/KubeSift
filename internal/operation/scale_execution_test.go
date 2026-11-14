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
	"strings"
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

func TestScaleRejectsChangedStatefulSetImpact(t *testing.T) {
	const endpoint = "/apis/apps/v1/namespaces/default/statefulsets/web"
	var changed atomic.Bool
	var actual atomic.Int32
	s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET " + endpoint:
			policy := "Retain"
			if changed.Load() {
				policy = "Delete"
			}
			fmt.Fprintf(w, `{"kind":"StatefulSet","apiVersion":"apps/v1","metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":"7"},"spec":{"replicas":1,"persistentVolumeClaimRetentionPolicy":{"whenScaled":%q,"whenDeleted":"Retain"}}}`, policy)
		case "GET " + endpoint + "/scale":
			testScaleResponse(w, 1, "7")
		case "POST /apis/authorization.k8s.io/v1/selfsubjectaccessreviews":
			fmt.Fprint(w, `{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":true}}`)
		case "PUT " + endpoint + "/scale":
			if r.URL.Query().Get("dryRun") != "All" {
				actual.Add(1)
			}
			testScaleResponse(w, 0, "7")
		default:
			t.Error("unexpected request")
			w.WriteHeader(404)
		}
	})
	req.Resource.Kind, req.ReplicasText = "StatefulSet", "0"
	ctx := context.Background()
	p, err := s.PrepareScale(ctx, req)
	if err != nil || p.Impact.WhenScaled != "Retain" {
		t.Fatalf("wrong impact %+v %v", p, err)
	}
	changed.Store(true)
	result, err := s.ExecuteScale(ctx, p.Token, "web")
	if err != nil || result.State != "stale" || actual.Load() != 0 {
		t.Fatalf("changed policy %+v %v", result, err)
	}
}

func TestScaleExecutionCapacity(t *testing.T) {
	const prefix = "/apis/apps/v1/namespaces/default/deployments/"
	arrived := make(chan string, 5)
	release := make(chan struct{})
	defer close(release)
	s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews" && r.Method == "POST" {
			fmt.Fprint(w, `{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":true}}`)
			return
		}
		rest, ok := strings.CutPrefix(r.URL.Path, prefix)
		if !ok {
			w.WriteHeader(404)
			t.Error("unexpected fixture path")
			return
		}
		name, isScale := strings.CutSuffix(rest, "/scale")
		kind, version, replicas := "Deployment", "apps/v1", 1
		if isScale {
			kind, version = "Scale", "autoscaling/v1"
		}
		if r.Method == "PUT" {
			if !isScale {
				w.WriteHeader(404)
				t.Error("parent write")
				return
			}
			replicas = 2
			if r.URL.Query().Get("dryRun") == "" {
				_, _ = io.Copy(io.Discard, r.Body)
				arrived <- name
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}
		}
		fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"name":%q,"namespace":"default","uid":"uid","resourceVersion":"7"},"spec":{"replicas":%d}}`, kind, version, name, replicas)
	})
	ctx := context.Background()
	reviews := make([]ScalePreview, 0, 5)
	for i := 0; i < 5; i++ {
		req.Resource.Name = fmt.Sprintf("fixture-%d", i)
		p, err := s.PrepareScale(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		reviews = append(reviews, p)
	}
	finished := make(chan ScaleResult, 4)
	for _, p := range reviews[:4] {
		go func() { result, _ := s.ExecuteScale(ctx, p.Token, ""); finished <- result }()
	}
	for range 4 {
		select {
		case <-arrived:
		case <-time.After(5 * time.Second):
			t.Fatal("four slot limit not exercised")
		}
	}
	if _, err := s.ExecuteScale(ctx, reviews[4].Token, ""); err == nil {
		t.Fatal("fifth execution escaped cap")
	} else {
		checkCode(t, err, "busy")
	}
	if _, err := s.ExecuteScale(ctx, reviews[4].Token, ""); err == nil {
		t.Fatal("busy token survived")
	} else {
		checkCode(t, err, "invalid_token")
	}
	retained := s.ListScaleResults()
	if len(retained) != 4 {
		t.Fatalf("recorded %d in-flight operations", len(retained))
	}
	for _, row := range retained {
		if row.State != "executing" {
			t.Fatal("active result was not retained")
		}
	}
	s.Close()
	for range 4 {
		select {
		case row := <-finished:
			if row.State != "unknown" {
				t.Fatalf("shutdown result %+v", row)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("shutdown did not finish active operation")
		}
	}
}
