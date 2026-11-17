# Phase 1E.1 Helm Inspection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Inspect namespace-scoped Helm releases and selected revision evidence through the native SDK and current UI.

**Architecture:** Add a focused native Helm reader using owned Kubernetes clients, bounded metadata inventory and SDK storage Get through request-bound adapters. Return narrow section DTOs, not full release objects. React reuses the current list/detail layout with per-tab Helm state.

**Tech Stack:** Existing Go 1.26/Wails/React; pin Helm SDK v4.3.1 and align Kubernetes modules to its v0.37.1 family if needed. No shipped Helm executable, plugins or new UI framework.

**Spec:** [1E.1 design](../specs/2026-11-17-phase-1e1-helm-inspection-design.md). Spec and plan requested together for review; application implementation occurs in a separate session. Baseline `b636125`.

## Global Constraints

- Read [engineering](../../engineering/engineering.html), [Kubernetes references](../../engineering/kubernetes-development.html), [security](../../security/security-model.html) and [Git workflow](../../engineering/gitworkflow.md). Preserve current UI, source headers, normalized bindings and exact-version resource navigation.
- Inspection only: no install/upgrade/rollback/uninstall, hook/test execution, chart network access or model requests. Kubernetes permissions are authoritative; no extra approval gates.
- Explicit connection generation, namespace and Secret/ConfigMap driver; default Secret, no Forbidden fallback or inherited HELM_DRIVER. Default Secret browsing stays metadata-only.
- Metadata pages 100, snapshot cap 1,000 records, exact-name selector when over capacity; summary pages 100. Complete metadata scan before claiming latest retained revision.
- Limits: 30-second request, 16 MiB API body, 8 MiB expanded release, 32 MiB aggregate expansion/request, 2 MiB section/bridge response, four active Helm reads app-wide, 500 manifest resource references. No unbounded decode/cache.
- Use existing checkout and `feature/phase-1e1-helm-inspection` from updated develop; no worktree. Only explicit kind-dev-local-a/b for development. No release or 1E.2 work.
- Minimal behavioral tests, existing checks/package, agent desktop verification and one whole-PR independent review. No per-component test explosion or duplicate user UAT checklist.

## Review Focus

- Helm SDK background contexts and unbounded decoding must not bypass cancellation/bounds: task 1 tests.
- Partial metadata, duplicate revisions and changed record identities must not produce a false latest release: task 2 tests.
- Stored evidence and current live resources must not be presented as one historical identity: task 3 tests.
- Values/notes/manifests may contain sensitive text and must not enter logs or executable markup: task 3 tests and task 5 native inspection.
- Cluster/namespace/driver changes and late replies must not mix content: task 4 state test.

---

## File map and contracts

| Files | Responsibility |
| --- | --- |
| `internal/helmreader/types.go`, `service.go`, `storage.go`, `decode.go` | Connection-bound SDK integration, bounded read-only storage adapters and payload preflight |
| `internal/helmreader/inventory.go`, `details.go`, `resources.go` | Snapshot grouping, selected revision sections and manifest references |
| `internal/desktop/helm.go`; existing desktop composition | Thin cancellable bridge |
| `frontend/src/helm/api.ts`, `model.ts`, `state.ts`, `useHelm.ts`, `HelmWorkspace.tsx`, `ReleaseDetail.tsx` | One feature folder using current shared presentation |
| Existing App/workspace navigation and composition | Enable Helm destination without turning releases into Kubernetes resource kinds |
| Focused native/state test files, integration test and local chart fixtures | Representative evidence and cleanup |

Use generated camelCase DTOs. Exact public contracts:

