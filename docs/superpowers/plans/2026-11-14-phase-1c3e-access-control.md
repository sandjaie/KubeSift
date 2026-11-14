# Phase 1C.3e Access Control Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans in the separate implementation session. Track verified steps with checkboxes. One independent whole-PR review after implementation/testing; no routine per-task reviews.

**Goal:** Read-only ServiceAccount/RBAC inspection with safe Pod, role and subject navigation.

**Architecture:** Dedicated bounded access-control inventory projections, existing resource detail/YAML/Event paths, and an allowlisted reference resolver returning the validated target capture. Extend current explorer state and components without permission simulation or a shell redesign.

**Tech stack:** Current pinned Go, k8s.io v0.37.0, Wails v3 and React/TypeScript. Follow the [toolchain guide](../../engineering/toolchain.html); no dependency upgrades.

**Spec:** [Phase 1C.3e design](../specs/2026-11-14-phase-1c3e-access-control-design.md).

## Global constraints

- Existing `/Users/sandjaieravi/dev/KubeSift` checkout; no worktree. Check current develop includes storage `eff5135`, preserve unrelated work, then create `feature/phase-1c3e-access-control`.
- ServiceAccount uses v1; four RBAC kinds use rbac.authorization.k8s.io/v1. Only ClusterRole/ClusterRoleBinding are new cluster kinds.
- API pages 100; cursor history 20; cursor 16 KiB; Back eight; deadline 30 seconds; active reads 16; decoded body 16 MiB; serialized bridge/detail/YAML/resolver 2 MiB; local nested pages 100.
- No product writes, tokens, impersonation, access-review APIs, effective-access calculation, Secret lookup, external identities or reverse binding scans.
- Preserve current UI, scope retention, validated target captures, lazy nested rendering, Secret protections and Logs. Progress in checkpoint/plan/report, not roadmap.
- Implementer owns automated/integration and packaged desktop verification; one report, no duplicate user UAT. Whole-PR review/fix rechecks and ready-PR merge follow the current Git workflow.

## Review focus

1. DNS-only validation rejects valid system RBAC names: path/name encoding tests in Task 1.
2. Subject namespace could be confused with grant scope or permit arbitrary target GETs: resolver tests in Task 2 and rendering tests in Task 3.
3. Joining rules/aggregation selectors can imply privileges never represented: grouping tests in Task 3.
4. A second initial GET can display unvalidated target evidence: capture/state tests in Tasks 2 and 4.
5. Large rule arrays and denied related GETs can disrupt other views: bounds tests in Task 1, lazy rendering in Task 3 and native/integration checks in Task 5.

## Task 1 — Inventories, names and details (A01, A02, A06, A08)

**Create:** `internal/kube/access_control.go`, `access_control_types.go`, `access_control_test.go`.
**Modify:** `internal/kube/details.go`, `networking.go` (current scope helper), `events.go`, `internal/desktop/workloads.go`, `internal/desktop/workloads_test.go`; generated bindings via generator only.

**Interfaces:** `AccessControlRow { Resource ResourceRef; CreatedAt *time.Time; ObjectJSON string }`; `AccessControlPage { Items []AccessControlRow; Page PageInfo }`; `ReadAccessControl(ctx context.Context, req PageRequest) (AccessControlPage, error)` plus desktop `(id string, req kube.PageRequest)`. `accessControlVersion(kind string) string` defines exact allowlist. Reuse ResourceDetail/YAMLDocument. Add private `validResourceName(kind, name string) bool`: four RBAC kinds use pinned path-segment validation with explicit nonempty check, all other kinds retain existing DNS validation.

- [ ] Add failing tests for five kind/API/scope combinations, unknown kinds, list/get denials, malformed/returned identity and UID replacement. Assert colon-containing RBAC names round-trip through actual HTTP paths; slash, percent, dot segments and empty reject before I/O. Confirm old-kind name rules do not widen.
- [ ] Test inventory summaries omit full rules/subjects/Secret-reference arrays and metadata; preserve nil automount versus true/false. Add empty continuing page, expired/mixed cursor, cancellation, HTTP and bridge-size boundary tests. Cluster Events reject before I/O; namespaced Events use checked UID.
- [ ] Run `go test ./internal/kube ./internal/desktop` to observe failures; implement typed bounded lists/details and explicit projections, scope/version/name dispatch and Events guards. Avoid touching Secret reader behavior. Treat absent collection versus missing GET separately; no fallback/discovery on forbidden.
- [ ] Add wire-level tests that all introduced product requests are GET/LIST for allowed resources/events, never Secret/token/access-review endpoints. Preserve error sanitation and final connection-generation validation.
- [ ] Run Go tests/vet; regenerate `wails3 task bindings`, inspect generated diff and run frontend typecheck. Commit reader contracts/source descriptions; no per-task review.

