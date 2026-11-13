# Phase 1C.3a remaining workloads implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Use superpowers:subagent-driven-development only if the user chooses delegated execution. Steps use checkbox syntax for tracking.

**Status:** Implemented and independently reviewed on the feature branch. [Technical evidence](../validation/phase-1c3a.html) and [pending user UAT](../validation/phase-1c3a-ui-uat.html) record remaining checks.

**Goal:** Add useful read-only StatefulSet, DaemonSet, Job and CronJob investigation to the existing explorer.

**Architecture:** Extend the typed workload service, checked-object path and bounded ownership readers. Reuse the shared detail shell, Events/YAML views and explicit relation navigation. Add kind-specific summaries instead of applying Deployment status semantics to every workload.

**Tech Stack:** Existing Go, Kubernetes v0.37.0 typed clients, Wails generated bindings, React/TypeScript and Vitest. No dependency upgrades or editor libraries required.

**Spec:** [Phase 1C.3a design](../specs/2026-11-13-phase-1c3a-remaining-workloads-design.md).

## Global constraints

- Work in the existing checkout; do not create a worktree.
- Planning is not implementation authorization. No release or merge is authorized by this plan.
- Preserve approved UI tokens, explicit immutable targets, read-only boundaries and existing Pod Logs behavior.
- Four new kinds only; no all-namespace mode, generic discovery, mutations or controller log aggregation.
- API pages 100, inventory history 20, relation navigation history eight, empty-page traversal eight, encoded cursors 16 KiB, object/YAML 2 MiB. Preserve the smaller existing internal Deployment relationship batch.
- Synthetic integration data only, using documented kind-dev-local-a/b with explicit contexts. No extra clusters or installed infrastructure.

## Review focus

- StatefulSet currentReplicas is a revision count, and DaemonSet counters count nodes: verify exact column meanings (Tasks 1, 3).
- Jobs can retry failures and have intermediate conditions before terminal completion: verify precedence and unknown observations (Tasks 1, 3).
- CronJob timezone and active references cannot prove next execution or current Job existence: avoid invented schedule/health claims (Tasks 1, 3–4).
- Equal labels, missing selectors, replaced parent UIDs and untrusted cross-relation cursors: ownership and scoping must remain correct (Task 2).
- New kinds must not regress namespace retention, late-response rejection, bounded back navigation or existing Logs cleanup (Tasks 4–5).

## Entry gate and source map

Inspected baseline: develop at `dcfea2c`, including Phase 1C.2 and workload refinement. Read docs/index.html, .superpowers/sdd/status.md, the linked spec and owning documents. Confirm current branch/dirty state and current develop before execution. Do not stash, discard or switch away from another session's dirty work. Follow repository feature-branch naming on the existing checkout; if shared work conflicts, stop and report rather than creating a worktree.

Main paths:

- `internal/kube/workload_types.go`: bounded DTOs; `workloads.go`: lists/projections/cursors; `details.go`: API identity and UID-checked object reads; `relationships.go`: ownership; `events.go`: existing event validation.
- Create `internal/kube/batch_workloads.go` for Job/CronJob projection/status logic only if needed to keep workloads.go focused. No generic resource registry framework.
- `internal/desktop/workloads.go`: ExplorerService request/cancellation adapters; generated bindings under `frontend/bindings/`.
- `frontend/src/workloads/{api.ts,model.ts,state.ts,useWorkloads.ts}`: kinds, sections, dispatch and navigation.
- `frontend/src/workspace/{ResourceNavigation.tsx,ResourceTable.tsx,ResourceDetail.tsx,ResourceOverview.tsx,DeploymentPods.tsx}` and `frontend/src/ui/ResourceIcon.tsx`: rendering and destinations.
- Create `frontend/src/workspace/RelatedJobs.tsx` for CronJob Jobs. Extract a focused batch overview component only if needed; keep shared facts/conditions reusable without a broad component rewrite.
- Tests: extend `internal/kube/workloads_test.go`, create `internal/kube/remaining_workloads_test.go` if cleaner; extend existing workload frontend tests and `tests/integration/workloads_test.go`. Fixture ownership lives in `tests/fixtures/explorer/`.

### Task 1: Typed inventories and checked details

**Interfaces:** Preserve `Service.ReadWorkloads(ctx context.Context, req PageRequest) (WorkloadPage, error)`, `ReadResource`, `ReadResourceYAML` and `ReadResourceEvents`. Extend supported Kind/APIVersion combinations: StatefulSet/DaemonSet apps/v1; Job/CronJob batch/v1.

