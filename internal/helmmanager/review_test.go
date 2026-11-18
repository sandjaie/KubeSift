/*
Purpose: Check review input, transport and content boundaries before execution exists.
Used by: Native Helm management tests.
Responsibilities: Reject ambiguous values, mutation attempts and invented section offsets.
Important: Review tests never contact a real cluster.
*/
package helmmanager

import (
	"bytes"
	"context"
	chartutil "helm.sh/helm/v4/pkg/chart/common/util"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHelmReview(t *testing.T) {
	t.Run("native namespace never falls back to default", func(t *testing.T) {
		getter := &nativeGetter{namespace: "reviewed-namespace"}
		namespace, _, err := getter.ToRawKubeConfigLoader().Namespace()
		if err != nil || namespace != "reviewed-namespace" {
			t.Fatalf("captured namespace was not usable: %q %v", namespace, err)
		}
	})
	t.Run("action previews", testReviewActions)
	t.Run("application diagnostics remain usable", func(t *testing.T) {
		s := NewService(nil)
		defer s.Close()
		slog.Info("synthetic application diagnostic")
	})
	t.Run("schema validation and payload logging", func(t *testing.T) {
		var output bytes.Buffer
		previous := slog.Default()
		outputBefore := log.Writer()
		slog.SetDefault(slog.New(helmLogFilter{slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})}))
		defer func() { slog.SetDefault(previous); log.SetOutput(outputBefore) }()
		ch, err := loadChart(context.Background(), tinyChart(t))
		if err != nil {
			t.Fatal(err)
		}
		v2 := ch.(*chartv2.Chart)
		v2.Values = map[string]any{"table": map[string]any{"key": "default"}}
		_, _ = chartutil.CoalesceValues(v2, map[string]any{"table": "private-sentinel"})
		if strings.Contains(output.String(), "private-sentinel") {
			t.Fatal("SDK leaked values")
		}
		v2.Schema = []byte(`{"type":"object","properties":{"message":{"type":"integer"}}}`)
		if err := chartutil.ValidateAgainstSchema(v2, map[string]any{"message": "invalid"}); err == nil {
			t.Fatal("schema validation bypassed")
		}
		if err := localSchema(map[string]any{"$ref": "file:///private/secret"}, 0); err == nil {
			t.Fatal("external schema accepted")
		}
	})
	t.Run("render cap and expiry", func(t *testing.T) {
		r := &reviewData{sections: map[string]string{"manifest": strings.Repeat("x", renderedLimit+1)}, offsets: map[string]map[int]bool{}}
		if err := reviewContent(r); err == nil {
			t.Fatal("render cap ignored")
		}
		s := NewService(nil)
		defer s.Close()
		r.Token = "expired"
		r.ExpiresAt = time.Now().Add(-time.Second)
		s.reviews[r.Token] = r
		if _, err := s.ReadReview(context.Background(), r.Token, "manifest", 0); err == nil {
			t.Fatal("expired review accepted")
		}
	})

	t.Run("values are one bounded mapping", func(t *testing.T) {
		for _, text := range []string{"- item", "a: 1\na: 2", "a: 1\n---\nb: 2", strings.Repeat("x", (1<<20)+1)} {
			if _, err := reviewValues(text); err == nil {
				t.Fatalf("invalid values accepted: %.50s", text)
			}
		}
		values, err := reviewValues("message: replacement\n")
		if err != nil || values["message"] != "replacement" || len(values) != 1 {
			t.Fatalf("override mapping: %v %v", values, err)
		}
	})
	t.Run("preview transport blocks all mutation verbs", func(t *testing.T) {
		transport := &operationTransport{ctx: context.Background(), readOnly: true}
		for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
			req, _ := http.NewRequest(method, "https://unused.invalid/api", nil)
			if _, err := transport.RoundTrip(req); err == nil {
				t.Fatalf("preview accepted %s", method)
			}
		}
	})
	t.Run("section offsets are issued and UTF8 safe", func(t *testing.T) {
		s := NewService(nil)
		defer s.Close()
		r := &reviewData{Review: Review{Token: "test"}, sections: map[string]string{"manifest": strings.Repeat("界", 800000)}, offsets: map[string]map[int]bool{"manifest": {0: true}}}
		r.ExpiresAt = time.Now().Add(time.Minute)
		s.reviews[r.Token] = r
		if _, err := s.ReadReview(context.Background(), r.Token, "manifest", 1); err == nil {
			t.Fatal("invented offset accepted")
		}
		part, err := s.ReadReview(context.Background(), r.Token, "manifest", 0)
		if err != nil || part.Done || part.NextOffset%3 != 0 {
			t.Fatalf("bad first chunk: %v", err)
		}
		text := part.Text
		for !part.Done {
			part, err = s.ReadReview(context.Background(), r.Token, "manifest", part.NextOffset)
			if err != nil {
				t.Fatal(err)
			}
			text += part.Text
		}
		if text != r.sections["manifest"] {
			t.Fatal("chunk content lost")
		}
	})
}

func TestReviseHelmReviewTransfersChart(t *testing.T) {
	s := NewService(nil)
	defer s.Close()
	raw := tinyChart(t)
	r := &reviewData{Review: Review{Token: "review", ChartName: "demo", ChartVersion: "1.0.0", Digest: chartHash(raw), ExpiresAt: time.Now().Add(time.Minute)}, raw: raw}
	s.reviews[r.Token] = r
	for range 3 {
		if _, err := s.reserve("", 0); err != nil {
			t.Fatal(err)
		}
	}
	chart, err := s.ReviseReview(r.Token)
	if err != nil || chart.Digest != r.Digest {
		t.Fatalf("chart transfer failed: %v", err)
	}
	if _, err := s.Execute(r.Token); err == nil {
		t.Fatal("old confirmation survived editing")
	}
	if _, err := s.reserve("", 0); err == nil {
		t.Fatal("chart transfer lost its draft reservation")
	}
	s.Discard(chart.ID)
}
