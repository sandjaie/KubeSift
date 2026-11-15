# Phase 1D.2 Reviewed Restart Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans in the separate implementation session. Track verified steps with checkboxes. One independent whole-PR review after complete implementation/testing, not per-task review gates.

**Goal:** Reviewed restart for apps/v1 Deployment, StatefulSet and DaemonSet through one conditional annotation patch, preserving the scaling safeguards and latest resource inspector.

**Spec:** [Reviewed restart](../specs/2026-11-15-phase-1d2-reviewed-restart-design.md), acceptance R01–R07.

**Architecture:** Keep action-specific native services; extract only shared token/lock/result coordination and safe HTTP helpers from ScaleService. Restart prepares an immutable JSON Patch and safe evidence, and shares Operation results with Scale. Preserve the marker experiment and read-only generic/Secret boundaries.

**Stack:** Existing Go/client-go/Wails/React pins and [toolchain](../../engineering/toolchain.html); no dependency upgrade or installed cluster infrastructure.

## Constraints and review focus

Existing checkout `/Users/sandjaieravi/dev/KubeSift`, no worktree. Verify develop contains scaling merge `9005fcc`, detail refinement `7b72850` and this planning package. Preserve concurrent edits; branch `feature/phase-1d2-reviewed-restart`. Read AGENTS.md, Kubernetes skill/reference index and actual source before changing it. No implementation in the planning session.

Five risks and their owning tests:

1. Restart changes sibling annotations/spec or leaks manifest values: Task 2 patch/projection/hash tests.
2. Paused/OnDelete/partitioned workloads appear to restart normally: Tasks 2/4 eligibility and evidence tests.
3. UID/version race, replay or cross-action token dispatch: Tasks 1/3 wire-count and race tests, Task 5 real server tests.
4. Separate action budgets/locks permit concurrent Scale and Restart on one target: Task 1 shared coordinator tests.
5. Navigation loses or retargets a submitted action or claims readiness: Tasks 3/4 native/result-state tests.

## Task 1 — Shared bounded operation coordination (R04–R05)

**Create:** `internal/operation/coordinator.go`, `coordinator_test.go`, `http.go` and tests if transport extraction requires a separate file.
**Modify:** `scale.go`, `scale_execution.go`, `scale_types.go`, existing tests and `README.html` in `internal/operation`; desktop service construction.

**Contracts:** one `Coordinator` injected into both ScaleService and RestartService. Add `OperationResult { Action string; OperationID string; Cluster string; Resource kube.ResourceRef; StartedAt time.Time; FinishedAt *time.Time; State string; Problem *model.Problem; Scale *ScaleResultData; Restart *RestartResultData }`. `ScaleResultData` holds Before/Requested/Reported; `RestartResultData` holds RequestedAt and ResponseDiffers bool. Action is fixed `scale|restart`, exactly one payload. Common `ListOperationResults() []OperationResult` and `GetOperationResult(id string) (OperationResult,error)` expose bounded mixed results. Existing Scale bridge contracts can remain compatibility wrappers over this one store; do not maintain two ledgers. Internal coordinator method naming is implementation discretion; invariants below are fixed.

- [x] Add `TestCoordinatorSharedTargetLock`: one active action per canonical target across Scale/Restart; a token sent to the wrong action entry point neither dispatches nor consumes the legitimate review. Multiple independent targets remain supported.
- [x] Add aggregate-limit tests: 128 pending, 16 preparing, four executing, 100 terminal records excluding active, review TTL five minutes and completed token lookup ten minutes. Retention, close cleanup and expiry apply across actions, not twice per action.
- [x] Refactor tested scale lifecycle behind the coordinator without changing its API/acknowledgement/no-op semantics. Existing scale tests must remain meaningful; add revoke/execute first-wins and result lookup after lost response tests using mixed action types.
- [x] Extract current bounded guarded HTTP helper only as needed to support an explicit allowlisted content type. Preserve no-redirect/non-replayable body behavior and existing credential handling; do not expose arbitrary HTTP methods/headers through the bridge.
- [x] Run `go test ./internal/operation ./internal/desktop` and `go test -race ./internal/operation ./internal/connection`. Commit the shared boundary before adding restart behavior.