- `Scope { Connection model.ConnectionRef; Namespace, Driver, ReleaseName string }`. ReleaseName is optional exact-name filter; Driver is `secret` or `configmap` only.
- `RecordRef { Connection model.ConnectionRef; Namespace, Driver, ReleaseName, Revision, RecordName, UID, ResourceVersion string }`. Revision remains exact decimal text. No Kubernetes UID is invented for a Helm release.
- `ReleaseSummary { Record RecordRef; Status, ChartName, ChartVersion, AppVersion string; UpdatedAt *time.Time; Problem *model.Problem }`.
- `ListRequest { Scope Scope; SnapshotID string; Page int }`, `ReleasePage { SnapshotID string; Items []ReleaseSummary; Page, Total int; FetchedAt time.Time }`. Empty SnapshotID starts a new bounded scan; Page is zero-based. Total is only complete snapshot release count, never global cross-namespace count.
- `HistoryRequest { Record RecordRef; Page int }`, `HistoryPage { Items []ReleaseSummary; Page, Total int; FetchedAt time.Time }`.
- `DetailRequest { Record RecordRef; Section string; ComputedValues bool }`, `ReleaseSection { Record RecordRef; Section, Text string; FetchedAt time.Time; Overview *ReleaseOverview; Resources []ManifestResource; Truncated bool; Problem *model.Problem }`. ReleaseOverview contains the spec's recorded metadata; ManifestResource carries apiVersion/kind/name/namespace plus resolvable/issue, never historical UID.
- Service methods `ListReleases(ctx context.Context, req ListRequest) (ReleasePage, error)`, `ReadHistory(ctx, HistoryRequest) (HistoryPage, error)`, `ReadSection(ctx, DetailRequest) (ReleaseSection, error)`, `CloseScope(connection model.ConnectionRef, namespace, driver string)`.

Thin ExplorerService bridge methods mirror reads with `(requestID string, req)` using existing cancellation. Native snapshots contain sanitized metadata only, bounded per active scope and cleared on replacement/disconnect; retain at most one scope snapshot per cluster tab and a native cap of 16 scopes, with explicit capacity response instead of eviction. Section payloads are not cached natively after response. Frontend retains only displayed section, not all release revisions. Lifecycle hooks invalidate snapshots/requests on scope changes.

## Task 1: Native SDK and bounded storage integration

**Files:** Add helmreader service/storage/decode/types and `storage_test.go`, package README.html; modify go.mod/go.sum only for necessary SDK dependency changes.

**Interfaces:** Internal `readRelease(ctx context.Context, ref RecordRef) (release.Releaser, error)` backed by SDK v4 storage Get; `metadataPage(ctx, scope, cursor)` returns only sanitized storage metadata. Service constructor consumes existing connection manager, not kubeconfig paths.

- [ ] Add `TestBoundedStorage` with an HTTP fake: selected namespace/driver path, request cancellation despite SDK background context, 8 MiB decompression rejection, Forbidden propagation, record UID/version mismatch and no write methods. Assert API calls never use mutable global context or fallback driver.
- [ ] Pin `helm.sh/helm/v4 v4.3.1`, inspect its relevant tagged storage/value APIs and resolve modules. Review Kubernetes family patch alignment, go mod diff, required notices and resulting package graph; do not upgrade unrelated UI dependencies. Run targeted tests and confirm the new behavior fails before adapter implementation.
- [ ] Implement request-local typed Secret/ConfigMap wrappers for SDK storage Get that substitute the owned request context, enforce existing connection transport/auth and disable writes. Bind Get to captured storage UID/version; never let SDK bypass that check. Do not call SDK List/Query for namespace scanning. Suppress content-bearing SDK logs using a safe discard handler rather than global logging changes.
- [ ] Before passing an immutable payload copy to SDK decode, bound base64 and optional gzip expansion using a limit+1 reader. Enforce decoded size and aggregate request budget. SDK may decode the verified copy again; never re-read an unchecked record between preflight and decoding. Scope metadata uses PartialObjectMetadata with explicit Helm selectors; if unsupported, use a bounded full-object request, strip bodies immediately, and never return them as metadata. Annotate this fallback honestly in code.
- [ ] Run `go test ./internal/helmreader` and `go test ./...`; expect pass. Record resolved versions and dependency/size implications for the later validation report. Commit `feat: integrate bounded native Helm storage reads`.

