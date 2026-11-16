# Phase 1D.4 YAML Editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users edit existing resources, including Secrets, through one Edit → diff/dry-run → Confirm flow governed by their Kubernetes permissions.

**Architecture:** Reuse native API discovery, bounded reads and the operation coordinator. Capture the full original object natively, generate a conditional JSON Patch and submit only the confirmed patch. Extend the existing YAML section and review UI; do not build another permission system.

**Tech Stack:** Existing pinned Go/client-go, YAML parser, Wails, React/TypeScript and Vitest. Start with the existing textarea/text/diff presentation primitives; no editor dependency is required.

**Spec:** [Approved simplified 1D.4 specification](../specs/2026-11-16-phase-1d4-reviewed-yaml-editing-design.md), approved 2026-11-16. Planning baseline `1be5325` on develop.

## Global Constraints

- Kubernetes decides authorization. No resource allowlist, kind-specific permission approvals, typed-name confirmation or repeated Secret reveal gates.
- Keep the current UI. One existing object per edit; creation, server-side apply, bulk manifests and separate subresource edits remain out of scope.
- Preserve immutable target and UID/version tests, dry-run, ordinary confirmation and no automatic replay. Unsupported API operations receive a clear error.
- Keep resource documents/patches bounded to 2 MiB and parsing bounded; payloads remain memory-only and out of logs, settings, retained operation results and AI.
- Follow [engineering](../../engineering/engineering.html), [Git workflow](../../engineering/gitworkflow.md) and [local development](../../engineering/local-development.html). Source headers, formatting and verified plan checkbox updates are required.
- Use only kind-dev-local-a/b with explicit contexts. Work in the existing checkout on `feature/phase-1d4-yaml-editing` from current develop; no worktree, release or next increment.
- Minimal behavioral tests, existing check gate and agent desktop verification. One independent whole-PR review; no routine task reviewers or duplicate user UAT checklist.

## Review Focus

- Unknown permission hints must not deny an otherwise server-authorized edit: task 1 route/permission case.
- Unknown fields, large integers and missing viewer fields must not be lost: task 2 patch cases.
- Same-name replacements, late version changes and consumed confirmations must not overwrite or retry: task 3 execution cases.
- Secret stringData and placeholders must not corrupt saved values or leak into retained results: tasks 2–4 cases.
- Late responses after tab/draft changes must not submit or display another target: task 4 state case.

---

## File map and shared contracts

| Files | Purpose |
| --- | --- |
| `internal/kube/edit_capture.go`, existing `discovery.go` / `generic_resources.go` | Resolve fixed and installed API resources through native discovery; read a full, bounded object for explicit editing |
| `internal/operation/edit_types.go`, `edit.go`, `edit_patch.go`, `edit_execution.go` | Edit capture lifecycle, lossless parser/patch, dry-run and existing coordinator integration |
| Existing `internal/operation/coordinator.go`, `http.go` | Reuse limits/transport; narrowly adapt resource lock identity for arbitrary GVRs |
| `internal/desktop/edit.go`, existing `service.go` / `explorer.go` | Thin bridge and lifecycle wiring |
| `frontend/src/operations/EditDialog.tsx`, `editState.ts`, existing `api.ts` / `OperationResults.tsx` | One editor/review flow and existing operation feedback |
| Existing `frontend/src/workspace/ResourceYaml.tsx`, `ResourceDetail.tsx`, `frontend/src/workloads/useWorkloads.ts` | Entry points for fixed, generic and Secret resources; refresh/navigation integration |
| `internal/operation/edit_test.go`, `edit_execution_test.go`, `internal/kube/edit_capture_test.go`, `frontend/src/operations/editState.test.ts`, `tests/integration/editing_test.go` | Small representative test groups, not one suite per kind |

Before coding, inspect current generic and Secret detail components and their callers; extend those actual entry points as needed rather than duplicating the detail tree. Keep related package README.html descriptions current. Generate bindings through `wails3 task bindings`, never handwritten glue.

Use these contracts; all public fields use existing camelCase JSON conventions:

- `kube.EditTarget`: Connection `model.ConnectionRef`; Group, Version, Resource, Kind, Namespace, Name, UID strings. Native discovery validates group/version/resource and scope; the frontend never supplies arbitrary URLs. Fixed views resolve their API version/kind through the same discovery, not a second route table.
- `kube.EditCapture`: Target `EditTarget`, ResourceVersion string, ObjectJSON `json.RawMessage`. This full capture stays native except for deliberate editor content. `CaptureEdit(ctx context.Context, target EditTarget) (EditCapture, error)` validates get/patch capability and UID, with 30-second bounded reads.
- `operation.EditDraft`: ID string, Target `kube.EditTarget`, ResourceVersion string, YAML string. `OpenEdit(ctx, target) (EditDraft, error)` creates a bounded native baseline and returns the editable projection. `CloseEdit(id string)` releases it and associated reviews.
- `operation.EditRequest`: DraftID, YAML strings. `EditPreview`: Token string, ExpiresAt time.Time, Target `kube.EditTarget`, BeforeYAML, ProposedYAML, ServerYAML strings, Warnings []string. Preview content is ephemeral only. Combined responses must respect existing bridge limits; fail clearly if too large rather than truncate valid YAML.
- `PrepareEdit(ctx context.Context, req EditRequest) (EditPreview, error)`, `ExecuteEdit(token string) (OperationResult, error)`, `RevokeEdit(token string)`. Confirm passes only the token; no typed acknowledgement. Use the coordinator's five-minute review expiry and 30-second operation deadline.
- Bridge methods on `ExplorerService`: `OpenEdit(requestID string, target)`, `PrepareEdit(requestID string, req)`, `ExecuteEdit(token)`, `RevokeEdit(token)`, `CloseEdit(draftID)`. Use the existing cancellable read-request adapter for capture/preparation and existing operation-result lookup for execution.

Native baseline ownership is per edit ID, not a global current context. Keep at most one active editor per cluster tab, replacing it only after explicit discard; aggregate captures reuse the coordinator's existing capacity budget. The plan does not require a new draft manager UI. Close/revoke on cancellation, tab closure and connection cleanup; expiry releases unused captures through existing cleanup machinery. Keep a draft locally across a temporary disconnect only as unsaved text requiring a fresh capture/review, never as a valid submission.

## Task 1: Capture any authorized existing resource for editing

**Files:** Create `internal/kube/edit_capture.go`, `edit_capture_test.go`, `internal/operation/edit_types.go`, `edit.go`; minimally extend discovery and existing service lifecycle wiring.

**Interfaces:** Produces `CaptureEdit`, `OpenEdit` and `CloseEdit` above. Reuses the current connection manager, bounded reader and exact YAML conversion.

- [x] Add `TestEditCapture` table cases: built-in namespaced, custom/cluster-scoped, explicit Secret read, unsupported patch and Forbidden. Assert route/scope/UID correctness; unknown permission hints do not block a request. Default Secret list paths still never fetch full bodies.
- [x] Run `go test ./internal/kube -run TestEditCapture -count=1`; confirm the new cases fail before implementation.
- [x] Implement native discovery route resolution for fixed and generic views; do not reuse generic-reader Secret exclusions as an edit prohibition. Fetch only the explicitly selected resource with the user's credentials. Reject wrong identity, unavailable API or unsupported operation with a specific message. No fallback URL, create or trial mutation.
- [x] Keep the full exact JSON baseline native. Produce YAML with fixed identity context and server-owned fields/status omitted from the editable projection; record that omission so it cannot become a removal. Preserve unknown fields. Add bounded capture ownership and cleanup without another connection manager.
- [x] Run `go test ./internal/kube ./internal/operation`; expect pass. Commit task files as `feat: capture existing resources for YAML editing`.

## Task 2: Generate a faithful conditional patch

**Files:** Create `internal/operation/edit_patch.go`, `edit_test.go`; extend edit types only as required by internal helpers.

**Interfaces:** `buildEditPatch(capture kube.EditCapture, editedYAML string) (editPatch, error)` where internal `editPatch` holds exact patch bytes and normalized proposed JSON. It consumes task 1 baseline; task 3 submits those exact bytes.

