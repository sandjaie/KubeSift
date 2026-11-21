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
	"helm.sh/helm/v4/pkg/engine"
	"reflect"
	"strings"
	"testing"
	"text/template"
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

func TestRendererPreservesSDKValues(t *testing.T) {
	request := renderRequest(`{{ if eq .Release.Revision 1 }}first{{ end }}|{{ .Values.large }}|{{ .Values.fraction }}|{{ .Values.empty | toJson }}|{{ .Values.mapping | toJson }}|{{ .Values.stamp | date "2006-01-02" }}|{{ (lookup "v1" "ConfigMap" "owned" "demo").number }}`)
	request.Release["Revision"] = 1
	request.Values = common.Values{"large": int64(9007199254740993), "fraction": 1.25, "empty": []any{}, "mapping": map[string]any{}, "stamp": time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)}
	lookup := func(string, string, string, string) (map[string]any, error) {
		return map[string]any{"number": int64(9007199254740993)}, nil
	}
	native := engine.Engine{CustomTemplateFuncs: template.FuncMap{"lookup": lookup}}
	want, err := native.RenderWithContext(context.Background(), request.Chart.Restore(), common.Values{"Values": request.Values, "Release": request.Release, "Capabilities": request.Capabilities})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Render(context.Background(), request, lookup)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("SDK render changed: got=%v want=%v error=%v", got, want, err)
	}
	_, err = Render(context.Background(), renderRequest(`{{ lookup "v1" "Secret" "owned" "denied" }}`), func(string, string, string, string) (map[string]any, error) {
		return nil, model.Fail("forbidden", "Lookup denied.")
	})
	var problem *model.Problem
	if !errors.As(err, &problem) || problem.Code != "forbidden" {
		t.Fatalf("lookup cause lost: %v", err)
	}
}