## Task 2: Release list and retained history

**Files:** Add `internal/helmreader/inventory.go`, `inventory_test.go`; extend service/types and desktop Helm read endpoints.

**Interfaces:** Implements ListReleases and ReadHistory using task 1 metadata and SDK reads; produces stable snapshot and RecordRef contracts for details.

- [ ] Add `TestReleaseInventory` table: multiple status/revision records, Secret/ConfigMap name isolation, exact large revision sorting, duplicate/corrupt record, forbidden namespace, 1,001 metadata entries and selected-record replacement. Assert no “latest” page on incomplete scans and a corrupt newest record never silently selects an older release.
- [ ] Run `go test ./internal/helmreader -run TestReleaseInventory -count=1`; confirm failing assertions first.
- [ ] Implement 100-record metadata pages with owner=helm and optional exact name selector. Validate metadata names/revision labels without reading arbitrary annotations. Finish within 1,000 cap, group release names and select maximum valid exact revision; ambiguous/malformed identities show explicit record problems. SDK-decode only current summary page, keep payloads out of snapshot storage and check record metadata against decoded identity.
- [ ] History scans the selected exact name within the same bounds; validates captured selected record before showing its revision set. Page results and fetched times are tied to snapshot identity. Expired/changed snapshots require Refresh, never mix pages. Register cleanup with existing connection lifecycle without replacing other subscribers.
- [ ] Run `go test ./internal/helmreader ./internal/desktop`, `wails3 task bindings`; expect pass. Commit `feat: browse Helm releases and retained revisions`.

## Task 3: Selected revision content and current-resource links

**Files:** Add `internal/helmreader/details.go`, `resources.go`, `details_test.go`; complete desktop section bridge.

**Interfaces:** Implements ReadSection. The selected RecordRef is fixed across overview/history selection and content sections. Current resource navigation consumes existing exact-version resolver, not a new live inventory engine.

- [ ] Add a compact `TestReleaseSections`: stored versus computed values, missing chart/coalesce error, exact multi-document manifest, inert notes, hook metadata, namespaced/cluster-scoped manifest references, missing API/name and a pruned record. Assert no render/lookup/chart network call, no live object prefetch, section cap and captured-record identity.
- [ ] Run `go test ./internal/helmreader -run TestReleaseSections -count=1`; confirm new cases fail.
- [ ] Implement requested-section projection only. Values default to stored overrides; use SDK chart/value coalescing for computed form and label its origin. Manifest and notes retain exact stored text. Hooks show recorded evidence, never run anything. Unknown/incomplete chart data can fail computed view without blanking available stored evidence. Record SDK numeric/coalescing limitations honestly; no precision claim stronger than the pinned SDK preserves.
- [ ] Parse manifest identities under bounded YAML/depth limits; keep 500 references maximum with explicit truncation. Resolve scope from API discovery. Cluster-scoped objects use empty namespace; missing namespace defaults only for known namespaced APIs. Unsupported/generated-name-only entries remain visible with reason. Existing live navigation performs a fresh name-based Get on user click and labels it “Open current resource”; no claim of historical UID or Helm ownership.
- [ ] Run `go test ./internal/helmreader ./internal/desktop` and `go test -race ./internal/helmreader`; regenerate bindings. Commit `feat: inspect Helm values manifests and resource references`.

## Task 4: Current workspace integration

**Files:** Create frontend helm feature files above plus `state.test.ts`; modify existing ResourceNavigation/Workspace/App composition as required, preserving collapsed groups, split preferences and resource Back navigation.

**Interfaces:** `useHelm(tabID, connection, namespace, visible)` owns selected driver/filter/snapshot/page/revision/section with request epochs and cancellation. Native SDK types stay behind generated API adapter. UI callback opens current resource and retains a Back-to-release reference.