## Task 2 — Restart eligibility, patch and preparation (R01–R03)

**Create:** `internal/operation/restart.go`, `restart_types.go`, `restart_test.go`.
**Modify:** shared HTTP helper and operation README.

**Interfaces:** `NewRestartService(connections *connection.Manager, coordinator *Coordinator, now func() time.Time) *RestartService`; `PrepareRestart(ctx context.Context, req RestartRequest) (RestartPreview,error)`. `RestartRequest { Resource kube.ResourceRef; SourceVersion string }`; `RestartPreview { Token; Cluster; Resource; SourceVersion; ExpiresAt; Permission; Warnings; RestartAt string; PreviousValue string; PreviousTruncated bool; Impact RestartImpact }`. `RestartImpact` includes strategy, count observations with explicit presence, default provenance, Recreate acknowledgement flag and zero-current-Pods notice. Native pending payload additionally owns exact patch bytes and expected canonical-spec hash; these never cross the bridge.

- [ ] Add `TestRestartEligibility`: fixed three-kind apps/v1 namespace targets only; UID/version and source GET must match; reject terminating, paused Deployment, OnDelete, nonzero partition, malformed template and unknown strategy before dry-run. Include legitimate omitted defaults and unknown observations without invented zero.
- [ ] Add `TestRestartPatchPreservesTemplate`: UID/version test operations, escaped restart key, existing/absent/null annotation maps, preserved sibling annotations/labels/spec, timestamp generated once and differing from previous value. Reject malformed maps. Preserve large unknown numbers during canonical comparison. Assert content type application/json-patch+json and patch cap 16 KiB.
- [ ] Add `TestRestartPermission`: parent patch attributes with empty subresource; explicit review deny blocks; HTTP denial/evaluation failure/undecided hint remains Unknown and relies on actual dry-run. No parent update, scale, delete, eviction or Pod request.
- [ ] Add `TestRestartDryRun`: mandatory dryRun=All; original version retained; missing annotation/identity mismatch or other admission spec changes reject. Metadata/status changes do not invalidate the spec comparison. Previous value rendering truncates at 1 KiB and is escaped; raw parent/env/config values do not enter DTOs/logs/errors.
- [ ] Implement bounded raw decoding and semantic canonical hash without float conversion or typed-field loss. Retain only patch/hash/safe projections after preparation; use shared five-minute token and limits. Run focused Go tests/vet and commit.

## Task 3 — Single-dispatch execution and bridge (R03–R05)

**Create:** `internal/operation/restart_execution.go`, `restart_execution_test.go`, `internal/desktop/restart.go`, `restart_test.go`.
**Modify:** desktop service construction/shutdown, scale bridge compatibility, `frontend/src/operations/api.ts`, generated bindings by generator only.

**Interfaces:** `ExecuteRestart(ctx context.Context, token, acknowledgement string) (OperationResult,error)`; `RevokeRestart(token string)`. Prepare desktop method accepts existing request ID for cancellation; Execute is native-operation-owned. List/get common results use Task 1 contract. Recreate requires exact resource name acknowledgement, checked natively. No target or new annotation input at Execute.

- [ ] Test execution re-read rejects stale UID/version/eligibility, altered strategy and changed annotation; exact prepared patch/timestamp goes out once after validation. A post-GET race is handled by JSON Patch tests, never by updating the expected version.
- [ ] Test known rejection/stale versus unknown on timeout/EOF/5xx/invalid success; JSON Patch 422 can use one diagnostic GET to verify staleness without resubmission. Valid identity/marker plus unexpected successful admission spec changes yields accepted with ResponseDiffers, not silent rollback or a claim of unchanged spec.
- [ ] Count actual HTTP requests for 429, redirects, authentication retry paths, bridge loss and repeated token execution. There is at most one persisted PATCH, separately counted from one dry-run PATCH. Never auto-retry an ambiguous result.
- [ ] Test pending revoke/connection close before dispatch, close after dispatch, app shutdown, native result retrieval and shared Scale/Restart race. Keep unknown truthful and never infer success from later matching annotation.
- [ ] Wire thin bridge, generate `wails3 task bindings`, inspect diff and add typed wrappers. Run Go tests/race and frontend typecheck. Commit.

## Task 4 — Restart review and mixed operation results (R01, R06)

