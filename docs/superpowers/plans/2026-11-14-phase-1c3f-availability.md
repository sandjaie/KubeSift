# Phase 1C.3f Availability Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans in the separate implementation session. Track verified steps with checkboxes. One independent whole-PR review after implementation/testing, not routine per-task reviews.

**Goal:** Read-only HPA, PDB, RC and ControllerRevision investigation with honest scaling/disruption/revision evidence.

**Architecture:** RC extends workload DTOs/readers; other three kinds use bounded availability projections and raw-preserving captures. Separate selector-associated Pods from UID-owned children. New resolver opens the exact validated target capture; state/UI reuse existing navigation and lazy rendering.

**Tech stack:** Current Go/k8s.io v0.37.0/Wails/React pins; no new infrastructure or dependency upgrades. [Toolchain](../../engineering/toolchain.html).

**Spec:** [Phase 1C.3f design](../specs/2026-11-14-phase-1c3f-availability-design.md).

## Global constraints

- Existing `/Users/sandjaieravi/dev/KubeSift` checkout; no worktree. Check current develop includes access-control merge `dea9197`, preserve unrelated work, then create `feature/phase-1c3f-availability`.
- All four kinds require selected namespace: HPA autoscaling/v2; PDB policy/v1; RC v1; ControllerRevision apps/v1. No version fallback/dynamic discovery.
- API page 100; cursor history 20; cursor 16 KiB; Back eight; deadline 30 seconds; active reads 16; decoded body 16 MiB; serialized output/detail/YAML/resolver 2 MiB; nested pages 100.
- Owned revisions: at most eight API pages/call. PDB selected Pods: one API page/call. No accumulated all-page data or global ordering claim.
- No product writes, /scale or eviction APIs, metrics reads/installations, rollback, release or later phase. Preserve existing UI, grouping, captured navigation, Secret and Logs behavior.
- Progress in checkpoint/plan/report, not roadmap. Agent native verification, one whole-PR review/fix rechecks and ready-PR merge; no duplicate user UAT.

## Review focus

1. Null versus empty PDB selector accidentally scans all Pods: negative I/O and selector tests in Task 2.
2. Stale/omitted generation or metrics become live/zero claims: raw-presence/native-comparison tests in Task 1 and rendering tests in Task 3.
3. Label matches or revision names masquerade as controller ownership: owner/UID and association tests in Task 2.
4. Revision int64/data lose precision or execute as a manifest: exact-summary/YAML tests in Tasks 1/3.
5. Empty filtered history pages, malformed arrays and late resolver results break navigation: budget/cursor tests in Task 2, rendering in Task 3, state tests in Task 4.

## Task 1 — Inventory and captured details (F01–F03, F05, F06, F08)

**Create:** `internal/kube/availability.go`, `availability_types.go`, `availability_test.go`.
**Modify:** `internal/kube/workloads.go`, `controller_workloads.go`, `workload_types.go`, `details.go`, `events.go`, `internal/desktop/workloads.go`, `internal/desktop/workloads_test.go`; generated bindings by generator only.

**Interfaces:** RC joins ReadWorkloads returning WorkloadRow. `AvailabilitySummary { Observation string; Revision string }`, with Observation `current`, `stale`, `unknown` or empty when inapplicable; Revision is decimal text for ControllerRevision only. `AvailabilityRow { Resource ResourceRef; CreatedAt *time.Time; ObjectJSON string; Summary AvailabilitySummary }`; `AvailabilityPage { Items []AvailabilityRow; Page PageInfo }`; `ReadAvailability(ctx context.Context, req PageRequest) (AvailabilityPage, error)` and desktop `(id string, req kube.PageRequest)`. Add optional `Availability *AvailabilitySummary` to ResourceDetail. Private availabilityVersion allowlists only HPA/PDB/ControllerRevision.

- [x] Write failing kind/API/scope, invalid identity, list/get denial, missing API, cursor/cap and Events tests. Include namespaces required, existing RBAC-name rules unchanged and supported APIs only.
- [x] Test RC desired/observed counters and zero/no-observation; extend typed workload projection without inventing rollout fields. Test HPA/PDB missing versus explicit zero, native generation comparison above JS safe integers and exact ControllerRevision revision string. Inventories omit full data/templates/metrics/selectors.
- [x] Run `go test ./internal/kube ./internal/desktop` to observe failures; implement bounded reads using storage's same-response raw/typed capture approach for availability kinds. Preserve raw quantities/field presence and exact YAML; compare generations in Go using decoded integer fields and presence evidence. Snapshot caps apply before bridge serialization too.
- [x] Integrate kind dispatch for details/YAML/Events and Summary projection. Test null/missing metrics/rules-like collections safely, oversized raw revision data and no writes/scale/metrics/Secret requests. Run Go tests/vet, `wails3 task bindings` and typecheck; inspect generated diff and commit contracts.

