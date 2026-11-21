/*
Purpose: Check mixed reviewed operation coordination.
Used by: Go unit and race tests.
Responsibilities: Verify action ownership, locking, shared capacity, expiry and replay.
Important: Synthetic records never contact a cluster.
*/
package operation

import (
	"context"
	"fmt"
	"sync/atomic"

	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

func saveFixtureReview(t *testing.T, c *Coordinator, action string, ref kube.ResourceRef) string {
	t.Helper()
	token, _, err := c.save(context.Background(), pendingReview{result: OperationResult{Action: action, Resource: ref}})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestCoordinatorSharedTargetLock(t *testing.T) {
	c := NewCoordinator(nil, nil)
	defer c.Close()
	ref := kube.ResourceRef{Name: "target"}
	scale := saveFixtureReview(t, c, "scale", ref)
	restart := saveFixtureReview(t, c, "restart", ref)
	_, _, err := c.begin("restart", scale, "")
	checkCode(t, err, "invalid_token")
	review, result, err := c.begin("scale", scale, "")
	if review == nil || err != nil {
		t.Fatalf("legitimate token was lost: %v", err)
	}
	_, _, err = c.begin("restart", restart, "")
	checkCode(t, err, "busy")
	independent := saveFixtureReview(t, c, "restart", kube.ResourceRef{Name: "independent"})
	if _, _, err := c.begin("restart", independent, ""); err != nil {
		t.Fatal(err)
	}
	result.State = "unknown"
	c.finish(result)
	_, _, err = c.begin("restart", scale, "")
	checkCode(t, err, "invalid_token")
	review, replay, err := c.begin("scale", scale, "")
	if review != nil || err != nil || replay.OperationID != result.OperationID || replay.State != "unknown" {
		t.Fatalf("bad replay: %+v %v", replay, err)
	}
}

func TestCoordinatorAggregateLimits(t *testing.T) {
	now := time.Now()
	c := NewCoordinator(nil, func() time.Time { return now })
	defer c.Close()
	for i := 0; i < 16; i++ {
		_, release, err := c.prepare(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}
	_, _, err := c.prepare(context.Background())
	checkCode(t, err, "busy")
	ref := kube.ResourceRef{Connection: model.ConnectionRef{ID: "one"}, Name: "object"}
	actions := []string{"scale", "restart", "delete"}
	for i := 0; i < 128; i++ {
		saveFixtureReview(t, c, actions[i%len(actions)], ref)
	}
	_, _, err = c.save(context.Background(), pendingReview{})
	checkCode(t, err, "busy")
	c.CloseConnection(ref.Connection)
	var active []OperationResult
	for i := 0; i < 4; i++ {
		action := actions[i%len(actions)]
		token := saveFixtureReview(t, c, action, kube.ResourceRef{Name: fmt.Sprint(i)})
		_, result, err := c.begin(action, token, "")
		if err != nil {
			t.Fatal(err)
		}
		active = append(active, result)
	}
	another := saveFixtureReview(t, c, "scale", ref)
	_, _, err = c.begin("scale", another, "")
	checkCode(t, err, "busy")
	c.finish(active[0])
	for i := 0; i < 101; i++ {
		action := actions[i%len(actions)]
		token := saveFixtureReview(t, c, action, ref)
		_, result, err := c.begin(action, token, "")
		if err != nil {
			t.Fatal(err)
		}
		c.finish(result)
	}
	if got := len(c.ListOperationResults()); got != 103 {
		t.Fatalf("retained %d results", got)
	}
	for _, r := range active[1:] {
		if _, err := c.GetOperationResult(r.OperationID); err != nil {
			t.Fatal("evicted active", err)
		}
	}
}

func TestCoordinatorCompletedTokenExpiresWithoutEvictingResult(t *testing.T) {
	start := time.Now()
	var elapsed atomic.Int64
	c := NewCoordinator(nil, func() time.Time { return start.Add(time.Duration(elapsed.Load()) * time.Second) })
	defer c.Close()
	ref := kube.ResourceRef{Name: "fixture"}
	revoked := saveFixtureReview(t, c, "restart", ref)
	c.revoke("scale", revoked)
	c.revoke("restart", revoked)
	_, _, err := c.begin("restart", revoked, "")
	checkCode(t, err, "invalid_token")
	token := saveFixtureReview(t, c, "restart", ref)
	_, result, err := c.begin("restart", token, "")
	if err != nil {
		t.Fatal(err)
	}
	c.revoke("restart", token)
	result.State = "unknown"
	c.finish(result)
	elapsed.Store(599)
	_, replay, err := c.begin("restart", token, "")
	if err != nil || replay.State != "unknown" {
		t.Fatalf("lost completed record: %v", err)
	}
	elapsed.Store(600)
	_, _, err = c.begin("restart", token, "")
	checkCode(t, err, "invalid_token")
	if _, err := c.GetOperationResult(result.OperationID); err != nil {
		t.Fatal(err)
	}
}

func TestCreateSharesCanonicalAddressWithExistingActions(t *testing.T) {
	c := NewCoordinator(nil, nil)
	defer c.Close()
	ctx := context.Background()
	ref := kube.ResourceRef{Connection: model.ConnectionRef{ID: "cluster", Generation: 1}, APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team", Name: "demo", UID: "old"}
	address := kube.ManifestAddress{Connection: ref.Connection, Group: "apps", Version: "v2", Resource: "deployments", Kind: "Deployment", Namespace: "team", Name: "demo"}
	save := func(result OperationResult) string {
		token, _, err := c.save(ctx, pendingReview{result: result})
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	token := save(OperationResult{Action: "create", Resource: address.Ref(), Create: &CreateResultData{Target: address}})
	_, running, err := c.begin("create", token, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"scale", "restart", "delete", "rollback", "edit"} {
		existing := OperationResult{Action: action, Resource: ref}
		if action == "edit" {
			existing.Resource.APIVersion = "apps/v3"
			existing.Edit = &kube.EditTarget{Group: "apps", Version: "v3", Resource: "deployments"}
		}
		_, _, err := c.begin(action, save(existing), "")
		checkCode(t, err, "busy")
	}
	running.Resource.UID = "new"
	c.finish(running)
	token = save(OperationResult{Action: "scale", Resource: ref})
	if _, _, err := c.begin("scale", token, ""); err != nil {
		t.Fatal("lock not released after result acquired UID", err)
	}
}
