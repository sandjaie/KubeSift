# Phase 1C.3d Storage Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans in the separate implementation session. Track steps with checkboxes. Repository policy requires one independent whole-PR review after implementation/verification, not routine per-task reviews.

**Goal:** Inspect six storage kinds and safely follow explicit Pod/claim/volume/class and CSI references.

**Architecture:** Extend current Go typed reader, desktop cancellation bridge and React explorer state. Dedicated bounded storage projections share ResourceRef/detail/YAML contracts; a storage resolver validates allowlisted source fields and cross-scope identity before target reads.

**Tech stack:** Existing pinned Go, client-go/k8s.io v0.37.0, Wails v3, React/TypeScript; no dependency upgrades. Use the current [toolchain guide](../../engineering/toolchain.html).

**Spec:** [Phase 1C.3d storage design](../specs/2026-11-14-phase-1c3d-storage-design.md).

## Global constraints

- Existing `/Users/sandjaieravi/dev/KubeSift` checkout; no worktree. Verify current develop includes merged 1C.3c and review policy, then create `feature/phase-1c3d-storage` without switching/stashing unrelated work.
- Planning-only until separate implementation authorization. Read-only product; no mounts, cloud disks, CSI installation, Secret access, snapshot workflow, release or later phase.
- PVC uses core v1 and selected namespace. PV is core v1/cluster; StorageClass, VolumeAttachment, CSIDriver and CSINode are storage.k8s.io/v1/cluster.
- API page 100; history 20; cursor 16 KiB; Back eight; reads 30 seconds/16 concurrent; decoded body 16 MiB; serialized bridge/detail/YAML 2 MiB; local nested table page 100.
- Preserve UI and current configuration/Secret/networking/Logs behavior. Progress goes to checkpoint, plan checkboxes and validation report; do not add progress to roadmap.
- Required agent desktop verification and one whole-PR review with fix rechecks; no duplicate user UAT or per-task review gates.

## Review focus

1. Cross-scope PV→PVC links could read arbitrary namespaces: source field/UID validation and negative I/O tests in Task 2.
2. Name reuse can silently link a released PV to a new claim: bidirectional/UID tests in Task 2.
3. Missing status/default fields can become false certainty: projection and rendering tests in Tasks 1/3.
4. Late cluster-scoped responses can overwrite a namespaced selection: state/Back tests in Task 4.
5. Synthetic CSI evidence could be mislabeled as live mount verification: fixture ownership and evidence matrix in Task 5.

## Task 1 — Storage inventories and detail contracts (S01–S05, S07)

**Create:** `internal/kube/storage.go`, `storage_types.go`, `storage_test.go`.
**Modify:** `internal/kube/details.go`, `networking.go` (current resourceScope owner), `events.go`, `internal/desktop/workloads.go`, `internal/desktop/workloads_test.go`; generated bindings via generator only.

**Interfaces:** `StorageRow { Resource ResourceRef; CreatedAt *time.Time; ObjectJSON string }`, `StoragePage { Items []StorageRow; Page PageInfo }`; `ReadStorage(ctx context.Context, req PageRequest) (StoragePage, error)` and desktop `ReadStorage(id string, req kube.PageRequest) (kube.StoragePage, error)`. Private `storageVersion(kind string) string` and `isClusterScopedResource(kind string) bool` centralize exact backend scope without weakening namespaced validation. Reuse ResourceDetail/YAMLDocument for selected objects.

- [ ] Write failing tests for six kinds, correct API/group/scope, invalid UID/address, unknown kind, list/get errors, empty continuations, expired/cross-kind/cross-generation cursor and body/bridge caps. Assert cluster Event requests reject before I/O and PVC Events retain UID filtering. Distinguish absent collection API from absent named target.
- [ ] Write inventory projection tests for spec-defined columns, explicit nil versus empty class, missing versus zero quantities and no arbitrary metadata/source/parameter/attachment payload in inventory. Include Attachment status absence versus explicit false. Preserve evidence before typed serialization if omission would erase this distinction; do not reconstruct status presence from zero values.
- [ ] Run `go test ./internal/kube ./internal/desktop` to observe meaningful failures; implement typed bounded lists/GETs and summary projections. Integrate version/scope dispatch into shared details and preserve Secret-specific rejection/metadata paths. Show unsupported APIs per kind without fallback/discovery.
- [ ] For VolumeAttachment list/get only, retain raw status-field presence from the same bounded REST response before decoding into the typed object. Use that presence in the inventory projection and detail ObjectJSON; omit absent status instead of serializing a zero-value typed status as reported false. Do not issue a second request to recover presence. Cover absent status, empty status and explicit attached=false/true in transport tests; absent attached is unavailable evidence.
- [ ] Cap every returned detail/YAML/page. Validate returned name/namespace/UID and connection generation. Add tests for oversized nested objects and malformed identity.
- [ ] Run Go tests/vet, `wails3 task bindings` and `npm run typecheck`; inspect generated changes. Commit reader contracts with source descriptions. No per-task reviewer.

