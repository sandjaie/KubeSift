# Phase 1C.3c Configuration Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans in the separate implementation session. Track steps with checkboxes. Repository policy requires one whole-PR independent review after implementation and verification, not per-task reviews.

**Goal:** Read-only namespace policy/configuration exploration, including metadata-only Secrets and explicit Pod reference navigation.

**Architecture:** Extend the existing Go typed read/desktop bridge/state/view path. Add configuration inventory DTOs and a strict Secret metadata reader; reuse common details only for supported nonsensitive paths. Keep resource and scope validation authoritative in Go.

**Tech stack:** Existing Go 1.26, k8s.io libraries v0.37.0, Wails v3, React/TypeScript and current test/build tools; no dependency upgrades.

**Spec:** [Phase 1C.3c design](../specs/2026-11-14-phase-1c3c-configuration-design.md).

## Global constraints

- Existing checkout only; no worktree. Confirm clean develop with merged 1C.3b and current review policy, then use `feature/phase-1c3c-configuration`. Never switch or stash unrelated in-progress work.
- No application implementation in the planning session; no writes/reveal/AI/discovery/next phase/release in this increment.
- Core v1 Namespace is cluster-scoped; ResourceQuota, LimitRange, ConfigMap and Secret require a selected namespace.
- API pages 100; cursor history 20; encoded cursor 16 KiB; Back eight; reads 30 seconds / 16 active; decoded body 16 MiB; serialized bridge payload/object/YAML 2 MiB; local key pages 100.
- Secret Overview contains only name, namespace, UID, resourceVersion and creation time. No full-object fallback or Secret YAML/Events/data. All Secret get/list paths use strict PartialObjectMetadata negotiation.
- Preserve approved UI and Logs behavior. Agent verifies desktop journeys; one report, no repeat manual UAT. Whole-PR review and merge follow [Git workflow](../../engineering/gitworkflow.md#independent-pr-review).

## Review focus

1. Generic YAML/Events/resolver paths could bypass metadata-only Secret reads: negative transport tests in Task 1 and end-to-end checks in Task 5.
2. Quota status is absent/late or units differ: exact-string and missing/zero tests in Tasks 1 and 3.
3. A referenced name is reused after a Pod snapshot: source version and target UID tests in Task 2.
4. Namespace navigation races a late inventory/error: reducer tests in Task 4.
5. Large multiline/binary configuration exposes values in inventory, logs or stale views: projection/cap tests in Task 1, rendering/copy tests in Task 3 and native verification in Task 5.

## Task 1 — Bounded inventories and safe details (C01, C03–C06, C08)

**Create:** `internal/kube/configuration.go`, `configuration_types.go`, `secret_metadata.go`, `configuration_test.go`, `secret_metadata_test.go`.
**Modify:** `internal/kube/details.go`, `bounded_reads.go`, `events.go`, `internal/desktop/workloads.go`, `internal/desktop/workloads_test.go`; generated `frontend/bindings/` through the generator only.

**Interfaces:** `ConfigurationRow { Resource ResourceRef; CreatedAt *time.Time; ObjectJSON string }`, `ConfigurationPage { Items []ConfigurationRow; Page PageInfo }`. `ReadConfiguration(ctx context.Context, req PageRequest) (ConfigurationPage, error)` and desktop `ReadConfiguration(id string, req kube.PageRequest) (kube.ConfigurationPage, error)`. ObjectJSON is an explicit inventory projection: namespace phase; quota resource count/scope summary; limit item types; ConfigMap key counts/immutable; empty object for Secret. Reuse ResourceDetail for detail projections, adding a Secret-specific dispatch before any typed read. Private `readSecretMetadata(ctx context.Context, ref ResourceRef) (ResourceDetail, error)` and `listSecretMetadata(ctx context.Context, req PageRequest) (ConfigurationPage, error)` must enforce the spec's strict transport contract and bounds.

- [ ] Add failing tests for all five kinds and scopes; wrong API/kind, empty namespace, invalid UID, denied get/list, replaced object, expired/mixed cursor and empty page with continuation. Tests assert inventory projection excludes ConfigMap data/binaryData and arbitrary Secret metadata.
- [ ] Add HTTP fixture tests asserting partial-only Accept negotiation for list/get, expected response kind/group, cancellation/body cap and safe errors. Assert 406 and full Secret responses cause an error and no second request; annotations with synthetic content never enter DTOs. Assert direct Secret YAML and Events calls reject without I/O, and unsupported Namespace Events do not enumerate all namespaces.
- [ ] Run `go test ./internal/kube ./internal/desktop` and confirm new tests fail on missing contracts/behavior; implement bounded configuration reads, strict Secret metadata transport and explicit kind/scope dispatch. Keep Secret value-bearing types out of the new reader. Reuse authenticated configuration and bounded transport without changing stream behavior.
- [x] Extend Overview/YAML for Namespace, quota, limit and ConfigMap; Events only for the three permitted namespaced kinds. Capture UID/version/time; verify response identity and cap every bridge response. Preserve raw quantity strings and absent fields.
- [ ] Run Go tests and `go vet ./...`; regenerate `wails3 task bindings` and run `npm run typecheck`. Inspect generated changes and commit this deliverable. Do not launch a per-task review.

## Task 2 — Pod references and scope routing (C02, C07)

**Create:** `internal/kube/configuration_references.go`, `configuration_references_test.go`.
**Modify:** `internal/desktop/workloads.go`, `frontend/src/workloads/api.ts` and generated bindings.

**Interfaces:** `ConfigurationReferenceRequest { Source ResourceRef; SourceVersion string; Target ResourceRef }`; `ResolvedConfigurationReference { Resource ResourceRef; IdentityEvidence string }`; `ResolveConfigurationReference(ctx context.Context, req ConfigurationReferenceRequest) (ResolvedConfigurationReference, error)` plus desktop `(id string, req ...)`. Source is Pod only. Return identity evidence `name-resolved`; optional supplied target UID is checked. Namespace browse uses an explicit fresh `ReadResource(DetailRequest)` UID check before routing; it is not this Pod resolver.

- [x] Test every allowed env/envFrom/volume/projected/imagePullSecrets reference across regular/init/ephemeral containers, duplicate targets and optional/key references. Assert no GET for disallowed plugin/CSI references, wrong connection/namespace/API, removed reference, stale source UID/version or forged target.
- [ ] Test missing/forbidden target, supplied target UID mismatch and Secret target metadata-only transport; capture current UID for name-only references. Do not test key presence by fetching Secret data.
- [x] Run tests to demonstrate failures, then implement source revalidation and field allowlist before target resolution. Share existing identity conventions without widening the networking resolver.
- [ ] Regenerate bindings, add frontend API wrapper and cancellation; run `go test ./internal/kube ./internal/desktop`, targeted race tests and typecheck. Commit reference contracts.

## Task 3 — Configuration and namespace views (C02–C06)

**Create:** `frontend/src/configuration/model.ts`, `ConfigurationTable.tsx`, `ConfigurationOverview.tsx`, `ConfigMapData.tsx`, `configurationEvidence.test.tsx`, `README.html`.
**Modify:** `frontend/src/ui/ResourceIcon.tsx`, relevant shared styles only where existing classes cannot express the views.

**Interfaces:** `ConfigurationTable({ rows: ConfigurationRow[], kind, selected, onSelect })` follows NetworkTable's selection contract. `ConfigurationOverview({ data: ResourceDetail, onBrowseNamespace: (kind: "ConfigMap" | "ResourceQuota" | "LimitRange") => void })`. `ConfigMapData({ data: ResourceDetail })` owns only local key selection/filter/page/wrap/copy state and resets when resource UID/version changes. Secret kind renders solely its allowlisted metadata, never generic metadata grids.

- [ ] Write rendering/projection tests for the spec tables: quota union of spec/status keys and missing/zero distinction; limit item boundaries; Namespace conditions/finalizers; metadata-only Secret with no hidden reveal/YAML affordance.
- [ ] Test ConfigMap text/binary/empty data, Unicode byte length, multiline exact copy, literal hostile strings, >100 keys, filtered-page reset and state clear on changed UID/version. Do not introduce snapshot tests that merely duplicate markup.
- [ ] Run `npm test` to confirm meaningful failures, implement the tables and structured views using existing tokens/components. Values remain inert and selectable; binary is base64 only. No inventory value prefetch or persistent cache.
- [x] Run frontend tests, lint, typecheck and build; update the source description and commit the views.

## Task 4 — Explorer integration and retained state (C01, C02, C07–C09)

**Modify:** `frontend/src/workloads/model.ts`, `state.ts`, `state.test.ts`, `useWorkloads.ts`, `api.ts`; `frontend/src/workspace/ResourceNavigation.tsx`, `ResourceDetail.tsx`, `Workspace.tsx`; Pod Overview component located from ResourceDetail imports; `frontend/src/App.tsx` only for required callbacks.

**Interfaces:** extend ResourceKind with the five new kinds and Section with `Data`. Inventory page union adds ConfigurationPage. `Data` consumes a captured ResourceDetail for ConfigMap only. Existing Reference/Back navigation remains authoritative; the configuration resolver supplies the same captured ResourceRef shape. Namespace browse callbacks validate source UID, then route selected namespace/kind while preserving independent cluster/tab state.

- [ ] Add reducer tests for every new kind/section, namespace changes retaining kind, cluster Namespace independence, related target absent from inventory page, stale success/error and source-version invalidation. Cover cancellation on namespace/tab/selection/refresh, ConfigMap Data clearing and Back to original Pod section/position.
- [x] Add Pod reference projection tests from captured ObjectJSON: correct location/key/optional fields, no unsupported references and no background API calls. Duplicate reference locations stay visible without duplicate speculative reads.
- [x] Wire registry, inventory dispatch, conditional tabs and components. Add compact Pod references with on-click resolver and local error behavior. Secret direct section attempts remain blocked by both UI and backend.
- [x] Run `npm test`, lint, typecheck/build and Go tests for bridge contracts; verify existing workloads/networking/Logs state tests still pass. Update touched source maps and commit integration.

## Task 5 — Full verification, one PR review and delivery (C01–C10)

**Create:** `tests/integration/configuration_test.go`, owned configuration fixtures under `tests/fixtures/explorer/`, `docs/superpowers/validation/phase-1c3c.html`.
**Modify:** fixture/integration READMEs, owning capability/UX docs, permanent plan checkboxes and `.superpowers/sdd/status.md` after actual results.

- [x] Add opt-in `TestConfigurationInvestigation` following networking_test.go's isolated config and loopback/context validation. Use only explicit kind-dev-local-a/b. Create owned isolated namespace quota/limit fixtures, synthetic ConfigMap/Secret/Pod references and restricted credentials using existing local setup. Do not install infrastructure or disturb existing workloads.
- [ ] Exercise real metadata negotiation, inventory/detail/UID checks, forbidden reads independent of other resources, scope routing and Pod references on both clusters. Keep mock-only full-response/406/status/cap cases identified separately. Cleanup only owned objects after capturing synthetic evidence.
- [ ] Run `scripts/check.sh`, `go test -race ./internal/kube ./internal/desktop`, `wails3 task bindings`, `npm run build`, and the existing opt-in integration command with the isolated configuration directory and new `TestConfigurationInvestigation` selector. Run workload/networking integration regressions too. Consult tests/integration/README.html for exact opt-in variables; do not substitute current kubeconfig context.
- [ ] Package with `wails3 task package`; use desktop computer use to verify C10 on that exact app, including Secret absent actions, explicit namespace routes, exact text/base64 copy, 200% zoom and rapid tab/scope changes. Record passed/failed/unrun checks honestly in one report; do not generate a duplicate mandatory user UAT checklist. Existing unrelated unrun checks remain named gaps.
- [ ] Update owning documents with implemented coverage and limits, preserving “planned” for deferred kinds. Record progress in `.superpowers/sdd/status.md`, task completion here and verification in the validation report. Change the roadmap only if scope, sequence, dependencies or exit criteria change; do not add delivery status there. Commit candidate; dispatch one fresh reviewer with the full candidate commit range, this spec/plan and actual evidence using `kubesift-code-review`. Fix confirmed findings, rerun affected checks and obtain targeted rechecks of the new head. Record reviewer identity, reviewed SHAs and dispositions; no review after each earlier task.
- [ ] Push the feature branch, create/mark ready the PR into develop with the completed template, satisfy actual required checks/approvals and squash-merge under standing authorization. Verify merge before marking integrated; clean up source branch only if it has no ongoing work. Report PR, merge, evidence and limitations. Do not release or start 1C.3d.

Unchecked steps have pending or partial evidence in the [validation report](../validation/phase-1c3c.html).

## Separate-session prompt

Implement Phase 1C.3c using this plan and its linked spec in the existing `/Users/sandjaieravi/dev/KubeSift` checkout; no worktree. Start from current develop including merged 1C.3b and create `feature/phase-1c3c-configuration` after checking for unrelated work. Deliver Namespaces, ResourceQuotas, LimitRanges, ConfigMaps and strict metadata-only Secrets plus allowlisted Pod configuration links. Preserve the existing UI, scope retention, bounds and Logs behavior. Follow all five tasks, run automated and packaged desktop verification, update actual status/docs, perform one fresh whole-PR review with targeted fix rechecks, then create a ready PR and merge into develop once gates pass. No duplicate manual UAT, Secret reveal, cluster writes in product, release or next phase.
