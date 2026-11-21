/*
Purpose: Execute one reviewed Helm action under native lifecycle ownership.
Used by: Desktop confirmation, cancellation and result polling.
Responsibilities: Revalidate storage, serialize release targets and retain bounded metadata results.
Important: Cancelling never releases a worker lock until the SDK worker has actually terminated.
*/
package helmmanager

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/sandjaie/KubeSift/internal/helmreader"
	"github.com/sandjaie/KubeSift/internal/model"
	"helm.sh/helm/v4/pkg/action"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	helmkube "helm.sh/helm/v4/pkg/kube"
)

type execution struct {
	result Result
	token  string
	cancel context.CancelFunc
}

// SetInvalidation connects the existing inspection cache without coupling SDK actions to desktop state.
func (s *Service) SetInvalidation(invalidate func(Target)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidate = invalidate
}

func (s *Service) Execute(token string) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id := s.tokens[token]; id != "" {
		return s.executions[id].result, nil
	}
	s.expireLocked()
	r := s.reviews[token]
	if s.closed || r == nil || r.cancel != nil {
		return Result{}, model.Fail("stale_target", "Review expired or was discarded. Prepare again.")
	}
	if s.running >= 2 || s.locks[r.Target] {
		return Result{}, model.Fail("capacity", "Two Helm operations are running, or this release already has an operation.")
	}
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(r.Options.TimeoutSeconds)*time.Second)
	stop := func() bool { return false }
	if s.connections != nil {
		lifetime, err := s.connections.Lifetime(r.Target.Connection)
		if err != nil {
			cancel()
			return Result{}, err
		}
		stop = context.AfterFunc(lifetime, cancel)
	}
	result := Result{ID: model.ID(), Action: r.Action, Target: r.Target, ContextName: r.ContextName, State: "running", Stage: "Revalidating reviewed release", StartedAt: time.Now().UTC()}
	e := &execution{result: result, token: token, cancel: cancel}
	s.executions[result.ID] = e
	s.tokens[token] = result.ID
	s.locks[r.Target] = true
	s.running++
	delete(s.reviews, token)
	invalidate := s.invalidate
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() { stop(); cancel() }()
		perform := s.perform
		if perform == nil {
			perform = s.performAction
		}
		observed, wrote, err := perform(ctx, r)
		if invalidate != nil {
			invalidate(r.Target)
		}
		s.mu.Lock()
		e.result.Observed = observed
		now := time.Now().UTC()
		e.result.FinishedAt = &now
		e.result.State, e.result.Stage = "succeeded", "Helm action completed"
		if err != nil {
			e.result.State, e.result.Stage = "failed", "Helm action failed; inspect the release for partial effects"
			var problem *model.Problem
			if !errors.As(safeHelmError(ctx, err), &problem) {
				problem = &model.Problem{Code: "failed", Message: "Helm action failed."}
			}
			e.result.Problem = problem
			if wrote && (ctx.Err() != nil || problem.Code == "timeout" || problem.Code == "unreachable" || problem.Code == "unknown_outcome") {
				e.result.State, e.result.Stage = "unknown", "Outcome uncertain; refresh release history before any recovery"
			}
		}
		e.cancel = nil
		delete(s.locks, r.Target)
		s.running--
		s.trimResultsLocked()
		s.mu.Unlock()
	}()
	return result, nil
}

func (s *Service) Cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.executions[id]
	if e == nil {
		return model.Fail("not_found", "This Helm result is no longer retained.")
	}
	if e.cancel != nil {
		e.result.Stage = "Cancellation requested; waiting for the native worker to stop"
		e.cancel()
	}
	return nil
}

func (s *Service) ListResults() []Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Result, 0, len(s.executions))
	for _, e := range s.executions {
		out = append(out, e.result)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

func (s *Service) trimResultsLocked() {
	completed := []*execution{}
	for _, e := range s.executions {
		if e.result.State != "running" {
			completed = append(completed, e)
		}
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].result.StartedAt.After(completed[j].result.StartedAt) })
	for _, e := range completed[min(128, len(completed)):] {
		delete(s.tokens, e.token)
		delete(s.executions, e.result.ID)
	}
}