## Task 2 — Ownership, selectors and reference resolution (F04–F08)

**Create:** `internal/kube/availability_relationships.go`, `availability_references.go` and corresponding `_test.go` files.
**Modify:** `internal/kube/relationships.go`, `internal/desktop/workloads.go`, `frontend/src/workloads/api.ts`, generated bindings.

**Interfaces:** `ReadPDBSelectedPods(ctx context.Context, req RelatedRequest) (SelectedPodPage, error)` returns `SourceVersion string; SelectorJSON string; Items []WorkloadRow; Page PageInfo; Complete bool; Problem *model.Problem`. This is not RelatedPod ownership. `ReadControllerRevisions(ctx context.Context, req RelatedRequest) (ControllerRevisionPage, error)` returns `SourceVersion string; CurrentRevision string; UpdateRevision string; Items []AvailabilityRow; Page PageInfo; Complete bool; Problem *model.Problem`. Marker fields populate only for StatefulSet. Extend existing ReadRelatedPods for RC.

`AvailabilityReferenceRequest { Source ResourceRef; SourceVersion string; Target ResourceRef }`; `ResolvedAvailabilityReference { Resource ResourceRef; Detail ResourceDetail; SourceVersion string; TargetVersion string; IdentityEvidence string; Association string }`; `ResolveAvailabilityReference(ctx context.Context, req AvailabilityReferenceRequest) (ResolvedAvailabilityReference, error)`. Identity is `name-resolved` for HPA, `uid-verified` for other routes; Association is `scale-target`, `selector-match` or `controller-owner`. Desktop variants include request ID and existing cancellation.

- [x] Test null PDB selector makes no Pod LIST, empty selector lists only namespace, expressions match correctly, invalid selectors reject, and membership is never ownership. Bind cursor to canonical selector/UID; test changed selector, empty continuing pages, denied lists, returned namespace/label mismatch and unchanged generation isolation.
- [x] Test RC selector narrowing plus controlling owner UID, rejecting orphan/name-only matches. Test revision owner API/kind/name/UID/controller flag, no controllers versus ambiguous controllers, eight-page budget/continuation, unrelated empty pages and StatefulSet captured marker names without latest-history claims.
- [x] Test resolver allowlist: HPA supported targets only; PDB captured Pod UID plus current selector; controller↔revision authoritative owner UIDs. All same namespace/connection. Assert no target I/O on forged source/ref/version; for PDB target GET, check UID/labels after bounded read. Unsupported HPA custom target and revision owners remain non-navigable.
- [x] Run failing tests, implement relationships/resolver and source revalidation. Return source version in every relationship page so clicks validate the page's actual source capture. Ensure target Detail is the exact validated capture, includes availability summary when relevant and is not fetched again on initial open.
- [x] Run Go tests/race, regenerate bindings, add frontend wrappers and typecheck; commit relationship contracts.

## Task 3 — Evidence views (F02–F06, F08)

**Create:** `frontend/src/availability/model.ts`, `AvailabilityTable.tsx`, `AvailabilityOverview.tsx`, `SelectedPods.tsx`, `ControllerRevisions.tsx`, `availabilityEvidence.test.tsx`, `README.html`.
**Modify:** `frontend/src/workspace/controllerOverview.ts`, `ResourceTable.tsx` if RC column dispatch needs it, `frontend/src/ui/ResourceIcon.tsx`.

**Interfaces:** AvailabilityKind/isAvailabilityKind for HPA/PDB/ControllerRevision; RC remains workload kind. `AvailabilityOverview({ data: ResourceDetail, onOpenReference })`, `AvailabilityTable({ rows, kind, selected, onSelect })`. `SelectedPods({ data: SelectedPodPage, onNext, onSelect })` displays association, never controller ownership. `ControllerRevisions({ data: ControllerRevisionPage, onNext, onSelect })` uses exact revision text and captured markers. Click callbacks use page SourceVersion and target UID via Task 4.

- [x] Write tests for HPA all five metric sources, separate spec/status lists, behavior/conditions, missing/zero status and unrecognized optional fields. No metrics pairing by index or live-data/scale recommendation language.
- [x] Test PDB IntOrString, selector tri-state, unknown eviction policy, stale/unknown observations, reported allowances and disruptedPods bookkeeping without name-based Pod links. Test no recalculation from displayed matching rows.
- [x] Test RC status/selector/template display and revision exact decimal strings, missing/unknown owners, arbitrary null/large data and no apply/rollback/reference execution. YAML copy must not parse/stringify revision numbers in JavaScript.
- [x] Implement lazy nested disclosures and local paging including nested metric/selector/data arrays; test >100 entries, null collections, hostile strings and collapse behavior. Use existing table/text/status tokens. Run frontend tests/lint/typecheck/build and commit views/source descriptions.

## Task 4 — Navigation, tabs and state (F01, F07–F09)

