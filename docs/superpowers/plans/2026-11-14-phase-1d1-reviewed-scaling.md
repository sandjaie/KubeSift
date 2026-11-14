# Phase 1D.1 Reviewed Scaling Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans in the separate implementation session. Check off verified steps. One independent whole-PR review after complete implementation/testing, not routine per-task reviews.

**Goal:** Deliver reviewed single-resource Deployment/StatefulSet scaling with immutable target binding, real server preconditions and no ambiguous-write retries.

**Spec:** [Reviewed scaling](../specs/2026-11-14-phase-1d1-reviewed-scaling-design.md), including S01–S07 and exact bounds.

**Architecture:** Separate native ScaleService next to the existing fixture-only marker service; guarded bounded clients; native pending/executing/result state; thin desktop adapters; shared styled review dialog and target-bound result feedback. No generic write framework or change to discovery read-only safeguards.

**Stack:** Existing Go/client-go/Wails/React pins and [toolchain](../../engineering/toolchain.html); no upgrades or infrastructure installations.

## Constraints and review focus

Use `/Users/sandjaieravi/dev/KubeSift`, no worktree. Verify current develop contains 1C.3g merge `48ffb33` and this package, preserve concurrent work, then branch `feature/phase-1d1-reviewed-scaling`. Follow AGENTS.md, Kubernetes skill and Git policy. Progress goes in checkpoint/plan, results in one validation report, never roadmap. Only task-owned fixtures in explicit kind-dev-local-a/b may be mutated during verification.

Five safety issues drive tests:

1. Source changes after review or after final GET: Tasks 1/2/5 test UID/version-enforced updates against fake and real API servers.
2. Transport/bridge replay sends another write: Tasks 2/3 test request counts, lost responses and result retrieval.
3. Permission hints become authorization, or dry-run silently changes the payload: Task 1 tests denied/unknown permission and admission mismatch.
4. Navigation or cancellation retargets/silently loses a write: Tasks 2/4 test atomic revoke/execute and late frontend responses.
5. Scale-down understates storage/disruption effects or claims readiness: Tasks 1/4 test retention policy, zero acknowledgement and outcome language.

## Task 1 — Native target inspection and preparation (S01–S03)

**Create:** `internal/operation/scale.go`, `scale_types.go`, `scale_test.go`.
**Modify:** `internal/operation/README.html`; a focused bounded transport helper only if needed. Inspect `internal/kube/bounded_reads.go`, `internal/connection/manager.go`, and `internal/operation/operation.go` first. Preserve marker Prepare/Execute behavior and fixture guards.

**Interfaces:** `NewScaleService(connections *connection.Manager, now func() time.Time) *ScaleService`; `PrepareScale(ctx context.Context, req ScaleRequest) (ScalePreview,error)`. `ScaleRequest { Resource kube.ResourceRef; SourceVersion string; ReplicasText string }`. `ScalePreview { Token string; Resource kube.ResourceRef; SourceVersion string; Before,After int32; ExpiresAt time.Time; Permission string; Warnings []string; Impact ScaleImpact; NoChange bool }`. `ScaleImpact` captures zero/down/up direction and StatefulSet retention policy values/default provenance, without parent manifest content. Native requests are exact two-kind apps/v1 routes, never caller-supplied URLs.

- [ ] Add `TestScaleRejectsInvalidTargetBeforeIO` for unsupported/generic kinds, empty UID/version, invalid name/scope, negative count, exponent notation, whitespace, fractions, overflow and malformed bridge values; accept only ASCII decimal digits natively before int32 conversion. Add `TestScaleNoChange` asserting no dry-run, execution or token for unchanged replicas.
- [ ] Add `TestScalePrepareIdentity` for displayed-parent mismatch, parent/Scale UID/version/name mismatch, terminating parent and source expiry. Test captured retention policy and no automatic HPA/namespace scans.
- [ ] Add `TestScalePermissionAndDryRun`: explicit deny blocks; HTTP 403 creating the permission review and successful undecided reviews are Unknown, which permits actual mandatory dry-run; all real API denials surface; dry-run preserves identity/desired value and original version baseline, unsupported dry-run has no bypass. Assert request uses only minimal autoscaling/v1 Scale and dryRun=All, not whole parent data.
- [ ] Run `go test ./internal/operation` to observe failures. Implement guarded bounded reads, permission hint, dry-run and five-minute/128-token preparation; apply deadline/concurrency/output bounds and sanitized warnings. Test token expiry and close cleanup. Verify dry-run retry suppression. Run focused Go tests/vet and commit.

