//go:build integration

/*
Purpose: Validate workload investigation against the two approved local kind clusters.
Used by: The opt-in Phase 1C.2 integration command.
Responsibilities: Check A/B UIDs, detail/YAML/events, owned Pods and bounded log cleanup.
Important: Only isolated kind-dev-local-a/b loopback sources are accepted; all application calls are reads.
*/
package integration

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkloadInvestigation(t *testing.T) {
	if os.Getenv("KUBESIFT_EXPLORER_INTEGRATION") != "1" {
		t.Skip("explicit opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("absolute isolated configuration directory required")
	}
	manager := connection.NewManager(nil)
	defer manager.CloseAll()
	events := make(chan model.Event, 128)
	sessions := session.NewManager(func(e model.Event) { events <- e })
	sessions.RequireBridgeAcknowledgements()
	defer sessions.Close()
	reader := kube.NewService(manager, sessions)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	uids := []string{}
	for _, suffix := range []string{"a", "b"} {
		snapshot, err := config.Load([]string{filepath.Join(dir, suffix+".kubeconfig")})
		if err != nil {
			t.Fatal(err)
		}
		contexts := snapshot.Contexts()
		if len(contexts) != 1 || contexts[0].Name != "kind-dev-local-"+suffix {
			t.Fatal("unexpected context")
		}
		resolved, err := snapshot.Resolve("kind-dev-local-" + suffix)
		if err != nil {
			t.Fatal(err)
		}
		endpoint, err := url.Parse(resolved.RESTConfig().Host)
		if err != nil || (endpoint.Hostname() != "localhost" && endpoint.Hostname() != "127.0.0.1") {
			t.Fatal("nonlocal endpoint rejected")
		}
		ref, err := manager.Open(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		page, err := reader.ReadWorkloads(ctx, kube.PageRequest{Connection: ref, Kind: "Pod", Namespace: "kubesift-explorer"})
		if err != nil {
			t.Fatal(err)
		}
		var failing kube.ResourceRef
		for _, row := range page.Items {
			if row.Resource.Name == "same-pod" {
				uids = append(uids, row.Resource.UID)
			}
			if row.Resource.Name == "missing-setting" {
				failing = row.Resource
			}
		}
		if failing.UID == "" {
			t.Fatal("failing fixture absent")
		}
		detail, err := reader.ReadResource(ctx, kube.DetailRequest{Resource: failing})
		if err != nil || !strings.Contains(detail.ObjectJSON, "missing-setting") {
			t.Fatal("overview", err)
		}
		yaml, err := reader.ReadResourceYAML(ctx, kube.DetailRequest{Resource: failing})
		if err != nil || !strings.Contains(yaml.WithoutManagedFields, "missing-setting") {
			t.Fatal("YAML", err)
		}
		if _, err = reader.ReadResourceEvents(ctx, kube.EventRequest{Resource: failing}); err != nil {
			t.Fatal("events", err)
		}
		deployments, err := reader.ReadWorkloads(ctx, kube.PageRequest{Connection: ref, Kind: "Deployment", Namespace: "kubesift-explorer"})
		if err != nil {
			t.Fatal(err)
		}
		var deployment kube.ResourceRef
		for _, row := range deployments.Items {
			if row.Resource.Name == "web" {
				deployment = row.Resource
			}
			if row.Resource.Name == "scaled-zero" && (row.Desired == nil || *row.Desired != 0) {
				t.Fatal("zero replicas lost")
			}
		}
		if deployment.UID == "" {
			t.Fatal("Deployment absent")
		}
		cursor := ""
		owned := 0
		var ownedPod kube.ResourceRef
		for range 20 {
			related, e := reader.ReadRelatedPods(ctx, kube.RelatedRequest{Resource: deployment, Cursor: cursor})
			if e != nil || related.Problem != nil {
				t.Fatal("relationships", e, related.Problem)
			}
			for _, row := range related.Items {
				if row.Pod.Resource.Name == "web-lookalike" {
					t.Fatal("lookalike included")
				}
				owned++
				ownedPod = row.Pod.Resource
			}
			cursor = related.Page.NextCursor
			if cursor == "" {
				break
			}
		}
		if owned == 0 || cursor != "" {
			t.Fatal("owned Pod traversal incomplete")
		}
		for _, previous := range []bool{false, true} {
			stream, e := reader.OpenPodLogs(ctx, kube.LogRequest{Resource: failing, Container: "app", Previous: previous, TailLines: 20})
			if e != nil {
				t.Fatal(e)
			}
			if e = sessions.Activate(stream); e != nil {
				t.Fatal(e)
			}
			done := false
			deadline := time.After(35 * time.Second)
			for !done {
				select {
				case event := <-events:
					if event.Session != stream {
						continue
					}
					if event.Kind == "data" {
						_ = sessions.Acknowledge(stream, event.Sequence)
					}
					if event.Kind == "error" {
						t.Fatalf("log snapshot failed: %s", event.Data)
					}
					if event.Kind == "state" && strings.HasPrefix(string(event.Data), "complete:") {
						done = true
					}
				case <-deadline:
					t.Fatal("snapshot deadline exceeded")
				}
			}
			if e = reader.ClosePodLogs(stream); e != nil {
				t.Fatal(e)
			}
		}
		follow, e := reader.OpenPodLogs(ctx, kube.LogRequest{Resource: ownedPod, Container: "app", Follow: true, TailLines: 5})
		if e != nil {
			t.Fatal(e)
		}
		if e = sessions.Activate(follow); e != nil {
			t.Fatal(e)
		}
		following := false
		followDeadline := time.After(15 * time.Second)
		for !following {
			select {
			case event := <-events:
				if event.Session != follow {
					continue
				}
				if event.Kind == "data" {
					_ = sessions.Acknowledge(follow, event.Sequence)
					following = true
				}
				if event.Kind == "error" {
					t.Fatalf("follow failed: %s", event.Data)
				}
			case <-followDeadline:
				t.Fatal("follow delivered no initial evidence")
			}
		}
		if e = sessions.Stop(follow); e != nil {
			t.Fatal(e)
		}
		if e = reader.ClosePodLogs(follow); e != nil {
			t.Fatal(e)
		}
		if err = manager.Close(ref); err != nil {
			t.Fatal(err)
		}
	}
	if len(uids) != 2 || uids[0] == uids[1] {
		t.Fatal("A/B identities not distinct")
	}
	if sessions.Stats().Sessions != 0 {
		t.Fatal("native sessions leaked")
	}
}
