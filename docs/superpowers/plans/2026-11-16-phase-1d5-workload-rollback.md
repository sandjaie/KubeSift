# Phase 1D.5 Workload Rollback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Select a retained Deployment, StatefulSet or DaemonSet revision and restore its Pod template through the existing diff/dry-run/Confirm flow.

**Architecture:** Extend existing owned-history readers and use the shared operation coordinator for conditional rollback patches. Keep source decoding native and retain the reviewed template, not a frontend-provided historical manifest. Reuse current detail/Revisions views and operation results.

**Tech Stack:** Existing pinned Go/client-go, Wails, React/TypeScript and repository test tools. No new framework, CLI dependency or history database.

**Spec:** [Approved 1D.5 specification](../specs/2026-11-16-phase-1d5-workload-rollback-design.md), approved 2026-11-16. Planning baseline `89aee8c` on develop.

## Global Constraints

- Preserve current UI; ordinary confirmation, no typed-name step or extra permission system. Kubernetes permissions and API validation are authoritative.
- Restore only `spec.template`; preserve live replicas, strategy, selectors, storage and other parent fields. No automatic Pod deletion, strategy changes or rollout repair.
- Exactly three controller adapters: Deployment, StatefulSet and DaemonSet. Other applicability follows the spec; no invented generic history. Helm/OpenShift-specific rollback remains later work.
- Reuse page size 100, existing bounded cursor/scan limits, 2 MiB documents, five-minute reviews and 30-second operation deadlines. No unbounded lists, retained payload history or automatic retries.
- Follow [engineering](../../engineering/engineering.html), [Kubernetes development guidance](../../engineering/kubernetes-development.html), [Git workflow](../../engineering/gitworkflow.md) and [local development](../../engineering/local-development.html). Maintain source headers and format only changed files.
- Use `feature/phase-1d5-workload-rollback` from updated develop in the existing checkout. No worktree. Only kind-dev-local-a/b with explicit contexts; preserve unrelated work.
- Minimal representative tests and agent desktop verification; one independent whole-PR review, no per-task review loop or duplicate user UAT checklist. No release or next increment.

## Review Focus

- Same-label history and reused parent/source names must not be accepted as ownership: task 1 reader cases and task 3 execution cases.
- Duplicate/large revision numbers and partial lists must not select an arbitrary “previous” revision: task 1 cases.
- ControllerRevision patch directives or Deployment hash labels must not corrupt templates or restore unrelated fields: task 2 cases.
- Parent/source races, double confirmation and network uncertainty must not replay a write: task 3 cases.
- Late responses, paused/OnDelete/partitioned strategy and accepted patches must not imply rollout success: task 4 state/presentation case and task 5 native checks.

---

## File map and shared contracts

| Files | Responsibility |
| --- | --- |
| `internal/kube/rollout_history.go`, `rollout_types.go`; existing `availability_relationships.go` | Common owned history DTOs and Deployment history, reusing bounded revision-reader mechanics |
| `internal/operation/rollback_types.go`, `rollback_template.go`, `rollback.go`, `rollback_execution.go` | Historical template extraction, review/dry-run and single-dispatch execution |
| Existing `internal/operation/coordinator.go`, `http.go`, `internal/desktop/service.go`; new `internal/desktop/rollback.go` | Narrow composition and shared result/lock integration |
| Existing `frontend/src/availability/ControllerRevisions.tsx`, `model.ts`; workspace `ResourceDetail.tsx`; workloads model/state/hook | Consistent Revisions view and explicit source selection |
| `frontend/src/operations/RollbackDialog.tsx`, `rollbackState.ts`; existing `api.ts`, `OperationResults.tsx` | Existing modal/diff patterns and common operation feedback |
| New reader/operation/state test files and `tests/integration/rollback_test.go` | Focused cases in the existing test harnesses |

Use generated TypeScript bindings, not manually copied DTOs. Introduce these contracts with camelCase JSON names:

- `kube.RolloutHistoryRequest { Resource ResourceRef; Cursor string }`.
- `kube.RolloutRevision { Resource ResourceRef; ResourceVersion, Revision string; CreatedAt *time.Time; Images []string; ChangeCause string }`. Source ResourceRef includes UID and distinguishes ReplicaSet from ControllerRevision. Optional evidence stays absent/unknown rather than guessed.
- `kube.RolloutHistoryPage { Items []RolloutRevision; SourceVersion string; CurrentRevision, UpdateRevision string; Page PageInfo; Complete bool; Problem *model.Problem }`. SourceVersion is the captured parent version. Current/update names are only supplied when the parent reports them.
- `ReadRolloutHistory(ctx context.Context, req RolloutHistoryRequest) (RolloutHistoryPage, error)` on kube.Service; bridge `ReadRolloutHistory(requestID string, req)` uses existing cancellation.
- `operation.RollbackRequest { Resource kube.ResourceRef; ResourceVersion string; Source kube.ResourceRef; SourceVersion, Revision string }`.
- `operation.RollbackPreview { Token string; ExpiresAt time.Time; Request RollbackRequest; BeforeYAML, ProposedYAML, ServerYAML string; Notices []string }`. Native review additionally retains normalized template and exact patch bytes. Preview documents stay ephemeral; reject an oversized combined bridge response rather than truncate YAML.
- `PrepareRollback(ctx context.Context, req RollbackRequest) (RollbackPreview, error)`, `ExecuteRollback(token string) (OperationResult, error)`, `RevokeRollback(token string)` on a RollbackService composed with existing reader/connections/coordinator. Bridge preparation takes requestID; execution takes token only.

Results add action `rollback` and safe source name/UID/revision identifiers; never include template/diff bodies. Use existing result lookup and target refresh behavior. Existing edit resource-lock identity already supports cross-action targeting; reuse it rather than introduce another lock table.

## Task 1: Common owned history for the three workload types

**Files:** Create `internal/kube/rollout_history.go`, `rollout_types.go`, `rollout_history_test.go`; reuse existing `availability_relationships.go` helpers. Add the read method in `internal/desktop/rollback.go`.

**Interfaces:** Produces ReadRolloutHistory and the DTOs above; consumes existing checked parent reads, owner-reference comparison and pagination.

- [x] Add one `TestRolloutHistory` table: owned Deployment ReplicaSet with valid revision, same-label wrong-owner exclusion, StatefulSet/DaemonSet ControllerRevision, exact large/duplicate revision values, Forbidden and partial continuation. Assert UID-based ownership, preserved source version and no invented global newest/current claim.
- [x] Run `go test ./internal/kube -run TestRolloutHistory -count=1`; confirm new cases fail before implementation.
- [x] Implement typed parent and history reads using explicit targets and 100-item pages. Reuse existing eight-page maximum scan per request and cursor scope binding. Preserve remaining cursor/partial status instead of scanning the whole namespace. Sort returned rows by exact integer revision, tie-break by source UID; duplicate numbers remain separate rows. Invalid Deployment revision annotations cannot become selectable rollback history.
- [x] Populate evidence from the captured source only; no per-row Secret or image-registry requests. Preserve current/update references for StatefulSet; do not infer DaemonSet/Deployment health from maximum revision. Keep existing ControllerRevision detail navigation working.
- [x] Run `go test ./internal/kube ./internal/desktop`, then `wails3 task bindings`; expect pass and inspect generated diff. Commit `feat: expose owned workload rollout history`.

## Task 2: Decode historical templates and build a narrow patch

**Files:** Create `internal/operation/rollback_template.go`, `rollback_types.go`, `rollback_test.go`. Reuse exact JSON/YAML and patch helpers where applicable; no whole editor draft flow needed.

**Interfaces:** `rollbackTemplate(parent kube.ResourceRef, sourceJSON json.RawMessage) (json.RawMessage, error)` extracts normalized PodTemplateSpec; `rollbackPatch(parentJSON, templateJSON json.RawMessage) ([]byte, error)` produces UID/version tests plus replacement of `/spec/template`. Task 3 owns all identity/source reads before calling these helpers.