## Task 2 — Single-dispatch execution and outcome tracking (S03–S04, S06)

**Create:** `internal/operation/scale_execution.go`, `scale_execution_test.go`.
**Modify:** ScaleService/types/README and existing lifecycle integration where appropriate.

**Interfaces:** `ExecuteScale(ctx context.Context, token, acknowledgement string) (ScaleResult,error)`; `RevokeScale(token string)`; `GetScaleResult(operationID string) (ScaleResult,error)`; `ListScaleResults() []ScaleResult`; `CloseConnection(ref model.ConnectionRef)` and `Close()` for service-owned cleanup. `ScaleResult` contains OperationID, immutable Resource, Before/Requested, optional Reported replicas, started/finished times, state `executing|accepted|rejected|stale|cancelled-before-send|unknown`, and safe problem. Internal token→operation lookup ensures duplicate Execute retrieves the existing result while retained rather than resending. It never authorizes a second dispatch.

- [ ] Add `TestScaleExecuteSingleUse` for expired/forged token, invalid zero-name acknowledgement, duplicate concurrent Execute, one target lock, four-execution capacity and consumed failures. Record operation identity before dispatch so result retrieval works after bridge failure.
- [ ] Add `TestScaleExecuteRevalidates`: changed parent/Scale UID/version, changed retention policy, terminating resource and invalid connection do not write. A race after GET must fail the write's UID/resourceVersion preconditions. No automatic conflict refresh.
- [ ] Add `TestScaleWriteIsNeverRetried` with counting HTTP transport: timeout/EOF/5xx/429/malformed success, lost bridge response and repeated token each produce at most one real PUT; distinct dry-run PUT is counted separately. Set client retries zero and audit lower transport retry behavior. Known rejection and unknown are distinct.
- [ ] Add `TestScaleRevokeExecuteRace`, close-before/after-dispatch and navigation-independent context tests. Implement atomic first-wins revocation, process-owned 30-second execution context, bounded result list (100 terminal records, never evict active) and ten-minute token-result lookup cleanup.
- [ ] Test admitted success with unexpected count displays reported/requested values separately; read-only refresh never changes unknown into proven success. Run `go test -race ./internal/operation ./internal/connection`, commit.

## Task 3 — Desktop bridge and connection lifecycle (S01–S04, S06)

**Create:** `internal/desktop/scale.go`, `scale_test.go`, `frontend/src/operations/api.ts`.
**Modify:** `internal/desktop/service.go`, `explorer.go`/connection-close adapters as actually used, generated bindings by generator only.

- [ ] Identify product service ownership and add one ScaleService instance; do not expose the fixture marker client as a generic write method. Preparation supports existing request-ID cancellation. Execution survives ordinary request-view cancellation but obeys shutdown/connection generation guards.
- [ ] Expose prepare/execute/revoke/get/list methods with typed DTOs. Frontend may pass target/count only to prepare, token/acknowledgement only to execute. Test malformed numbers at binding boundary, revoked connections, repeated calls and sanitized errors.
- [ ] Wire connection close/app shutdown to pending-token cleanup and executing outcome tracking. Add tests proving stale generations cannot write even with retained UI state; bounded completed results remain distinguishable from pending review.
- [ ] Run `go test ./internal/desktop ./internal/operation`, generate `wails3 task bindings`, inspect generated diff, add typed frontend wrappers and run documented typecheck; commit.

## Task 4 — Review dialog and target-bound result UX (S05–S06)