## Task 2 — Reference resolver with captured evidence (A05–A07)

**Create:** `internal/kube/access_control_references.go`, `access_control_references_test.go`.
**Modify:** `internal/desktop/workloads.go`, `frontend/src/workloads/api.ts` and generated bindings.

**Interfaces:** `AccessControlReferenceRequest { Source ResourceRef; SourceVersion string; Target ResourceRef }`; `ResolvedAccessControlReference { Detail ResourceDetail; Resource ResourceRef; SourceVersion string; TargetVersion string; IdentityEvidence string }`; `ResolveAccessControlReference(ctx context.Context, req AccessControlReferenceRequest) (ResolvedAccessControlReference, error)` and desktop variant with request ID. IdentityEvidence is `name-resolved`; Detail and Resource/version must describe the same single capped target capture.

- [ ] Test every route in spec section 4. Include RoleBinding→Role local scope, RoleBinding→ClusterRole, ClusterRoleBinding→ClusterRole, Pod→SA, explicit cross-namespace SA subject and omitted RoleBinding subject namespace fallback. Missing ClusterRoleBinding subject namespace is non-navigable. Wrong roleRef/subject API groups and ClusterRoleBinding→Role reject.
- [ ] Test User/Group subjects including `system:serviceaccount:`-looking strings cause no GET; duplicate valid subjects work without broadening malformed ones. Empty Pod serviceAccountName and obsolete alias do not invent a default link.
- [ ] Test source UID/version mismatch, reference removal, forged target namespace/kind/API/connection, missing/denied/reused target UID, and no target read before source validation. Name-only refs remain name-only even if an earlier same-name object existed.
- [ ] Run tests to observe failures; implement source validation, exact allowlist matching and one target capture. Reuse storage's validated-capture convention without broadening its resolver. Cap complete response and validate generation/cancellation. Test that Detail is the same capture validated by resolver, not a subsequent GET.
- [ ] Run Go/race tests, regenerate bindings, add cancellable API wrapper and typecheck; commit resolver.

## Task 3 — Structured access-control views (A03–A06, A08)

**Create:** `frontend/src/access-control/model.ts`, `AccessControlTable.tsx`, `AccessControlOverview.tsx`, `PolicyRules.tsx`, `BindingSubjects.tsx`, `PodIdentity.tsx`, `accessControlEvidence.test.tsx`, `README.html`.
**Modify:** `frontend/src/ui/ResourceIcon.tsx`; reuse existing layout/style classes.

**Interfaces:** export AccessControlKind/accessControlKinds/isAccessControlKind and labels. `AccessControlTable({ rows: AccessControlRow[], kind, selected, onSelect })` follows existing table contract. `AccessControlOverview({ data: ResourceDetail, onOpenReference: (target: ResourceRef) => void })`; `PodIdentity` takes the same props for Pod-only evidence. `PolicyRules({ rules, aggregationRule })` and `BindingSubjects({ source: ResourceRef, subjects, onOpenReference })` use locally defined types matching pinned RBAC fields; unknown strings stay visible, never coerced into supported targets.

- [ ] Write rendering tests for rule boundaries, core group, wildcards, subresources, resource-name restrictions, non-resource paths, empty arrays and malformed mixed rules. Aggregation tests preserve selector grouping without recomputing rules or fetching matches.
- [ ] Test RoleBinding namespace versus cluster role object scope and subject namespace, SA namespace fallback/absence, User/Group inert rows, duplicate subjects, invalid references and exact reason for unavailable links. Explicitly reject effective-permission or ownership labels.
- [ ] Test nil/true/false automount, nonempty/empty Pod account, ServiceAccount Secret refs inert and no prefetch. Test >100 rules/subjects/selector requirements, collapsed content not mounted, nested pagination, long hostile identity strings and reset on new UID/version.
- [ ] Run frontend tests to observe failures; implement compact, keyboard-accessible evidence tables, paged/lazy expansion and literal text. Display resolution namespace separately from binding scope. Run tests/lint/typecheck/build and commit views/source map.

## Task 4 — Navigation and state integration (A01, A07–A09)

**Modify:** `frontend/src/workloads/model.ts`, `state.ts`, `state.test.ts`, `useWorkloads.ts`, `api.ts`; `frontend/src/workspace/ResourceNavigation.tsx`, `ResourceDetail.tsx`, `ResourceOverview.tsx`, `Workspace.tsx`; `frontend/src/App.tsx` only if callback wiring requires it.

**Interfaces:** ResourceKind/page union gains AccessControlKind/AccessControlPage; existing Overview/YAML/Events sections suffice. Expand isClusterScoped only for ClusterRole/ClusterRoleBinding. Add `openAccessControlReference(source: ResourceDetail, target: ResourceRef)` matching the existing captured-navigation pattern: open returned Detail once, retain target UID/versions, clear resolution badge on refresh/new selection. Related navigation may change namespace only to the backend-validated target namespace.