- [x] Add `TestRollbackTemplate` table with all three source formats, removal of Deployment `pod-template-hash` only, preservation of other template metadata, malformed revision rejection, no-op and unchanged replicas/strategy/claim settings. Assert exact numbers and only template changes in the patch.
- [x] Run `go test ./internal/operation -run TestRollbackTemplate -count=1`; confirm failure first.
- [x] Inspect controller-produced revision encoding and Kubernetes rollback source matching the pinned dependency version. Document that version in the test fixture comment. Extract the known StatefulSet/DaemonSet `data.spec.template` representation, consuming recognized strategic-merge directives such as template-level `$patch: replace` without putting directives in the Pod template. Reject unrecognized/incomplete representations; never apply arbitrary source patches to the entire live parent. Preserve unknown ordinary template fields through lossless decoding.
- [x] For Deployment extract ReplicaSet spec.template and remove only its controller-generated hash label. Compare with the live template and return a safe no-changes result when equal. Build the conditional patch with native UID/version and whole template replacement. Render before/proposed templates with existing bounded YAML helpers.
- [x] Run `go test ./internal/operation`; expect pass. Commit `feat: derive conditional workload rollback patches`.

## Task 3: Review, dry-run and single-dispatch rollback

**Files:** Create `internal/operation/rollback.go`, `rollback_execution.go`, `rollback_execution_test.go`; extend coordinator result shape and desktop composition/rollback bridge narrowly.

**Interfaces:** Produces PrepareRollback, ExecuteRollback and RevokeRollback. Uses task 2 patch bytes and existing operationClient/coordinator. All source routes derive natively from parent kind, not a frontend URL.

- [ ] Add one `TestRollbackExecution` group using the existing operation HTTP harness: dry-run denial, server preview changes, replaced/changed source, late parent conflict, double token execution, ambiguous transport outcome and shared edit/rollback lock. Assert at most one real parent patch and no writes to history, replicas, strategy or Pods.
- [ ] Run `go test ./internal/operation -run TestRollbackExecution -count=1`; confirm new cases fail.
- [ ] Prepare by reading parent and source, validating both captured UID/versions, exact controller ownership and requested revision. Use one typed source adapter per supported controller. Build patch; dry-run with the normal user credentials; return before/proposed/server template preview and warnings. No fallback write on denied/unsupported dry-run and no extra approval policy.
- [ ] Generate plain notices from current strategy: RollingUpdate, OnDelete, partition, paused Deployment and zero replicas. Preserve all settings; do not locally prohibit a template change merely for these strategies. Include the StatefulSet stuck-rollout limitation without claiming diagnosis or promising Pod replacement.
- [ ] Save a short-lived action-bound review using existing limits. Execution atomically claims/consumes the token through the coordinator before starting source revalidation, checks source UID/version/owner/revision and parent identity/version, then dispatches the stored conditional patch once. If revalidation fails, produce a rejected result and require new review. Never rebuild the patch or choose another source. A source mutation after that check cannot change the already reviewed payload; do not claim cross-object atomicity.
- [ ] Preserve accepted/rejected/unknown outcome semantics. Keep only safe source identifiers and target/result metadata in retained results. Revoke pending reviews on cancellation/connection cleanup, with existing generation checks. Reuse original-target refresh to report conditions/generation/counts; never label acceptance as rollout completion or silently open a replacement UID.
- [ ] Run `go test ./internal/operation ./internal/desktop`, `go test -race ./internal/operation`, then `wails3 task bindings`; expect pass. Commit `feat: review and execute workload rollback`.

## Task 4: Extend current Revisions and review UI

**Files:** Modify existing ControllerRevisions, availability model, workload state/hook/model, ResourceDetail and operation API/results. Create `frontend/src/operations/RollbackDialog.tsx`, `rollbackState.ts`, `rollbackState.test.ts`.

**Interfaces:** API adapter exposes readRolloutHistory/prepareRollback/executeRollback/revokeRollback using generated bindings. RollbackDialog receives RollbackRequest and close/refresh callbacks; Confirm sends only the preview token. Workload state retains the captured parent and source identities per tab.

