# ReplicaSets explorer implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Use superpowers:subagent-driven-development instead only if the user chooses delegated execution. Steps use checkbox syntax for tracking.

**Status:** Implemented on the existing Phase 1C.2 feature branch. See [technical results and outstanding checks](../validation/workload-explorer-refinement.html). User acceptance is pending.

**Goal:** Provide full read-only ReplicaSet browsing and trustworthy navigation among Deployments, ReplicaSets and Pods.

**Architecture:** Extend the existing typed Go workload readers and desktop bridge, not a parallel resource framework. Add bounded UID-verified relationship reads and reuse the approved detail tables. Frontend navigation retains explicit references and bounded return history.

**Tech Stack:** Existing Go/client-go, Wails generated bindings, React/TypeScript and Vitest. No new dependency by default.

**Spec:** [Workload explorer refinement](../specs/2026-11-13-workload-explorer-refinement-design.md), especially §§5–7.

## Global constraints

- Planning is not implementation authorization. No release or merge is authorized by this plan.
- Existing shell, neutral light/dark tokens, compact controls and remembered detail split remain.
- No automatic export, persistence, redaction claim or fetching of referenced Secrets.
- Use synthetic data and only the documented kind-dev-local-a/b contexts with explicit context.
- API page size 100; inventory cursor history 20; related traversal at most eight steps per request; relation navigation history at most eight entries.
- Explicit connection generation, namespace and UID on every resource operation. Permission denial never causes an alternate API bypass.

## Review focus

- Same labels or reused names under another controller: UID ownership wins (Task 2).
- Zero/absent ReplicaSet counters and generation lag: do not apply Deployment-only normalization blindly (Tasks 1, 3).
- Permission denied midway through enumeration: show partial evidence, not false emptiness (Task 2).
- Back navigation after parent replacement, scope change or tab switch: no cross-target evidence (Task 4).
- Multiple empty owner pages and expired cursors: bounded work with honest continuation (Task 2).

## Dependencies and file map

Execute after the reviewed [UI refinement plan](2026-11-13-workload-ui-refinement.md) and Phase 1C.2 implementation are available; do not implement against the older planning-only develop snapshot. Inspect current methods before edits: source map was captured from feature/phase-1c2-workload-investigation at 963977f.

Backend: `internal/kube/{workload_types.go,workloads.go,details.go,relationships.go,events.go,workloads_test.go}`, `internal/desktop/workloads.go` (`ExplorerService`), generated bindings under `frontend/bindings/`. Frontend: `frontend/src/workloads/{api.ts,model.ts,state.ts,useWorkloads.ts,state.test.ts}`, `frontend/src/workspace/{ResourceNavigation.tsx,ResourceTable.tsx,ResourceDetail.tsx,ResourceOverview.tsx,DeploymentPods.tsx}`, `frontend/src/ui/ResourceIcon.tsx`, `frontend/src/styles.css`. Create `frontend/src/workspace/ReplicaSetList.tsx` for Deployment-owned sets. Search all Kind/Section switches and bridge callers before extending contracts.

### Task 1: Typed ReplicaSet inventory and detail reads

**Interfaces:** Extend `ReadWorkloads(ctx context.Context, req PageRequest) (WorkloadPage, error)` and existing detail/YAML checked-object dispatch to Kind `ReplicaSet`, apps/v1. Extend `WorkloadRow` with optional `Current *int32` if needed; keep `Desired`, `Ready`, `Available`, `CreatedAt` meanings unchanged. Do not reinterpret `Updated` as current.

- [ ] Add failing Go tests for ReplicaSet inventory, namespace scoping, explicit connection generation, known zero vs unavailable counters, zero desired replicas, GET forbidden/not-found/replaced UID and invalid kinds. Confirm expected failures with `go test ./internal/kube -run 'ReplicaSet' -v`.
- [x] Extend the existing readers/checked-object implementation, preserving bounded decode, page tokens and 2 MiB detail/YAML limits. Reuse events reads after confirming UID validation supports ReplicaSets. No Logs dispatch for ReplicaSets.
- [ ] Add fixtures for generation lag and absent status; document count normalization from actual typed API fields. Fetch current Kubernetes/client-go docs with Context7 before settling version-specific semantics.
- [ ] Run `go test ./internal/kube ./internal/desktop` and `go vet ./...`; commit the reader extension.

### Task 2: Bounded controller relationships

**Interfaces:** Add `Service.ReadRelatedReplicaSets(ctx context.Context, req RelatedRequest) (RelatedReplicaSetPage, error)` and matching desktop method using the existing request/cancellation convention. Define `RelatedReplicaSetPage { Items []WorkloadRow; Page PageInfo; Complete bool; Problem *model.Problem }` with existing JSON naming. Extend `ReadRelatedPods` to accept Deployment or ReplicaSet parents while preserving `RelatedPage`/`RelatedPod` compatibility. For a ReplicaSet parent, each RelatedPod.ReplicaSet is that validated parent.