**Create:** `frontend/src/operations/ScaleDialog.tsx`, `scaleState.ts`, `scaleState.test.ts`, `ScaleDialog.test.tsx`, `OperationResults.tsx`, `README.html`.
**Modify:** `frontend/src/workspace/ResourceDetail.tsx`, `Workspace.tsx`, `frontend/src/workloads/useWorkloads.ts` only for captured-target refresh/open hooks, and shared styles minimally.

- [ ] Write state tests: idle→editing→preparing→review→executing→result; editing or navigating revokes pending token; late preparation is discarded/revoked; stale connection/tab/namespace replies cannot update current selection. Recover results with list/get after remount without repeating Execute.
- [ ] Add Scale action only to Deployment/StatefulSet dedicated detail. Reuse approved dialog/focus patterns. Test exact integer input, immutable target, before/after, expiry, unknown/denied permission, required zero-name acknowledgement, scale-down/PVC/controller notices and no-op state.
- [ ] Implement Review/Confirm/Cancel and native revocation with race-safe execution. Double-click disables immediately. Navigation closes pending review; dispatched action remains available in compact app-level feedback across tabs. No new Sessions dock or automatic current-target retargeting.
- [ ] Implement accepted/stale/rejected/unknown states, reported versus requested counts, safe target-open UID validation and explicit read-only refresh. Never infer ready/complete from accepted. Test keyboard focus restoration, Escape behavior, stale-policy review and safe plain-text errors.
- [ ] Run full frontend tests/lint/typecheck, focused Go tests and commit. Preserve discovery, Secret, Logs and existing Back behavior.

## Task 5 — Real-server proof, native verification and delivery (S01–S07)

**Create:** `tests/integration/scaling_test.go`, `docs/superpowers/validation/phase-1d1.html`.
**Modify:** integration README, owning documentation, checkpoint and this task ledger.

- [ ] Create task-owned Deployment/StatefulSet fixtures without PVC templates on both explicit approved kind contexts. Create restricted credentials for read-only scale and denied permission-review/allowed dry-run cases. No system workload mutations; track fixture identities for cleanup.
- [ ] Prove preview does not persist, valid scale succeeds, stale UID/version update fails even when target changes after re-read, denied update fails, and 1→2→1 plus 1→0→1 recovery works. Inspect actual Scale UID/version behavior; a missing enforceable precondition blocks delivery rather than permitting unconditional writes.
- [ ] Use a scoped test transport for ambiguous-response/request-count cases against owned fixtures; no cluster outage injection or installed admission controllers. Verify independent target contexts and cleanup. Use fake policy evidence instead of deleting PVCs.
- [ ] Run documented full Go tests/vet, targeted race suites, frontend tests/lint/typecheck, bindings consistency, build/package, scaling integration on both contexts and affected existing read/marker integration. Record exact commands/outcomes and distinguish fake from real-server proof.
- [ ] On the final packaged app use desktop computer-use for prepare/cancel, confirmed scale, zero acknowledgement and restore, permission denial, externally changed target requiring re-review, switching tabs/namespace during review, target-bound result/Back and both contexts. Verify keyboard, narrow split, appearances and zoom; record limitations without duplicate manual UAT.
- [ ] Delete only owned fixtures/temporary credentials, verify both-context cleanup. Update validation report with actual evidence and remaining limits, checklist and checkpoint. Keep roadmap scope-only and detailed execution logs ignored. Run documentation checks/formatting.
- [ ] Request one fresh independent whole-PR review using `.agents/skills/kubesift-code-review/SKILL.md` with base/head/spec and safety evidence; fix findings and perform targeted rechecks. Refresh review evidence if head changes. Push feature branch, open ready PR and merge into develop after required gates under standing authorization. No release or subsequent phase.

## Plan self-review

S01–S07 map to tasks above. API/DTO names align across tasks; race, permission, precision, identity and result-retention tests are explicit. Product scaling is separate from fixture writes and generic API discovery. Implementation must verify server preconditions before delivery; historical 1C.3g verification limits remain unchanged unless specifically retested. All tasks are unchecked: this package authorizes no implementation in the planning session.
