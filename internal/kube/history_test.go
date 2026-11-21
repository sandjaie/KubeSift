/*
Purpose: Verify bounded historical evidence projection.
Used by: Historical metrics reader tests.
Responsibilities: Check evaluation grids, scope labels and independent chart states.
Important: Inputs are synthetic Prometheus matrices.
*/
package kube

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHistoricalMetricsMatrix(t *testing.T) {
	labels := map[string]string{"node": "test", "id": "/"}
	base := `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"node":"test","id":"/"},"values":[%s]}]},"warnings":["private upstream text"]}`
	for _, tc := range []struct {
		values, state string
		count         int
	}{
		{`[30,"0"],[60,"NaN"],[120,"1.25"]`, "available", 2},
		{`[30,"0"],[30,"1"]`, "unsupported", 0},
		{`[31,"1"]`, "unsupported", 0},
		{`[150,"1"]`, "unsupported", 0},
		{`[60,"-1"]`, "unsupported", 0},
	} {
		got := projectHistory([]byte(fmt.Sprintf(base, tc.values)), labels, "uid", true, 30, 120, 30)
		if got.State != tc.state || len(got.Points) != tc.count {
			t.Errorf("%s: %+v", tc.values, got)
		}
		if tc.count > 0 && (got.Points[0].Value != 0 || got.Points[1].Value != 1250 || !got.Qualified) {
			t.Fatal(got)
		}
	}
	if got := projectHistory([]byte(strings.ReplaceAll(fmt.Sprintf(base, `[30,"1"]`), "test", "swapped")), labels, "uid", false, 30, 120, 30); got.State != "unsupported" {
		t.Fatal(got)
	}
}

