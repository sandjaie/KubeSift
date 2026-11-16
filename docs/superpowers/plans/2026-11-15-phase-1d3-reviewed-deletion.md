# Phase 1D.3 Reviewed Deletion Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans in the separate implementation session. Check off verified steps. One independent whole-PR review after implementation/testing, not per-task reviews.

**Goal:** Delete one supported workload with explicit impact/name confirmation, conditional server preconditions and truthful post-delete inspection.

**Specification:** [Reviewed deletion](../specs/2026-11-15-phase-1d3-reviewed-deletion-design.md), acceptance D01–D07. See the [validation report](../validation/phase-1d3.html) for results and coverage limits.

**Architecture:** Add action-specific DeleteService to the existing Coordinator and bounded HTTP helper. Keep fixed routes/options native. Extend mixed OperationResult with a deletion payload and an operation-ID-bound readback method. Reuse current review modal, result panel and captured-target navigation.

**Stack:** Existing pins and [toolchain](../../engineering/toolchain.html); no upgrades, installed infrastructure or generic write framework.

## Constraints and review focus

Use the existing KubeSift checkout without a worktree. Verify develop contains restart merge `74dea68` and this package; preserve unrelated edits. Branch `feature/phase-1d3-reviewed-deletion`. Read AGENTS.md, repository Kubernetes skill and current implementations. Only task-owned fixtures in explicit kind-dev-local-a/b may be mutated during verification.

Five risks drive tests:

1. Dry-run or omitted options accidentally performs an unsafe delete: Task 1 serialized DeleteOptions and real preview assertions in Task 5.
2. UID/version race deletes a replacement or changed object: Task 2 wire tests and Task 5 real-server proof.
3. Cascade/owner/PVC policy evidence understates consequences: Task 1 eligibility/projection and Task 4 rendering tests.
4. Accepted/unknown is confused with removal, or readback targets a replacement: Task 3 observations and Task 4 navigation tests.
5. A third action bypasses shared locks/limits or replays DELETE: Task 2 coordinator/transport tests and Task 5 regressions.

## Task 1 — Eligibility, impact and mandatory preview (D01–D03)

**Create:** `internal/operation/delete.go`, `delete_types.go`, `delete_test.go`.
**Modify:** `internal/operation/http.go`, operation README; reuse existing Coordinator without duplicating its token/limit state.

**Interfaces:** `NewDeleteService(connections *connection.Manager, coordinator *Coordinator, now func() time.Time) *DeleteService`; `PrepareDelete(ctx context.Context, req DeleteRequest) (DeletePreview,error)`. `DeleteRequest { Resource kube.ResourceRef; SourceVersion string }`. `DeletePreview { Token string; Cluster string; Resource kube.ResourceRef; SourceVersion string; ExpiresAt time.Time; Permission string; Warnings []string; Impact DeleteImpact }`. `DeleteImpact` includes fixed propagation/default-grace labels, optional observed Pod grace/counts, bounded direct-owner/finalizer projections with completeness flags and StatefulSet retention/default provenance. Native pending payload owns serialized DeleteOptions; no manifest.

- [x] Add `TestDeleteFixedTargets` for all eight spec mappings, exact namespaces/name/API/UID/version, unsupported/generic/cluster-scoped targets and path tampering before I/O. Test terminating/mirror Pod/StatefulSet Delete or malformed retention policy rejection; default/explicit Retain is eligible.
- [x] Add `TestDeleteImpactProjection`: controlling owner versus malformed/ambiguous ownership, kind-specific cascade/recreation warnings, missing counts, Pod grace provenance, bounded/truncated finalizers and no raw annotation/env/spec values in DTOs/errors/logs.
- [x] Verify the options in `TestDeletePermissionAndPreview`: v1/DeleteOptions, UID and resourceVersion, Background, dryRun=[All] in body for preview only, gracePeriodSeconds omitted (not zero), orphanDependents and ignoreStoreReadErrorWithClusterBreakingPotential absent. Reject caller-selectable policies/URLs; cap request 16 KiB.
- [x] Add `TestDeletePermissionAndPreview`: delete verb on exact parent with no subresource, Allowed/Denied/Unknown hints, unavailable hint followed by actual server validation, no dry-run bypass. Success may be matching object or v1 Status Success with optional details; validate supplied name/UID/group/resource-plural details. Failure, inconsistent identity and malformed response cannot issue token.
- [x] Implement guarded bounded preparation after failing tests; preserve original UID/version rather than simulated preview response metadata. Safe warnings/impact only cross bridge; no retained full parent. Run `go test ./internal/operation` and relevant vet, commit.