- [ ] Add one `state.test.ts` scenario: A namespace release/revision → B or driver switch → delayed A section response, then Refresh. Assert discarded response, original revision identity, cleared payloads and preserved per-tab navigation without hidden scans. Run `npm test --workspace frontend -- src/helm/state.test.ts` and confirm failure.
- [ ] Enable current Helm sidebar entry. Reuse list/detail layout, Refresh, current table controls and theme tokens. Add explicit Secret/ConfigMap selector and optional exact release-name filter, with clear capacity/empty/Forbidden messages and snapshot paging. No all-namespace or new dashboard.
- [ ] Add the seven detail sections from the spec with lazy reads, selected revision/time labels and existing copy/search controls. One list grouping plus History is sufficient; no separate timeline library. Render all content inertly. Values/manifest reads need no extra permission prompt, but never claim they are redacted or persist their content.
- [ ] Wire current-resource navigation/Back; cancel and clear payloads on scope changes. Preserve the Helm source revision when returning; revalidate if changed. Ordinary live Secret views remain metadata-only. Opening Settings/theme changes must not spawn duplicate requests. Keep Ask AI's existing behavior; do not submit Helm data.
- [ ] Run targeted Vitest, `npm run lint`, `npm run typecheck`; expect pass. Commit `feat: add Helm release inspection workspace`.

## Task 5: Local compatibility, verification and delivery

**Files:** Add `tests/fixtures/helm/` tiny local chart/README, `tests/integration/helm_test.go`, `docs/superpowers/validation/phase-1e1.html`; update integration/validation indexes, checkpoint and verified plan steps.

**Interfaces:** One opt-in Helm inspection integration group follows existing isolated configs and context/endpoint guards. Fixture creation is development-only, not a product endpoint. Do not use a real chart repository or create extra clusters.

- [ ] Prepare owned releases with two retained revisions on kind-dev-local-a/b, using Secret and ConfigMap drivers. Record actual Helm 3 and Helm 4 producer versions. Use available development tools or a versioned fixture generator without introducing a CLI runtime dependency; if live Helm 3 production is unavailable, use a traceable synthetic Helm 3 encoded fixture and label that compatibility coverage rather than claiming a live install. Keep names/namespace ownership explicit and clean them up.
- [ ] Run one walkthrough covering list/history, revision-specific values/manifests, resource link and denied access. Verify no app writes, hooks or chart-network requests. Verify oversized/corrupt cases through native fixtures instead of installing large charts. Document exact commands, skips and limits.
- [ ] Run `wails3 task bindings`, `npm run build`, `scripts/check.sh`, `wails3 task package`; expect pass. Compare package size to baseline and record dependency impact, without inventing a performance success claim. Check source headers and formatter output.
- [ ] Agent-run packaged verification: both clusters, namespace/driver isolation, select old revision, read Values/Manifest/Hooks/Notes, open current resource/Back, no releases/Forbidden/corrupt feedback and both themes. Prove the packaged path works without invoking a Helm executable. Use synthetic content only in screenshots.
- [ ] Publish one technical report with exact SDK/client versions, tested record producer versions/backends, commands, native evidence and unsupported/unrun cases. Update checkpoint/plan and validation index; detailed logs stay ignored. Commit `test: validate native Helm inspection`.
- [ ] Obtain one independent whole-PR review through repository skill, resolve blocking findings and rerun affected checks. Push branch, open ready PR and complete authorized merge to develop under current workflow. No duplicate UAT gate, release or 1E.2 implementation.

## Self-review and handoff

Native compatibility/cancellation/bounds → task 1; complete metadata/latest/history → task 2; revision evidence/live-link semantics → task 3; existing UI and target isolation → task 4; actual compatibility/delivery → task 5. The five review risks have focused tests. Shared DTOs stay consistent; no step is complete during planning.

Review this plan with its spec, then implement in a separate session. The proposed Phase 1E sequence remains Helm inspection, Helm management, current metrics, historical metrics and node administration; none is silently dropped.
