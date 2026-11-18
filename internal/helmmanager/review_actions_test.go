/*
Purpose: Exercise all four review paths with synthetic Helm storage.
Used by: The focused Helm review test group.
Responsibilities: Check current-head capture, replacement values, schema failures and review effects.
Important: A fake SDK Kubernetes client is used only for preview; HTTP storage reads remain real and bounded.
*/
package helmmanager

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sandjaie/KubeSift/internal/helmreader"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/kube/fake"
	stored "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func testReviewActions(t *testing.T) {
	for _, verb := range []string{"install", "upgrade", "rollback", "uninstall"} {
		t.Run(verb, func(t *testing.T) {
			raw := archive(t, map[string]string{
				"demo/templates/probe.yaml": `{{- $probe := lookup "v1" "ConfigMap" .Release.Namespace "demo-probe" -}}
# lookup result: {{ $probe.metadata.name }}
`,
				"demo/Chart.yaml":          "apiVersion: v2\nname: demo\nversion: 1.0.0\n",
				"demo/values.yaml":         "message: default\n",
				"demo/templates/cm.yaml":   "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}\n  annotations:\n    helm.sh/resource-policy: keep\ndata:\n  message: {{ .Values.message | quote }}\n  old: {{ default \"absent\" .Values.oldonly | quote }}\n  time: {{ now | quote }}\n",
				"demo/templates/hook.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo-hook\n  annotations:\n    helm.sh/hook: pre-install,pre-upgrade\n",
				"demo/crds/example.yaml":   "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: examples.test.invalid\n",
			})
			chart, err := loadChart(context.Background(), raw)
			if err != nil {
				t.Fatal(err)
			}
			target := Target{Namespace: "owned", Driver: "configmap", ReleaseName: "demo"}
			memory := driver.NewMemory()
			records := []core.ConfigMap{}
			for revision := 1; verb != "install" && revision <= 2; revision++ {
				rel := &stored.Release{Name: "demo", Namespace: "owned", Version: revision, Chart: chart.(*chartv2.Chart), Config: map[string]any{"oldonly": "old"}, Manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\n", Info: &stored.Info{Status: "deployed"}}
				name := "sh.helm.release.v1.demo.v" + strconv.Itoa(revision)
				if err := memory.Create(name, rel); err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(rel)
				records = append(records, core.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "owned", UID: types.UID(fmt.Sprint(revision)), ResourceVersion: "1", Labels: map[string]string{"owner": "helm", "name": "demo", "version": strconv.Itoa(revision)}}, Data: map[string]string{"release": base64.StdEncoding.EncodeToString(encoded)}})
			}
			var writes atomic.Int32
			var denied atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if denied.Load() {
					w.WriteHeader(403)
					fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403}`)
					return
				}
				if r.Method != "GET" {
					writes.Add(1)
					w.WriteHeader(403)
					return
				}
				if r.URL.Path == "/api/v1" {
					json.NewEncoder(w).Encode(metav1.APIResourceList{GroupVersion: "v1", APIResources: []metav1.APIResource{{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}}}})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/configmaps/demo-probe") {
					json.NewEncoder(w).Encode(core.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}, ObjectMeta: metav1.ObjectMeta{Name: "demo-probe", Namespace: "owned"}})
					return
				}
				for _, record := range records {
					if strings.HasSuffix(r.URL.Path, "/"+record.Name) {
						json.NewEncoder(w).Encode(record)
						return
					}
				}
				json.NewEncoder(w).Encode(core.ConfigMapList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMapList"}, Items: records})
			}))
			defer server.Close()
			client, _ := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			s := NewService(nil)
			defer s.Close()
			s.configure = func(ctx context.Context, _ Target, readonly bool) (*configuration, error) {
				if !readonly {
					t.Fatal("preview requested writable configuration")
				}
				cfg := &action.Configuration{KubeClient: &fake.FailingKubeClient{PrintingKubeClient: fake.PrintingKubeClient{Out: io.Discard}, CreateError: fmt.Errorf("unexpected preview create"), UpdateError: fmt.Errorf("unexpected preview update"), DeleteError: fmt.Errorf("unexpected preview delete"), WatchUntilReadyError: fmt.Errorf("unexpected preview hook")}, Releases: storage.Init(memory), Capabilities: common.DefaultCapabilities.Copy(), RESTClientGetter: &nativeGetter{config: &rest.Config{Host: server.URL}, namespace: "owned"}}
				cfg.SetLogger(slog.DiscardHandler)
				cfg.Releases.SetLogger(slog.DiscardHandler)
				return &configuration{sdk: cfg, client: client}, nil
			}
			req := PrepareRequest{Action: verb, Target: target}
			if verb == "install" || verb == "upgrade" {
				c, _ := s.reserve("", 0)
				if err := s.finishChart(context.Background(), c, raw); err != nil {
					t.Fatal(err)
				}
				req.ChartID, req.ValuesYAML = c.ID, "message: replacement\n"
			}
			if verb == "rollback" {
				req.RollbackRecord = &helmreader.RecordRef{Scope: target, Revision: "1", RecordName: records[0].Name, UID: "1", ResourceVersion: "1"}
			}
			review, err := s.Prepare(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if writes.Load() != 0 || review.Options.TimeoutSeconds != 300 || len(review.Warnings) == 0 {
				t.Fatalf("unsafe or misleading preview: %+v", review)
			}
			if verb != "install" && (review.Current == nil || review.Current.Revision != "2") {
				t.Fatal("historical selection became current head")
			}
			if verb == "install" || verb == "upgrade" {
				part, err := s.ReadReview(context.Background(), review.Token, "manifest", 0)
				if err != nil || !strings.Contains(part.Text, `message: "replacement"`) || !strings.Contains(part.Text, `old: "absent"`) {
					t.Fatalf("replacement overrides lost: %v %s", err, part.Text)
				}
				if len(s.reviews[review.Token].sections["hooks"]) == 0 || len(s.reviews[review.Token].sections["crds"]) == 0 || !strings.Contains(s.reviews[review.Token].sections["resources"], "keep") {
					t.Fatal("hook, CRD or keep-policy impact missing")
				}
			}
			denied.Store(true)
			if req.ChartID != "" {
				c, _ := s.reserve("", 0)
				if err := s.finishChart(context.Background(), c, raw); err != nil {
					t.Fatal(err)
				}
				req.ChartID = c.ID
			}
			if _, err := s.Prepare(context.Background(), req); err == nil {
				t.Fatal("Forbidden preparation accepted")
			}
			denied.Store(false)
			s.Discard(review.Token)
			if _, err := s.ReadReview(context.Background(), review.Token, "manifest", 0); err == nil {
				t.Fatal("discarded review survived")
			}
		})
	}
}