## Task 2 — Single-dispatch deletion and mixed-action coordination (D04–D05)

**Create:** `internal/operation/delete_execution.go`, `delete_execution_test.go`.
**Modify:** `coordinator.go`, coordinator tests, operation result DTOs and README.

**Interfaces:** `ExecuteDelete(ctx context.Context, token, acknowledgement string) (OperationResult,error)`; `RevokeDelete(token string)`. Add `Delete *DeleteResultData` to OperationResult, action `delete`, exactly one action payload. `DeleteResultData { Propagation string; GracePolicy string }` retains reviewed policy only; observation is separate from execution outcome.

- [x] Add `TestDeleteExecutionRechecksIdentity`: exact typed name required natively, re-read identity/version and eligibility before dispatch, stale policy/finalizers/ownership changes invalidate by version, and UID/version preconditions remain in the real DELETE body after the final GET. Execute accepts no replacement target/options.
- [x] Extend the coordinator tests: wrong action tokens do not consume other reviews, atomic revoke/execute, expired/duplicate token, shared scale/restart/delete target lock, aggregate 128/16/4/100 limits and existing expiry/ten-minute lookup. Do not introduce hierarchy locks or claim them.
- [x] Add `TestDeleteNeverReplays` with counting HTTP transport for 429/5xx/redirect/auth responses, EOF/timeout, oversized/malformed success and lost bridge reply. At most one real DELETE, separately count preview. Preserve non-replayable body despite DELETE idempotency.
- [x] Classify documented 200/202 Status/object success as accepted only after response validation; definite denial/rejection, conflict/missing/replacement stale and ambiguous post-dispatch failures unknown. Preserve 412 rejected unless verified stale. Test close-before-send versus close-after-send and view cancellation independence.
- [x] Run `go test -race ./internal/operation ./internal/connection`, confirm existing scale/restart tests still pass and commit.

## Task 3 — Operation-bound observation and desktop bridge (D05–D06)

**Create:** `internal/operation/delete_observation.go`, `delete_observation_test.go`, `internal/desktop/delete.go`, `delete_test.go`.
**Modify:** service initialization/lifecycle, `frontend/src/operations/api.ts`, generated bindings by generator only.

**Interfaces:** `CheckDeleteTarget(ctx context.Context, operationID string) (DeleteObservation,error)` resolves a retained terminal delete operation natively; rejects wrong action, executing/evicted/unknown IDs and stale connection generation before target I/O. `DeleteObservation { OperationID string; Resource kube.ResourceRef; CheckedAt time.Time; State string; Problem *model.Problem }`, states present/terminating/absent/replaced/unavailable. It never changes the stored execution outcome or includes replacement object details. Desktop prepare/check use request-ID cancellation and existing bounded read admission; Execute remains native-operation-owned.

- [x] Test same UID present, same UID deletionTimestamp, object-specific Status NotFound absent, different UID replaced, 403/timeout/missing API route/ambiguous 404 unavailable; verify no follow-on read of replacement, namespace list, watch or execute. Extend the HTTP helper narrowly to retain bounded safe error Status identity fields without exposing raw messages/bodies; require requested name/group/resource matches for absent. Invalid operation ID/action/generation must not yield a caller-chosen route.
- [x] Wire Prepare/Execute/Revoke/Check methods and shared result list/get. Test result retention after readback failure, bridge response loss/remount and wrong-action checks; require typed acknowledgement in native service, not only UI.
- [x] Count observation requests and enforce 30-second/cap/concurrency limits through shared infrastructure. Closing tab cancels only its check, not any dispatched write. No unbounded observation cache.
- [x] Run `go test ./internal/desktop ./internal/operation`, `wails3 task bindings`, inspect generated diff, add typed frontend wrappers and typecheck; commit.

## Task 4 — Destructive review and deletion-aware result navigation (D02, D06)

