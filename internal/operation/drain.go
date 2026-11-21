/*
Purpose: Execute a confirmed Node operation and retain bounded per-Pod progress.
Used by: The operation coordinator independently of the initiating desktop view.
Responsibilities: Revalidate before writes, wait for disappearance and verify final inventory.
Important: Only explicit throttling or budget rejection permits another eviction attempt.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

func (s *Coordinator) drainLimit(ref model.ConnectionRef) bool {
	count := 0
	for _, r := range s.results {
		if r.result.FinishedAt == nil && r.result.Node != nil && r.result.Node.Action == "drain" {
			count++
			if r.result.Resource.Connection == ref {
				return true
			}
		}
	}
	return count >= 2
}
func (s *Coordinator) CancelDrain(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.results[id]; r != nil && r.result.Node != nil && r.result.Node.Action == "drain" && r.cancel != nil {
		r.cancel()
	}
}
func (s *Coordinator) RevokeNodeReview(token string) { s.revoke("node", token) }
func (s *Coordinator) ExecuteNodeAction(token, ack string) (OperationResult, error) {
	review, result, e := s.begin("node", token, ack)
	if review == nil {
		return result, e
	}
	p := review.payload.(NodePreview)
	s.mu.Lock()
	record := s.results[result.OperationID]
	ctx, cancel := record.ctx, record.cancel
	s.mu.Unlock()
	worker := copyOperationResult(result)
	go func() {
		defer s.nodeWorkers.Done()
		defer cancel()
		if s.connections.Validate(p.Resource.Connection) != nil {
			cancel()
		}
		state, reason := s.runNode(ctx, p, &worker)
		worker.State, worker.Node.Reason = state, reason
		worker.Node.Counts = nodeCounts(worker.Node.Rows)
		worker.Node.UpdatedAt = s.now()
		s.finish(worker)
	}()
	return result, nil
}
func (s *Coordinator) publishNode(r *OperationResult) {
	r.Node.Counts = nodeCounts(r.Node.Rows)
	r.Node.UpdatedAt = s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[r.OperationID].result = copyOperationResult(*r)
}
func waitNode(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func interruption(ctx context.Context) (string, string) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout", "The total wait expired. Review remaining work."
	}
	return "cancelled", "Cancelled. In-flight requests may have taken effect."
}
func (s *Coordinator) runNode(ctx context.Context, p NodePreview, r *OperationResult) (string, string) {
	c, e := newOperationClient(s.connections, p.Resource.Connection)
	if e != nil {
		return "cancelled", "Connection closed before execution"
	}
	c.client.Timeout = 10 * time.Second
	n, e := c.node(ctx, p.Resource, p.SourceVersion)
	if e != nil {
		r.Node.Cordoned = nil
		if ctx.Err() != nil {
			return interruption(ctx)
		}
		return "blocked", "Cannot verify the reviewed Node version"
	}
	if ctx.Err() != nil {
		return interruption(ctx)
	}
	if p.Action == "drain" {
		fresh, e := c.inventory(ctx, n.Name)
		if e != nil {
			return "blocked", "Cannot repeat complete Pod inventory"
		}
		byUID := map[string]NodePod{}
		for _, row := range p.Rows {
			byUID[row.UID] = row
		}
		for _, row := range fresh {
			old, ok := byUID[row.UID]
			if !ok || old != row {
				return "blocked", "Assigned Pod identity or impact changed. Prepare a fresh review."
			}
			delete(byUID, row.UID)
		}
		for i, row := range r.Node.Rows {
			if _, gone := byUID[row.UID]; gone {
				r.Node.Rows[i].State = "gone"
			}
		}
	}
	want := p.Action != "uncordon"
	if n.Spec.Unschedulable != want {
		if ctx.Err() != nil {
			return interruption(ctx)
		}
		e = c.scheduling(ctx, n, want, false)
		if e != nil {
			r.Node.Cordoned = nil
			var status operationHTTPStatus
			if errors.As(e, &status) && status >= 400 && status < 500 {
				return "blocked", "Kubernetes rejected the conditional scheduling change"
			}
			return "unknown", "Scheduling request outcome is uncertain. Refresh the Node."
		}
	}
	n, e = c.node(ctx, p.Resource, "")
	if e != nil {
		r.Node.Cordoned = nil
		return "unknown", "Cannot observe Node scheduling state"
	}
	r.Node.Cordoned = &n.Spec.Unschedulable
	if n.Spec.Unschedulable != want {
		return "partial", "Observed scheduling state differs from review"
	}
	if p.Action != "drain" {
		return "completed", "Verified scheduling state"
	}
	s.publishNode(r)
	for i := range r.Node.Rows {
		row := &r.Node.Rows[i]
		if row.State == "skipped" || row.State == "gone" {
			continue
		}
		state, reason := s.drainPod(ctx, c, p, r, row)
		if state != "" {
			return state, reason
		}
	}
	rows, e := c.inventory(ctx, p.Resource.Name)
	if ctx.Err() != nil {
		return interruption(ctx)
	}
	if e != nil {
		return "partial", "Final assigned Pod inventory unavailable"
	}
	n, e = c.node(ctx, p.Resource, "")
	if e != nil {
		r.Node.Cordoned = nil
		return "unknown", "Cannot verify final Node identity or scheduling"
	}
	r.Node.Cordoned = &n.Spec.Unschedulable
	if !n.Spec.Unschedulable {
		return "partial", "Node was externally uncordoned"
	}
	reviewedSkips := map[string]NodePod{}
	for _, row := range r.Node.Rows {
		if row.State == "skipped" {
			reviewedSkips[row.UID] = row
		}
	}
	for _, row := range rows {
		if old, ok := reviewedSkips[row.UID]; !ok || row != old {
			r.Node.Rows = rows
			return "partial", "Final inventory contains new or changed Pods. Review remaining work."
		}
	}
	return "completed", "Node observed cordoned with only documented skips remaining"
}
func (s *Coordinator) drainPod(ctx context.Context, c operationClient, p NodePreview, r *OperationResult, row *NodePod) (string, string) {
	observing := row.State == "terminating"
	approved := *row
	for {
		if ctx.Err() != nil {
			return interruption(ctx)
		}
		var pod core.Pod
		var missing operationStatus
		_, e := c.send(ctx, "GET", podPath(*row), false, nil, "", &pod, &missing)
		matched := pod.Kind == "Pod" && pod.APIVersion == "v1" && pod.Name == row.Name && pod.Namespace == row.Namespace
		if (errors.Is(e, operationHTTPStatus(404)) && objectNotFound(missing, row.Name, "", "pods")) || (e == nil && matched && pod.UID != "" && string(pod.UID) != row.UID) {
			row.State, row.Reason = "gone", "Reviewed UID is gone"
			s.publishNode(r)
			return "", ""
		}
		if e != nil || !matched || pod.Spec.NodeName != p.Resource.Name || string(pod.UID) != row.UID || pod.ResourceVersion == "" {
			row.State = "unknown"
			return "partial", "Cannot verify reviewed Pod"
		}
		if observing || pod.DeletionTimestamp != nil {
			observing = true
			if pod.DeletionTimestamp != nil {
				row.State, row.Reason = "terminating", "Waiting for reviewed UID to disappear"
			}
			s.publishNode(r)
			if waitNode(ctx, 2*time.Second) != nil {
				return interruption(ctx)
			}
			continue
		}
		checked, e := c.classify(ctx, pod, map[string]bool{})
		if e != nil || checked != approved {
			row.State = "changed"
			return "partial", "Pod ownership or local storage changed"
		}
		n, e := c.node(ctx, p.Resource, "")
		if e != nil {
			r.Node.Cordoned = nil
			return "partial", "Cannot verify Node before eviction"
		}
		r.Node.Cordoned = &n.Spec.Unschedulable
		if !n.Spec.Unschedulable {
			return "partial", "Node was externally uncordoned. Evictions stopped."
		}
		if ctx.Err() != nil {
			return interruption(ctx)
		}
		body, e := json.Marshal(policy.Eviction{TypeMeta: meta.TypeMeta{APIVersion: "policy/v1", Kind: "Eviction"}, ObjectMeta: meta.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace}, DeleteOptions: &meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &pod.UID, ResourceVersion: &pod.ResourceVersion}}})
		if e != nil {
			return "blocked", "Cannot encode eviction"
		}
		var rejected evictionRejection
		var response meta.Status
		_, e = c.sendRequest(ctx, http.MethodPost, podPath(*row)+"/eviction", false, body, "application/json", &response, requestOptions{eviction: &rejected})
		if e == nil {
			row.State, row.Reason = "accepted", "Eviction admitted. Waiting for disappearance."
			observing = true
			s.publishNode(r)
			if waitNode(ctx, 2*time.Second) != nil {
				return interruption(ctx)
			}
			continue
		}
		if errors.Is(e, operationHTTPStatus(429)) {
			row.State, row.Reason = "throttled", "API server throttled the request"
			if rejected.budget {
				row.State, row.Reason = "budget-blocked", "A PodDisruptionBudget protects application availability"
			}
			s.publishNode(r)
			if rejected.delay < 1 {
				rejected.delay = 5
			}
			deadline, _ := ctx.Deadline()
			if float64(rejected.delay) > time.Until(deadline).Seconds() {
				return "timeout", "Retry delay exceeds remaining wait"
			}
			if waitNode(ctx, time.Duration(rejected.delay)*time.Second) != nil {
				return interruption(ctx)
			}
			continue
		}
		var code operationHTTPStatus
		if errors.As(e, &code) && code >= 400 && code < 500 {
			row.State = "denied"
			if code == 404 || code == 409 || code == 422 {
				row.State = "changed"
			}
			return "partial", "Eviction was rejected. Review remaining Pods."
		}
		row.State, row.Reason = "unknown", "Eviction may have taken effect"
		if ctx.Err() != nil {
			return interruption(ctx)
		}
		return "unknown", "Eviction outcome uncertain. No further dispatch."
	}
}
