# Neutral Dark Palette and Workspace Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Implement the approved neutral dark colours, searchable namespace dropdown and remembered Pod-details split.

**Architecture:** React owns the selector and effective split geometry; Go owns strict preference migration and persistence. Existing tab ownership, service calls and responsive layout remain intact.

**Tech Stack:** React 18, TypeScript, CSS, Go, Wails 3; no new dependencies.

**Spec:** [Approved design](../specs/2026-11-12-dark-palette-workspace-controls-design.md)

## Global Constraints

- No worktrees; use feature/workspace-theme-and-controls from develop.
- Only kind-dev-local-a/b, explicit targets; no new capabilities or AI integration.
- Flat neutral dark surfaces; preserve Light colours and existing layout apart from requested controls.
- Source descriptions, readable formatting and ignored execution notes follow engineering practice.

## Review Focus

- Version-1 files retain appearance and geometry; missing version-2 dimensions fail (Task 1 test).
- Small window clamping does not destroy saved shares (Task 2 native resize/restart check).
- Complete discovery rejects unlisted choices while restricted discovery supports explicit names (Task 2 test).
- Tab switching/dismissal does not commit stale typed input (Task 2 native check).
- Blue buttons, error text and metadata remain legible on every surface (Task 3 contrast check).

### Task 1: Persist independent detail splits

**Files:** internal/preferences/store.go, store_test.go; frontend/src/settings/preferences.ts; generated bindings.
**Interfaces:** Preferences version 2 adds detailWidthPercent and detailHeightPercent (integers, 20–80, default 50).

- [x] Extend preference tests: valid v1 load preserves prior fields and supplies 50/50; v2 saves 35/65; missing/null/out-of-range percentages rejected. Run go test ./internal/preferences and observe failure.
- [x] Implement strict in-memory migration and validation; regenerate Wails bindings and update frontend defaults.
- [x] Run go test ./internal/preferences and npm run typecheck; expect pass. Commit feat: persist Pod detail split preferences.

### Task 2: Namespace dropdown and draggable details

**Files:** frontend/src/workspace/NamespaceControl.tsx, namespaceOptions.ts, namespaceOptions.test.ts, ResourceSplit.tsx, Workspace.tsx; frontend/src/ui/ResizeHandle.tsx; frontend/src/App.tsx; frontend/src/styles.css.
**Interfaces:** namespaceOptions(query,names,complete,current) returns {value,manual} options. ResourceSplit receives widthPercent, heightPercent, onResize(axis,value), onCommit and list/detail children. ResizeHandle supports optional pixelsPerUnit and step, defaulting to existing pixel behavior.

- [x] Write namespace behavior tests for filtering, current explicit namespace, valid manual option only with incomplete discovery, empty/invalid input. Run focused Vitest and observe missing implementation failure.
- [x] Implement accessible button/popover/search/listbox; remove old inline NamespaceControl and identity breadcrumb. Keep selection routed through existing readPods.
- [x] Add measured responsive split wrapper and reuse pointer/keyboard separator. Wire both saved percentages via App. Keep minimum visible areas and independent orientation values.
- [x] Run frontend tests/lint/typecheck and packaged manual checks in Task 3. Commit feat: simplify namespace selection and resize Pod details.

### Task 3: Palette, mocks and verified handoff

**Files:** frontend/src/styles.css; docs/ux/workflows.html; docs/ux/mocks/index.html plus current reference artifacts; docs/index.html; .superpowers/sdd/status.md; docs/superpowers/validation/2026-11-12-workspace-refinement.html.
**Interfaces:** semantic CSS tokens distinguish success, interaction, hover, overlay and selection without changing Light values.

- [x] Apply approved dark tokens to explicit and System-dark, overlays, success statuses and selection; format changed files.
- [x] Check text/button contrast mathematically; run scripts/check.sh and wails3 task package, expect pass.
- [x] Launch package and exercise real A/B namespace selection, keyboard/dismissal, both split directions, themes and restart persistence. Save curated screenshots; label generated mock illustrative and keep historical image references.
- [x] Update UX owner, current mock gallery and status; report actual results, unrun checks and source migration. Run docs/source-description/diff checks.
- [x] Fresh whole-branch correctness/hygiene review; fix material findings with focused regressions. Result: no findings.
- [x] Commit/push, open PR to develop, no merge/release. [PR #4](https://github.com/sandjaie/KubeSift/pull/4) remains open.

Self-review: all approved changes are covered by the three tasks; persistence precedes its UI consumers; no new service or dependency is required. The user approved the mock and explicitly requested spec, plan and implementation in this session; proceed without another permission loop.

## Follow-up discussion after Tasks 1–3

Requested by the maintainer during implementation on 12 November 2026. Keep further UI/UX refinements in this plan after the current implementation is complete; propose and confirm their scope before adding executable tasks.

- [x] Review coloured dots beside Pod phase labels from the mock. Preserve the text label, distinguish phase from readiness, and agree on semantic colour mapping.
- [x] Present other focused UI/UX improvements based on the implemented workspace, then record the selected scope here.


### Task 4: Compact context and connection controls

**Files:** App.tsx, connections/ConnectionControls.tsx, workspace/Workspace.tsx, styles.css and source map.
**Interfaces:** ConnectionControls receives tab and explicit retry/cancel callbacks. Workspace receives navigation visibility. Preserve existing connection controller behavior.

- [x] Implement two header rows and status disclosure; keep failure Retry, connecting Cancel and helper review visible. Remove source/action bar and separate resource toolbar; move Refresh into Pods heading.
- [x] Run lint/typecheck and native checks in Task 6. Verify menu dismissal and target ownership; commit the compact controls.

### Task 5: Readable Pod inventory and compact details

**Files:** workspace/PodPhase.tsx, ResourceTable.tsx, ResourceDetail.tsx, styles.css and source map.
**Interfaces:** PodPhase renders actual phase text with a decorative semantic dot, including unknown fallback; no health inference.

- [x] Add semantic phase dots, sticky table headings and compact single-namespace rows. Collapse UID/source into a labeled disclosure; preserve context/namespace and readiness.
- [x] Run frontend tests/lint/typecheck. Verify scrolling, disclosure and phase/readiness distinctions in Task 6; commit.

### Task 6: Packaged review and existing PR handoff

**Files:** Same spec/plan, UX owner, mock gallery, validation report, curated screenshots and portable status.

- [x] Build package and run repository checks. Native local A/B: two header rows, namespace selection, status disclosure/dismissal, reconnect/disconnect/cancel, Refresh, phase dots, sticky headings, compact detail disclosure and existing divider behavior. Record unrun cases separately.
- [x] Update current mock gallery and final screenshots; update report and portable checkpoint. Fresh independent review, fix material findings, check completed plan steps and push to PR #4. No merge or release.

Follow-up test strategy: these presentation changes reuse already-tested target routing; use focused native behavior checks rather than mirrored markup tests or new test dependencies. Existing suites remain required.

Final follow-up review: no material defects. KSF-001 subsequently resolved: dedicated blue Running token in Light; Succeeded remains green. Native failure recovery, activated cancellation and compact Light presentation remain unrun as recorded in the validation report.

### Task 7: Light phase colour correction and curated screenshots

- [x] Give Running a dedicated blue token in Light while preserving Dark and green Succeeded.
- [x] Keep the current approved mock and three compact captures; remove superseded images and replace historical gallery content with text. Update report and portable checkpoint.
- [x] Check token contrast and documentation links, commit and push to PR #4. No repeat full UI verification.
