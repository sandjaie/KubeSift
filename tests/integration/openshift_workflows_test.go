//go:build integration

/*
Purpose: Exercise reviewed Classic OLM workflows against the controlled desktop fixture.
Used by: Phase 1F local compatibility validation.
Responsibilities: Create an isolated loopback API, verify submitted operations and stop the fixture.
Important: No caller kubeconfig is read; the test owns the temporary config and server.
*/
package integration

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/operation"
	"github.com/sandjaie/KubeSift/internal/session"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenShiftWorkflowContracts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dir := t.TempDir()
	fixture := exec.CommandContext(ctx, "python3", "-B", "../../scripts/project-fixture.py", "--workflows", dir)
	if err := fixture.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = fixture.Wait() }()
	file := filepath.Join(dir, "projects.kubeconfig")
	for {
		if _, err := os.Stat(file); err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("fixture did not start")
		}
		time.Sleep(25 * time.Millisecond)
	}
	snap, err := config.Load([]string{file})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := snap.Resolve("openshift-project-fixture")
	if err != nil {
		t.Fatal(err)
	}
	addr, err := url.Parse(resolved.RESTConfig().Host)
	if err != nil || addr.Scheme != "https" || addr.Hostname() != "127.0.0.1" {
		t.Fatal("fixture escaped loopback")
	}
	con := connection.NewManager(nil)
	defer con.CloseAll()
	ref, err := con.Open(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}
	sessions := session.NewManager(nil)
	defer sessions.Close()
	reader := kube.NewService(con, sessions)
	co := operation.NewCoordinator(con, nil)
	defer co.Close()
	ops := operation.NewCreateService(con, reader, co)
	cap, err := reader.WorkflowDiscovery(ctx, ref, true)
	if err != nil || len(cap.Resources) < 10 || len(cap.Problems) != 0 {
		t.Fatal(cap, err)
	}
	for _, action := range []string{"buildconfigs/instantiate", "builds/clone", "builds/log"} {
		if !cap.BuildActions[action] {
			t.Fatal("missing", action)
		}
	}
	query := func(kind, name, uid string) kube.WorkflowQuery {
		return kube.WorkflowQuery{Connection: ref, Namespace: "team-a", ProjectUID: "fixture-a", Kind: kind, Name: name, UID: uid}
	}
	q := query("Subscription", "test-install", "")
	install := operation.OperatorRequest{Action: "install-operator", Target: q, Catalog: "catalog", CatalogNamespace: "team-a", Package: "demo", Channel: "wrong"}
	if _, err := ops.PrepareOperator(ctx, install); err == nil {
		t.Fatal("unknown channel allowed")
	}
	install.Channel = "stable"
	run := func(r operation.OperatorRequest) operation.OperationResult {
		p, err := ops.PrepareOperator(ctx, r)
		if err != nil {
			t.Fatal(r.Action, err)
		}
		if strings.Contains(p.Before+p.Admitted, "fixture-private-sentinel") || strings.Contains(p.Before, `"manifest":`) {
			t.Fatal("sensitive review")
		}
		if !p.Typed {
			t.Fatal("missing acknowledgement")
		}
		result, err := ops.ExecuteOperator(r.Action, p.Token, p.Name)
		if err != nil || result.State != "accepted" {
			t.Fatal(r.Action, result, err)
		}
		duplicate, err := ops.ExecuteOperator(r.Action, p.Token, p.Name)
		if err != nil || duplicate.OperationID != result.OperationID {
			t.Fatal("duplicate dispatch")
		}
		return result
	}
	run(install)
	q = query("Subscription", "demo", "demo-uid")
	approve := operation.OperatorRequest{Target: q, Version: "3", Action: "approve-plan"}
	p, err := ops.PrepareOperator(ctx, approve)
	if err != nil || !strings.Contains(p.Before, "cluster-wide") {
		t.Fatal("missing cluster impact", err)
	}
	ops.RevokeOperator(approve.Action, p.Token)
	run(approve)
	remove := approve
	remove.Action = "remove-subscription"
	res := run(remove)
	if res.InstalledCSV == nil || res.InstalledCSV.Name != "demo.v1" {
		t.Fatal("did not retain installed CSV identity")
	}
	csv := operation.OperatorRequest{Target: query("ClusterServiceVersion", "demo.v1", "demo.v1-uid"), Action: "remove-csv", Version: "3", RemovalID: res.OperationID}
	run(csv)
	if _, err := reader.ReadWorkflow(ctx, csv.Target); err == nil {
		t.Fatal("removed CSV still visible")
	}
}