- [ ] Add failing tests for Deployment-owned ReplicaSets, standalone ReplicaSet-owned Pods, equal-label nonowners, recreated parent UID, forbidden child list and unknown parent kind. Assert controller=true and exact owner UID, not labels/name alone.
- [ ] Extend relationship readers using checkedObject and signed/scoped cursor patterns already present. Cursor scope must distinguish parent kind, namespace, UID and relation endpoint; old Deployment→Pods cursors must not work for ReplicaSet→Pods or Deployment→ReplicaSets.
- [ ] Test 100-item limits, eight empty traversal steps, continuation after that bound, final empty complete pages and expired/mismatched cursors. Keep partial results/errors explicit and response allocations bounded.
- [x] Expose the new method through the actual desktop workload service and regenerate bindings with `wails3 task bindings`; do not hand-edit generated files. Verify only intended contracts changed.
- [ ] Run Go tests/vet, frontend typecheck and relevant race tests; commit relationship readers and bindings together.

### Task 3: ReplicaSets destination and detail views

**Interfaces:** Extend frontend `Kind` to `"Pod" | "Deployment" | "ReplicaSet"`; `Section` gains `"ReplicaSets"`; `Evidence` includes generated `RelatedReplicaSetPage`. `ReplicaSetList({data, onNext, onSelect})` renders Deployment-owned sets; reuse ResourceTable for the independent inventory. Use existing controller API wrappers for the new bridge method.

- [ ] Add state/rendering tests for new kind initialization, namespace-kind retention, independent per-kind pages, zero counters and empty/forbidden ReplicaSet inventory. Confirm a standalone ReplicaSet does not require Deployment access.
- [x] Register ReplicaSets directly after Deployments in navigation and icon mapping. Extend inventory columns and all Kind dispatch sites; leave other future destinations disabled.
- [x] Implement ReplicaSet Overview/YAML/Events/Pods with the shared presentation. Add Deployment ReplicaSets tab, requesting only the active section. Preserve direct Deployment Pods and no ReplicaSet Logs tab.
- [x] Show old zero-replica ReplicaSets without a failure badge or history/rollback claim. Keep absent conditions/counters honest and generation evidence visible.
- [x] Run frontend tests/lint/typecheck and Go tests, then commit the browseable explorer.

### Task 4: Explicit related-resource and return navigation

**Interfaces:** Replace Deployment-only `back` with `NavigationEntry { resource: ResourceRef; section: Section }[]` owned by a tab's current scope, maximum eight entries. Provide `openRelated(resource: ResourceRef): void` and `backToResource(): void`. Resolve/validate the target through existing detail reads; do not invent a populated WorkloadRow from an owner name. Keep `openPod(row)` as a thin caller if useful, updating every existing caller in the same task.

- [ ] Add tests for Deployment→ReplicaSet→Pod→Back→Back, direct Deployment→Pod, Pod→owner ReplicaSet, duplicate suppression and the eight-entry cap. Assert captured namespace, UID, connection generation and originating section throughout.
- [x] Make explicit links in related tables and Overview owners; unsupported owners remain plain metadata. Save origin inventory page/filter/scroll and section, restore them only in a still-valid scope, and revalidate target identity before displaying details. Opening an object not in the current inventory page must not be immediately cleared by an unrelated inventory response.
- [ ] Test namespace/connection change clears history, sidebar selection clears relation history, independent cluster tabs, stale late replies, missing/deleted/replaced/forbidden return targets and a target outside the displayed inventory page.
- [ ] Preserve active Logs cleanup when leaving a Pod; coordinate with its current controller rather than changing stream internals. Verify keyboard links and explicit Back labels in the native app.
- [ ] Run tests/lint/typecheck and commit navigation.

### Task 5: Integration, acceptance and publication

- [ ] Extend existing synthetic integration fixtures in the documented local clusters for a standalone ReplicaSet, a Deployment with two owned ReplicaSets (one zero desired), misleading labels, partial permissions and multiple pages. Reuse `scripts/dev-clusters.sh`; never create extra clusters or trial-mutate external contexts.
- [x] Run `scripts/check.sh`, `wails3 task bindings`, `npm run build`, and the repository's opt-in integration/package workflow with explicit local contexts. Record exact commands and results; no browser-only claim of native acceptance.
- [ ] Extend the refinement validation report and user UAT checklist from the UI plan with the relation journeys, permission states and namespace switching for all three kinds. Verify native dark/light, narrow split, 200% zoom and keyboard access.
- [ ] Update roadmap/UX/capability documentation to implemented only after verified delivery. Keep the remainder of 1C.3 deferred; update status and plan checkboxes without asserting user acceptance.
- [ ] Review complete diff, commit/push authorized feature work and open/update its PR under repository rules. No release or merge is implied.

## Separate-session prompt

“Implement `docs/superpowers/plans/2026-11-13-replicasets-explorer.md` after confirming its UI refinement and Phase 1C.2 dependencies are present and reviewed. Read the linked spec and repository instructions first. Preserve ongoing Logs work, use only explicit documented local cluster contexts for integration, and record automated/native evidence separately from user UAT. Do not start unrelated 1C.3 work, release or merge.”
