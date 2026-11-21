# Workload UI refinement implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Use superpowers:subagent-driven-development instead only if the user chooses delegated execution. Steps use checkbox syntax for tracking.

**Status:** Merged into develop through PR #7 as `c74940a`. See [technical results and outstanding checks](../validation/workload-explorer-refinement.html). User acceptance is pending.

**Goal:** Implement the accepted detail layouts, preserve workload kind across namespace changes and suppress unwanted linguistic suggestions in technical inputs.

**Architecture:** Keep Go resource reads and immutable targets unchanged for this UI slice. Reuse the workload reducer/controller and existing detail components; extract Overview rendering from ResourceDetail only to keep its responsibility focused. Coordinate the small Logs input change with the concurrent Logs owner.

**Tech Stack:** Existing Go/Wails, React/TypeScript, shared CSS tokens and Vitest. No new dependency by default.

**Spec:** [Workload explorer refinement](../specs/2026-11-13-workload-explorer-refinement-design.md).

## Global constraints

- Planning is not implementation authorization. No release or merge is authorized by this plan.
- Existing shell, neutral light/dark tokens, compact controls and remembered detail split remain.
- No automatic export, persistence, redaction claim or fetching of referenced Secrets.
- Use synthetic data and only the documented kind-dev-local-a/b contexts with explicit context.
- API page size remains 100, cursor history 20, YAML/object bound 2 MiB; no new unbounded caches.
- Logs redesign/collection changes are outside this plan.

## Review focus

- Namespace changes during in-flight reads: preserve kind, reject late data and avoid an intermediate Pods fetch (Task 1).
- Missing/zero/false observations: avoid misleading health or fabricated defaults (Task 2).
- Long or malicious cluster strings: selectable inert content, bounded rendering and exact copy (Tasks 3–4).
- Native text correction and IME composition: literal queries without breaking normal typing (Task 5).
- Refresh and managed-fields toggles during search: correct match positions against the current displayed document (Task 4).

## Entry gate and file map

The isolated planning checkout is based on develop, which at drafting time precedes the Phase 1C.2 implementation. The source inspection used feature/phase-1c2-workload-investigation at 963977f. Before execution, identify the reviewed implementation head and concurrent Logs head; do not reset, stash or overwrite either session. Follow [git workflow](../../engineering/gitworkflow.md) for a feature branch incorporating both approved baseline and these planning documents. Stop for reconciliation if baseline files differ materially from the map below.

Existing files: `frontend/src/workloads/{state.ts,state.test.ts,useWorkloads.ts,model.ts}`, `frontend/src/workspace/{ResourceDetail.tsx,ResourceEvents.tsx,ResourceYaml.tsx,DeploymentPods.tsx,ResourceTable.tsx,NamespaceControl.tsx}`, `frontend/src/sessions/LogViewer.tsx`, `frontend/src/styles.css`.

Create only where useful: `frontend/src/workspace/ResourceOverview.tsx` for typed presentation; `frontend/src/workspace/yamlSearch.ts` and `.test.ts` for bounded literal match logic; focused renderer tests under the same workspace directory. Reuse existing formatters after searching their callers. No generic component framework or all-purpose resource schema.

### Task 1: Preserve workload kind on scope changes

**Interfaces:** Existing `reduceWorkloads(state: WorkloadsState, action: Action): WorkloadsState` and `useWorkloads(...)` controller. Keep the scope action shape; preserve prior tab kind and each kind's auto preference, invalidate evidence. The ReplicaSets plan extends the same kind registry later.

- [ ] Add reducer regressions: Deployment in namespace A → namespace B retains Deployment, clears rows/cursors/details/filter/scroll/back, preserves auto preferences; same scope is unchanged; tab B is untouched; old-scope response is rejected. Initial tab still starts at Pod. Assert exact kind/state, not implementation syntax.
- [ ] Run `npm run test --workspace frontend -- --reporter=verbose` and record the expected retention failure before changing logic.
- [x] Update scope transition in `state.ts` and the reconciliation fallback in `useWorkloads.ts`; do not let a missing new-scope state select Pod transiently. Preserve cancellation and monotonically changing request epochs.
- [ ] Add a controller-level test with deferred requests: namespace change while Deployment read is pending results in a new Deployment request only, and old response cannot paint new scope. Cover empty and forbidden responses.
- [ ] Run `npm test`, `npm run typecheck`, `npm run lint`; commit only Task 1 changes.

### Task 2: Structured Overview presentation

**Interfaces:** `ResourceOverview({ data }: { data: ResourceDetail })` consumes the existing captured objectJSON/ResourceRef. Keep `overviewObject` compatible with its current callers or update every caller in the same change. No new backend health classification.