**Create:** `frontend/src/operations/DeleteDialog.tsx`, `deleteState.ts`, `deleteState.test.ts`, `DeleteDialog.test.tsx`.
**Modify:** OperationResults/api/README, `frontend/src/workspace/ResourceDetail.tsx`, `Workspace.tsx` and captured-detail navigation hooks only where required.

- [x] Add Delete action to the eight supported Overview kinds, with eligibility reasons and destructive styling using existing tokens. Preserve PR #16 inspector and existing Scale/Restart actions. No bulk/select-all shortcut or new Sessions dock.
- [x] Add rendering tests for immutable target, typed exact name, Background/server-grace labels, Pod grace observation, direct-owner/cascade/PDB warnings, finalizer truncation, unknown counts and blocked mirror/unsafe-retention targets. No promise of all children/storage being removed or retained.
- [x] Implement review/confirm/cancel with existing modal focus and mounted invalidation patterns; test Escape, late prepare revocation, source refresh/namespace/tab/generation changes, typed mismatch and disabled duplicate confirmation. No optimistic removal before accepted result.
- [x] Extend mixed results with Delete label/policies, accepted-not-removed copy and explicit Check target. Store readback separately with checkedAt; stale observations cannot overwrite newer checks. Test present/terminating/absent/replaced/unavailable, result retained on denied readback and no unknown→success inference.
- [x] Original selected detail may refresh after acceptance with exact UID; absent/replaced targets must not silently show same-name replacement. Offer Back to inventory; explicit inventory selection is a fresh action. Test existing Open target actions and prior Scale/Restart result semantics. No cluster polling/watch for deletion completion.
- [x] Run frontend full tests/lint/typecheck and focused Go tests; commit.

## Task 5 — Server proof, packaged verification and delivery (D01–D07)

**Create:** `tests/integration/deletion_test.go`, `docs/superpowers/validation/phase-1d3.html`.
**Modify:** integration README, owning docs, checkpoint and this plan with actual evidence.

- [x] Create task-owned fixtures for eight kinds on explicit kind-dev-local-a/b, known local images, no PVC templates and bounded DaemonSet placement. Include denied-delete and unavailable-hint credentials, StatefulSet Retain/default/Delete-policy cases without claims, one owned finalizer-bearing fixture and representative parent/child cascade fixtures.
- [x] Verify all fixed mappings, preview without persisted object/dependent deletion, accepted deletion, correct forbidden behavior, native preconditions after same-name recreation/concurrent changes and unchanged blocked policies. Prove real server UID/version enforcement, including after final GET; fakes alone are insufficient.
- [x] Verify finalizer keeps same-UID target terminating, explicit Check target states and eventual fixture removal. Test parent absence does not assert child absence. Confirm representative Deployment and CronJob owned dependents are eventually cleaned. Remove only the test-owned finalizer during explicit-context cleanup, never through product UI.
- [x] Run full Go tests/vet, targeted race, frontend tests/lint/typecheck, generated binding consistency, build/package and deletion+scale+restart+affected read integration on approved contexts. Keep historical marker integration limitation; do not use unapproved contexts.
- [x] Desktop computer-use on final package: standalone Pod and controller review/cancel/confirm, typed-name mismatch, stale/denied target, finalizer state, Check target, same-name replacement handling, mixed results, Back and both contexts. Verify keyboard/focus, appearances, narrow split and prior detail refinements. State which other kinds are integration-only observations. No duplicate user UAT checklist.
- [x] Clean task-owned fixtures/dependents/credentials and verify both-context cleanup. Record outcomes/limitations in one report, update checked tasks/checkpoint, keep roadmap scope-only. Run documentation checks and formatting; detailed logs remain ignored.
- [x] Request one fresh independent whole-PR review using `.agents/skills/kubesift-code-review/SKILL.md` with base/head/spec and safety evidence. Resolve findings and targeted rechecks/rebuilds. Push feature branch, open ready PR and merge into develop after required gates under standing authorization. No release or next-phase implementation.

## Plan self-review

D01–D07 map to task implementation/tests. DeleteOptions fields, shared limits, conditional dispatch, accepted-versus-removed states and replacement handling are explicit. Native DTOs exclude full sensitive object content. Exact route mapping/precondition behavior requires real-server proof. The validation report records what was run.