- [x] Add one table-driven `TestBuildEditPatch`: additions/removals, escaped annotation keys, array order, integer above JavaScript safe precision, untouched unknown field, omitted server fields, invalid/duplicate YAML, no-op, and Secret stringData precedence. Assert the patch begins with UID/version tests and changes only intended fields. No generic Secret placeholders are generated or submitted.
- [x] Run `go test ./internal/operation -run TestBuildEditPatch -count=1`; confirm failure before implementation.
- [x] Parse one bounded mapping using the pinned YAML node API, rejecting duplicate keys, custom tags and aliases rather than expanding them. Bound depth to 64 and input/output to 2 MiB. Preserve scalar/number precision through exact JSON; reject malformed scalar types with a useful editing error.
- [x] Compare normalized editable content against the baseline. Keep route/identity fixed and server-owned fields untouched; reject attempts to change fixed identity. Use deterministic JSON Patch with correct pointer escaping and whole-array replacement. Finalizers, owners, Secret values and other ordinary fields get no local policy veto. Normalize core/v1 Secret stringData into base64 data before diffing; ordinary custom resources named similarly receive no Secret transformation.
- [x] Run targeted test and `go test ./internal/operation`; expect pass. Commit `feat: generate conditional YAML edit patches`.

## Task 3: Dry-run, confirmation and single-dispatch execution

**Files:** Extend `internal/operation/edit.go`; create `edit_execution.go`, `edit_execution_test.go`, `internal/desktop/edit.go`; modify coordinator/http and desktop composition narrowly. Update generated bindings.

**Interfaces:** Produces `PrepareEdit`, `ExecuteEdit`, `RevokeEdit` and bridge contracts. Consumes task 2 patch bytes. Existing result APIs carry action `edit` with resource identity and safe outcome only, never preview/payload text.

- [ ] Add a compact `TestEditExecution` table using the existing HTTP/coordinator harness: dry-run rejection, server-defaulted preview, late UID/version conflict, double Confirm/token reuse, ambiguous transport failure and denied patch. Assert dryRun=All, exact reviewed real patch, at most one real write and no document content in retained results. Add one shared-lock case with an existing action on the same resource.
- [ ] Run `go test ./internal/operation -run TestEditExecution -count=1`; confirm new cases fail before implementation.
- [ ] Prepare by validating capture ownership and live UID/version, building the patch and performing required server dry-run. Return baseline/proposed/server YAML and warnings in one ephemeral preview. Use server-supported validation; never quietly discard unknown fields or treat failed dry-run as success. No typed confirmation or local RBAC policy.
- [ ] Extend coordinator identity to cover dynamic GVR targets without weakening existing action locks: same connection/generation/group/resource/namespace/name/UID must collide across actions and served versions. Keep existing result consumers compatible through additive identity fields/adapters; avoid broad refactoring. Reuse current capacity/expiry/deadlines.
- [ ] Execution consumes the edit token before network work, verifies connection/target and sends the stored conditional patch exactly once. Report accepted/rejected/unknown. Keep detailed server feedback inside the ephemeral edit flow; retained history contains safe categories only. Refresh is explicit evidence, never automatic replay. Cancellation while execution is uncertain must not claim failure or resend.
- [ ] Wire bridge cancellation and cleanup; regenerate with `wails3 task bindings`. Run `go test ./internal/kube ./internal/operation ./internal/desktop` and `go test -race ./internal/operation`; expect pass. Commit `feat: review and execute conditional YAML edits`.

## Task 4: One simple editor and review flow, including Secrets

**Files:** Create `frontend/src/operations/EditDialog.tsx`, `editState.ts`, `editState.test.ts`; modify existing operation API/results and resource YAML/detail entry points, including current generic and Secret views. Use current style tokens.

**Interfaces:** API adapter exports `openEdit`, `prepareEdit`, `executeEdit`, `revokeEdit`, `closeEdit` from generated contracts. `EditDialog` consumes a captured target and close/refresh callbacks; local state captures draft ID and revision. It never constructs API routes, patches or permissions.

