# Phase 1C.3g Cluster Resources and API Discovery Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans in the separate implementation session. Check off verified steps. One fresh independent whole-PR review after implementation and testing; no routine per-task reviews.

**Goal:** Seven dedicated infrastructure destinations plus safe, bounded installed-API browsing.

**Specification:** [Cluster resources and discovery design](../specs/2026-11-14-phase-1c3g-cluster-discovery-design.md). It owns G01–G09 acceptance, navigation and limits.

**Architecture:** Fixed dedicated cluster readers remain independent of discovery. Native scoped discovery issues cache-bound handles to a separate GVR-aware generic reader. Existing dedicated GVRs reuse their safe readers. Raw-preserving captures cross the bridge only for explicit details. Frontend reuses shared explorer tables/state with a distinct generic route.

**Stack:** Existing Go, client-go, Wails and React pins; no upgrades or new cluster infrastructure. See [toolchain](../../engineering/toolchain.html).

## Execution constraints

- Work in `/Users/sandjaieravi/dev/KubeSift`, no worktree. Verify develop contains 1C.3f merge `8cf3fe3` and this planning package; preserve unrelated edits. Branch `feature/phase-1c3g-cluster-discovery` using repository Git policy.
- Planning does not implement code. In the authorized implementation session, read AGENTS.md, repository Kubernetes skill/reference index, this spec, checkpoint and actual source before editing.
- Preserve fixed-kind ResourceRef behavior, metadata-only Secrets, exact validated capture navigation, namespace retention and existing Logs. Do not widen existing HPA/custom-target resolver behavior through discovery.
- No product writes/watch/proxy/subresources, installers, metrics, release or Phase 1D. Only approved explicit local kind contexts for task-owned test fixtures.
- Progress in this checklist and tracked checkpoint; evidence in `docs/superpowers/validation/phase-1c3g.html`; roadmap remains scope-only. Follow agent verification and whole-PR review/ready-merge policy.

[Validation evidence](../validation/phase-1c3g.html) records observed results and remaining gaps.

## Review focus

1. Discovery bypasses Secret/dedicated boundaries or constructs arbitrary routes: Task 2/3 negative-I/O tests.
2. Partial discovery, cache eviction and reconnect reuse a stale target: Task 2 cache tests and Task 5 state tests.
3. Unstructured data loses unknown fields/numbers or leaks whole inventory objects: Task 3 capture/projection tests.
4. Node/Lease/Event status misstates health/history or opens recreated objects: Task 1 evidence/resolver tests and Task 4 rendering tests.
5. API extension browsing creates unbounded requests, allocations or UI trees: Tasks 2/3 cap tests, Task 5 rendering limits and Task 6 native verification.

## Task 1 — Dedicated readers and Event inventory (G01–G03)

**Create:** `internal/kube/cluster_resources.go`, `cluster_resource_types.go`, `cluster_resources_test.go`, `event_inventory.go`, `event_inventory_test.go`.
**Modify:** `internal/kube/details.go`, `events.go`, `networking.go` scope helper, `workload_types.go`, `internal/desktop/workloads.go` and tests. Inspect existing availability/storage raw-capture readers before choosing shared helpers.

**Contracts:** `ReadClusterResources(ctx, PageRequest) (ClusterResourcePage, error)` with row metadata and bounded per-kind summaries, no embedded schemas/conversion payloads. Extend ResourceDetail with optional cluster summary where raw presence/exact values need native projections. `ReadEventInventory(ctx, PageRequest) (EventInventoryPage,error)` retains actual Event ResourceRef/API family, normalized evidence and PageInfo. `ResolveEventTarget(ctx, EventTargetRequest) (ResolvedEventTarget,error)` takes source ResourceRef/sourceVersion, derives target from captured source, verifies target UID and returns the exact validated ResourceDetail. Desktop methods use existing request ID/cancellation contracts.

- [ ] Add failing tests for every fixed API/scope, namespace requirement, invalid identity, missing/forbidden API, malformed collections, pagination and output caps. Dedicated readers must not perform discovery.
- [ ] Test exact numbers/quantities and absent/false/zero, Node Ready versus scheduling, Lease evidence without expiry inference, CRD versions/conditions and APIService local versus service-backed configuration. Use bounded raw fixed-GVR reads for CRD/APIService without new client dependencies.
- [ ] Test Event modern/core fallback on absence only, no fallback on denial, actual family bound into continuation, page-only sort, missing time/count, series normalization and UID mismatch. Reject unsafe source/target scopes before target reads; no Events-on-Event recursion.
- [x] Run focused Go tests to observe failures, implement readers/DTOs/details and event resolver. Register fixed version/scope and existing YAML capture integration; verify no writes, direct service/webhook calls or cluster-wide Event scans. Run `go test ./internal/kube ./internal/desktop` and commit.

## Task 2 — Scoped discovery catalog and guarded handles (G04–G05)