DTO decisions: reuse Desired/Ready/Updated/Available only with documented kind meanings. Add `Total *int32` for StatefulSet total Pods, leaving current-revision count in Overview. DaemonSet uses Desired/Current/Ready/Updated/Available for node counts. Add optional `Job *JobSummary` and `CronJob *CronJobSummary` fields to WorkloadRow for disjoint batch columns rather than reusing Pod Phase or replicas. `JobSummary` carries Status string, Active/Succeeded/Failed/Completions *int32; `CronJobSummary` carries Schedule string, TimeZone *string, Suspended *bool, Active int32, LastScheduled *time.Time. Use lower-camel JSON field names. Job status summarization is computed once in Go and supplied to both inventory and detail: add optional Job summary to ResourceDetail rather than duplicating precedence in TypeScript.

- [ ] Add table-driven failing tests for all four list kinds and checked GET/YAML/events dispatch, malformed version-kind combinations, same-name/new-UID, forbidden and API unavailable, positive/zero/unobserved counters and namespace scoping. Run `go test ./internal/kube -run 'RemainingWorkloads|BatchStatus' -count=1` and confirm failures are due to missing coverage.
- [x] Implement four typed projections and dispatches. Preserve limits/cancellation and controller-specific count rules from the spec. StatefulSet total comes from status.replicas, not currentReplicas. Job counters and status follow the exact specified precedence; do not infer Complete from count equality.
- [x] Add focused Job cases for retry with failed>0, FailureTarget, SuccessCriteriaMet, conflicting terminal conditions, suspension, absent completions and explicit zero. Assert CronJob absent timezone stays absent and suspended=true does not clear active references.
- [x] Extend checkedObject/validResource explicitly; no catch-all new kind routing to Deployment. Search all current callers and validators before editing. Run `go test ./internal/kube ./internal/desktop` and `go vet ./...`.
- [x] Commit the independently testable backend read extension with phase/task naming.

### Task 2: Direct Pods and CronJob-owned Jobs

**Interfaces:** Extend `ReadRelatedPods(ctx, RelatedRequest) (RelatedPage, error)` to StatefulSet/DaemonSet/Job. Rename `RelatedPod.ReplicaSet` to `Controller ResourceRef` (JSON controller) and update all producers, tests, consumers and generated bindings together; keep Deployment traversal yielding actual ReplicaSet controller refs. Add `ReadRelatedJobs(ctx context.Context, req RelatedRequest) (RelatedJobPage, error)` with `Items []WorkloadRow`, `Page PageInfo`, `Complete bool`, `Problem *model.Problem`. Add matching `ExplorerService.ReadRelatedJobs(id string, req kube.RelatedRequest) (kube.RelatedJobPage, error)` using existing cancellation lifecycle.

- [ ] Write failing fake-client cases: valid direct owner, non-controller reference, equal-label other owner, absent/invalid selector, deleted/replaced parent, forbidden child API and cross-kind/cross-UID cursor reuse. Assert returned controller identity is correct for old and new kinds.
- [x] Implement direct-parent Pod list using validated selector and owner UID filtering. Keep Deployment's two-level traversal intact. Implement CronJob-owned Job enumeration with bounded namespace pages and exact controller UID filtering; never restrict to status.active or assume labels establish ownership.
- [ ] Test eight empty pages return continuation, eventual empty completion, expired cursors, oversized cursor/response rejection and cancellation/generation validation. Preserve current result replacement semantics and partial problems.
- [x] Regenerate bindings using `wails3 task bindings`; update the existing DeploymentPods owner consumer to controller and preserve its visible ReplicaSet label for Deployment. Run Go tests, relevant race tests and `npm run typecheck`; commit contracts and their consumers atomically.

### Task 3: Destinations, inventory columns and Overview

**Interfaces:** Extend `workloadKinds` with the four kinds, `Section` with `Jobs`, `Evidence` with RelatedJobPage and `workloadAPI` with ReadRelatedJobs. Keep ResourceOverview's existing `data`/`onOpenOwner` props. Use Job summary from Task 1 for shared status; use returned objectJSON for structured configuration.

- [ ] Add frontend assertions for all four kind registry entries, scope-reset initialization, exact inventory headers/units and unknown values. Add synthetic rendering fixtures for spec A02–A05, hostile strings, absent conditions and oversized index text.
- [x] Enable navigation in the spec's order and add matching icons; extend ResourceTable via explicit kind branches instead of the current Pod/non-Pod assumption. Render Job completions absent as Not specified, and label CronJob active count as references.
- [x] Extend ResourceOverview to the four schemas, including nested CronJob jobTemplate.spec.template, template metadata, regular/init containers and configured probes/resources. Ensure shared Pod rendering is unchanged. Show config-only service/claim references without new API reads or disabled fake links.
- [x] Add explicit tab sets: three direct controllers get Pods; CronJob gets Jobs; none get Logs. Reuse shared YAML/Events and capture/stale/error UI. Include the typed-reader unknown-field limitation in the YAML information disclosure without claiming raw/lossless server output. Hook active Jobs section dispatch into useWorkloads using the existing request epoch/cancellation pattern.
- [ ] Run `npm test`, `npm run lint`, `npm run typecheck`, `npm run build`; inspect light/dark and narrow pane layouts with synthetic data. Record that no new visual system was introduced; commit the four usable views.