- [ ] Add one `editState.test.ts` scenario covering edit A → pending preparation → changed draft/tab → late response. Assert stale responses ignored, old tokens revoked, fixed target retained and confirmation enabled only for the matching successful preview. Include Secret close cleanup and a denial message without adding an approval gate.
- [ ] Run `npm test --workspace frontend -- src/operations/editState.test.ts`; confirm new assertions fail.
- [ ] Add Edit to existing YAML views. For Secrets expose Edit / Reveal YAML that fetches the explicitly selected content directly; no typed-name check, second reveal step or masked placeholders. Permission unknown leaves the request available; a server denial is explained. Unsupported verbs have an explanatory unavailable state.
- [ ] Implement editable text with Cancel and Review changes, parse errors and unsaved-discard handling. Present one diff review with dry-run status and expandable server changes/warnings. Use existing modal focus/escape patterns; add ordinary Confirm only. Reuse text styling and simple line-diff rendering, with bounded/collapsible output for large documents; no new editor dependency unless existing tooling demonstrably cannot meet the requirement.
- [ ] Revoke preparation on draft changes; close native captures on discard/tab close. Do not retarget when another cluster is selected. Use existing operation results and refresh the original target after accepted save. Clear sensitive references on close; never persist draft or error bodies. Preserve clipboard behavior as user-initiated only.
- [ ] Run targeted Vitest, `npm run lint`, `npm run typecheck`; expect pass. Commit `feat: add common resource YAML editing UI`.

## Task 5: Focused local verification, report and delivery

**Files:** Create `tests/integration/editing_test.go`, `docs/superpowers/validation/phase-1d4.html`. Extend existing fixture/README files only where needed; update validation index, checkpoint and verified plan steps. No new manual user UAT checklist.

**Interfaces:** One opt-in `TestYAMLEditing` group follows the existing scaling/restart/deletion integration setup and isolated config guards. Only explicit kind-dev-local-a/b accepted; follow current `tests/integration/README.html` for environment variables and setup rather than invent a new harness.

- [ ] Inspect existing fixtures. Reuse owned workload, ConfigMap, Secret, Service and installed test custom resource fixtures; use a disposable cluster-scoped test object for scope coverage. Add only missing fixtures. Do not mutate system resources or install infrastructure.
- [ ] Implement one integration walkthrough with allowed conditional saves, Secret stringData, a server-rejected immutable/invalid field, RBAC denial and stale-version rejection. Simulated ambiguous transport/UID replacement stays in task 3 tests unless live coverage exposes a new risk. Record actual commands/config prerequisites in the integration README and run the explicit two-context test; skips are not passes. Clean up owned changes.
- [ ] Run `wails3 task bindings`, `npm run build`, `scripts/check.sh`, `wails3 task package`; expect success. Format only changed files with repository tools and inspect generated diffs. Reuse existing suites instead of adding per-kind duplicates.
- [ ] Verify the packaged application: edit a normal resource and Secret, inspect diff/dry-run, confirm, refresh actual values; exercise forbidden/conflict and unsaved navigation; check original target retained across cluster tabs. Confirm current themes/layout remain intact. No sensitive real content in screenshots; use synthetic fixture values.
- [ ] Write one technical validation report with actual checks, tested contexts/platform, remaining limitations and unrun checks. Link it from the validation index; update checkpoint and checked plan steps only after verification. Keep operational logs under ignored `.superpowers/sdd/`. Commit `test: validate YAML editing and record results`.
- [ ] Obtain one independent whole-PR review using the repository code-review skill, resolve blocking findings and rerun affected checks. Push the feature branch and open a ready PR to develop using the repository template. Complete the authorized merge under the current Git workflow after checks/review pass. No additional approval or duplicate user UAT gate; report genuine blockers. No release.

## Self-review and handoff

Coverage: permission/resource breadth → task 1; faithful edit and Secret conversion → 2; dry-run/conditional single dispatch → 3; simple consistent UI → 4; local evidence and delivery → 5. The five review risks map to focused cases above. Contracts are shared across tasks, with implementation details left to the builder where they do not change behavior.

No implementation steps are complete at planning time. The next session reads this plan and the approved simplified spec, implements only 1D.4 and reports the final commit/PR/merge plus actual verification and limitations.