**Create:** `internal/kube/discovery.go`, `discovery_types.go`, `discovery_test.go`.
**Modify:** `internal/kube/read.go` service cache/lifecycle, connection closure integration as found in `internal/desktop`, and desktop read adapter/tests. Reuse bounded transport and cancellation rather than a second unbounded client.

**Contracts:** `ReadAPIGroups(ctx, connection, refresh) (APIGroupCatalog,error)` returns root-specific problems plus successful bounded group/version projections. `ReadAPIResources(ctx, connection, group,version,refresh) (APIResourceCatalog,error)` validates selected advertised group/version. Descriptor includes opaque Handle, GVR, Kind, Namespaced, advertised readable verbs and route mode `dedicated|generic|unavailable` plus reason. Handles are native cache entries; caller never supplies executable path/URL. Catalog responses carry fetchedAt/epoch/partial status. Expose bounded groups and per-version descriptors separately.

- [ ] Test independent `/api` and `/apis` failures, preferred version mismatch, inconsistent groupVersion, duplicate descriptors, malformed paths and slash subresources. Pin Accept to unaggregated JSON; assert no OpenAPI, aggregate-Service or all-group fan-out calls.
- [ ] Test five-minute TTL, eight-document LRU, generation/epoch binding, explicit refresh, close cleanup, limits (512 groups/32 versions/1,024 descriptors), response caps and expired/evicted handles. Reject oversize documents explicitly.
- [x] Build a canonical dedicated group/resource/GVR registry and Secret guard, verified against existing dispatch: every supported GVR routes dedicated, unsupported versions of those group/resources unavailable; all core secrets generic routes blocked before I/O regardless of claimed kind/scope/version/alias. Do not infer paths from short names or Kind.
- [x] Implement minimal scoped cache/registry and methods after failing tests. Verify actual LIST/GET RBAC errors remain distinct from advertised verbs; no trial authorization mutation/review. Run unit/race tests and commit.

## Task 3 — Generic inventory and raw captures (G05–G07)

**Create:** `internal/kube/generic_resources.go`, `generic_resource_types.go`, `generic_resources_test.go`.
**Modify:** desktop adapters/tests and generated bindings through generator only.

**Contracts:** `GenericResourceRef { Connection; Group; Version; Resource; Kind; Namespace; Name; UID }` is distinct from fixed ResourceRef. `ReadGenericResources(ctx, GenericPageRequest) (GenericResourcePage,error)` takes connection, descriptor handle, namespace, continuation; limit fixed at 100. Rows contain only identity/createdAt. `ReadGenericDetail(ctx, GenericDetailRequest) (GenericResourceDetail,error)` resolves handle and address/expected UID and returns Resource, resourceVersion, fetchedAt, raw ObjectJSON and captured YAML representation within the shared cap. Native capture conversion must preserve number tokens. No generic user-selected HTTP options.

- [ ] Write negative-I/O tests for forged/mismatched/expired handles, namespace/scope, path escape, subresources, Secret bypasses, unsupported dedicated versions and cross-context/cross-GVR continuation. Verify custom resources with colliding Kind remain distinct.
- [ ] Test one page/call, expired continuation, metadata-only DTOs despite rich native response, object/list GVK and identity validation, forbidden list versus get, missing API and stale UID. No whole-list raw object data in bridge/error/log output.
- [ ] Test unknown JSON fields, integer beyond JS safe range, decimals, nulls, unusual valid object names, YAML capture/copy fidelity, oversized compressed/decoded bodies, output cap and cancellation. Reuse raw capture pipeline only if it passes these precision tests; do not marshal via float64/unstructured number conversion blindly.
- [x] Implement guarded list/get using descriptor-bound paths and existing transport. Regenerate `wails3 task bindings`, add wrappers in `frontend/src/workloads/api.ts` and typecheck. Commit backend contracts and generated changes.

## Task 4 — Dedicated evidence views (G01–G03, G08)

**Create:** `frontend/src/cluster/model.ts`, `ClusterTable.tsx`, `ClusterOverview.tsx`, `EventInventory.tsx`, `clusterEvidence.test.tsx`, `README.html`.
**Modify:** `frontend/src/workspace/ResourceNavigation.tsx`, `ResourceDetail.tsx`, `ResourceOverview.tsx`, shared table dispatch, `frontend/src/ui/ResourceIcon.tsx`.

- [ ] Add failing rendering tests for all seven kinds, missing versus zero/false, Node scheduling independent from Ready, unknown Lease health, Event count/time/source and honest page ordering. Large CRD schemas must start collapsed and render bounded pages/text only.
- [x] Implement approved grouping/columns/Overview/YAML and source details; use established tokens, controls and expansion patterns. No new dashboard or redesign. CRD Browse instances and APIService View resources emit descriptor-validated navigation intents, never direct URLs.
- [ ] Wire Event target open to exact resolver result with no second GET; non-navigable incomplete/unsupported references remain useful text. Source refresh/reentry invalidates stale evidence. Add tests for target replacement/denial and Back.
- [x] Run frontend tests, lint/typecheck and commit dedicated views.

