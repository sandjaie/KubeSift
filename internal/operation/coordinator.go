/*
Purpose: Coordinate confirmed workload operations across action types.
Used by: Native reviewed resource operations.
Responsibilities: Bind tokens, share target locks and retain bounded session results.
Important: Wrong-action tokens are never consumed or revealed.
*/
package operation

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

const reviewTTL = 5 * time.Minute
const operationDeadline = 30 * time.Second

type OperationResult struct {
	Action      string              `json:"action"`
	Cluster     string              `json:"cluster"`
	OperationID string              `json:"operationID"`
	Resource    kube.ResourceRef    `json:"resource"`
	StartedAt   time.Time           `json:"startedAt"`
	FinishedAt  *time.Time          `json:"finishedAt"`
	State       string              `json:"state"`
	Problem     *model.Problem      `json:"problem"`
	Scale       *ScaleResultData    `json:"scale"`
	Restart     *RestartResultData  `json:"restart"`
	Delete      *DeleteResultData   `json:"delete"`
	Rollback    *RollbackResultData `json:"rollback,omitempty"`
	Edit        *kube.EditTarget    `json:"edit,omitempty"`
}

// operationKey shares locks across API versions of the same canonical resource.
func operationKey(result OperationResult) kube.ResourceRef {
	key := result.Resource
	if result.Edit != nil {
		key.APIVersion, key.Kind = result.Edit.Group, result.Edit.Resource
	} else if _, plural := deleteTarget(key); plural != "" {
		group, _, grouped := strings.Cut(key.APIVersion, "/")
		if !grouped {
			group = ""
		}
		key.APIVersion, key.Kind = group, plural
	}
	return key
}

type ScaleResultData struct {
	Before    int32  `json:"before"`
	Requested int32  `json:"requested"`
	Reported  *int32 `json:"reported"`
}

type RestartResultData struct {
	RequestedAt     string `json:"requestedAt"`
	ResponseDiffers bool   `json:"responseDiffers"`
}

type pendingReview struct {
	result          OperationResult
	expires         time.Time
	acknowledgement string
	payload         any
	draftID         string
	revision        uint64
}

type operationRecord struct {
	token  string
	result OperationResult
}

type Coordinator struct {
	mu          sync.Mutex
	connections *connection.Manager
	now         func() time.Time
	ctx         context.Context
	stop        context.CancelFunc
	prepares    chan struct{}
	pending     map[string]pendingReview
	captures    map[string]editCapture
	results     map[string]*operationRecord
	byToken     map[string]string
	active      map[kube.ResourceRef]bool
	order       []string
	expiry      *time.Timer
}

func NewCoordinator(connections *connection.Manager, now func() time.Time) *Coordinator {
	if now == nil {
		now = time.Now
	}
	ctx, stop := context.WithCancel(context.Background())
	c := &Coordinator{connections: connections, now: now, ctx: ctx, stop: stop, prepares: make(chan struct{}, 16),
		pending: map[string]pendingReview{}, captures: map[string]editCapture{}, results: map[string]*operationRecord{}, byToken: map[string]string{}, active: map[kube.ResourceRef]bool{}}
	if connections != nil {
		connections.AddCleanup(c.CloseConnection)
	}
	return c
}

func (s *Coordinator) prepare(ctx context.Context) (context.Context, func(), error) {
	select {
	case s.prepares <- struct{}{}:
	default:
		return nil, nil, model.Fail("busy", "Too many operation preparations")
	}
	ctx, cancel := context.WithTimeout(ctx, operationDeadline)
	stop := context.AfterFunc(s.ctx, cancel)
	release := func() { stop(); cancel(); <-s.prepares }
	if ctx.Err() != nil || s.ctx.Err() != nil {
		release()
		return nil, nil, model.Fail("cancelled", "Operation review cancelled")
	}
	return ctx, release, nil
}

func (s *Coordinator) save(ctx context.Context, review pendingReview) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.ctx.Err() != nil {
		return "", time.Time{}, model.Fail("cancelled", "Operation review cancelled")
	}
	if s.connections != nil {
		if err := s.connections.Validate(review.result.Resource.Connection); err != nil {
			return "", time.Time{}, err
		}
	}
	now := s.now()
	s.prune(now)
	if review.draftID != "" {
		if draft, ok := s.captures[review.draftID]; !ok || draft.revision != review.revision {
			return "", time.Time{}, model.Fail("expired", "Edit capture changed or expired")
		}
	}
	if len(s.pending)+len(s.captures) >= 128 {
		return "", time.Time{}, model.Fail("busy", "Too many pending operation reviews")
	}
	token := model.ID()
	review.expires = now.Add(reviewTTL)
	if review.draftID != "" {
		draft := s.captures[review.draftID]
		draft.expires = review.expires
		s.captures[review.draftID] = draft
	}
	s.pending[token] = review
	s.scheduleExpiry(now)
	return token, review.expires, nil
}