- [ ] Add synthetic fixtures/assertions for Running but unready, terminating, waiting/terminated containers, zero desired replicas, generation lag, unknown counts, no conditions, multiple Pod IPs and init/ephemeral groups. Use existing test tooling and static rendered markup where appropriate; do not add a test framework solely for presentation.
- [x] Implement Spec §1–2 using compact facts, conditions/container tables and disclosures. Keep all currently supported evidence reachable, including uncommon probe/resource/volume configuration. Preserve supported owner metadata pending Task 4 of the ReplicaSets plan.
- [ ] Apply only scoped CSS rules with existing tokens; fix duplicate active-tab underlines if still present, without changing Logs content styles. Preserve the detail header contract.
- [ ] Run frontend tests/lint/typecheck. Visually compare dark/light, a narrow split and 200% zoom against the approved references; confirm keyboard disclosure operation and full long-value access.
- [ ] Record actual results and commit the independently usable Overview redesign.

### Task 3: Events and related Pods tables

**Interfaces:** Preserve `ResourceEvents({ data: EventPage, onNext: (cursor: string) => void })` and `DeploymentPods({ data: RelatedPage, onNext, onSelect })` until the ReplicaSets navigation task explicitly extends them. No fetch on filter/expand.

- [ ] Test page-local warning filter counts, unknown event count/time, literal HTML-like messages and independent occurrence count. Test related Pods empty-complete vs partial/error/continuation states.
- [x] Implement Spec §3: event table, expandable details, All/Warnings, exact metadata and honest page-order/continuation wording. Reset view-local state on UID changes; never infer global latest ordering.
- [x] Implement Spec §5 Pods table with existing phase/readiness/restart evidence and explicit Pod links. Keep ReplicaSet name readable until the full explorer task makes it navigable. Show counts as “N shown” unless completion supports a total. Load more replaces the bounded batch.
- [ ] Run tests/lint/typecheck; verify long warning messages, zero/unknown counts, complete/partial pages and keyboard behavior in the rendered app. Commit this slice.

### Task 4: Read-only YAML reader

**Interfaces:** Preserve `ResourceYaml({ data }: { data: YAMLDocument })`. Proposed pure search interface: `yamlMatches(text: string, query: string): number[]` with a bounded result strategy documented by the implementation; displayed count must disclose any cap. Matching stays literal and case-insensitive, as existing behavior; empty query has no matches. A helper is optional if existing code provides the same contract.

- [ ] Add tests for empty/no-match, forward/backward wrap, mixed case, multiline text, Unicode offset correctness, managed-fields switching and refreshed-document reset. Assert copied output equals the active YAML string with no line numbers or markup.
- [x] Implement compact toolbar, selectable read-only source, keyboard search, match feedback and exact copy per Spec §4. Decide and record bounded highlighting behavior before introducing a dependency; prefer a safe plain-text fallback for large content.
- [x] Confirm query/selection reset is scoped to ResourceRef UID and refresh; the two managed-fields representations remain from one capture. Escape hostile strings and preserve whitespace/newlines.
- [ ] Run frontend checks and exercise near-2-MiB synthetic text in the native app: search, scrolling, selection/copy and resize remain usable. Record any fallback threshold and limitations rather than claiming performance from a small sample.
- [ ] Commit the independently usable YAML reader.

### Task 5: Literal technical input behavior

**Interfaces:** Existing input elements in resource filtering, namespace query, YAML search and `LogViewer.tsx`; field-level settings first. Do not add a global input wrapper unless the repository already has one that fits.

- [x] Re-read the concurrent Logs changes and identify its actual search input before editing. Inventory all technical search/filter fields and distinguish intentional namespace suggestions from linguistic correction.
- [ ] Add assertions that field settings disable spellcheck/correction/capitalization/autocomplete. Preserve existing event handlers and IME behavior. Fetch current platform documentation using the repository's Context7 workflow if implementing library/platform-specific syntax; do not assume attributes prove native behavior.
- [ ] Verify packaged macOS typing with OS correction enabled: `bootstrap`, mixed-case identifiers, hyphenated names, punctuation, paste and IME composition stay literal and do not show linguistic correction bubbles. Also verify intentional namespace results still work.
- [ ] If field settings fail, gather native evidence before choosing the smallest scoped WebView adjustment; no system-wide preferences changes. Run applicable frontend/native checks, then commit.

### Task 6: Integration and handoff

- [x] Run `scripts/check.sh` and `npm run build`; if bindings/native code changed, regenerate with `wails3 task bindings` and run the package workflow from local development docs.
- [x] Add `docs/superpowers/validation/workload-explorer-refinement.html` and a separate `workload-explorer-refinement-ui-uat.html`; report automated/native evidence separately from user acceptance. Keep operational output under ignored `.superpowers/sdd/`.
- [ ] Verify the shared header and inputs have not regressed the separately implemented Logs tab. Do not claim to solve its high-volume work.
- [ ] Update owning UX references, concise status and verified plan checkboxes. Commit/push only this task's files per git workflow; no merge/release without applicable authorization.

## Separate-session prompt

“Implement the reviewed workload UI refinement plan at `docs/superpowers/plans/2026-11-13-workload-ui-refinement.md` after reading its spec and repository instructions. Confirm the Phase 1C.2 and concurrent Logs baseline first. Preserve unrelated work. Execute the checked scope only, record real verification, and leave the ReplicaSets plan for its own authorized execution unless I explicitly request both. No release or merge.”