## Task 2 — Validated storage references (S03, S06)

**Create:** `internal/kube/storage_references.go`, `storage_references_test.go`.
**Modify:** `internal/desktop/workloads.go`, `frontend/src/workloads/api.ts`, generated bindings.

**Interfaces:** `StorageReferenceRequest { Source ResourceRef; SourceVersion string; Target ResourceRef }`; `ResolvedStorageReference { Resource ResourceRef; SourceVersion string; TargetVersion string; IdentityEvidence string; BindingEvidence string }`. `ResolveStorageReference(ctx context.Context, req StorageReferenceRequest) (ResolvedStorageReference, error)` and same desktop name with request ID. IdentityEvidence is `name-resolved` or `uid-verified`; BindingEvidence is `not-applicable`, `references-agree` or `unconfirmed`. Map these to the exact spec labels in the UI.

- [ ] Test every allowlisted source→target route in spec section 4 and no others. Assert no target GET for stale source version/UID, missing/forged reference, wrong kind/API, connection mismatch and illegal namespace. PV→PVC may read only the claimRef namespace. Pod→PVC never leaves its namespace.
- [ ] Test PVC→PV missing claimRef, prebinding with no UID, matching pair, other-claim conflict and recreated PVC UID. Test PV→PVC with empty versus conflicting target volumeName; enforce supplied target UID and authoritative claimRef UID together. Record source and target versions without claiming atomicity.
- [ ] Test direct CSI driver references, no StorageClass provisioner guessing, nil/empty class without lookup, inline/generic ephemeral no fabricated PVC/PV, missing driver and local target denial.
- [ ] Run failing tests, implement resolver using bounded reads and shared identity conventions; reuse primitives without widening existing networking/configuration resolver allowlists. Return target UID and honest evidence labels. Run Go tests and targeted race tests; regenerate bindings, add cancellable frontend wrapper and typecheck. Commit resolver.

## Task 3 — Storage views and Pod volume evidence (S02–S06)

**Create:** `frontend/src/storage/model.ts`, `StorageTable.tsx`, `StorageOverview.tsx`, `PodStorage.tsx`, `storageEvidence.test.tsx`, `README.html`.
**Modify:** `frontend/src/ui/ResourceIcon.tsx`; existing shared style classes only as needed, no new design system.

**Interfaces:** `StorageTable({ rows: StorageRow[], kind, selected, onSelect })` follows existing ConfigurationTable selection contract. `StorageOverview({ data: ResourceDetail, onOpenReference: (target: ResourceRef) => void })` uses the captured source held by the caller. `PodStorage({ data: ResourceDetail, onOpenReference: (target: ResourceRef) => void })` emits only explicit PVC targets; add separate projection helpers in model.ts for tests. StorageKind/isStorageKind exports integrate in Task 4.

- [ ] Write tests covering exact inventory/Overview contract: quantities, class tri-state, claim refs, reclaim policy, access/mode evidence, source/affinity/mount options, default annotations, omitted boolean fields, attachment/error evidence and CSI driver entry boundaries/counts.
- [ ] Test hostile/long strings as inert text, >100 nested rows, expandable source attributes, unknown fields gracefully omitted from structured interpretation and YAML serialization limitation copy. No fake health/utilization or inferred free-attachment counts.
- [ ] Test Pod volume type/mount/device projection across regular/init/ephemeral containers. Only direct PVC references get links; generic ephemeral/inline sources stay configuration. ConfigMap/Secret refs remain in their current component without extra fetches.
- [ ] Run `npm test` to demonstrate failures, implement views with existing compact table/tab typography, wrapping, accessible text/status labels and per-captured-object local pagination. Raw paths/endpoints never become execution actions. Run frontend tests/lint/typecheck/build and commit views/source map.

## Task 4 — Explorer wiring and cross-scope navigation (S01, S07–S09)

**Modify:** `frontend/src/workloads/model.ts`, `state.ts`, `state.test.ts`, `useWorkloads.ts`, `api.ts`; `frontend/src/workspace/ResourceNavigation.tsx`, `ResourceDetail.tsx`, `ResourceOverview.tsx`, `Workspace.tsx`; `frontend/src/App.tsx` if callback plumbing requires it.

