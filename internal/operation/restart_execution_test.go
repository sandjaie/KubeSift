/*
Purpose: Verify restart execution and its single-dispatch outcome classification.
Used by: Native operation regression tests.
Responsibilities: Count real HTTP requests across stale, rejection and uncertain outcomes.
Important: Only synthetic loopback API servers see these requests.
*/
package operation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRestartExecution(t *testing.T) {
	for _, tc := range []struct {
		mode, state   string
		writes, reads int32
		differs       bool
	}{
		{"accepted", "accepted", 1, 2, false}, {"changed-admission", "accepted", 1, 2, true},
		{"stale-before", "stale", 0, 2, false}, {"changed-marker-before", "stale", 0, 2, false}, {"stale-422", "stale", 1, 3, false},
		{"rejected-422", "rejected", 1, 3, false}, {"rate-limit", "rejected", 1, 2, false}, {"unauthorized", "rejected", 1, 2, false},
		{"redirect", "unknown", 1, 2, false}, {"ambiguous-500", "unknown", 1, 2, false}, {"malformed", "unknown", 1, 2, false}, {"lost", "unknown", 1, 2, false},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			var reads, writes, dry atomic.Int32
			var mu sync.Mutex
			var prepared string
			original := restartFixture("Deployment", "{"+templateFixture+"}", "{}")
			marked := strings.Replace(original, `"sibling":"secret-fixture"`, `"sibling":"secret-fixture","kubectl.kubernetes.io/restartedAt":"`+fixtureTime+`"`, 1)
			scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost:
					w.WriteHeader(403)
				case r.Method == http.MethodGet && r.URL.Path == scalePath:
					n := reads.Add(1)
					if tc.mode == "stale-before" && n > 1 || tc.mode == "stale-422" && n > 2 {
						fmt.Fprint(w, strings.Replace(original, `"resourceVersion":"7"`, `"resourceVersion":"8"`, 1))
						return
					}
					if tc.mode == "changed-marker-before" && n > 1 {
						fmt.Fprint(w, marked)
						return
					}
					fmt.Fprint(w, original)
				case r.Method == http.MethodPatch && r.URL.Path == scalePath:
					raw, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					mu.Lock()
					defer mu.Unlock()
					if r.URL.RawQuery == "dryRun=All" {
						dry.Add(1)
						prepared = string(raw)
						fmt.Fprint(w, marked)
						return
					}
					writes.Add(1)
					if string(raw) != prepared || r.URL.RawQuery != "" {
						t.Error("dispatched a different patch")
					}
					switch tc.mode {
					case "stale-422", "rejected-422":
						w.WriteHeader(422)
					case "rate-limit":
						w.Header().Set("Retry-After", "0")
						w.WriteHeader(429)
					case "unauthorized":
						w.WriteHeader(401)
					case "redirect":
						http.Redirect(w, r, "/must-not-follow", http.StatusTemporaryRedirect)
					case "ambiguous-500":
						w.WriteHeader(500)
					case "malformed":
						fmt.Fprint(w, original)
					case "lost":
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
						} else {
							_ = conn.Close()
						}
					case "changed-admission":
						fmt.Fprint(w, strings.Replace(marked, "9007199254740993", "9007199254740992", 1))
					default:
						fmt.Fprint(w, marked)
					}
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			})
			service := NewRestartService(scale.connections, scale.coordinator, func() time.Time { parsed, _ := time.Parse(time.RFC3339, fixtureTime); return parsed })
			ctx := context.Background()
			p, err := service.PrepareRestart(ctx, RestartRequest{Resource: req.Resource, SourceVersion: req.SourceVersion})
			if err != nil {
				t.Fatal(err)
			}
			_, err = scale.ExecuteScale(ctx, p.Token, "")
			checkCode(t, err, "invalid_token")
			r, err := service.ExecuteRestart(ctx, p.Token, "")
			if err != nil || r.State != tc.state || r.Restart == nil || r.Restart.ResponseDiffers != tc.differs {
				t.Fatalf("unexpected result %+v %v", r, err)
			}
			replay, err := service.ExecuteRestart(ctx, p.Token, "")
			if err != nil || replay.OperationID != r.OperationID || replay.State != r.State {
				t.Fatalf("bad replay %+v %v", replay, err)
			}
			if dry.Load() != 1 || writes.Load() != tc.writes || reads.Load() != tc.reads {
				t.Fatalf("GET=%d dry=%d writes=%d", reads.Load(), dry.Load(), writes.Load())
			}
		})
	}
}

func TestRestartRecreateAcknowledgementAndClose(t *testing.T) {
	var writes atomic.Int32
	arrived := make(chan struct{})
	release := make(chan struct{})
	original := restartFixture("Deployment", `{"strategy":{"type":"Recreate"},`+templateFixture+"}", "{}")
	marked := strings.Replace(original, `"sibling":"secret-fixture"`, `"sibling":"secret-fixture","kubectl.kubernetes.io/restartedAt":"`+fixtureTime+`"`, 1)
	scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			fmt.Fprint(w, original)
		case http.MethodPost:
			w.WriteHeader(403)
		case http.MethodPatch:
			if r.URL.RawQuery != "" {
				fmt.Fprint(w, marked)
				return
			}
			writes.Add(1)
			close(arrived)
			<-release
			fmt.Fprint(w, marked)
		}
	})
	defer close(release)
	service := NewRestartService(scale.connections, scale.coordinator, func() time.Time { parsed, _ := time.Parse(time.RFC3339, fixtureTime); return parsed })
	ctx := context.Background()
	prepare := func() RestartPreview {
		p, err := service.PrepareRestart(ctx, RestartRequest{Resource: req.Resource, SourceVersion: req.SourceVersion})
		if err != nil || !p.Impact.Recreate {
			t.Fatalf("bad review %+v %v", p, err)
		}
		return p
	}
	invalid := prepare()
	_, err := service.ExecuteRestart(ctx, invalid.Token, "different")
	checkCode(t, err, "acknowledgement")
	revoked := prepare()
	service.RevokeRestart(revoked.Token)
	_, err = service.ExecuteRestart(ctx, revoked.Token, "web")
	checkCode(t, err, "invalid_token")
	active := prepare()
	pending := prepare()
	done := make(chan OperationResult, 1)
	go func() {
		result, err := service.ExecuteRestart(ctx, active.Token, "web")
		if err != nil {
			t.Error(err)
		}
		done <- result
	}()
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("restart did not dispatch")
	}
	if err := scale.connections.Close(req.Resource.Connection); err != nil {
		t.Fatal(err)
	}
	_, err = service.ExecuteRestart(ctx, pending.Token, "web")
	checkCode(t, err, "invalid_token")
	select {
	case result := <-done:
		if result.State != "unknown" || writes.Load() != 1 {
			t.Fatalf("wrong close result %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close did not end the request")
	}
}
