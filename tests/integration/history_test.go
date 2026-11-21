//go:build integration

/*
Purpose: Compare guarded Pod and Node historical usage with synthetic Prometheus evaluation.
Used by: Opt-in local Phase 1E.4 verification.
Responsibilities: Check resets, exact cluster scope, missing and ambiguous series on both approved contexts.
Important: Private fixture configuration is supplied by the operator and never logged.
*/
package integration

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/session"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHistoricalMetricsIntegration(t *testing.T) {
	if os.Getenv("KUBESIFT_HISTORY_INTEGRATION") != "1" {
		t.Skip("explicit history integration opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("private fixture directory required")
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(b))
	}
	first, err := strconv.ParseInt(read("series-start"), 10, 64)
	if err != nil {
		t.Fatal("invalid fixture start")
	}
	origin := read("origin")
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || !net.ParseIP(u.Hostname()).IsLoopback() {
		t.Fatal("expected loopback HTTPS fixture")
	}
	manager := connection.NewManager(nil)
	defer manager.CloseAll()
	sessions := session.NewManager(nil)
	defer sessions.Close()
	service := kube.NewService(manager, sessions)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	var prevUID string
	for i, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			ref, client := openLocalOperationFixture(t, ctx, manager, dir, suffix, "")
			ns, err := client.CoreV1().Namespaces().Get(ctx, "kubesift-history-check", metav1.GetOptions{})
			if err != nil || string(ns.UID) != read("namespace-"+suffix+".uid") {
				t.Fatal("unexpected fixture namespace")
			}
			pod, err := client.CoreV1().Pods(ns.Name).Get(ctx, "historical-usage", metav1.GetOptions{})
			if err != nil || string(pod.UID) == prevUID {
				t.Fatal("expected distinct Pod identity", err)
			}
			prevUID = string(pod.UID)
			node, err := client.CoreV1().Nodes().Get(ctx, "dev-local-"+suffix+"-control-plane", metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			cfg := kube.HistoryConfig{Origin: origin, CAFile: filepath.Join(dir, "server.pem"), Mode: "project", Cluster: suffix, Bearer: read("token")}
			source, err := service.SetHistorySource(ref, "", cfg)
			if err != nil {
				t.Fatal(err)
			}
			wait := time.Until(pod.CreationTimestamp.Add(6 * time.Minute))
			if wait > 0 {
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			req := kube.HistoryRequest{Resource: kube.ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Pod", Namespace: ns.Name, Name: pod.Name, UID: string(pod.UID)}, Preset: "1h", SourceID: source.Revision, Container: "app"}
			check := func(factor float64) {
				t.Helper()
				out, err := service.ReadHistoricalMetrics(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
				if len(out.CPU.Points) == 0 || len(out.Memory.Points) == 0 {
					t.Fatalf("expected points, CPU %s memory %s", out.CPU.State, out.Memory.State)
				}
				checked := 0
				for _, p := range out.CPU.Points {
					if p.Time < first+300 {
						continue
					}
					checked++
					if math.Abs(p.Value-200*factor) > 0.001 {
						t.Fatalf("reset-adjusted rate %f, expected %f", p.Value, 200*factor)
					}
				}
				if checked == 0 {
					t.Fatal("no complete five-minute CPU windows")
				}
				for _, p := range out.Memory.Points {
					if p.Value != 32*(1<<20)*factor {
						t.Fatal("unexpected working set")
					}
				}
				t.Logf("%s %s: %d CPU and %d memory evaluations", suffix, req.Resource.Kind, len(out.CPU.Points), len(out.Memory.Points))
			}
			check(float64(i + 1))
			set := func() {
				source, err = service.SetHistorySource(ref, source.Revision, cfg)
				if err != nil {
					t.Fatal(err)
				}
				req.SourceID = source.Revision
			}
			cfg.Cluster = "absent"
			set()
			out, err := service.ReadHistoricalMetrics(ctx, req)
			if err != nil || out.CPU.State != "missing" || out.Memory.State != "missing" {
				t.Fatal("expected missing series", err)
			}
			cfg.Cluster = ""
			cfg.SingleCluster = true
			set()
			out, err = service.ReadHistoricalMetrics(ctx, req)
			if err != nil || out.CPU.State != "ambiguous" || out.Memory.State != "ambiguous" {
				t.Fatal("expected duplicate cluster series", err)
			}
			cfg.Cluster = suffix
			cfg.SingleCluster = false
			cfg.Mode = "cluster"
			set()
			req.Resource = kube.ResourceRef{Connection: ref, APIVersion: "v1", Kind: "Node", Name: node.Name, UID: string(node.UID)}
			req.Container = ""
			check(4 * float64(i+1))
		})
	}
}