**Create:** `frontend/src/operations/RestartDialog.tsx`, `restartState.ts`, `restartState.test.ts`, `RestartDialog.test.tsx`.
**Modify:** `frontend/src/operations/OperationResults.tsx`, `ScaleDialog.tsx`/existing hook only where the shared result type requires it, `api.ts`, README; `frontend/src/workspace/ResourceDetail.tsx`, `Workspace.tsx` and existing captured-target hooks.

- [ ] Add Restart beside Scale in eligible Overview actions with accessible disabled reasons for unsupported strategies. Preserve PR #16 controls/inspector, existing layout and styles. No redesigned toolbar or new Sessions dock.
- [ ] Test review fields: immutable target, strategy/count evidence, annotation previous/proposed, expiry/warnings, zero-Pod notice, Recreate typed name and no readiness/PDB guarantee. Reuse existing ReviewDialog and focus restoration.
- [ ] Test prepare→review→execute transitions, late prepared-token revocation, source/refresh/generation invalidation, Escape and disabled double-confirm. Native modal blocks background interaction; state tests still cover programmatic navigation races. Execution survives navigation and remains visible against original target.
- [ ] Rename shared feedback to Operation results, add action discriminator and distinct count versus timestamp content. Test mixed retained results after remount/bridge loss; fetching results or refreshing target must not call Execute. Preserve UID-safe Open target and Back, including stale-target error presentation fixed in 1D.1.
- [ ] Run full frontend tests/lint/typecheck plus relevant Go tests. Commit UI with source descriptions and updated README.

## Task 5 — Real-server proof, packaged verification and delivery (R01–R07)

**Create:** `tests/integration/restart_test.go`, `docs/superpowers/validation/phase-1d2.html`.
**Modify:** integration README, owning docs and checkpoint/checklist with actual evidence only.

- [ ] Create only task-owned fixture namespaces on explicit kind-dev-local-a/b: eligible three kinds, paused/OnDelete/partition/recreate variants, no PVC templates and scoped permission credentials. Bound DaemonSet placement to inspected local nodes. Reuse approved local fixture images, no external/system workload mutation.
- [ ] Verify dry-run leaves template/version unchanged, confirmed patch changes marker and reconciles owned local Pods, sibling annotations survive, denied patch fails, unavailable hint can still dry-run, and blocked strategies persist unchanged. Verify fresh Pod UID/ready observations separately from accepted operation state.
- [ ] Prove real JSON Patch UID/version tests after replacement/concurrent change for each supported API kind; add narrow test seam for the post-GET race if necessary, never a production bypass. Run scale/restart lock and lost-response tests with controlled HTTP transport. Test no automatic replay without interrupting the cluster.
- [ ] Run documented full Go tests/vet, focused race tests, frontend tests/lint/typecheck, bindings consistency, build/package, restart+scaling and affected read integration on both approved contexts. Historical marker integration uses other contexts: retain its documented limitation, run unit coverage and do not silently use unapproved contexts.
- [ ] Use desktop computer-use on the final package for all three kinds, review/Escape, confirmation, Recreate typed acknowledgement, stale/denied state, mixed scale/restart results, target Open/Back and two contexts; keyboard, narrow split, both appearances and zoom. Recheck PR #16 Overview/YAML/Events/ConfigMap Data. Record actual gaps in one validation report, no duplicate manual UAT.
- [ ] Clean only task-owned resources/credentials and verify both-context cleanup. Update plan/checkpoint/report and documentation links/formatting; detailed logs remain ignored. Roadmap owns scope only.
- [ ] Request one fresh independent whole-PR review with `.agents/skills/kubesift-code-review/SKILL.md`, full base/head/spec and safety evidence. Resolve findings, targeted rechecks and required native rebuild/retests. Push feature branch, create ready PR and merge into develop when required gates pass under standing authorization. No release or next-phase work.

## Plan self-review

R01–R07 map to task tests and delivery. Shared budgets/locks prevent per-action gaps, conditional patch proof uses real servers, and raw manifest evidence stays native. Distinct scale/restart result payloads preserve existing semantics. Optional helper names may follow current architecture; no per-task review gates or duplicate manual acceptance. All boxes are initially unchecked because this is a separate-session planning package.