**Modify:** `frontend/src/workloads/model.ts`, `state.ts`, `state.test.ts`, `useWorkloads.ts`, `api.ts`; `frontend/src/workspace/ResourceNavigation.tsx`, `ResourceDetail.tsx`, `ResourceOverview.tsx`, `Workspace.tsx`; App callbacks if necessary.

**Interfaces:** RC added to workloadKinds; AvailabilityKind/page union added separately. Section union gains `Selected Pods` and `Revisions`; Evidence union gains both relationship pages. StatefulSet/DaemonSet get Revisions explicitly; PDB gets Selected Pods explicitly; HPA/ControllerRevision must not inherit owned Pods merely from sidebar placement. Add availability reference handler following current exact-capture navigation.

- [x] Test explicit tabs, namespace-kind retention, per-cluster isolation, relationship pages with source version, Back position restoration and related targets absent from inventory. Cover HPA→target, PDB→Pod→Logs and revision→owner loops within eight-history bound.
- [x] Test stale success/error on selector/source refresh, kind/tab/namespace changes and cancelled navigation; current/update markers clear on new parent capture and resolver evidence does not persist across refresh. Null relationship lists and empty continuation remain usable.
- [x] Wire readers/components/routes; preserve RC owned-Pod opening and all existing UI groups, ConfigMaps/Secrets placement, Secret-only Overview, ConfigMap Data clearing and Logs lifetime. Source page version, not an older Overview version, must accompany relationship clicks.
- [x] Run frontend tests/lint/typecheck/build and bridge tests; update source maps and commit. No per-task review gate.

## Task 5 — Verification and whole-PR delivery (F01–F10)

**Create:** `tests/integration/availability_test.go`, owned availability fixtures under `tests/fixtures/explorer/`, `docs/superpowers/validation/phase-1c3f.html`.
**Modify:** fixture/integration READMEs, verified capability/UX claims, this plan's checkboxes and `.superpowers/sdd/status.md`. No roadmap progress updates.

- [x] Add opt-in `TestAvailabilityInvestigation` using current explicit loopback kind-dev-local-a/b setup and isolated configuration directory. Fixtures include RC/owned Pod, PDB selectors, an HPA min=max on an owned target, and owned StatefulSet/DaemonSet revision history. Avoid existing workload targets; no metrics installation, eviction, manual scaling or revision mutation. Use synthetic transport fixtures for status/precision/invalid selectors/history caps not reproducible safely on kind.
- [x] Test all routes on both clusters and restricted source-readable/target-or-relationship-denied identity. Record actual live versus synthetic evidence separately; cleanup only owned fixtures/controllers and report leftovers. Credentials remain isolated/untracked.
- [x] Run `scripts/check.sh`, `go test -race ./internal/kube ./internal/desktop`, `wails3 task bindings`, `npm run build`. With `KUBESIFT_EXPLORER_INTEGRATION=1` and absolute owned `KUBESIFT_EXPLORER_CONFIG_DIR`, run `go test -tags=integration ./tests/integration -run TestAvailabilityInvestigation`; run existing access-control/storage/configuration/networking/workload regressions under their README setup. Never use ambient context.
- [x] Package via `wails3 task package`; agent desktop verification covers F10 on that exact artifact, including true PDB selector semantics and revision lazy display/Back. Check keyboard, themes, narrow split, native 200% zoom and exact YAML copy. Record unmet native gates if tools/cases are unavailable; no duplicate manual UAT checklist or browser-for-native substitution.
- [ ] Update checkpoint, plan, validation report and verified coverage docs. Commit full candidate; dispatch one fresh `kubesift-code-review` agent with spec/plan, base/merge-base/head and evidence. Resolve findings, rerun affected checks and obtain targeted final-head rechecks; record reviewer/dispositions.
- [ ] Push feature branch, create/mark ready PR into develop and merge after applicable checks/reviews pass without another instruction. Verify merge before checkpoint update; clean source branch only when no ongoing work remains. Report PR/merge/evidence and limitations. No release or 1C.3g implementation.

Validation: [Phase 1C.3f report](../validation/phase-1c3f.html).

## Separate-session handoff

Implement Phase 1C.3f using this plan and linked spec in `/Users/sandjaieravi/dev/KubeSift`; no worktree. Start from current develop containing access-control merge, preserve unrelated work, then create `feature/phase-1c3f-availability`. Deliver HPA/PDB/RC/ControllerRevision views, scale-target links, selector-associated PDB Pods, RC-owned Pods and StatefulSet/DaemonSet revision history with exact validated captures. Preserve grouping, bounds, Secret/Logs behavior and honest observation semantics. Complete automated/integration and packaged desktop checks, update progress outside roadmap, obtain one whole-PR independent review/fix rechecks, then ready PR and merge when gates pass. No duplicate user UAT, metrics installation, product writes, release or next phase.