**Interfaces:** extend ResourceKind/resourceKinds with StorageKind; inventory union adds StoragePage; existing Overview/YAML/Events sections suffice. Expand isClusterScoped explicitly. Dispatch storage references separately from configuration/networking, retaining source version. Preserve returned identity/binding evidence in the opened detail context and Back entries; clear it on unrelated selection/refresh so stale link badges cannot survive newer evidence. Extend state types explicitly if they currently retain only ResourceRef.

- [ ] Write reducer tests for each new scope/tab, namespace-kind retention, disabled namespace chooser, independent cluster tabs and PVC→PV→class→Back. PV→PVC must route to explicit claim namespace and restore previous scope on Back.
- [ ] Test related targets absent from current inventory, late inventory clearing attempts, late detail/resolver successes and errors, refreshed/changed source, cancelled navigation, per-kind page history and transient binding-badge invalidation.
- [ ] Wire Storage group, inventory/detail dispatch, PodStorage and reference handling with explicit no-prefetch behavior. Enforce PVC-only Events and no Logs/operation tabs. Use existing eight-entry Back history; no generalized dynamic resolver.
- [ ] Run frontend tests/lint/typecheck/build plus Go bridge tests. Preserve configuration Data departure clearing, Secret-only Overview restrictions and all workload/networking/Logs tests. Update touched source maps and commit integration.

## Task 5 — Integration, desktop verification and full-PR delivery (S01–S10)

**Create:** `tests/integration/storage_test.go`, owned storage fixture manifests under `tests/fixtures/explorer/`, `docs/superpowers/validation/phase-1c3d.html`.
**Modify:** fixture/integration READMEs, capability/UX coverage as verified, this plan's checkboxes and `.superpowers/sdd/status.md`. Roadmap changes only for scope/sequence/dependency/exit-criteria changes.

- [ ] Add opt-in `TestStorageInvestigation` using existing isolated configuration directory and explicit loopback kind-dev-local-a/b validation. Add restricted identity with allowed PVC reads but denied cluster storage reads, using synthetic local fixtures and existing setup conventions.
- [ ] Provision only the spec's owned API fixtures: isolated namespace, nondefault no-provisioner class, Retain static PV, prebound and pending PVC, and unscheduled reference Pod. Collision-check cluster names, record ownership, and avoid host-directory/mount operations. Do not install CSI or fabricate live attachment success. Reuse genuine CSI resources if available; absent CSI edge cases use separately labeled unit/transport evidence.
- [ ] Run `scripts/check.sh`, `go test -race ./internal/kube ./internal/desktop`, `wails3 task bindings`, `npm run build`. Use `KUBESIFT_EXPLORER_INTEGRATION=1` and an absolute owned `KUBESIFT_EXPLORER_CONFIG_DIR`; run `go test -tags=integration ./tests/integration -run TestStorageInvestigation`. Run existing configuration/networking/workload integration regressions under the same approved setup. Follow the current integration README for credential files; do not use ambient context.
- [ ] Package with `wails3 task package`, verify S10 using desktop computer use on that exact artifact: complete claim/volume/class/Back journey, cross-scope PV→claim route, namespace retention, keyboard, light/dark, narrow split, 200% zoom and exact displayed-YAML copy. Verify honest CSI empty/denied views where available. Record actual results and unrun cases, package identity and synthetic fixture cleanup; no duplicate manual UAT checklist.
- [ ] Update checkpoint, plan and permanent report with actual evidence; update capability/UX claims only when verified. Review the entire committed candidate once with a fresh `kubesift-code-review` agent, supplying spec/plan, base/merge-base/head SHAs and test evidence. Fix findings, rerun affected checks and obtain targeted rechecks for final-head coverage. Record dispositions/reviewer identity in report and PR template.
- [ ] Push feature branch, create/mark ready PR into develop and merge once applicable checks/reviews pass under standing authorization. Do not stop at a draft or ask again for merge instructions. Verify merge before status update; delete branch only if no ongoing work. Report PR, merge and limitations. No release or 1C.3e work.

## Separate-session handoff

Implement Phase 1C.3d from this plan and linked spec in `/Users/sandjaieravi/dev/KubeSift`; no worktree. Start from current develop containing merged 1C.3c, inspect unrelated changes, then create `feature/phase-1c3d-storage`. Deliver six read-only storage kinds and safe explicit Pod/PVC/PV/StorageClass/CSI reference navigation, preserving UI, scopes, bounds and Secret/Logs behavior. Complete all five tasks, automated/integration and packaged desktop verification, record progress outside the roadmap, perform one independent whole-PR review and fix rechecks, then ready PR and merge into develop when gates pass. No duplicate user UAT, CSI installation, product mutations, release or next phase.