- [ ] Add reducer/controller tests for all kinds, namespace-kind retention, cluster chooser state, per-tab scopes, RoleBinding→ClusterRole→Back and cross-namespace SA→Back. Preserve original filter/page/scroll within existing history bounds.
- [ ] Test related object outside list page, late inventory/detail/resolver success/error, source refresh/cancellation, target capture applied without second GET and badge invalidation. Verify outgoing ConfigMap Data clearing and existing storage binding evidence remain correct.
- [ ] Add Access control group, retain Cluster Namespaces/Nodes, remove combined placeholder; dispatch inventories/sections and mount PodIdentity. No new token/action/log tabs; no automatic role/subject fetch on selection.
- [ ] In the same navigation registry, move ConfigMaps and Secrets into Workloads after CronJobs; retain ResourceQuotas and LimitRanges under Configuration. Keep each destination once and preserve kind IDs, configuration routing and existing tabs/security. Do not change workloadKinds or controller behavior merely to change sidebar placement. Include this in the packaged walkthrough: both destinations open their existing views, namespace changes retain kind, Pod reference/Back navigation works, and Secrets still expose only metadata Overview.
- [ ] Run frontend tests/lint/typecheck/build and Go bridge tests. Update source descriptions; commit integration without per-task reviews.

## Task 5 — Verification, one whole-PR review and merge (A01–A10)

**Create:** `tests/integration/access_control_test.go`, owned RBAC fixtures under `tests/fixtures/explorer/`, `docs/superpowers/validation/phase-1c3e.html`.
**Modify:** fixture/integration READMEs, verified capability/UX claims, this plan and `.superpowers/sdd/status.md`. Do not record delivery progress in roadmap.

- [ ] Add opt-in `TestAccessControlInvestigation` following existing explicit loopback/context/config-directory setup. Use only kind-dev-local-a/b and owned isolated namespaces/ServiceAccounts. Fixtures cover local/cluster roleRefs, cross-namespace SA subjects, colon-containing RBAC names and uniquely labeled aggregated roles. Narrow read-only grants only; no built-in/admin/system-group edits or credential output. Malformed objects use synthetic HTTP/rendering fixtures.
- [ ] Test real list/get authorization separately, including readable binding with denied target role GET and allowed namespaced read while cluster RBAC is denied. Use dedicated restricted fixture credentials, not trial writes or product access-review calls. Record ownership/collision checks and cleanup only owned objects; keep credentials/config files untracked.
- [ ] Run `scripts/check.sh`, `go test -race ./internal/kube ./internal/desktop`, `wails3 task bindings`, `npm run build`; with `KUBESIFT_EXPLORER_INTEGRATION=1` and absolute owned `KUBESIFT_EXPLORER_CONFIG_DIR`, run `go test -tags=integration ./tests/integration -run TestAccessControlInvestigation`. Run existing storage/configuration/networking/workload integration regressions using the current README's setup. No ambient context.
- [ ] Package with `wails3 task package`; use desktop computer use to verify A10 on that exact artifact, including real system-name role inspection, local/cluster/cross-namespace journeys, denial where fixtures support it, lazy rules/subjects, narrow layout, both themes, keyboard, native 200% zoom and exact YAML copy. If tooling or scenarios are unavailable, record unmet gates honestly; browser screenshots do not substitute for native proof.
- [ ] Record actual automated/integration/native results, package identity, cleanup and unrun limits in one permanent report; update checkpoint/plan and verified coverage docs. Commit the full candidate, dispatch one fresh `kubesift-code-review` agent with spec/plan, base/merge-base/head and evidence. Fix findings, rerun affected checks and obtain targeted final-head rechecks; record reviewer identity/dispositions.
- [ ] Push feature branch, create/mark ready PR into develop, satisfy applicable checks/reviews and merge under standing authorization without another instruction. Verify merge before recording integration; delete branch only when no ongoing work remains. Report PR/merge/evidence and limits. No release or 1C.3f work.

## Separate-session handoff

Implement Phase 1C.3e using this plan and linked spec in `/Users/sandjaieravi/dev/KubeSift`; no worktree. Start from current develop including merged storage, inspect unrelated work, then create `feature/phase-1c3e-access-control`. Deliver five read-only ServiceAccount/RBAC views, path-segment-safe RBAC names and validated Pod/binding references with exact target captures. Preserve UI, scope/Back, bounds, Secret and Logs behavior. Complete tests and packaged desktop verification, update status/plan/report outside roadmap, perform one whole-PR independent review and targeted fix rechecks, then ready PR and merge into develop once gates pass. No duplicate user UAT, tokens/access-review APIs, product writes, release or next phase.