### Task 4: Relationship UI and complete investigation journeys

**Interfaces:** `RelatedJobs({ data, onNext, onSelect }: { data: RelatedJobPage; onNext: (cursor: string) => void; onSelect: (resource: ResourceRef) => void })`. Reuse controller `openRelated(resource)` and `backToResource()`; no parallel navigation state. Generalize DeploymentPods to render controller refs without a ReplicaSet assumption, retaining its current name unless renaming reduces confusion without churn.

- [ ] Add tests for CronJob→Job→Pod→Back→Back with originating sections preserved; StatefulSet/DaemonSet→Pod and Pod→owner; unknown/unsupported owners remain text. Cover missing owner, same name/new UID, forbidden target, target outside current inventory, eight-entry bound and namespace reset.
- [x] Render related Jobs with inventory status/counts, creation time, “N shown”, explicit continuation and retention caveat. Sort by created time descending within the batch with stable UID tie-break and unknown dates last; label page-local ordering. Do not claim global newest Job.
- [x] Show related Pod node, phase/reason, ready and restarts. Hide repeated owner only when parent identity is explicit; Deployment still exposes ReplicaSet links. Extend owner link kind/version validation for the new controllers.
- [ ] Verify two cluster tabs and rapid namespace changes retain each selected kind and reject late replies. Confirm CronJob suspended with active refs still permits inspection of owned Jobs. Returning from Logs continues its existing cleanup behavior.
- [x] Run frontend tests/lint/typecheck and backend regression tests; commit the complete relation journeys.

### Task 5: Local integration, validation and handoff

- [x] Inspect existing `tests/fixtures/explorer/inventory.json` and documented cluster fixtures before creating anything. Reuse StatefulSet/DaemonSet/Job/CronJob examples; add only small owned fixtures needed for missing acceptance cases. Keep scheduling/log output bounded and CronJobs suspended by default. Restricted credentials must be configured by the authorized fixture setup; never test rights with trial mutations.
- [x] Extend `TestWorkloadInvestigation` or add `TestRemainingWorkloads` in `tests/integration/workloads_test.go` for correct lists/details/YAML/events/ownership on explicit kind-dev-local-a/b and same-named distinct objects. Use unit fixtures for generation lag, intermediate/conflicting Job conditions and pagination boundaries instead of forcing unsafe live states.
- [x] Run `scripts/check.sh`, `wails3 task bindings`, `npm run build`. Run `KUBESIFT_EXPLORER_INTEGRATION=1 KUBESIFT_EXPLORER_CONFIG_DIR=<absolute-fixture-config-dir> go test -tags=integration ./tests/integration -run 'TestWorkloadInvestigation|TestRemainingWorkloads' -count=1` after validating those kubeconfigs target only the documented local contexts. Run `go test -race ./internal/kube ./internal/desktop` and the documented macOS package workflow.
- [ ] Create `docs/superpowers/validation/phase-1c3a.html` and `phase-1c3a-ui-uat.html`. Map A01–A10 to test/native evidence and limitations. User results start Not tested; no inferred user acceptance from automated/native agent checks. Test keyboard, 200% zoom, dark/light, narrow split and full navigation/copy/log cleanup in the packaged app.
- [x] Keep command logs, screenshots not curated for durable evidence, and detailed progress in ignored `.superpowers/sdd/2026-11-13-phase-1c3a/`. Update source descriptions, relevant README files, UX/roadmap/index, portable status and verified plan checkboxes.
- [ ] Obtain independent correctness/hygiene review per repository rules, resolve findings, re-run affected checks, commit/push the feature branch and prepare a PR to develop. Leave merge/release to explicit authorization and applicable acceptance gates. Do not start 1C.3b.

## Separate-session implementation prompt

“Implement Phase 1C.3a using `docs/superpowers/plans/2026-11-13-phase-1c3a-remaining-workloads.md` and its linked spec. This authorizes this plan only. Use the existing checkout; do not create a worktree. Inspect current develop and preserve unrelated work before making the implementation branch. Add read-only StatefulSets, DaemonSets, Jobs and CronJobs with the specified investigation journeys; reuse the approved UI and existing Logs. Execute the tests and local-context validation, record user UAT separately, and prepare the PR. No merge, release or later phase.”
