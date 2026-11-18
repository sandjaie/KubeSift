/*
Purpose: Prove the isolated renderer's memory, deadline and lookup boundaries.
Used by: Native Helm rendering regression tests.
Responsibilities: Reject oversized output and allocations without leaving CPU work running.
Important: Only synthetic templates and an in-memory lookup callback are evaluated.
*/
package helmrender

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/helmrender/wire"
	"github.com/sandjaie/KubeSift/internal/model"
	"helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
)

func renderRequest(text string) wire.Request {
	return wire.Request{
		Chart:  wire.Capture(&chartv2.Chart{Metadata: &chartv2.Metadata{Name: "demo", Version: "1.0.0", APIVersion: "v2"}, Templates: []*common.File{{Name: "templates/demo.yaml", Data: []byte(text)}}}),
		Values: common.Values{"message": "hello"}, Release: map[string]any{"Name": "demo", "Namespace": "owned"}, Capabilities: common.DefaultCapabilities.Copy(),
	}
}

func TestBoundedRenderer(t *testing.T) {
	calls := 0
	lookup := func(api, kind, namespace, name string) (map[string]any, error) {
		calls++
		if api != "v1" || kind != "ConfigMap" || namespace != "owned" || name != "demo" {
			t.Fatal("lookup target changed")
		}
		return map[string]any{"data": map[string]any{"message": "found"}}, nil
	}
	files, err := Render(context.Background(), renderRequest(`{{ .Values.message }} {{ (lookup "v1" "ConfigMap" .Release.Namespace .Release.Name).data.message }}`), lookup)
	if err != nil || calls != 1 || !strings.Contains(files["demo/templates/demo.yaml"], "hello found") {
		t.Fatalf("native lookup render failed: %v %v", files, err)
	}
	for _, text := range []string{`{{ repeat 9437184 "x" }}`, `{{ repeat 1073741824 "x" }}`} {
		_, err := Render(context.Background(), renderRequest(text), lookup)
		var problem *model.Problem
		if !errors.As(err, &problem) || problem.Code != "capacity" {
			t.Fatalf("unbounded render accepted: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = Render(ctx, renderRequest(`{{range until 1000000}}{{range until 1000000}}{{end}}{{end}}`), lookup)
	if err == nil || ctx.Err() == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("CPU-bound renderer did not join on deadline: %v", err)
	}
}
