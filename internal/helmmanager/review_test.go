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
	"github.com/sandjaie/KubeSift/internal/helmrender"
	"github.com/sandjaie/KubeSift/internal/helmrender/wire"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chartutil "helm.sh/helm/v4/pkg/chart/common/util"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/kube/fake"
	stored "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	"io"
	"k8s.io/client-go/rest"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBothActionRendersAreBounded(t *testing.T) {
	raw := archive(t, map[string]string{
		"demo/Chart.yaml":        "apiVersion: v2\nname: demo\nversion: 1.0.0\n",
		"demo/templates/cm.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\ndata:\n  value: {{ repeat 9437184 \"x\" | quote }}\n",
	})
	for _, preview := range []bool{true, false} {
		cfg := &action.Configuration{KubeClient: &fake.PrintingKubeClient{Out: io.Discard}, Releases: storage.Init(driver.NewMemory()), Capabilities: common.DefaultCapabilities.Copy(), RESTClientGetter: &nativeGetter{config: &rest.Config{Host: "https://unused.invalid"}, namespace: "owned"}}
		cfg.SetLogger(slog.DiscardHandler)
		req := PrepareRequest{Action: "install", Target: Target{Namespace: "owned", ReleaseName: "demo"}, Options: Options{TimeoutSeconds: 60}}
		ch, err := loadChart(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if preview {
			_, err = previewRelease(context.Background(), cfg, req, ch, map[string]any{})
		} else {
			err = invokeAction(context.Background(), cfg, &reviewData{Review: Review{Action: "install", Target: req.Target, Options: req.Options}, request: req, raw: raw})
		}
		if err == nil {
			t.Errorf("preview=%v accepted oversized render", preview)
		}
	}
}

func TestManagementStorageOutputBudget(t *testing.T) {
	ch, err := loadChart(context.Background(), tinyChart(t))
	if err != nil {
		t.Fatal(err)
	}
	rel := &stored.Release{Name: "demo", Namespace: "owned", Version: 1, Info: &stored.Info{Status: "deployed"}, Chart: ch.(*chartv2.Chart), Manifest: "kind: ConfigMap\n"}
	rel.Chart.Files = []*common.File{{Name: "large.txt", Data: []byte(strings.Repeat("x", 7<<20))}}
	d := &managementStorage{Driver: driver.NewMemory()}
	if err := validateStoredSize(rel); err == nil {
		t.Fatal("oversized proposed release passed preflight")
	}
	if err := d.Create("demo", rel); err == nil {
		t.Fatal("oversized release reached storage")
	}
	if err := d.Update("demo", rel); err == nil {
		t.Fatal("oversized release update reached storage")
	}
}

func TestHelmReview(t *testing.T) {
	warmRenderer(t)
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

func warmRenderer(t *testing.T) {
	t.Helper()
	ch, err := loadChart(context.Background(), tinyChart(t))
	if err != nil {
		t.Fatal(err)
	}
	// Compile trusted code before timing action cancellation; -race instrumentation makes compilation much slower.
	_, err = helmrender.Render(context.Background(), wire.Request{Chart: wire.Capture(ch.(*chartv2.Chart)), Values: common.Values{"message": "warm"}, Release: map[string]any{"Name": "warm", "Namespace": "owned"}, Capabilities: common.DefaultCapabilities.Copy()}, nil)
	if err != nil {
		t.Fatal(err)
	}
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

func TestBoundedActionPreservesDependenciesAndStoredChart(t *testing.T) {
	raw := archive(t, map[string]string{
		"demo/Chart.yaml":                          "apiVersion: v2\nname: demo\nversion: 1.0.0\ndependencies:\n- name: child\n  version: 1.0.0\n  alias: renamed\n- name: disabled\n  version: 1.0.0\n  condition: disabled.enabled\n",
		"demo/values.yaml":                         "renamed:\n  message: inherited\ndisabled:\n  enabled: false\n",
		"demo/templates/cm.yaml":                   "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ include \"child.name\" . }}\ndata:\n  value: {{ .Subcharts.renamed.Values.message | quote }}\n",
		"demo/charts/disabled/Chart.yaml":          "apiVersion: v2\nname: disabled\nversion: 1.0.0\n",
		"demo/charts/disabled/templates/fail.yaml": `{{ fail "disabled dependency rendered" }}`,
		"demo/templates/hook-a.yaml":               "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: hook-a\n  annotations:\n    helm.sh/hook: pre-install\n",
		"demo/templates/hook-b.yaml":               "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: hook-b\n  annotations:\n    helm.sh/hook: post-install\n",
		"demo/templates/NOTES.txt":                 "Installed {{ .Release.Name }} in {{ .Release.Namespace }}",
		"demo/charts/child/Chart.yaml":             "apiVersion: v2\nname: child\nversion: 1.0.0\n",
		"demo/charts/child/templates/_helpers.tpl": "{{ define \"child.name\" }}child-name{{ end }}",
		"demo/charts/child/templates/cm.yaml":      "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: child\ndata:\n  value: {{ .Values.message | quote }}\n",
	})
	cfg := &action.Configuration{KubeClient: &fake.PrintingKubeClient{Out: io.Discard}, Releases: storage.Init(&managementStorage{Driver: driver.NewMemory()}), Capabilities: common.DefaultCapabilities.Copy(), RESTClientGetter: &nativeGetter{config: &rest.Config{Host: "https://unused.invalid"}, namespace: "owned"}}
	cfg.SetLogger(slog.DiscardHandler)
	r := &reviewData{Review: Review{Action: "install", Target: Target{Namespace: "owned", ReleaseName: "demo"}, Options: Options{TimeoutSeconds: 60, DisableHooks: true}}, raw: raw}
	if err := invokeAction(context.Background(), cfg, r); err != nil {
		t.Fatal(err)
	}
	storedRelease, err := cfg.Releases.Last("demo")
	if err != nil {
		t.Fatal(err)
	}
	rel := storedRelease.(*stored.Release)
	if len(rel.Hooks) != 2 || rel.Hooks[0].Path != "demo/templates/hook-a.yaml" || rel.Hooks[1].Path != "demo/templates/hook-b.yaml" {
		t.Fatalf("original hook paths lost: %#v %#v", rel.Hooks[0], rel.Hooks[1])
	}
	if !strings.Contains(rel.Manifest, "child-name") || strings.Count(rel.Manifest, "inherited") != 2 || rel.Info.Notes != "Installed demo in owned" || len(rel.Chart.Dependencies()) != 1 {
		t.Fatalf("SDK semantics changed: %s %q", rel.Manifest, rel.Info.Notes)
	}
	for _, ch := range []*chartv2.Chart{rel.Chart, rel.Chart.Dependencies()[0]} {
		for _, file := range ch.Templates {
			if strings.Contains(string(file.Data), "kubesiftBoundedRender") {
				t.Fatal("temporary adapter template leaked into stored release")
			}
		}
	}
}
