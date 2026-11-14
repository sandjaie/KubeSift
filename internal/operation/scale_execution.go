/*
Purpose: Dispatch one confirmed scale and retain target-bound feedback.
Used by: The scaling bridge and operation tests.
Responsibilities: Consume tokens atomically, revalidate preconditions and classify uncertain writes.
Important: Result lookup and duplicate confirmation cannot submit another write.
*/
package operation

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
)

type scaleRecord struct {
	token  string
	result ScaleResult
}

func (s *ScaleService) prune(now time.Time) {
	for token, p := range s.pending {
		if !now.Before(p.ExpiresAt) {
			delete(s.pending, token)
		}
	}
	for token, id := range s.byToken {
		result := s.results[id].result
		if result.FinishedAt != nil && !now.Before(result.FinishedAt.Add(10*time.Minute)) {
			delete(s.byToken, token)
		}
	}
	// Preserve active records and only drop the earliest completed operation.
	if len(s.results)-len(s.active) <= 100 {
		return
	}
	var oldest string
	for _, id := range s.order {
		result := s.results[id].result
		if result.FinishedAt != nil && (oldest == "" || result.FinishedAt.Before(*s.results[oldest].result.FinishedAt)) {
			oldest = id
		}
	}
	if oldest == "" {
		return
	}
	delete(s.byToken, s.results[oldest].token)
	delete(s.results, oldest)
	for i, id := range s.order {
		if id == oldest {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

func (s *ScaleService) scheduleExpiry(now time.Time) {
	if s.expiry != nil {
		s.expiry.Stop()
		s.expiry = nil
	}
	if s.ctx.Err() != nil {
		return
	}
	var earliest time.Time
	consider := func(expiry time.Time) {
		if earliest.IsZero() || expiry.Before(earliest) {
			earliest = expiry
		}
	}
	for _, p := range s.pending {
		consider(p.ExpiresAt)
	}
	for _, id := range s.byToken {
		if finished := s.results[id].result.FinishedAt; finished != nil {
			consider(finished.Add(10 * time.Minute))
		}
	}
	if earliest.IsZero() {
		return
	}
	s.expiry = time.AfterFunc(earliest.Sub(now), func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		now := s.now()
		s.prune(now)
		s.scheduleExpiry(now)
	})
}

func copyScaleResult(result ScaleResult) ScaleResult {
	if result.Problem != nil {
		p := *result.Problem
		result.Problem = &p
	}
	if result.Reported != nil {
		value := *result.Reported
		result.Reported = &value
	}
	if result.FinishedAt != nil {
		value := *result.FinishedAt
		result.FinishedAt = &value
	}
	return result
}

func (s *ScaleService) ExecuteScale(_ context.Context, token, acknowledgement string) (ScaleResult, error) {
	s.mu.Lock()
	review, ok := s.pending[token]
	delete(s.pending, token)
	now := s.now()
	s.prune(now)
	if id := s.byToken[token]; id != "" {
		existing := copyScaleResult(s.results[id].result)
		s.mu.Unlock()
		return existing, nil
	}
	var err error
	switch {
	case !ok:
		err = model.Fail("invalid_token", "Scale review is unavailable. Prepare a new review.")
	case !now.Before(review.ExpiresAt):
		err = model.Fail("expired", "Scale review expired. Prepare a new review.")
	case review.After == 0 && acknowledgement != review.Resource.Name:
		err = model.Fail("acknowledgement", "The workload name did not match. Prepare a new review.")
	case s.ctx.Err() != nil:
		err = model.Fail("cancelled", "Scaling service is closed")
	case len(s.active) >= 4 || s.active[review.Resource]:
		err = model.Fail("busy", "A scale is already in progress or the execution limit was reached")
	}
	if err != nil {
		s.mu.Unlock()
		return ScaleResult{}, err
	}
	result := ScaleResult{Cluster: review.Cluster, OperationID: model.ID(), Resource: review.Resource, Before: review.Before, Requested: review.After, StartedAt: now, State: "executing"}
	s.active[review.Resource] = true
	s.byToken[token] = result.OperationID
	s.results[result.OperationID] = &scaleRecord{token: token, result: result}
	s.order = append(s.order, result.OperationID)
	s.mu.Unlock()

	// Execution belongs to this operation after consumption, independent of the initiating view.
	ctx, cancel := context.WithTimeout(s.ctx, scaleDeadline)
	defer cancel()
	result.State, result.Reported, result.Problem = s.runScale(ctx, review)
	s.mu.Lock()
	defer s.mu.Unlock()
	finished := s.now()
	result.FinishedAt = &finished
	s.results[result.OperationID].result = result
	delete(s.active, review.Resource)
	s.prune(finished)
	s.scheduleExpiry(finished)
	return copyScaleResult(result), nil
}

func (s *ScaleService) runScale(ctx context.Context, review ScalePreview) (string, *int32, *model.Problem) {
	cancelled := &model.Problem{Code: "cancelled", Message: "Scale was cancelled before submission"}
	stale := &model.Problem{Code: "stale_target", Message: "Workload changed. Refresh and prepare a new review."}
	ref := review.Resource
	path, _ := scaleTarget(ref)
	client, err := s.client(ref.Connection)
	if err != nil {
		return "cancelled-before-send", nil, cancelled
	}
	before, impact, err := client.inspect(ctx, path, ref, review.SourceVersion)
	if ctx.Err() != nil || s.connections.Validate(ref.Connection) != nil {
		return "cancelled-before-send", nil, cancelled
	}
	if err != nil {
		problem := scaleReadProblem(err)
		if problem.Code == "stale_target" {
			return "stale", nil, stale
		}
		if problem.Code == "forbidden" {
			return "rejected", nil, problem
		}
		return "cancelled-before-send", nil, problem
	}
	impact.Direction = scaleDirection(before, review.After)
	if before != review.Before || impact != review.Impact {
		return "stale", nil, stale
	}
	var response scalePayload
	_, err = client.call(ctx, http.MethodPut, path+"/scale", false, scaleInput(review), &response)
	if err == nil && matchingScale(response, ref) {
		count := response.Spec.desired()
		return "accepted", &count, nil
	}
	var status scaleHTTPStatus
	if errors.As(err, &status) {
		switch status {
		case 404, 409, 410:
			return "stale", nil, stale
		case 400, 401, 403, 405, 415, 422, 429:
			return "rejected", nil, &model.Problem{Code: "rejected", Message: "Kubernetes rejected the scaling request"}
		}
	}
	var problem *model.Problem
	if errors.As(err, &problem) && (problem.Code == "stale_target" || problem.Code == "trust_required") {
		return "cancelled-before-send", nil, cancelled
	}
	return "unknown", nil, &model.Problem{Code: "uncertain", Message: "The scale outcome is unknown. Refresh the target to inspect its current state."}
}

func (s *ScaleService) RevokeScale(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, token)
}
func (s *ScaleService) GetScaleResult(id string) (ScaleResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result := s.results[id]; result != nil {
		return copyScaleResult(result.result), nil
	}
	return ScaleResult{}, model.Fail("not_found", "Scale result is unavailable")
}
func (s *ScaleService) ListScaleResults() []ScaleResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(s.now())
	results := make([]ScaleResult, 0, len(s.order))
	for i := len(s.order) - 1; i >= 0; i-- {
		results = append(results, copyScaleResult(s.results[s.order[i]].result))
	}
	return results
}
func (s *ScaleService) CloseConnection(ref model.ConnectionRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, review := range s.pending {
		if review.Resource.Connection == ref {
			delete(s.pending, token)
		}
	}
}
func (s *ScaleService) Close() {
	s.stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expiry != nil {
		s.expiry.Stop()
		s.expiry = nil
	}
	clear(s.pending)
}