func TestHistoricalMetricsRead(t *testing.T) {
	for _, scenario := range []string{"project", "node", "node-project", "young", "unconfigured", "gone", "wrong-uid", "deny-before", "deny-after", "changed", "provider-denied", "missing-cpu", "ambiguous", "oversize", "slow-replace", "busy"} {
		t.Run(scenario, func(t *testing.T) {
			var gets, calls atomic.Int32
			now := time.Date(2026, 11, 21, 13, 42, 17, 0, time.UTC)
			created := now.Add(-time.Hour)
			if scenario == "young" {
				created = now
			}
			node := strings.HasPrefix(scenario, "node")
			s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				i := gets.Add(1)
				if scenario == "gone" {
					w.WriteHeader(404)
					return
				}
				if scenario == "deny-before" || scenario == "deny-after" && i == 2 {
					w.WriteHeader(403)
					return
				}
				id := "uid"
				if scenario == "changed" && i == 2 {
					id = "replaced"
				}
				ns := "testing"
				path := "/api/v1/namespaces/testing/pods/target"
				if node {
					ns = ""
					path = "/api/v1/nodes/target"
				}
				if r.URL.Path != path {
					t.Error("unexpected core read")
				}
				fmt.Fprintf(w, `{"metadata":{"uid":%q,"name":"target","namespace":%q,"creationTimestamp":%q},"spec":{"containers":[{"name":"app"}]}}`, id, ns, created.Format(time.RFC3339))
			})
			started := make(chan struct{}, 2)
			backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				i := calls.Add(1)
				q := r.URL.Query()
				if q.Get("limit") != "2" || q.Get("step") != "30" || r.URL.Path != "/api/v1/query_range" || r.Header.Get("Authorization") != "Bearer fixture" {
					t.Error("incorrect provider request")
				}
				if scenario == "slow-replace" || scenario == "busy" {
					started <- struct{}{}
					<-r.Context().Done()
					return
				}
				if scenario == "provider-denied" {
					w.WriteHeader(403)
					return
				}
				if scenario == "oversize" {
					fmt.Fprint(w, strings.Repeat(" ", (1<<20)+1))
					return
				}
				labels := map[string]string{"namespace": "testing", "pod": "target", "container": "app", "cluster": "a"}
				if node {
					labels = map[string]string{"node": "target", "id": "/", "cluster": "a"}
				}
				if (scenario == "project") != (q.Get("namespace") == "testing") {
					t.Error("wrong tenancy")
				}
				expr := q.Get("query")
				for k, v := range labels {
					if !strings.Contains(expr, k+"="+strconv.Quote(v)) {
						t.Error("missing matcher", k)
					}
				}
				if !strings.HasSuffix(expr, "[5m])") || i == 1 && (!strings.HasPrefix(expr, "rate(container_cpu_usage_seconds_total{") || !strings.Contains(expr, `cpu=~"total|"`)) || i == 2 && !strings.HasPrefix(expr, "last_over_time(container_memory_working_set_bytes{") {
					t.Error("incorrect expression")
				}
				encoded, _ := json.Marshal(labels)
				series := fmt.Sprintf(`{"metric":%s,"values":[[%s,"0"]]}`, encoded, q.Get("end"))
				if scenario == "missing-cpu" && i == 1 {
					series = ""
				}
				if scenario == "ambiguous" && q.Get("dedup") == "false" {
					series += "," + series + "," + series
				}
				fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[%s]}}`, series)
			}))
			defer backend.Close()
			ca := filepath.Join(t.TempDir(), "ca.pem")
			if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: backend.Certificate().Raw}), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := HistoryConfig{Origin: backend.URL, CAFile: ca, Mode: "cluster", Cluster: "a", Bearer: "fixture"}
			if scenario == "project" || scenario == "node-project" {
				cfg.Mode = "project"
			}
			v, err := s.SetHistorySource(ref, "", cfg)
			if err != nil {
				t.Fatal(err)
			}
			r := HistoryRequest{Resource: ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Pod", Namespace: "testing", Name: "target", UID: "uid"}, Preset: "1h", SourceID: v.Revision, Container: "app"}
			if node {
				r.Resource.Kind = "Node"
				r.Resource.Namespace = ""
				r.Container = ""
			}
			if scenario == "wrong-uid" {
				r.Resource.UID = "other"
			}
			if scenario == "unconfigured" {
				r.SourceID = "unknown"
			}
			if scenario == "busy" || scenario == "slow-replace" {
				n := 1
				if scenario == "busy" {
					n = 2
				}
				done := make(chan error, n)
				for range n {
					go func() { _, err := s.readHistoricalMetrics(context.Background(), r, now); done <- err }()
					select {
					case <-started:
					case <-time.After(3 * time.Second):
						t.Fatal("provider did not start")
					}
				}
				if n == 2 {
					if _, err := s.readHistoricalMetrics(context.Background(), r, now); err == nil {
						t.Fatal("queued excess work")
					}
				}
				if _, err := s.ClearHistorySource(ref, v.Revision); err != nil {
					t.Fatal(err)
				}
				for range n {
					select {
					case err := <-done:
						if err == nil {
							t.Fatal("obsolete read")
						}
					case <-time.After(time.Second):
						t.Fatal("cancellation stalled")
					}
				}
				if len(s.historyReads) != 0 {
					t.Fatal("capacity retained")
				}
				return
			}
			out, err := s.readHistoricalMetrics(context.Background(), r, now)
			switch scenario {
			case "gone":
				if err == nil || !strings.Contains(err.Error(), "stale_target") || calls.Load() != 0 {
					t.Fatal("object loss not classified", err)
				}
			case "unconfigured", "node-project", "deny-before", "wrong-uid":
				if err == nil || calls.Load() != 0 {
					t.Fatal("unexpected dispatch")
				}
			case "changed", "deny-after", "provider-denied":
				if err == nil || len(out.CPU.Points) > 0 || scenario == "provider-denied" && calls.Load() != 1 {
					t.Fatal("retained denied data")
				}
			case "young":
				if err != nil || calls.Load() != 0 || out.CPU.State != "missing" {
					t.Fatal(out, err)
				}
			default:
				if err != nil || calls.Load() != 2 || gets.Load() != 2 || out.EffectiveStart < created.Unix()+300 || out.End != now.Unix()/30*30 || out.EffectiveStart%30 != 0 {
					t.Fatal(out, err)
				}
				want := "available"
				switch scenario {
				case "ambiguous":
					want = "ambiguous"
				case "oversize":
					want = "capacity"
				case "missing-cpu":
					want = "missing"
				}
				if out.CPU.State != want || scenario == "missing-cpu" && out.Memory.State != "available" {
					t.Fatal(out)
				}
			}
		})
	}
}
