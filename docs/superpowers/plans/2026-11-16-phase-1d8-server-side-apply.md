# Phase 1D.8 Server-Side Apply and Phase Closure Plan

> **For agentic workers:** Use superpowers:executing-plans in a separate session after 1D.7 is merged. Check off verified steps. One independent whole-PR review after complete implementation/testing, no per-task reviews.

**Goal:** Apply declarative configuration to one existing resource with explicit field ownership and reviewed conflicts, then establish an evidence-backed Phase 1D completion checkpoint.

**Spec:** [Phase 1D completion design](../specs/2026-11-16-phase-1d-completion-design.md), shared contracts, 1D.8 and closure sections.

**Architecture:** Reuse 1D.7 route/address/lock helpers and 1D.4 explicit full-object captures/parser/editor. Add a separate ApplyService using Kubernetes SSA, not JSON Patch merge emulation. Existing Create and Edit retain their own semantics. API-server preview owns value/ownership effects.

## Reconciled implementation baseline

Planning refresh on 2026-11-16: 1D.7 is merged as `fea4175` (PR #22), with checkpoint `00101a7`. This is the next increment; do not repeat creation or binding relocation. Preserve collapsible resource-sidebar sections, normalized `frontend/bindings/internal` imports, and current inspector/Secret behavior.

Reuse the delivered code deliberately:

- `internal/kube/manifest_target.go`: ManifestAddress, ResolveManifestTarget, CollectionPath/Path and ReadIdentity already exist. Creation/reading/patch capability checks are independent; apply requires Get and Patch, never List or Create. Keep exact-version routes and connection/proxy exclusions.
- `internal/operation/coordinator.go`: address locks already ignore UID/version; storeDraft/reviseDraft/closeDraft and pending draft revisions are shared. Add apply to the existing result union and operationKey canonicalization; do not introduce a parallel draft store or use a Kind-only fallback for generic targets.
- `internal/operation/http.go`: createPost/editPatch wrap sendRequest with bounded, non-replayable requests. Add a narrow SSA wrapper with fixed media type/fieldManager/Strict options and explicitly bound force. Preserve create/edit/deletion status and error handling.
- `internal/operation/edit.go` / `edit_patch.go`: operationDraft, captured baseline and lossless parser/stringData helpers are available. Apply must reuse parsing/capture, not JSON Patch's field-merging semantics.
- `frontend/src/App.tsx`, operations/CreateDialog.tsx, EditDialog.tsx and reviewState.ts own editor suspension/review state. Preserve the 1D.7 review fix: submission suspends its draft, and a late result cannot discard newer text. Uncertain results restore the unchanged submitted draft without automatic replay.
- Accepted-resource navigation now supports exact-version readable APIs without List. Apply refresh/Open must keep that support, revalidate UID and preserve ordinary Secret metadata-only browsing outside explicit edit/reveal.

The [1D.7 validation](../validation/phase-1d7.html) records actual two-context/native evidence and review fixes. Do not interpret its passes as unrun SSA tests. The historical marker suite remains outside the approved context pair; baseline custom APIs must not be removed during fixture cleanup.

## Prerequisites and review focus

Use `/Users/sandjaieravi/dev/KubeSift`, no worktree; preserve unrelated edits. Verify merged 1D.7, its normalized frontend/bindings/internal layout and actual source first, adapt helper paths without changing contracts, then branch `feature/phase-1d8-server-side-apply`. Follow AGENTS.md, Kubernetes skill, current toolchain/Git workflow and approved 1D.4 permission simplicity. Kubernetes decides permissions; no kind allowlist or repeated Secret approval.

1. SSA recreates a deleted target or updates a replacement: Task 1 real protocol proof, Task 3 race tests.
2. Apply claims every live field or omission deletes unseen owned values: Tasks 2/4 identity-only draft and server diff/ownership tests.
3. Force silently overrides other managers or survives changed draft: Tasks 2–4 conflict classification/token/state tests.
4. Precision, list semantics or Secret stringData are corrupted: Task 2 parser and real SSA cases in Task 5.
5. Closure overclaims parity/compatibility or ignores acceptance gaps: Task 5 eight-increment evidence audit.

## Task 1 — Existing-target SSA protocol proof

**Create:** `internal/operation/apply_protocol_test.go`, initial `tests/integration/apply_test.go` cases.
**Modify:** `internal/operation/http.go` via a narrow `applyPatch` wrapper for fixed apply media/query options; reuse ManifestAddress and existing CaptureEdit rather than recreating discovery.

- [x] On task-owned ConfigMap and custom API fixtures in both approved kind contexts, prove PATCH application/apply-patch+yaml accepts native-injected original metadata.uid/resourceVersion with fieldManager kubesift-apply. Verify dry-run is non-persistent and real update preserves the target UID.
- [x] Prove server rejection of stale version, recreated same-name object and deletion after final GET without recreating it. Include force=true to prove ownership takeover cannot override identity/version guards. Verify returned statuses and actual object content, not just mock expectations.
- [x] If an API cannot enforce these guards, return an explicit unsupported-safety limitation for that endpoint. Representative built-in/custom proof is a delivery gate: never silently drop preconditions, substitute unconditional apply, or treat HTTP 201 as an ordinary update.
- [x] Add fixed fieldManager, Strict validation, optional explicit force and dryRun query support to the non-replayable bounded transport. No arbitrary bridge options. Run focused protocol tests before building the UI; commit test-backed plumbing.

## Task 2 — Apply drafts, field ownership and dry-run review

**Create:** `internal/operation/apply.go`, `apply_types.go`, `apply_ownership.go` and focused tests.
**Modify:** existing edit capture/parser helpers and coordinator capture support as needed, without changing Edit semantics.

**Contracts:** `OpenApply(ctx, kube.EditTarget) (ApplyDraft,error)` captures current identity/version/full baseline natively and returns identity-only desired YAML plus bounded baseline reference. `PrepareApply(ctx, ApplyRequest{DraftID,YAML,TakeOwnership bool}) (ApplyReview,error)`. Review carries successful Token/ExpiresAt only after dry-run, ProposedYAML, BeforeYAML, ServerYAML, bounded ownership changes/warnings, or structured conflict evidence and no token. `CloseApply(id)` clears draft/payload/ownership references. Native stored capture includes draft revision and server identity; fieldManager is fixed, never bridge-selected.

- [x] Reuse coordinator storeDraft/reviseDraft/closeDraft and lossless parse/stringData normalization. Bind native conflict evidence to the same draft revision so a forged TakeOwnership flag cannot bypass matching conflict review. Reject changed target and server-owned fields; inject native UID/version only into wire payload. Do not seed the full live object as desired configuration, synthesize last-applied fields or treat identity-only/same-value apply as no-op.
- [x] Test omitted versus null fields, schema-driven list semantics (no local merging), exact numbers/unknown fields and secret normalization. Bound draft/review total at 2 MiB, native ownership parsing to decoded cap and rendered ownership summary to 100 entries/8 KiB with explicit truncation.
- [x] Perform SSA dry-run with manager kubesift-apply and force false/absent. Diff baseline against server result; project ownership acquired/released/shared where determinable from managedFields without fabricating completeness. Never submit managedFields or retain values in operation results.
- [x] Distinguish structured ownership conflicts from stale version/UID, Forbidden, immutable fields and generic 409. Show bounded manager/path evidence transiently. TakeOwnership true is allowed only for a currently matching conflict review/draft, invalidates the old review and reruns dry-run. Reset on changed draft/baseline; no auto-force.
- [x] Test Secret content stays only in deliberate draft/review, server-admission differences stay in the same review and raw error bodies do not reach retained history/logs. Run focused Go/parser/ownership tests, commit.

## Task 3 — Conditional single-dispatch apply and bridge

**Create:** `internal/operation/apply_execution.go`, execution tests, `internal/desktop/apply.go` and tests.
**Modify:** coordinator union/lifecycle, operation HTTP helper, frontend API, generated bindings through the canonical normalized generation task only. Do not restore old project-prefix import paths.

**Contracts:** `ExecuteApply(token string) (OperationResult,error)`, `RevokeApply(token string)`. Add action apply payload with manager/force-used and canonical existing target identity only. No desired document/values/managedFields in retained results. Execute cannot accept a new target, force choice or body.

- [x] Extend operationKey and result copying with explicit apply group/resource identity, testing same-address cross-action/cross-version locks for a custom kind as well as built-ins. Revalidate current source UID/version and send exact prepared SSA payload once with original guards; remove only dryRun. Shared address lock covers Create/Edit/Scale/Restart/Delete/Rollback; action tokens and replay lookup remain isolated.
- [x] Test post-GET race, wrong action, expiry, duplicate submission, draft revision and explicit force binding. Returned 200 must have correct UID/name/scope and usable version; unexpected 201 or invalid/oversize/lost success becomes Unknown and a test failure for unintended creation. No follow-up deletion/rollback.
- [x] Count actual wire requests for timeout/429/5xx/redirect/auth/bridge failure and repeated token. Never retry an uncertain write, rebase a version or auto-force conflicts. Valid accepted response can differ due to admission; preserve outcome without claiming convergence.
- [x] Wire thin preview/execute/revoke/close methods and connection lifecycle. Regenerate bindings, inspect changes and add typed wrappers. Run Go tests/race plus frontend typecheck and commit.

## Task 4 — Apply configuration UI in the existing YAML workflow

**Create:** `frontend/src/operations/ApplyDialog.tsx`, `applyState.ts`, focused tests.
**Modify:** `frontend/src/App.tsx`, current ResourceYaml/fixed/generic/Secret action entry points, shared editor/diff/reviewState primitives, OperationResults/api and READMEs. Keep imports under normalized bindings/internal.

- [x] Add Apply configuration separately from Edit YAML for fixed/generic/explicit Secret views. Start with identity-only manifest and an existing-resource reference; display stable manager kubesift-apply and clarify omissions can remove previously owned fields.
- [x] Reuse normal review/Confirm: desired document, actual server value diff and expandable ownership summary. Conflicts show paths/managers and explicit Take over conflicting fields toggle in that flow, not a second approval wizard. No takeover for unrelated errors or stale captures.
- [x] Add mounted delayed-response tests for suspended submission: resume/edit cannot race an in-flight apply, acceptance clears only that submitted draft and uncertainty preserves its exact text. Test changed draft/target/baseline resets force and review, late results/revocation, unsaved-draft navigation, disconnect, Secret isolation, bounded diff and keyboard/focus. Same values may still change ownership; do not suppress Confirm solely because a textual value diff is empty.
- [x] Preserve existing Edit/Create flows, collapsible sidebar state and mixed operation results. Exact-version target refresh must work when Get/Patch are advertised but List is not. Accepted refresh uses captured UID, Unknown never auto-retries; copy remains explicit. Run full frontend tests/lint/typecheck and focused backend tests; commit.

## Task 5 — Verification, whole-PR review and Phase 1D closure

**Create:** `docs/superpowers/validation/phase-1d8.html`, `docs/superpowers/validation/phase-1d-closure.html`.
**Modify:** apply integration suite/README, capability inventory, checkpoint and this checklist.

- [x] Use task-owned synthetic fixtures only in kind-dev-local-a/b. Cover built-in/cluster-scope/custom/Secret examples, denied patch, manager conflicts, deliberate takeover, ownership-only change, previously-owned field omission, other-manager preserved fields, list semantics and late UID/version/deletion races. Verify managedFields on real server; do not install product infrastructure.
- [x] Run full repository checks, targeted coordinator/parser/HTTP races, apply plus affected create/edit/operation/session integration, binding consistency, build/package. Packaged desktop verifies apply→conflict→explicit takeover→Confirm, Secret, denial/staleness, ownership-only review, preserved Create/Edit, exact-version readable-without-List navigation, sidebar collapse and both contexts/appearances/focus. No duplicate user UAT.
- [x] Verify cleanup of owned resources/custom objects before CRDs/namespaces and temporary credentials on both contexts. Record real versus synthetic/native limits precisely.
- [x] Produce closure matrix linking all eight specs/plans/merge evidence/reports. Audit outstanding checkboxes and every Phase 1D exit criterion; classify remaining items as blocking or approved exclusions/compatibility limits. Carry 1D.6 stream identity/remote-child/throughput/platform limits explicitly. Run focused shared-component regressions where this change justifies them, not every historical scenario indiscriminately.
- [ ] Request one fresh independent whole-PR review using repository review skill over complete code plus closure claims. Fix findings and targeted rechecks, update report/checklist. No completion claim if committed acceptance remains unverified or a blocker unresolved.
- [ ] Push ready PR and merge into develop after required gates under existing authorization. After merge update checkpoint with verified merge and Phase 1D complete for specified scope only if closure has no blocker; identify Phase 1E planning as next. Progress stays out of roadmap. No release, automatic 1E implementation or parity claim.

## Self-review

Creation remains in 1D.7; this plan implements existing-target SSA only. Real-server precondition proof precedes higher layers, explicit ownership/force semantics have native and UI tests, and the final completion claim depends on evidence rather than numbering. Checked work is backed by the [1D.8 validation report](../validation/phase-1d8.html) and [exit audit](../validation/phase-1d-closure.html).
