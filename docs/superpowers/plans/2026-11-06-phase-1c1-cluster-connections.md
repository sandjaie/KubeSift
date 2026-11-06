# Phase 1C.1 Cluster Entry and Connections Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task in a separate implementation session. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace development-mode navigation with a real cluster-list entry, Settings-managed sources and truthful, isolated live cluster workspaces, then provide a user UI UAT checklist.

**Architecture:** Reuse the guarded connection/trust/session services. Add backend-owned source snapshots and an asynchronous connection coordinator; keep the desktop bridge thin and the frontend responsible for presentation. Product startup mounts neither synthetic previews nor the legacy validation controller.

**Tech Stack:** Existing pinned Go/client-go/Wails v3, React/TypeScript, CSS and Vitest. No framework upgrade, cloud SDK, login shell, new state library or UI test framework. Use a small local SVG resource-icon registry.

**Spec:** [Approved Phase 1C.1 specification](../specs/2026-11-06-phase-1c1-cluster-connections-design.md), approved 2026-11-06. User also requires a post-implementation HTML UI UAT checklist.

**Status:** Plan prepared; no implementation performed. Read the reviewed planning commit and actual branch before starting. The [roadmap](../../roadmap/roadmap.html#phase-1c) owns ordering, not this plan.

## Global constraints

- Follow [AGENTS.md](../../../AGENTS.md), [engineering](../../engineering/engineering.html), [security](../../security/security-model.html), [UX](../../ux/workflows.html#live-entry) and [Git workflow](../../engineering/gitworkflow.md). Every project-owned Go/TS/TSX file needs its plain-English source description.
- “Startup loads context summaries from configured sources locally. It does not open a cluster, execute a helper, probe every context or reconnect last-session tabs automatically.”
- “The user runs `oc login`, AWS SSO authentication or Azure authentication in their own terminal.” No app-managed login/token form/browser/device flow; approved noninteractive token retrieval helpers remain supported.
- “Only read state may resume after a successful reconnection.” No replay of commands/mutations or automatic restoration of stopped forwards.
- Only `kind-dev-local-a` and `kind-dev-local-b` for new local development/validation. Inspect existing resources and preserve both clusters; never use another context implicitly.
- Connection attempt deadline 30 seconds including helper acquisition; one check in flight per connection; routine checks at most every 30 seconds; evidence stale after 90 seconds; transport backoff capped at 60 seconds. Use cancellable timers and injected time in tests.
- Source setup/labels/trust live in Settings. Product cluster selector only selects existing contexts. No synthetic resource fallback, product validation screen or misleading green status.
- Full diagnostics, generic discovery/pagination, YAML/events/logs belong to 1C.2/1C.3. General operations/exec/forwarding remain in later increments. Preserve existing backend tests and developer capabilities without exposing them as product features.
- Work on a `feature/*` implementation branch. Operational logs/reviews belong in ignored `.superpowers/sdd/2026-11-06-phase-1c1-cluster-connections/`; concise tracked status is the portable checkpoint. No progress/plans in root README.

## Review focus

1. Refresh A while B is selected, equal context names and malformed source: isolate snapshots and keep B usable (Task 1 test).
2. Connection cancel/close/retry races: late work never resurrects a tab or replaces a new generation (Task 3 test and Task 5 state case).
3. HTTP 401 versus 403, TLS and timeout: do not report every failure as expired login; green requires actual resource-read evidence (Task 2 test).
4. External credential/endpoint changes and trust revocation: reread captured source, invalidate targets and never replay commands (Task 3 test and Task 6 walkthrough).
5. Preview controls surviving the refactor: packaged startup and keyboard paths must not load fixtures, hidden live validation or simulated actions (Task 5 manual check and Task 7 UAT).

## File map and integration decisions

Inspect existing code before moving files; update callers and descriptions together. Preserve `internal/session`, `internal/operation`, `frontend/src/bridge.ts` and `Terminal.tsx` lifecycle behavior unless integration reveals a specific necessary fix.

| Paths | Responsibility |
| --- | --- |
| `internal/sources/store.go`, `registry.go`, `registry_test.go` | Strict local source/label settings and independent bounded configuration snapshots |
| `internal/config/kubeconfig.go` | Existing safe parsing; expose safe metadata needed for source/credential identity without leaking config |
| `internal/kube/read.go`, `status.go`, `status_test.go` | Small namespace/probe reads and classified errors; keep existing Pod summary boundary |
| `internal/explorer/coordinator.go`, `coordinator_test.go` | Attempts, connection status, checks, Retry/reload and close/remove lifecycle; no Wails imports |
| `internal/desktop/explorer.go`, `service.go`, `app.go` | Typed ExplorerService bridge using the same auth/connection/kube/session instances as existing services |
| `frontend/src/connections/api.ts`, `model.ts`, `state.ts`, `state.test.ts`, `useConnections.ts`, `ClusterList.tsx`, `ConnectionStatus.tsx` | Bridge adapter, explicit tab/attempt state, cluster list and status display |
| `frontend/src/settings/SourcesSettings.tsx`, `TrustSettings.tsx`, `SettingsPage.tsx` | Real settings controls; remove validation links |
| `frontend/src/workspace/Workspace.tsx`, `ClusterTabs.tsx`, `ResourceTable.tsx`, `ResourceNavigation.tsx`, `ResourceDetail.tsx`, `commands.ts` | Reuse presentation for live summary data, honest unavailable actions and distinct resource icons |
| `frontend/src/ui/ResourceIcon.tsx`, `App.tsx`, `main.tsx`, `styles.css` | Compact product entry/header and icon registry |
| `frontend/src/dev/ValidationEntry.tsx`, `frontend/validation.html`, `frontend/vite.config.ts` | Explicit development-only validation entry; ordinary production bundle excludes it |
| `scripts/dev-clusters.sh`, `tests/fixtures/explorer/` | Idempotent small fixture expansion on the two approved clusters |
| `docs/superpowers/validation/phase-1c1.html`, `phase-1c1-ui-uat.html` | Separate agent technical report and user acceptance checklist, created after implementation |

**Preferences decision:** use a separate strict version-1 `sources.json` at `os.UserConfigDir()/kubesift/`, leaving the existing appearance/geometry `preferences.json` version 1 unchanged. Preserve appearance settings byte-for-byte during source operations. Source metadata is not kubeconfig; never write kubeconfig/current-context.

**Lifecycle decision:** the backend owns connection attempts and bounded polling/check state. The UI polls safe snapshots at 500 ms only while connecting and at 5 seconds while displaying open tabs; these RPCs do not themselves trigger API reads. Stop UI polling on unmount and let native attempt/check cancellation own cleanup. This avoids a new connection event stream and subscribe-before-start race. Backgrounded UI may lag, so compute age from timestamps and immediately refresh status on foreground return.

### Shared contract

Define source/catalog DTOs in `internal/sources/registry.go`, shared `Problem` in `internal/model/types.go`, and attempt/connection DTOs in `internal/explorer/coordinator.go` (or a focused adjacent `types.go` if needed). Use generated TypeScript types through `connections/api.ts`; do not maintain duplicate wire schemas. Dependencies point from explorer to sources/kube/connection, never back from sources to explorer.

- `SourceID` is an opaque persisted ID; the built-in group uses `default`. `SourceConfig{ID, Path string; Enabled bool}`: default Path is empty and resolves effective defaults; added sources are individual absolute paths. Reject duplicate canonical added-file paths. Use backend IDs, not user-controlled paths, in subsequent requests.
- `CatalogEntry{SourceID, SnapshotID, Context, Namespace, Endpoint, SourceDisplay, Environment string; LabelNeedsReview bool}`. Each `(SourceID, Context)` is one tab identity; sanitized endpoint excludes userinfo/query. No credentials in DTOs.
- `SourceResult{SourceID, SnapshotID string; Contexts []CatalogEntry; Problem *Problem}` allows per-source failures without discarding valid groups.
- `Problem{Code, Message string}` uses safe categories: `auth_required`, `forbidden`, `unreachable`, `certificate`, `timeout`, `trust_required`, `helper_missing`, `external_login_required`, `invalid_source`, `stale_target`, `capacity`, `cancelled`, `failed`. Do not forward raw stderr/HTTP bodies.
- `ConnectRequest{SourceID, SnapshotID, Context, Namespace string}`. Empty Namespace uses resolved kubeconfig namespace/default. It never denotes all namespaces.
- `Attempt{ID string}` issued by native code before its worker starts. `ConnectionView{AttemptID, SourceID, Context, Namespace, State string; Ref *model.ConnectionRef; Problem *Problem; LastSuccessAt string; Namespaces []string; NamespacesComplete bool; Pods []kube.PodSummary; PodProblem *Problem}`. UTC timestamps are metadata, not inferred frontend clock success.
- Public ExplorerService RPCs: `ListSources() ([]SourceResult,error)`, `ReloadSource(id string) (SourceResult,error)`, `AddSource(path string) (SourceResult,error)`, `SetSourceEnabled(id string, enabled bool, confirmed bool) error`, `RemoveSource(id string, confirmed bool) error`, `SetLabel(sourceID, contextName, label string) error`, `StartConnection(req ConnectRequest) (Attempt,error)`, `GetAttempt(id string) (ConnectionView,error)`, `CancelAttempt(id string) error`, `GetConnection(ref model.ConnectionRef) (ConnectionView,error)`, `Disconnect(ref model.ConnectionRef) error`, `ReadPods(ref model.ConnectionRef, namespace string) (ConnectionView,error)`.
- Helper RPCs on ExplorerService: `ReviewHelper(sourceID, snapshotID, contextName string) (trust.PreviewRecord,error)`, `ApproveHelper(sourceID, snapshotID, contextName, reviewID string) error`, `TrustedHelpers() ([]trust.Entry,error)`, `RevokeHelper(fingerprint string) error`. Delegate to the existing guard; do not create a second trust store. Review/approve never starts connection implicitly.

The desktop bridge delegates all of these to source/coordinator services and existing trust guard. Put one coordinator in the existing native composition root; do not create a second connection manager. Go domain constructor details are builder choices, with injectable probes, clock and source loading for tests.

## Task 1: Independent source registry and local configuration

**Files:** create `internal/sources/store.go`, `registry.go`, `registry_test.go`; modify `internal/config/kubeconfig.go` only for necessary safe metadata. Existing `internal/preferences` remains unchanged.

**Interfaces:** `sources.NewRegistry(path string) (*Registry,error)`, `List() []SourceResult`, `Reload(id string) (SourceResult,error)`, `Resolve(sourceID,snapshotID,contextName string) (config.Resolved,error)`; Add/enable/remove/label operations back the shared bridge contract. Use internal source DTOs or aliases to avoid sources importing explorer and creating a cycle; explorer consumes sources, never the reverse.

- [ ] Read approved spec, engineering/toolchain and current diff; create the implementation feature branch from the reviewed planning commit. Do not upgrade dependencies as part of setup.
- [ ] Write one focused `TestSourcesIsolateSnapshotsAndPersistMetadata` with subcases: two files share a context name; reloading A invalidates A's old snapshot but not B's; malformed A leaves B usable; missing/default source yields an honest result; duplicate canonical path is rejected; add/remove/label never changes kubeconfig or appearance settings. Include strict-store rejection/preserved-file assertions using temp paths. Run `go test ./internal/sources` and observe failure.
- [ ] Implement the default loading group and independent additional-file groups using existing Kubernetes parsing. Startup summary loading does not resolve helper executables or authenticate. Keep origin display safe; default group precedence follows the loader rather than merging all added files together.
- [ ] Implement strict source metadata persistence with atomic saves, 0600 file permissions, unknown/duplicate/null/trailing-field rejection and bounded 64 KiB input. Limit 32 groups, 2,000 displayed contexts per group and labels to 30 characters; show capacity errors rather than silently truncate or discard configured groups. Cache only the newest snapshot per group, while connections keep captured configs. These are planning limits, visibly reported and adjustable later, not Kubernetes API limits.
- [ ] Store labels against `(SourceID, Context)` plus a non-secret identity fingerprint for endpoint/CA/user configuration. On identity change mark review-needed and suppress the old label as an authoritative environment tag until user confirms it in Settings. No token values, raw helper args/env or resource data in this store.
- [ ] Run `go test ./internal/sources ./internal/config`; expect pass. Commit `feat: add isolated kubeconfig source registry`.

## Task 2: Truthful resource-read checks and classified failures

**Files:** create `internal/kube/status.go`, `status_test.go`; modify `internal/kube/read.go`, `internal/config/kubeconfig.go` as needed.

**Interfaces:** `CheckAccess(ctx context.Context, ref model.ConnectionRef, namespace string) (AccessResult,error)` on kube.Service. `AccessResult{Namespaces []string; NamespacesComplete bool; Namespace string; Pods []PodSummary; AuthorizedRead bool; PodProblem *model.Error}`. `ClassifyError(ctx context.Context, err error) *model.Error` preserves safe typed categories; `config.Resolved` exposes credential-mode metadata only to Go.

- [ ] Write `TestAccessEvidenceAndErrorCategories` with injected clients/transports: successful resource read establishes evidence; namespace-list 403 plus successful namespaced Pod read establishes evidence; both forbidden do not; 401, 403, unknown CA, timeout and cancellation remain distinct. Include a successful namespace read with failed Pod list: connection evidence survives but Pod view retains its error. Observe failure with `go test ./internal/kube -run TestAccessEvidenceAndErrorCategories`.
- [ ] Correct the existing Unauthorized→forbidden mapping. Classify typed status/TLS/network/context errors without string-matching arbitrary server messages. Unknown failures stay failed; missing/noninteractive helper requirements keep their own category. Preserve existing cancellation/stream behavior when sharing this mapper, and update callers/tests only where semantics actually change.
- [ ] Check namespace access with a bounded list page (100 names); a continuation means NamespacesComplete=false with manual namespace entry available, not a complete namespace inventory. If denied, still try the resolved/manual namespace. Use the existing capped Pod reader; an over-capacity list is a Pod-view capacity result, not an authentication failure. Never treat public `/version` or discovery as proof of credential validity.
- [ ] Explicitly reject anonymous configuration for this increment with a safe unsupported/failed explanation; do not set green on anonymous success. Approved helpers and token/certificate flows still use the guarded transport. No mutation, privileged introspection or trial permission writes.
- [ ] Run `go test ./internal/kube ./internal/config` and `go vet ./internal/kube`; expect pass. Commit `feat: classify cluster access and connection evidence`.

## Task 3: Cancellable connection coordinator and Settings bridge

**Files:** create `internal/explorer/coordinator.go`, `coordinator_test.go`, `internal/desktop/explorer.go`; modify `internal/desktop/service.go`, `app.go`; small fixes in `internal/connection/manager.go`, `internal/trust/guard.go` and `internal/model/types.go` only where the new lifecycle or typed errors require them.

**Interfaces:** shared contract above. `explorer.NewCoordinator(...)` owns async workers/status and exposes the corresponding bridge methods; `Close()` shuts all its workers down. Native source mutation and connection operations share coordination so remove/disable cannot race a new connection. Product source-loading methods use the registry; do not wire them through the legacy single-snapshot LoadContexts/resolve path.

- [ ] Write `TestConnectionLifecycle` with a controllable fake probe/clock and source loader: Start registers an attempt before returning; cancel before/during probe never yields connected; delayed result after Retry/Disconnect cannot replace a new reference; refreshed source invalidates only its old pending attempts; revocation closes affected product views; removing source requires confirmation and cancels owned work; terminal auth/trust failure stops retries. Use explicit synchronization and fake time, not sleeps or a real cluster. Observe failure with `go test ./internal/explorer`.
- [ ] Implement native-issued attempt IDs, max 16 open/connecting contexts, one active attempt per `(source,context)`, and a 64-entry completed-attempt cache with five-minute expiry. Capacity returns a visible error; never evict active connections to make room. An existing open selection is focused by UI, not automatically reopened.
- [ ] Make StartConnection resolve the expected snapshot, reload that same source before open, and compare its captured safety identity. If it changed, return a fresh-review/stale-target outcome rather than authenticate stale data. Retry obtains a freshly loaded snapshot and starts a replacement attempt. Close/invalidate the old generation first; retain only stale display data while replacing. Do not use unrelated contexts as fallback.
- [ ] Hold a 30-second request deadline for the initial check including helper execution; use the coordinator's longer-lived owned connection context for the established connection so expiration of the attempt context cannot kill a successful connection. Mark green only from Task 2 evidence. Maintain 30-second checks, 90-second stale aging and transport backoff 30→60 seconds with no overlapping requests. Use a previously successful resource-read path; manual namespace selection updates only that connection's probe target. Polling native snapshots triggers no extra Kubernetes request.
- [ ] Gate helpers through the existing guard. Approval does not run a helper; an explicit Connect follows. Revocation uses the current close-all policy, cancels coordinator attempts/checks and updates every affected tab. Do not revive native client-go exec, interactive stdin, a browser login or the old single-snapshot approval path for product requests.
- [ ] Add source removal/disable confirmation enforcement in the backend: return the affected context list/safe conflict when confirmed=false; confirmed=true cancels attempts and closes connections, then updates the store. Persistence failure must be visible; never report successful source removal when the file save failed. A refreshed source change marks existing view stale and invalidates old work rather than silently retargeting. Unchanged reload leaves unrelated live connections usable.
- [ ] Register ExplorerService over the shared native core. Keep legacy service methods for dev harness/tests until callers move; product code uses the new API. Regenerate bindings with `wails3 task bindings`. Run `go test ./...`, `go test -race ./internal/explorer ./internal/connection ./internal/sources`, `go vet ./...`; expect pass. Commit `feat: coordinate explicit cluster connection lifecycle`.

## Task 4: Real source/label/trust settings and context list

**Files:** create `frontend/src/connections/api.ts`, `model.ts`, `state.ts`, `state.test.ts`, `useConnections.ts`, `ClusterList.tsx`, `ConnectionStatus.tsx`, `settings/SourcesSettings.tsx`, `TrustSettings.tsx`; modify `settings/SettingsPage.tsx`.

**Interfaces:** api.ts wraps generated ExplorerService DTOs; `useConnections()` exposes `{state, refreshSources, connect, retry, cancel, disconnect, readPods}` with source/attempt/ref arguments. `reduceConnections(state:ConnectionsState, action:ConnectionsAction):ConnectionsState` rejects late IDs/generations. `ClusterList({sources,connections,onSelect,onSettings})`, `ConnectionStatus({view,now})`; Settings consumes explicit service callbacks rather than a validation-screen link.

- [ ] Write one focused frontend case `isolates connection attempts and namespaces` in `connections/state.test.ts`: equal-named A/B contexts remain separate, a cancelled attempt response is ignored, a namespace change rejects the old namespace's result, and stale timestamps cannot render green. Mock only the adapter seam. Run `npm exec --workspace frontend -- vitest run src/connections/state.test.ts`; observe failure.
- [ ] Implement state keyed by source/context, backend attempt ID and reference generation. Keep request epochs for namespace reads separate from tab selection. A response can update its own inactive tab, never whichever tab happens to be active. Poll active native attempt/state snapshots with one coordinator hook; stop timers on unmount, foreground-refresh immediately and surface polling failure as unknown/stale rather than green indefinitely.
- [ ] Build the searchable cluster list with stable name/source ordering, namespace/label/state and safe source/endpoint detail. Selecting an already-open context focuses it. Startup loads summaries only; zero contexts and a broken source get distinct UI. Configuration actions link to Settings rather than inline add-cluster forms.
- [ ] Build Settings source add (explicit path is sufficient), reload, disable/remove and default-group restore. Show exact affected tabs before closing them; Cancel does nothing. Invalid settings/source errors remain visible without damaging other groups. Existing appearance controls remain intact.
- [ ] Move helper approvals/revocation to Settings using the existing redacted preview fields. Connect-required review from a tab navigates to the relevant Settings review and back; approval leaves an explicit Connect action. Explain close-all revocation before confirm. Add user environment-label editing and identity-change review using Task 1 metadata.
- [ ] Show gray Not connected, amber Connecting/Review required/Access restricted/Stale and red classified failures with text, not color alone. Show green only from native successful resource-read evidence within age. External-login failures explain “Refresh credentials in your terminal, then Retry”; no fabricated expiry claim, login command execution or token box.
- [ ] Run the focused state test, `npm run lint`, `npm run typecheck`; expect pass. Commit `feat: add cluster selection and connection settings`.

## Task 5: Integrate live workspace and retire product development modes

**Files:** modify `frontend/src/App.tsx`, `main.tsx`, `styles.css`, workspace components/model/commands and `ai/InvestigationPanel.tsx`; create `ui/ResourceIcon.tsx`, `dev/ValidationEntry.tsx`, `frontend/validation.html`; modify `frontend/vite.config.ts` and dev-only imports as necessary.

**Interfaces:** App consumes `useConnections`; workspace receives an explicit ConnectionView and PodSummary rows. Use a small presentation adapter to reuse table/basic detail components, not fabricated `yaml/events/logs` fields. `ResourceIcon({kind,size?})` uses one local SVG registry. Normal product startup does not import the legacy validation controller/profile or fixture factory.

- [ ] Extend the single connection state test for same-named resources across A/B, close-tab response rejection and preserving independent namespace/filter/selection state through Settings. Run targeted test and observe the new failing assertions before code.
- [ ] Replace welcome/preview/validation routing with cluster list, open workspace tabs and Settings. Use the compact two-row header, existing-context selector and meaningful icons. Remove phase copy, synthetic state selectors, mode banners, hidden mounted validation, inline label editing and live-workspace buttons. Keep context/namespace and statuses readable with overflow on the approved sizes.
- [ ] Connect namespace selection to native discovered names or explicit manual entry when list is denied/incomplete. For 1C.1 support one explicit namespace at a time; show all-namespaces/cluster-resource browsing as unavailable rather than mapping it to an empty string. Show real basic Pods, honest empty/forbidden/capacity/disconnected states and summary fields only. On namespace change cancel/ignore older reads; on connection failure show retained data as stale, never current.
- [ ] Keep general resource navigation coherent but later kinds/details/actions visibly unavailable. Remove simulated mutations/logs/terminal/forward commands from the product registry and keyboard paths; no hidden shortcut may invoke them. Keep the real Sessions dock empty or closed with an honest empty state until a supported product session exists; do not show inert sample sessions.
- [ ] Keep Ask AI/Hide AI available from list, workspace and Settings with the right-side availability explanation only. No sample diagnosis, evidence payload or provider configuration pretends to operate on a connected cluster. Preserve themes/layout state and command-palette input guards.
- [ ] Isolate existing validation/preview fixtures in `validation.html` and `dev/ValidationEntry.tsx`, served only in Vite development. Production build inputs remain only `index.html`; guard/exclude the dev route from packaged assets. Normal App never calls ValidationConfiguration or profile autorun; retain Ping/FrontendReady as appropriate without a routine readiness badge. Do not remove safety services/tests merely because the old UI is no longer shipped.
- [ ] Run `npm test`, lint, typecheck and production build. Inspect production entry/dependency output to confirm the dev entry and fixture imports are excluded. Manually check keyboard input/focus, closing all tabs, Settings return and no obsolete buttons/copy in normal packaged navigation. Commit `feat: connect product workspace and remove validation navigation`.

## Task 6: Populate safe local fixtures and validate two-cluster behavior

**Files:** modify `scripts/dev-clusters.sh`; create focused manifests under `tests/fixtures/explorer/`; extend one opt-in integration scenario in `tests/integration/explorer_test.go` (or existing integration suite after checking its owner).

**Interfaces:** fixture labels identify KubeSift explorer ownership and A/B markers. Setup has an explicit fixture-expansion command/flag, uses only the two allowed contexts and never changes/deletes their existing resources without collision checks.

- [ ] Inspect both named local contexts and existing namespace/object inventories using explicit context flags. Plan additions under dedicated owned namespaces (prefer `kubesift-explorer`) to preserve existing shop/billing samples. Confirm any name collision/ownership rather than adopting objects. Do not run the historical 1A cluster-creation tool.
- [ ] Implement idempotent fixture expansion: healthy bounded-log service workload; missing-setting crashing Pod; missing-image Pod; multi-container Pod; completed/failed Job and suspended CronJob; small StatefulSet/DaemonSet; Services/Ingress objects; ConfigMaps/fake Secrets; PVC with honest Pending if unsupported; ServiceAccount/Role/RoleBinding/Quota/LimitRange/NetworkPolicy/PDB; one synthetic namespaced CRD/custom resource. Use approved pinned images or record selected digests; no metrics/operator/storage/ingress stack. Same-named A/B Pods have distinct markers. Small replicas/requests and bounded logs; no load flood here.
- [ ] Add one opt-in scenario exercising source→connect A/B→namespaces→basic Pod reads→close, with no implicit current-context access. Restricted-permission evidence uses a dedicated least-privilege local test identity and read requests; never test permissions by trial mutation. Keep kubeconfigs/credentials in ignored local storage; report synthetic summaries only. The broader inventory prepares later phases and is not a claim of new UI coverage.
- [ ] Run the new integration scenario only after explicit fixture setup, and record actual outcomes. Exercise external-renewal/retry via synthetic helper/config replacement in the automated contract tests or a controlled fixture harness, not invented EKS/AKS/OpenShift compatibility. Do not stop/delete either shared cluster for a connectivity test; use synthetic transport failures or isolated unreachable test source.
- [ ] Run affected shell/fixture checks and `go test ./...`; retain opt-in guard behavior. Commit `test: add two-cluster explorer fixtures and connection validation`.

## Task 7: Packaged verification, technical report and user UAT checklist

**Files:** create `docs/superpowers/validation/phase-1c1.html` and `phase-1c1-ui-uat.html`; update docs index/roadmap/status and local-development inventory to actual results. Curated screenshots under `docs/superpowers/evidence/phase-1c1/` with a brief manifest only when useful.

**Deliverables:** agent technical results and separate, initially untested user UI checklist. Checklist is written after implementation using actual controls/navigation, not hypothetical labels.

- [ ] Run `scripts/check.sh`, relevant coordinator/source race checks if changed since Task 3, and `wails3 task package`. Launch the resulting app from Finder. Verify normal startup does not execute helpers or contact a cluster until selected. Record actual artifact/host and commands; do not call a Vite browser preview packaged validation.
- [ ] Walk spec C11–C18 through the packaged app using only the two local clusters. Check tab/namespace identity, actual Pod rows, canceled actions, Settings source operations, trust/retry outcomes, keyboard/focus, both themes, existing laptop/wide sizes and AI availability toggle. Verify cleanup/timers on close/app exit. Mark scenarios unavailable to the agent unrun; do not broaden into exhaustive monitor/OS or provider tests. Carry forward backgrounded WebKit log loss and other existing qualifications.
- [ ] Perform correctness and code-hygiene review per engineering practice, fix material findings and rerun only affected checks. Retain short operational logs/reviews in ignored SDD; durable report records outcomes and limitations. No phase pass or user visual approval is inferred from technical checks.
- [ ] Create `phase-1c1.html` with C11–C18 results, real/synthetic test distinctions, artifact/check evidence, screenshots and known limits. Create a separate linked `phase-1c1-ui-uat.html` using the shared documentation styling, readable in a browser and printable.
- [ ] Give each UAT row a stable ID, prerequisites, numbered actions, expected visible result, result selector (`Not tested` default / `Pass` / `Fail` / `Blocked`) and notes area. Include tester/date/app commit/OS fields. Use an HTML form with print styles and a Print/save-as-PDF control; do not imply form values persist after reload. Clearly tell the user to print/save the completed checklist before closing. This avoids a new results-storage system; any added persistence is optional, local-only and must not record credentials.
- [ ] Populate approximately 12–15 short UAT cases covering the matrix below. Supply exact local context names, actual fixture namespace/Pod discovery instructions and actual menu/control labels. Core everyday cases are executable with healthy local clusters; auth/error cases use safe isolated test fixtures or are marked optional/Blocked if prerequisites are unavailable. Do not ask the user to break their normal kubeconfig, revoke real cloud credentials, delete shared resources or paste tokens into the checklist.
- [ ] Keep every user result `Not tested`. Agent validation passes belong only in the technical report. User will perform UAT after implementation; leave the approval gate pending and do not use standing phase-merge permission to merge before that requested acceptance is recorded.
- [ ] Run `python3 scripts/check-docs.py`, `python3 scripts/source-descriptions.py check`, `git diff --check`; include the newly created report/checklist in link verification. Commit `docs: publish Phase 1C.1 validation and UI UAT checklist`, push feature branch and open the PR to develop with actual checks/review limits. Do not merge pending user UAT, release or begin 1C.2. Return inline handoff with branch/commit/PR, launch instructions, report and checklist links, known limitations and exactly what the user should test next.

### UAT coverage required in the final checklist

| User journey | Expected coverage |
| --- | --- |
| Start and choose | No marketing/preview/validation navigation; searchable kubeconfig contexts; no auto-connect |
| A and B | Connect both, status text/dots, focus existing tab without duplicates, same-named Pod isolation |
| Namespace and Pods | Discovered/manual namespace, actual basic Pod data, empty/filter/error/capacity behavior honestly presented |
| Settings sources | Add valid/invalid isolated source, reload, duplicate context distinction, cancel/confirm source removal without deleting kubeconfig |
| Labels/appearance | User labels in Settings; System/Light/Dark and existing appearance saved across restart; no guessed environment |
| Trust and renewal | Review then explicit connect; changed helper/revoke impact; external renewal and Retry using safe fixture if available |
| Failure indicators | Auth versus permission versus network/TLS; old green becomes stale; red explains observed cause |
| Navigation/layout | Compact header, icons, keyboard focus/input guards, laptop/wide usability, Settings return |
| AI and exclusions | Right panel toggles; honest future availability; no fake diagnosis or simulated live actions |
| Cleanup | Cancel connect, close tab/all tabs, disconnect and reopen; no orphan activity or hidden validation workspace |

## Minimal test budget and self-review

Keep existing suites. New tests are a small source-registry/store case, a compact table of access/error outcomes, one coordinator lifecycle case with fake time, one frontend state case and one opt-in two-cluster integration scenario. Subcases should be readable; do not inflate a single test into an unrelated mega-test merely to count fewer tests. Add a targeted regression only for an observed defect or a material uncovered boundary. No snapshots, coverage threshold or native UI automation framework. The checklist is user acceptance, not an excuse to omit target/cancellation/credential checks.

Self-review completed against the approved spec: sources/Settings → Tasks 1/4; statuses and credentials → Tasks 2/3; cluster entry/UI/removal of dev modes → Tasks 4/5; fixtures → Task 6; C11–C18 and new user UAT requirement → Task 7. Existing preferences are preserved via a separate store, not an unnecessary schema migration. The source/coordinator dependency direction prevents a Go package cycle. No implementation occurs in this planning session.

## Separate-session implementation prompt

> Implement Phase 1C.1 only using this plan and its approved specification. Read AGENTS.md, engineering and Git workflow; ensure the reviewed planning commit from develop is in your feature branch. Use Superpowers executing-plans task by task. Replace product development modes with the real cluster-list flow, Settings-only setup, external-login/Retry and truthful connection states. Use only kind-dev-local-a and kind-dev-local-b, preserving existing resources and explicit targets. Keep tests minimal per the plan and add plain-English source descriptions. Create the technical HTML report and the separate post-implementation HTML UI UAT checklist with all user results Not tested. Commit/push and create the phase PR; leave it unmerged pending user UAT. Do not start 1C.2 or release. Finish with inline launch/UAT instructions, branch/commit/PR, report/checklist links and actual limitations.