- [ ] Add one focused `rollbackState.test.ts` group: selecting revision A then B while preparation is pending ignores late A, changing tab cannot retarget Confirm, refresh invalidates changed selection, and rejected/no-op preparation cannot enable Confirm. Cover a strategy notice and accepted-result wording without a new per-component suite.
- [ ] Run `npm test --workspace frontend -- src/operations/rollbackState.test.ts`; confirm new cases fail.
- [ ] Extend the existing Revisions table for Deployment and unify the two existing controller views on the common DTO. Keep source inspection links, explicit row selection, page-local sort/partial indicators and original parent context. No automatic previous/latest shortcut. Link source detail back to its captured owner when available rather than offering standalone source rollback.
- [ ] Add Roll back for the selected row. Reuse current modal/diff patterns for one before/proposed view, expandable server changes, dry-run state, strategy notice, Cancel and ordinary Confirm. No editable historical YAML, typed name or visual redesign. No-op/empty/Forbidden states explain what happened without blanking other details.
- [ ] Show existing OperationResults for rollback with source identifiers. Refresh/Check workload reads the original target and existing status evidence; no new polling framework. Revoke tokens on stale selection, close or target changes. Keep namespace/cluster and keyboard/focus behavior intact.
- [ ] Run targeted Vitest, `npm run lint`, `npm run typecheck`; expect pass. Commit `feat: add workload revision rollback controls`.

## Task 5: Focused local verification and delivery

**Files:** Create `tests/integration/rollback_test.go`, `docs/superpowers/validation/phase-1d5.html`; extend owned fixtures and integration README only as needed. Update validation index/checkpoint and verified plan steps.

**Interfaces:** One opt-in `TestWorkloadRollback` group follows the existing operation integration harness/environment guards in `tests/integration/README.html`. Only explicit kind-dev-local-a/b and owned synthetic fixture resources.

- [ ] Inspect current fixtures and reuse owned Deployment/StatefulSet/DaemonSet where safe. Prepare two recognizable Pod templates for each; wait for retained revision evidence with bounded deadlines. Do not use system DaemonSets, create clusters or install infrastructure. Record exact fixture ownership and cleanup commands.
- [ ] Implement the two-context walkthrough: list/select actual owned revision, prepare/confirm, verify template restored and replicas/strategy unchanged. Include one denial or conflict case; rely on task 3 fakes for deterministic late races and ambiguous transport. Run the documented opt-in command; a skip is not success. Clean up/restore owned fixtures afterward.
- [ ] Run `wails3 task bindings`, `npm run build`, `scripts/check.sh`, `wails3 task package`; expect success. Inspect generated diffs and apply required source headers/formatting to changed files. Add tests only for new actual defects or uncovered material risk.
- [ ] Verify the packaged app for all three controller history/review flows, a successful selected-template rollback, no-op/error feedback, original target across cluster tabs and current theme/layout. Check OnDelete/partition/paused notices using representative fixtures or mocked evidence; record exactly what was and was not exercised live. Do not perform automatic Pod deletion to make a rollout pass.
- [ ] Write one technical validation report with real commands/results, tested contexts/platform and limitations, including template acceptance versus Pod convergence. Link it from validation index; update portable checkpoint and check off only verified plan steps. Keep operational logs ignored. Commit `test: validate workload rollback and record evidence`.
- [ ] Obtain one independent whole-PR review through the repository code-review skill; fix blocking findings and rerun affected checks. Push the feature branch, open a ready PR using the template and complete the authorized merge into develop under current Git workflow. No duplicate user UAT gate, release or next-phase work. Report genuine blockers rather than claiming completion.

## Self-review and handoff

Spec coverage: history/applicability → task 1; exact template restoration → task 2; source/parent identity, permissions, strategy notices and execution → task 3; current UI and clear results → task 4; minimal verification and delivery → task 5. The five review risks map to focused cases above. Shared types and method names are consistent across tasks; no implementation steps are complete during planning.

The separate implementation session reads this plan and approved spec, implements only 1D.5, and reports actual verification, commit/PR/merge and remaining limitations.