func (s *Coordinator) begin(action, token, acknowledgement string) (*pendingReview, OperationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	review, ok := s.pending[token]
	if ok && review.result.Action == action {
		delete(s.pending, token)
	} else {
		ok = false
	}
	s.prune(now)
	if record := s.results[s.byToken[token]]; record != nil && record.result.Action == action {
		return nil, copyOperationResult(record.result), nil
	}
	var err error
	switch {
	case !ok:
		err = model.Fail("invalid_token", "Operation review is unavailable. Prepare a new review.")
	case !now.Before(review.expires):
		err = model.Fail("expired", "Operation review expired. Prepare a new review.")
	case review.acknowledgement != "" && review.acknowledgement != acknowledgement:
		err = model.Fail("acknowledgement", "The workload name did not match. Prepare a new review.")
	case s.ctx.Err() != nil:
		err = model.Fail("cancelled", "Operation service is closed")
	case len(s.active) >= 4 || s.active[operationKey(review.result)]:
		err = model.Fail("busy", "An operation is already in progress or the execution limit was reached")
	}
	if err != nil {
		return nil, OperationResult{}, err
	}
	if review.draftID != "" {
		s.dropEdit(review.draftID)
	}
	result := review.result
	result.OperationID, result.StartedAt, result.State = model.ID(), now, "executing"
	s.active[operationKey(result)] = true
	s.byToken[token] = result.OperationID
	s.results[result.OperationID] = &operationRecord{token: token, result: copyOperationResult(result)}
	s.order = append(s.order, result.OperationID)
	return &review, result, nil
}

func (s *Coordinator) finish(result OperationResult) OperationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	result.FinishedAt = &now
	s.results[result.OperationID].result = result
	delete(s.active, operationKey(result))
	s.prune(now)
	s.scheduleExpiry(now)
	return copyOperationResult(result)
}

func (s *Coordinator) revoke(action, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending[token].result.Action == action {
		delete(s.pending, token)
	}
}

func copyOperationResult(result OperationResult) OperationResult {
	if result.Problem != nil {
		value := *result.Problem
		result.Problem = &value
	}
	if result.FinishedAt != nil {
		value := *result.FinishedAt
		result.FinishedAt = &value
	}
	if result.Scale != nil {
		value := *result.Scale
		if value.Reported != nil {
			reported := *value.Reported
			value.Reported = &reported
		}
		result.Scale = &value
	}
	if result.Restart != nil {
		value := *result.Restart
		result.Restart = &value
	}
	if result.Delete != nil {
		value := *result.Delete
		result.Delete = &value
	}
	if result.Rollback != nil {
		value := *result.Rollback
		result.Rollback = &value
	}
	if result.Edit != nil {
		value := *result.Edit
		result.Edit = &value
	}
	return result
}

func (s *Coordinator) GetOperationResult(id string) (OperationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record := s.results[id]; record != nil {
		return copyOperationResult(record.result), nil
	}
	return OperationResult{}, model.Fail("not_found", "Operation result is unavailable")
}

func (s *Coordinator) ListOperationResults() []OperationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(s.now())
	results := make([]OperationResult, 0, len(s.order))
	for i := len(s.order) - 1; i >= 0; i-- {
		results = append(results, copyOperationResult(s.results[s.order[i]].result))
	}
	return results
}

func (s *Coordinator) prune(now time.Time) {
	for id, draft := range s.captures {
		if !now.Before(draft.expires) {
			s.dropEdit(id)
		}
	}
	for token, p := range s.pending {
		if !now.Before(p.expires) {
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

func (s *Coordinator) scheduleExpiry(now time.Time) {
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
		consider(p.expires)
	}
	for _, capture := range s.captures {
		consider(capture.expires)
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

func (s *Coordinator) CloseConnection(ref model.ConnectionRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, draft := range s.captures {
		if draft.baseline.Target.Connection == ref {
			s.dropEdit(id)
		}
	}
	for token, review := range s.pending {
		if review.result.Resource.Connection == ref {
			delete(s.pending, token)
		}
	}
}
func (s *Coordinator) Close() {
	s.stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expiry != nil {
		s.expiry.Stop()
		s.expiry = nil
	}
	clear(s.pending)
	clear(s.captures)
}
