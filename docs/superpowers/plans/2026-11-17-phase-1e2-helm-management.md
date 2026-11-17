# Phase 1E.2 Helm Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install, upgrade, roll back and uninstall Helm releases using local, HTTPS and OCI chart sources in the existing desktop workspace.

**Architecture:** Extend the native SDK integration with a focused Helm management service, request-owned action configuration and bounded native chart/review snapshots. Keep the existing reader read-only. React reuses current editors, diffs and result presentation; native workers retain exact release targets across navigation.

**Tech Stack:** Existing Go/Wails/React/TypeScript; Helm SDK v4.3.1 and Kubernetes modules v0.37.1. No runtime Helm CLI, SDK fork or UI framework change.

**Spec:** [Phase 1E.2 design](../specs/2026-11-17-phase-1e2-helm-management-design.md). Baseline `8ec0327`; incorporate the planning commit before implementation. One integrated plan keeps source resolution, review and execution contracts together; each task produces independently checkable behavior.

## Global Constraints

- Follow AGENTS.md, [engineering](../../engineering/engineering.html), [security](../../security/security-model.html), [compatibility policy](../../engineering/toolchain.html#compatibility) and [Git workflow](../../engineering/gitworkflow.md). Preserve current UI and plain-English source descriptions.
- Explicit connection generation/namespace/driver/name; no mutable global kubeconfig, implicit driver fallback, trial mutations, plugins or automatic recovery.
- Credentials native session-only and origin-scoped. Store only source metadata/TLS file references in preferences. Charts/values/review bodies remain ephemeral; no content logs or AI submission.
- Preparation: resolution 120 seconds, review 60 seconds, token TTL 5 minutes. Execution: default 5 minutes, selectable 1–30 minutes, selected timeout is also the hard overall deadline.
- Bounds: archive 20 MiB, expanded chart 64 MiB, 4,096 entries/depth 32; index 10 MiB/10,000 entries/pages 100; values 1 MiB; rendered review 8 MiB/1,000 resource identities; release expansion 8 MiB; bridge chunks/pages 2 MiB.
- Four drafts/preparations, two running Helm operations, one per release key, 128 completed metadata results. Never free a worker slot/lock until termination. Use existing inspection bounds for reading stored evidence.
- Hooks and wait-for-readiness/Jobs enabled by default; optional disable-hooks, create-namespace and keep-history are reviewed. No automatic rollback/cleanup, force, adoption or arbitrary flags; MaxHistory=0 preserves history.
- Use `feature/phase-1e2-helm-management` from updated develop in the existing checkout. Only kind-dev-local-a/b and synthetic fixtures for cluster tests. No implementation of metrics, next phases or release.
- Minimal behavioral tests; agent packaged-app verification and one whole-PR independent review. No routine per-task reviewers or duplicate user UAT checklist.

## Review Focus

- Private-source redirects/OCI token exchange must not leak credentials or use ambient helpers: task 1.
- Expanding archives, embedded dependency charts and large rendered content must obey limits before SDK allocation: tasks 1–2.
- Dry-run with lookup/CRDs/hooks is a limited preview; no writes or hooks may occur while preparing: task 2.
- Repeated confirm, changed history and rollback/uninstall cancellation must retain identity and truthful partial outcomes: task 3.
- Navigation/reconnect while editing or executing must not retarget work or revive stale confirmations: task 5.

## File map and interfaces

| Location | Responsibility |
| --- | --- |
| New `internal/helmmanager/types.go`, `service.go`, `sources.go`, `charts.go` | Source/auth ownership, bounded charts and native handles |
| New `internal/helmmanager/configuration.go`, `review.go`, `execution.go` | Owned SDK configuration, preparation, tokens and workers |
| Existing `internal/helmreader/` | Reuse bounded inspection through narrow helpers where necessary; never enable writes on its read adapter |
| New `internal/desktop/helm_management.go`; existing `service.go` | Thin management bridge, native pickers and lifecycle composition |
| Existing `internal/preferences/`, `frontend/src/settings/SettingsPage.tsx`; new `frontend/src/settings/HelmSourcesSettings.tsx` | Nonsecret source preferences and session credential controls |
| Existing `frontend/src/helm/`; new `managementApi.ts`, `managementState.ts`, `HelmActionDialog.tsx`, `HelmOperationResults.tsx` | Action flow and stable status presentation |
| Existing `frontend/src/App.tsx`, `frontend/src/operations/YAMLDiff.tsx` and result presentation | Compose actions/results using current patterns, no global redesign |
| New focused native tests, `frontend/src/helm/HelmManagement.test.tsx`, `tests/integration/helm_management_test.go`; existing `tests/fixtures/helm/` | Small behavioral coverage and one integrated fixture suite |

Native contracts (all live in helmmanager; use existing camelCase JSON convention):

- `Source { ID, Name, Kind, URL, CAFile, CertFile, KeyFile string }`: Kind is https/oci; no credentials. `SourceCredentials { Username, Password string }` is input-only. Source path selection is native-owned.
- `ChartRequest { SourceID, LocalHandle, Chart, Version, Digest string }`; local uses LocalHandle, HTTPS uses SourceID/Chart/Version, OCI uses SourceID/Chart and Version or Digest. Reject ambiguous combinations.
- `ChartSnapshot { ID, Name, Version, Digest string; ExpiresAt time.Time }`: opaque ID; content remains native. Resolution pins archive bytes/dependencies. The raw source URI shown to UI is sanitized.
- `Target { Connection model.ConnectionRef; Namespace, Driver, ReleaseName string }`; reuse helmreader.RecordRef for captured existing records, never synthesize a kube.ResourceRef.
- `Options { DisableHooks, CreateNamespace, KeepHistory bool; TimeoutSeconds int }`; reject irrelevant options for an action. Wait and other fixed behavior follow the spec.
- `PrepareRequest { Action string; Target Target; ChartID, ValuesYAML string; RollbackRecord *helmreader.RecordRef; Options Options }`. Action is install/upgrade/rollback/uninstall. Snapshot and rollback identity must belong to the native review, not be trusted from display labels.
- `Review { Token string; Target Target; Action, ChartName, ChartVersion, Digest string; ExpiresAt time.Time; Current, Rollback *helmreader.RecordRef; Options Options; Sections []string; Warnings []string }` is metadata only.
- `ReviewSection { Text string; Offset, NextOffset int; Done bool }`: UTF-8-safe native byte offsets with at most 2 MiB serialized response. Reject offsets not issued for that snapshot. Render diff against stored current manifest, not claimed live-object diff.
- `Result { ID, Action string; Target Target; State, Stage string; StartedAt time.Time; FinishedAt *time.Time; Observed *helmreader.ReleaseSummary; Problem *model.Problem }`. States running/succeeded/failed/unknown. Stage is sanitized metadata, not arbitrary SDK logs.
- Service `ResolveChart(ctx context.Context, req ChartRequest) (ChartSnapshot, error)`, `Prepare(ctx context.Context, req PrepareRequest) (Review, error)`, `ReadReview(ctx context.Context, token, section string, offset int) (ReviewSection, error)`, `Execute(token string) (Result, error)`, `Cancel(id string) error`, `ListResults() []Result`, `Discard(id string)` and `Close()`.
- Source methods `ListSources() []Source`, `SaveSource(source Source) error`, `DeleteSource(id string) error`, `SetCredentials(id string, credentials SourceCredentials) error`, `ForgetCredentials(id string)`, `ListCharts(ctx context.Context, sourceID, filter, cursor string) (ChartPage, error)`. ChartPage contains chart/version entries plus next cursor; one bounded source snapshot supplies pages.

Implementer may consolidate small files or split a growing file, but must preserve these responsibilities. No generic source/plugin framework. Bridge endpoint names prefix these with Helm as needed, with the existing request ID cancellation pattern for reads/preparation. Native file picker endpoints return handles for chart/values imports; source TLS paths are selected natively. Reuse existing preference storage and connection lifecycle APIs rather than new global state.

## Task 1 — Resolve charts and private sources

**Files:** source/chart/type/service files above; focused `internal/helmmanager/sources_test.go`; preferences source metadata; native picker endpoints.

**Consumes:** connection-independent source metadata and native file handles. **Produces:** immutable ChartSnapshot and bounded ChartPage, session credential methods.

- [ ] Add table-driven `TestChartSources`: local packaged chart, authenticated HTTPS index/download and OCI digest resolution; assert exact versions/hashes, no ambient helper call, no Authorization on unrelated redirect and rejection of TLS downgrade. Include expired token, custom CA/client certificate, archive traversal/symlink/embedded expansion and index/entry limit cases in this one group.
- [ ] Run `go test ./internal/helmmanager -run TestChartSources -count=1`; establish failing behavior before implementation.
- [ ] Inspect pinned SDK getters/registry/loader APIs. Implement native source CRUD and in-memory credential injection, immutable chart acquisition and pre-decode bounds. Do not load default Helm/Docker credential stores or persist passwords. Keep preferences migration compatible with existing settings; credentials are write-only to frontend. Expose native pickers with restricted purpose and handle lifetime.
- [ ] Implement bounded HTTPS index filtering/pages and exact OCI version/digest requests, deadline cleanup and missing-dependency errors. Show provenance honestly; require dependencies packaged, and preserve fetched bytes until consumed/discarded/expired. Expire idle source/chart snapshots and clean owned temporary artifacts.
- [ ] Run `go test ./internal/helmmanager ./internal/preferences ./internal/desktop`; expect pass. Commit `feat: phase1e2-task1 resolve Helm chart sources`.

## Task 2 — Prepare an honest Helm review

**Files:** configuration/review files above, `internal/helmmanager/review_test.go`; narrow helmreader helper extraction if needed.

**Consumes:** task 1 chart handles, connection.Manager owned configuration, existing release evidence. **Produces:** Prepare/ReadReview/Discard and native tokens.

- [ ] Add `TestHelmReview` covering four actions: preparation sends zero mutating Kubernetes calls and runs no hooks; values overrides replace the old override mapping; rollback/uninstall use current head plus selected history correctly; hook/CRD/keep-policy impact is visible; invalid schema/Forbidden stop preparation. Verify 2 MiB chunking, 8 MiB cap and expired/changed inputs revoke reviews. Include a lookup/time-dependent chart proving the preview is not advertised as immutable output.
- [ ] Run `go test ./internal/helmmanager -run TestHelmReview -count=1`; confirm failures.
- [ ] Build SDK Configuration from the captured connection without reading global kubeconfig. Make separate read-only preparation and writable execution configurations. Bind discovery/storage/HTTP requests to request context, bounds and credential helper trust. Reuse safe release decoding; never feed unchecked SDK storage history into unbounded decompression.
- [ ] Implement action-specific preparation, Helm dry-run paths and stored evidence diff. Capture current head/absence and rollback record identity; reject pending state or incomplete release scans. No trial write permissions. Represent dry-run limitations explicitly, especially hooks, CRDs, lookup and server admission; reject unexpected mutation attempts at the native transport boundary.
- [ ] Save immutable charts/values/options under a 5-minute token, enforce atomic draft capacity and section chunk reads, clear payloads on expiry/discard. Keep the user-facing review metadata small; do not silently omit large effects. Chart resolve-to-review transfer retains its reservation instead of leaking a second slot.
- [ ] Run `go test ./internal/helmmanager ./internal/helmreader`; expect pass. Commit `feat: phase1e2-task2 prepare Helm operation reviews`.

## Task 3 — Execute all four actions with bounded lifecycle

**Files:** execution/service/configuration files, `internal/helmmanager/execution_test.go`.

**Consumes:** immutable review token from task 2. **Produces:** Execute/Cancel/ListResults and termination cleanup.

- [ ] Add `TestHelmExecution` with a controllable SDK/client seam: duplicate Execute returns one ID and one mutation; stale head/rollback record or appeared install release fails before writes; same release is serialized, two-operation cap holds. Exercise a late transport failure and cancellation of both RunWithContext and Run actions; assert lock/temp cleanup occurs only when worker terminates and failure/unknown never claims rollback.
- [ ] Run `go test ./internal/helmmanager -run TestHelmExecution -count=1`; establish failures.
- [ ] Implement prompt Execute dispatch to native workers, exact option mapping and no automatic retries/recovery. Revalidate native connection/head before starting, preserve token idempotence and serialize by release key. Respect API/SDK errors without app-specific role gates. Create namespace only for the explicitly reviewed install option.
- [ ] Apply full context/deadline plumbing to SDK storage, API clients, watches/waiters and hook waits, including rollback/uninstall Run. Do not let SDK background contexts outlive the operation. If SDK cancellation needs an adapter, implement the smallest adapter and test it, not a goroutine timeout that abandons work.
- [ ] Capture metadata-only results, bounded latest-observation refresh and real partial/unknown status. Polling/refresh is read-only. Retain active work across UI navigation; cancel on native disconnect/shutdown and preserve results for the session. No automatic pending-record deletion. Refresh inspection snapshots after any submitted operation, including failures that may have effects.
- [ ] Run `go test ./internal/helmmanager ./internal/helmreader` and `go test -race ./internal/helmmanager`; expect pass. Commit `feat: phase1e2-task3 execute reviewed Helm actions`.

## Task 4 — Connect the desktop bridge and source settings

**Files:** `internal/desktop/helm_management.go`, `internal/desktop/service.go`, generated bindings; `frontend/src/helm/managementApi.ts`, `frontend/src/settings/HelmSourcesSettings.tsx`, existing settings/preferences integration.

**Consumes:** tasks 1–3 native service methods. **Produces:** generated management API and source settings in current Settings screen.

- [ ] Register native manager construction/shutdown and connection cleanup without replacing reader/session cleanup. Expose cancellable resolution/preparation/read endpoints and immediate Execute/Cancel/status endpoints; do not put long execution under the 30-second workload read wrapper. Add one focused bridge test for native input validation and explicit connection routing.
- [ ] Implement Helm sources settings with named HTTPS/OCI sources, custom CA/client-cert selectors and session-only username/password/token controls. Clear password inputs after submission and never refill from native responses; source deletion/Forget clears credentials and revokes affected unresolved drafts. Show that session credentials expire on app exit.
- [ ] Generate bindings with `wails3 task bindings`; add a typed frontend API adapter following helm/api.ts request cancellation. Keep source metadata in existing preferences; no credential-bearing localStorage or debug output.
- [ ] Run `go test ./internal/desktop ./internal/preferences` and `npm run typecheck`; expect pass. Commit `feat: phase1e2-task4 connect Helm management settings`.

## Task 5 — Add actions to the existing Helm UI

**Files:** `frontend/src/helm/HelmWorkspace.tsx`, `ReleaseDetail.tsx`, `useHelm.ts`, new management state/dialog/results files, `frontend/src/App.tsx`, `frontend/src/helm/HelmManagement.test.tsx`; reuse YAMLDiff and current dialog/result styling.

**Consumes:** task 4 generated API. **Produces:** complete Prepare → Review → Confirm → Result journey and scoped refresh.

- [ ] Add one mounted `HelmManagement.test.tsx` workflow with small variants: edit upgrade values then review, failed prepare, double confirm, active-tab/namespace change with late response, and ongoing operation status after navigation. Assert one native execution, fixed labels and no stale confirmation. Include keyboard focus and error announcement assertions instead of snapshots for every component.
- [ ] Run `npm test --workspace frontend -- src/helm/HelmManagement.test.tsx`; establish failures.
- [ ] Add Install plus selected release Upgrade/Rollback/Uninstall actions. Implement source/version selection, values mapping/file import and reviewed settings. Historical selection stays inspection evidence; upgrade/uninstall identify current head. Default options exactly follow the spec; no arbitrary flag controls.
- [ ] Reuse current diff/editor controls for values/manifests and show hook/CRD/namespace impact with accurate dry-run labels. Fetch bounded review sections; disable confirmation during incomplete/failed preparation. One explicit action-specific confirmation suffices; no repeated Secret approvals or extra typed-name gate.
- [ ] Display running/terminal Helm results alongside existing operation results using shared visual conventions, with fixed target, cancel and Refresh release/history. Do not coerce Helm targets into kube.ResourceRef; keep a small Helm result component if adapting the generic type would require a broad rewrite. Confirm once before disconnecting/closing a tab with active Helm work. No automatic repeat execution on remount/reconnect.
- [ ] Run targeted Vitest, `npm run lint`, `npm run typecheck`; expect pass. Commit `feat: phase1e2-task5 add Helm management workspace actions`.

## Task 6 — Verify, document and deliver

**Files:** `tests/integration/helm_management_test.go`, existing `tests/fixtures/helm/`, test fixture README; `docs/superpowers/validation/phase-1e2.html` and index; checkpoint, verified plan boxes and affected capability documentation.

**Consumes:** complete product flow. **Produces:** actual evidence and reviewed merged implementation.

- [ ] Extend tiny fixtures with two chart versions, a benign hook, failing-hook variant, keep-policy resource and isolated CRD/custom resource. Provide development-machine HTTPS/OCI fixtures with synthetic auth and trusted test CA, not infrastructure required by the app. Use only explicit kind-dev-local-a/b and owned namespaces; record cleanup for cluster-scoped fixture CRDs too.
- [ ] Run one opt-in `TestHelmManagement` suite: local install, authenticated HTTPS upgrade, rollback, uninstall and authenticated OCI install; same-name isolation on both clusters; denied mutation, hook failure, cancellation and cleanup. Also exercise mutation of a Helm 3-produced release to establish actual compatibility with stored records. No claim of cloud-registry compatibility from only local fake passes.
- [ ] Run `KUBESIFT_HELM_INTEGRATION=1 go test -tags=integration ./tests/integration -run '^TestHelmManagement$' -count=1 -v`, `go test -race ./internal/helmmanager`, `wails3 task bindings`, `npm run build`, `scripts/check.sh` and `wails3 task package`. Expect pass; record real skips/blockers rather than substituting mock success.
- [ ] Agent-run packaged macOS walkthrough covers the four actions, private source Settings, precise context labels, hooks/CRD impact, cancellation and failure state, both themes, keyboard/focus, existing inspection and current-resource navigation. Verify no runtime Helm process or credentials in app files/logs and no background requests after native cleanup. No duplicate manual UAT gate.
- [ ] Write the validation report with exact SDK/client/server/fixture versions, supported storage and source coverage, commands and native screenshots, limitations and cleanup. Keep detailed operational logs in ignored `.superpowers/sdd/`. Update checkpoint and plan only from verified outcomes. Commit `test: phase1e2-task6 validate Helm management`.
- [ ] Apply the repository's independent whole-PR review once to the complete candidate, fix blocking findings and do targeted rechecks. Push a ready PR to develop and finish the authorized merge after checks/review pass. Report real blockers if any; no release, main merge or next-phase implementation.

## Self-review and implementation handoff

Coverage: sources/auth/bounds → task 1; preview/identity/content → task 2; all four SDK actions/concurrency/cancellation/results → task 3; desktop/preferences → task 4; current UI and navigation → task 5; live compatibility/desktop/review/merge → task 6. Each Review Focus risk has a named behavioral test. The spec owns exact limits and defaults; this plan does not create another status ledger.

Implement in a separate session using executing-plans, with the current engineering practices and minimal tests. Start from updated develop containing this plan, use the existing checkout, and preserve unrelated work. Phase 1E.3 current metrics remains next after this increment.