## Task 5 — API browser and route/state integration (G04–G08)

**Create:** `frontend/src/discovery/model.ts`, `APIResourceBrowser.tsx`, `GenericResourceTable.tsx`, `GenericResourceDetail.tsx`, `discovery.test.tsx`, `README.html`.
**Modify:** `frontend/src/workloads/model.ts`, `state.ts`, `state.test.ts`, `useWorkloads.ts`, `api.ts`; `frontend/src/workspace/Workspace.tsx`, `ResourceNavigation.tsx`, detail dispatch and existing style tokens only where needed.

- [ ] Introduce a discriminated dedicated/generic/catalog route; generic route identity includes connection generation, GVR, scope/namespace and handle epoch. Audit every existing fixed-kind dispatch for unsafe assumptions rather than broadening ResourceRef to arbitrary strings.
- [ ] Write state tests for namespace retention, cluster-scope retention, separate connection catalogs/routes, changing group/version/resource, TTL/LRU eviction, removed APIs, refresh, Back, cancellation and out-of-order results. Expired handles require visible refresh/reselection; never silently map to a different resource.
- [x] Implement lazy group/version/resource selection, loaded-catalog filtering, partial/denied/cap/error states and dedicated redirect explanation. No implicit fallback version, auto-loaded all-group catalogs, automatic all-page fetch or Pods reset.
- [x] Implement metadata tables and escaped collapsed generic raw view/YAML using exact captured text. Search and expansion must be bounded; disable text correction on identifier/search controls. CRD/APIService catalog jumps require current descriptor match and correct scope.
- [ ] Test no hidden spec/status inventory payload, no eager recursive generic rendering, matching YAML copy, accessible keyboard controls and nested pagination. Run full frontend tests/lint/typecheck plus relevant Go tests; commit integrated UI.

## Task 6 — Integration, native verification and delivery (G01–G09)

**Create:** `tests/integration/cluster_discovery_test.go`, `docs/superpowers/validation/phase-1c3g.html`.
**Modify:** integration README, owning implementation READMEs, checkpoint and this checklist. Correct UX/spec only if implementation exposes a real contract conflict; do not silently relax safeguards or acceptance.

- [x] Add task-owned synthetic fixtures on both explicit kind-dev-local-a/b: namespaced and cluster-scoped custom CRDs without webhooks/controllers, large-number custom values, duplicate Kind across groups, Event+Pod, core Endpoints, PriorityClass/RuntimeClass/Lease and restricted discovery/read credentials. Inspect existing Nodes/APIServices; never modify their operational state. Reject external contexts. Track cleanup identities.
- [ ] Exercise native Go readers against both clusters for dedicated and generic paths, denied list/get, strict Secret route, CRD→instances matching, removed discovery and namespace isolation. Use HTTP fakes for malformed discovery, bounds, stale async races and unusual status combinations; report this distinction. Existing installed Gateway/snapshot APIs are optional observations, not required installations.
- [x] Run documented Go full tests/vet/race, frontend tests/lint/typecheck, binding generation consistency, build and package. Run affected integration and prior workload/network/configuration/storage/access/availability suites with their documented opt-in settings. Preserve prior validation limitations as history, not newly passed claims.
- [x] Use desktop computer-use on the final packaged build for spec journeys: Node, Event→Pod→Back, CRD→instances→object→YAML exact copy→Back, APIService→catalog, namespace retention and both contexts; keyboard, narrow split, light/dark and native zoom. Record actual checks and specific blocked/unrun cases in one report; no duplicate user UAT checklist.
- [ ] Delete only task-owned fixtures (custom objects before CRDs), remove temporary credentials and verify both-context cleanup. Run documentation links/formatting checks and record evidence. Keep detailed operational logs under ignored `.superpowers/sdd/`.
- [ ] Request one fresh independent whole-PR review with `.agents/skills/kubesift-code-review/SKILL.md`, full base/head diff and this spec. Address findings, targeted rechecks and any required rebuild/native retest. Report genuine unresolved blockers; do not claim successful verification without evidence.
- [ ] Update checkpoint/report and checked tasks, verify local author/remote/auth, push feature branch, open ready PR, satisfy checks/review and merge into develop under standing authorization. Update merge checkpoint and provide result. No draft waiting gate, release or automatic next-phase work.

## Planning self-review

Specification and plan cover scope, identity, API failures, bounds, negative-I/O safety, exact capture navigation, user-facing states, tests and delivery. Generic reads remain separate from known-resource safeguards. Remaining uncertainty is environmental API availability and native test access, which must be recorded during execution rather than replaced with invented results. All task boxes start unchecked because no application implementation is authorized in this planning session.