func sameRecord(a, b *helmreader.RecordRef) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (s *Service) performAction(ctx context.Context, r *reviewData) (*helmreader.ReleaseSummary, bool, error) {
	cfg, err := s.configuration(ctx, r.Target, false)
	if err != nil {
		return nil, false, err
	}
	current, _, err := helmreader.CaptureHead(ctx, cfg.client, r.Target)
	if err != nil {
		return nil, false, err
	}
	if !sameRecord(current, r.Current) {
		return nil, false, model.Fail("stale_target", "The release head changed after review. Prepare again.")
	}
	if r.Rollback != nil {
		if _, err := helmreader.ReadCaptured(ctx, cfg.client, *r.Rollback); err != nil {
			return nil, false, err
		}
	}
	s.mu.Lock()
	if e := s.executions[s.tokens[r.Token]]; e != nil {
		e.result.Stage = "Running reviewed Helm action"
	}
	s.mu.Unlock()
	err = cfg.transport.outcome(invokeAction(ctx, cfg.sdk, r))
	wrote := cfg.transport.writes.Load()
	var observed *helmreader.ReleaseSummary
	if ctx.Err() == nil {
		readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		ref, rel, readErr := helmreader.CaptureHead(readCtx, cfg.client, r.Target)
		if readErr == nil && ref != nil && rel != nil {
			observed = &helmreader.ReleaseSummary{Record: *ref}
			if rel.Info != nil {
				if len(rel.Info.Status) <= 128 {
					observed.Status = string(rel.Info.Status)
				}
			}
			if rel.Chart != nil && rel.Chart.Metadata != nil {
				if len(rel.Chart.Metadata.Name)+len(rel.Chart.Metadata.Version)+len(rel.Chart.Metadata.AppVersion) <= 1024 {
					observed.ChartName, observed.ChartVersion, observed.AppVersion = rel.Chart.Metadata.Name, rel.Chart.Metadata.Version, rel.Chart.Metadata.AppVersion
				} else {
					observed.Problem = &model.Problem{Code: "capacity", Message: "Stored chart metadata exceeds the display limit."}
				}
			}
		}
	}
	return observed, wrote, err
}

func invokeAction(ctx context.Context, cfg *action.Configuration, r *reviewData) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	timeout := time.Duration(r.Options.TimeoutSeconds) * time.Second
	waitOptions := []helmkube.WaitOption{helmkube.WithWaitContext(ctx)}
	// SDK cancellation otherwise returns before its internal mutation goroutine joins.
	// Storage, transport and waiters retain the actual cancellable operation context.
	joined := context.WithoutCancel(ctx)
	switch r.Action {
	case "install", "upgrade":
		ch, err := loadChart(ctx, r.raw)
		if err != nil {
			return err
		}
		v2, ok := ch.(*chartv2.Chart)
		if !ok {
			return model.Fail("unsupported", "This chart format is not supported by Helm actions.")
		}
		finish := boundedActionRender(ctx, cfg, v2)
		defer func() {
			if err := finish(); err != nil {
				resultErr = err
			}
		}()
		values, err := reviewValues(r.request.ValuesYAML)
		if err != nil {
			return err
		}
		if r.Action == "install" {
			install := action.NewInstall(cfg)
			install.ReleaseName, install.Namespace = r.Target.ReleaseName, r.Target.Namespace
			install.Timeout, install.WaitStrategy, install.WaitOptions, install.WaitForJobs = timeout, helmkube.StatusWatcherStrategy, waitOptions, true
			install.DisableHooks, install.CreateNamespace = r.Options.DisableHooks, r.Options.CreateNamespace
			_, err = install.RunWithContext(joined, ch, values)
			return err
		}
		upgrade := action.NewUpgrade(cfg)
		upgrade.Namespace = r.Target.Namespace
		upgrade.Timeout, upgrade.WaitStrategy, upgrade.WaitOptions, upgrade.WaitForJobs = timeout, helmkube.StatusWatcherStrategy, waitOptions, true
		upgrade.DisableHooks, upgrade.ResetValues, upgrade.MaxHistory = r.Options.DisableHooks, true, 0
		_, err = upgrade.RunWithContext(joined, r.Target.ReleaseName, ch, values)
		return err
	case "rollback":
		rollback := action.NewRollback(cfg)
		rollback.Version, _ = strconv.Atoi(r.Rollback.Revision)
		rollback.Timeout, rollback.WaitStrategy, rollback.WaitOptions, rollback.WaitForJobs = timeout, helmkube.StatusWatcherStrategy, waitOptions, true
		rollback.DisableHooks, rollback.MaxHistory = r.Options.DisableHooks, 0
		return rollback.Run(r.Target.ReleaseName)
	case "uninstall":
		uninstall := action.NewUninstall(cfg)
		uninstall.Timeout, uninstall.WaitStrategy, uninstall.WaitOptions = timeout, helmkube.StatusWatcherStrategy, waitOptions
		uninstall.DisableHooks, uninstall.KeepHistory = r.Options.DisableHooks, r.Options.KeepHistory
		_, err := uninstall.Run(r.Target.ReleaseName)
		return err
	}
	return model.Fail("invalid", "Unsupported reviewed Helm action.")
}
