# Phase 1A Desktop Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans in the separate implementation session, or superpowers:subagent-driven-development if explicitly selected. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and validate the smallest packaged desktop integration that proves safe multi-cluster operation, credential-helper trust and concurrent Kubernetes sessions.

**Architecture:** A thin Wails adapter exposes framework-independent Go services to a minimal React interface. All work carries immutable connection/generation and session identities; an existing local cluster and a disposable second cluster establish real integration behavior. This is a validation application, not the full explorer.

**Tech Stack:** Wails v3, Go, React/TypeScript, Vite, client-go, kind, Vitest, ESLint, Husky/lint-staged; xterm.js for the container terminal. Pin exact compatible versions in Task 1, before application implementation.

**Spec:** [Approved Phase 1A specification](../specs/2026-11-05-phase-1a-validation-design.md).

**Status:** Ready for plan review. The specification was approved on 2026-11-05; this planning chat does not implement the plan.

## Global Constraints

- “This is a narrow integration milestone, not the full explorer or a polished product shell.”
- “No monitoring agents or supporting components are installed by the application.”
- “Use only local fixtures for mutation/fault injection.”
- “No handler resolves its target from the currently selected UI tab.”
- “Cancellation must reach Go and Kubernetes network work, not merely unsubscribe React listeners.”
- “Never retry an uncertain write automatically.”
- Initial validation is macOS arm64; this does not establish a minimum supported macOS or other-platform readiness.
- Preserve the repository's HTML docs, approved specification and Apache-2.0 license. Follow [engineering practice](../../engineering/engineering.html) and [Git workflow](../../engineering/gitworkflow.md).
- Tests are deliberately minimal per the user's instruction: representative safety tests, one opt-in integration scenario and a recorded manual checklist. No coverage threshold, exhaustive UI suite or test per getter/configuration value.
- Existing approved acceptance outcomes still apply. Manual checks may cover behavior that does not warrant an additional automated test.

## Review Focus

1. Kubeconfig extension fields and same-named contexts from different sources must not break loading or alias different connections — Task 2 representative test.
2. PATH/configuration changes or trust revocation must prevent an unapproved helper execution, including a later credential refresh — Task 3 representative test.
3. Events arriving after tab close or for another generation must not appear in the active tab — Task 4 representative frontend test.
4. Cancellation during stream setup or log flooding must not leave listeners/workers accumulating — Task 4 representative lifecycle test and Task 7 soak.
5. A resource recreated under the same name, concurrent edit, duplicate confirmation or uncertain response must not cause a wrong or repeated write — Task 6 representative operation test.

## Execution boundaries and versions

No runtime installation, app scaffolding, cluster creation or code is performed by creating this document. Execution begins in a separate chat after review.

Use a `feature/phase-1a-validation` branch from `develop`. At the first implementation start, create `develop` from the latest reviewed planning commit if absent; never reset an existing integration branch. Follow the Git workflow's bootstrap exception for planning-only commits. Do not push, change rulesets, merge, tag or release unless authorized in the implementation session.

Version selection is a bounded preflight decision, not an invitation to use floating dependencies: start with the official Wails v3 stable release and its required Go toolchain; use a supported Node LTS compatible with Vite, matching Kubernetes `client-go`/API/apimachinery versions, and a stable kind release with its published node image digest. Resolve exact versions/checksums once in Task 1, record them in `docs/validation/toolchain.html`, and commit `go.sum` and `package-lock.json`. Do not copy old alpha examples into a v3 stable project. If a required stable release cannot be verified, stop and report the discrepancy before scaffolding.

Sources for preflight: [Wails FAQ](https://wails.io/faq/), [Wails CLI](https://v3.wails.io/guides/cli), [kind quick start](https://kind.sigs.k8s.io/docs/user/quick-start/), [client-go compatibility](https://github.com/kubernetes/client-go#compatibility-matrix), [kubeconfig API](https://kubernetes.io/docs/reference/config-api/kubeconfig.v1/). Verify exact APIs against the pinned releases at execution time.

## File map

These files are future deliverables, not files created by this planning change. Generated Wails support files follow the selected template; retain only what the desktop build requires.

| Files | Responsibility |
| --- | --- |
| `main.go`, `go.mod`, `go.sum`, `Taskfile.yml`, `build/` | Desktop entry point and pinned build |
| `package.json`, `package-lock.json`, `frontend/package.json`, `frontend/vite.config.ts`, `frontend/tsconfig.json`, `frontend/eslint.config.js` | npm workspace, frontend build/test/lint/typecheck |
| `frontend/index.html`, `frontend/src/main.tsx`, `frontend/src/App.tsx`, `frontend/src/styles.css` | Minimal validation UI |
| `internal/model/types.go` | Shared domain identifiers and event/error shapes |
| `internal/config/kubeconfig.go`, `internal/config/kubeconfig_test.go` | Loading and safe context previews |
| `internal/trust/store.go`, `internal/trust/guard.go`, `internal/trust/guard_test.go` | Persistent approvals and execution-boundary checks |
| `internal/connection/manager.go` | Resolved connection identity and client lifecycle |
| `internal/session/manager.go`, `internal/session/buffer.go`, `internal/session/manager_test.go` | Cancellation, bounded delivery and session ownership |
| `internal/kube/read.go`, `internal/kube/streams.go`, `internal/kube/terminal.go`, `internal/kube/forward.go` | Kubernetes operations through explicit connections |
| `internal/operation/configmap.go`, `internal/operation/configmap_test.go` | Prepared, single-use validation mutation |
| `internal/desktop/service.go`, `internal/desktop/app.go` | Wails translation, event bridge and lifecycle wiring |
| `frontend/src/bridge.ts`, `frontend/src/state.ts`, `frontend/src/state.test.ts`, `frontend/src/Terminal.tsx` | Typed bridge, tab/session routing and terminal rendering |
| `tests/integration/validation_test.go`, `tests/fixtures/phase1a.yaml`, `tests/fixtures/helper/main.go` | One local integration scenario, synthetic workloads and exec helper |
| `scripts/local-clusters.sh`, `scripts/check.sh`, `scripts/check-docs.py` | Explicit test-cluster lifecycle and repeatable checks |
| `.gitignore`, `.husky/pre-commit`, `.husky/pre-push`, `.github/workflows/check.yml`, `.github/pull_request_template.md` | Hygiene, hooks and checks |
| `.cursor/rules/ponytail-lite.mdc`, `.agents/skills/{kubesift-code-review,kubesift-code-hygiene,release-this}/SKILL.md` | Repo-specific engineering handoff |
| `docs/validation/toolchain.html`, `docs/validation/phase-1a.html`, `docs/validation/measurements.json` | Actual environment, results and measurements |

Add a concise `README.html` in each handwritten source directory per the engineering playbook (root, frontend/src, internal and its packages, tests/integration, tests/fixtures/helper, scripts). Do not duplicate module maps in generated bindings or dependencies. No README is required for an empty placeholder directory; do not create speculative directories.

Approved visual guidance is in the [UX specification](../../ux/workflows.html). Keep this milestone minimal; full adaptive explorer/theme implementation belongs in 1B and AI in Phase 2.

## Shared contract

Task 2 creates `internal/model/types.go`. Later tasks consume these types rather than independently inventing equivalent shapes. Use JSON tags with the camelCase field names below in the bridge. Generate frontend bindings from the actual Go service.

```go
type ConnectionRef struct { ID string; Generation uint64 }
type Target struct {
    Connection ConnectionRef
    Namespace, Name, Container string
}
type SessionRef struct { ID string; Connection ConnectionRef }
type Event struct {
    Session SessionRef
    Sequence uint64
    Kind string // snapshot, update, data, truncated, state, error
    Data []byte // base64 on JSON boundary; terminal decoding preserves bytes
}
```

Use a small typed error with `Code` and a safe `Message`: `invalid`, `forbidden`, `disconnected`, `trust_required`, `stale_target`, `expired`, `consumed`, `uncertain`, `cancelled`. No raw kubeconfig, auth header or helper output crosses to React. Ordinary Kubernetes details needed for diagnosis can be mapped to safe structured fields.

## Task 1: Reproducible desktop shell and engineering checks

**Files:** create root/build/frontend scaffolding, hook/check/CI/template/skill files and `docs/validation/toolchain.html` from the map. Modify engineering docs only if actual generated paths differ.

**Interfaces:** produce `internal/desktop.Service.Ping() string` returning `ready`; root npm workspace scripts `test`, `lint`, `typecheck`, `build`; `scripts/check.sh` for Go and frontend gates. The service is registered by `internal/desktop/app.go`; `main.go` only starts it.

- [ ] Verify clean branch/base, host architecture, developer tools and container runtime availability. Record exact pins in `toolchain.html`; install prerequisites only as part of this explicitly authorized execution. A missing running runtime blocks cluster tests, not Go/TypeScript unit work.
- [ ] Generate the pinned Wails React/TypeScript template in a temporary directory and copy required files into the repo without overwriting docs/license. Set Go module to `github.com/sandjaie/KubeSift`; use root npm workspace with `frontend`. Validate with `go mod tidy` and `npm install`, then preserve lockfiles. Do not add a separate sidecar/server.
- [ ] Add `Ping`, render its result and verify a production `.app` launched from Finder shows `ready`. Generated bootstrap wiring needs a build/smoke check, not an artificial unit test. Wails commands: `wails3 doctor`, `wails3 build`, `wails3 package`; record exact artifact path from the pinned template.
- [ ] Configure strict TypeScript, Vitest, ESLint/typescript-eslint recommended, no Prettier. Root scripts delegate to the frontend workspace. Before Task 4 adds tests, use Vitest's explicit pass-with-no-tests mode; remove it when the state test exists.
- [ ] Configure Husky at repo root: staged TS/TSX → frontend-local ESLint `--fix` with paths preserved; staged Go → `gofmt -w`. Pre-push runs `go test ./...` and root `npm test`; integration tests are opt-in. `scripts/check.sh` additionally runs `go vet ./...`, `npm run lint`, `npm run typecheck`, and checks Go formatting without rewriting it. CI repeats these gates with pinned toolchains. Validate staged-file handling using a temporary test file, then remove it; no test suite for hook configuration.
- [ ] Add PR template with summary, test plan, actual commands/not-run checks and `KSF-001` findings. Write small repo review/hygiene skills and the release procedure from the playbook using the skill-authoring workflow; they are procedures, not authorization to release. Add the ponytail-lite rule. Do not configure remote rulesets in this task.
- [ ] Add `.gitignore` entries for dependencies, generated build outputs, `.local/` test state and platform artifacts; preserve intended tracked generated sources per the Wails template. Document `npm ci`, `scripts/check.sh` and the packaged smoke procedure.
- [ ] Run `scripts/check.sh` and packaged Ping check. Expected: all applicable commands exit 0 and Finder displays `ready`. Commit: `chore: establish desktop validation shell and checks`.

## Task 2: Explicit kubeconfig and connection identity

**Files:** create `internal/model/types.go`, `internal/config/kubeconfig.go`, its test, `internal/connection/manager.go`; update desktop service.

**Interfaces:**
- `config.Load(paths []string) (*config.Snapshot, error)`; empty paths use standard Kubernetes defaults. Snapshot retains resolved client settings privately. `Resolved` holds a source identity, context name, immutable REST configuration and helper configuration; expose no credential-bearing fields as Wails bindings.
- `(*Snapshot).Contexts() []config.ContextSummary` contains source/name/namespace and safe endpoint display, no credentials.
- `(*Snapshot).Resolve(name string) (config.Resolved, error)` validates missing references without executing helpers.
- `connection.NewManager(gate connection.AuthGate) *connection.Manager`.
- `(*Manager).Open(ctx context.Context, resolved config.Resolved) (model.ConnectionRef, error)`.
- `(*Manager).Client(ref model.ConnectionRef) (kubernetes.Interface, error)` and `RESTConfig(ref model.ConnectionRef) (*rest.Config, error)` for adapters; both reject stale references and install auth gating.
- `(*Manager).Close(ref model.ConnectionRef) error`; `CloseAll()` cancels owned sessions through `SetCleanup(func(model.ConnectionRef))`, supplied in Task 4.
- `AuthGate` exposes `Check(resolved config.Resolved) error`; implement deny-all-exec initially until Task 3. Certificate/token configurations still work.

- [ ] Write one table-driven `TestLoadAndConnectionIdentity` using temporary synthetic kubeconfigs: extensions accepted; missing context rejected; different snapshots with the same context name create distinct connection IDs; a generation change rejects the old ref. Assert input bytes and current-context are unchanged. No network access in this test.
- [ ] Run `go test ./internal/config -run TestLoadAndConnectionIdentity -v`; expected red until implementation exists.
- [ ] Implement loading via client-go loading rules, retain source identity and config snapshots, and assign opaque connection IDs. Changes to endpoint/CA/user/configuration create a new generation and invalidate pending work. Do not persist raw resolved credentials or allow default client-go exec to bypass `AuthGate`.
- [ ] Expose explicit open/select behavior; startup merely lists safe local configuration summaries and does not contact clusters or invoke credential helpers.
- [ ] Run the focused test then `scripts/check.sh`. Expected green and no file mutation. Commit: `feat: resolve explicit cluster connections`.

## Task 3: Reviewed and revocable credential-helper trust

**Files:** create `internal/trust/store.go`, `guard.go`, `guard_test.go`, `tests/fixtures/helper/main.go`; update connection/auth integration and desktop service.

**Interfaces:**
- `trust.Preview(resolved config.Resolved) (trust.PreviewRecord, error)` resolves executable to an absolute path, builds opaque fingerprint and redacted display metadata; never invokes the executable. `PreviewRecord` includes ID, fingerprint, source/context, resolved executable and redacted args/env; `Entry` contains fingerprint, safe display label and approval time.
- `trust.OpenStore(path string) (*trust.Store, error)` uses a strict versioned record format and atomic owner-readable storage.
- `(*Store).Approve(fingerprint string) error`, `Revoke(fingerprint string) error`, `List() []trust.Entry`.
- `trust.NewGuard(store *Store) *trust.Guard` implements `connection.AuthGate.Check(config.Resolved) error`.
- Desktop approval accepts only a backend-issued preview ID, rechecks it against current configuration and then approves its fingerprint; never approve arbitrary frontend-supplied executable text.

- [ ] Write one `TestHelperTrustLifecycle` with a fake clock and injected runner counter: preview/start untrusted → calls 0; approve unchanged configuration → calls 1; changed path/args/env or revoked trust on a subsequent refresh → calls unchanged and `trust_required`. Reload store to verify approval persistence. Corrupt/unknown-version records fail closed.
- [ ] Run `go test ./internal/trust -run TestHelperTrustLifecycle -v`; expected red before guard/store implementation.
- [ ] Fingerprint resolved executable path, ordered args, normalized configured env overrides and supported exec configuration fields. Use protected fingerprints rather than storing secret input values in the trust file. Display redacted values. Kubeconfig extensions remain compatible; application-owned trust records reject unknown keys.
- [ ] Integrate the guard at the actual credential-execution boundary, including client-go refresh and direct streaming transports. Freeze the resolved executable path in each connection snapshot and revalidate trust before each request/stream start; verify wrapper ordering so native exec cannot run ahead of the guard. Close/invalidate affected clients on revocation/configuration changes. This is not a one-time `Open` check. If the pinned client cannot enforce this boundary, stop and revise the adapter rather than ship an unguarded fallback.
- [ ] Synthetic helper reads a short-lived local fixture credential from an ignored path and emits valid ExecCredential JSON; add modes for failure/expired result and an invocation counter. Never log credential content; don't supply it through command-line arguments. Manual checklist exercises missing executable and error rendering.
- [ ] Run the focused test and `scripts/check.sh`. Expected green; no execution on preview/revocation. Commit: `feat: gate credential helpers with persistent trust`.

## Task 4: Bounded session ownership and tab routing

**Files:** create session manager/buffer/test, `frontend/src/bridge.ts`, `state.ts`, `state.test.ts`; modify App and desktop event bridge.

**Interfaces:**
- `session.NewManager(emit func(model.Event)) *session.Manager`.
- `(*Manager).Start(parent context.Context, target model.Target, kind string, run func(context.Context, *session.Writer) error) (model.SessionRef, error)`.
- `(*Manager).Stop(ref model.SessionRef) error`, `StopConnection(ref model.ConnectionRef)`, `Close()`; stop is idempotent and waits for worker cleanup with a bounded timeout/error.
- `(*Writer).Send(kind string, data []byte) error`; canceled/full interactive writes return errors, never false success. `Stats()` reports session count, queued bytes and dropped log bytes for validation.
- Frontend `reduceEvent(state: WorkspaceState, event: SessionEvent): WorkspaceState`; `WorkspaceState` maps tabs/connections/sessions, never routes by active tab alone.

- [ ] Write `TestSessionCancellationAndBounds`: cancel during worker setup and after data begins; call stop twice; assert worker exits and session count returns to zero. Feed log data beyond capacity; assert queue remains bounded and a truncation event appears. Use channels/fake clocks, not sleep-based timing.
- [ ] Write one Vitest `it('routes by session and ignores stale generations')`: create tabs A/B, activate B, deliver A data, verify only A changes; close A and deliver delayed A data, verify no state update. Remove pass-with-no-tests mode from Task 1.
- [ ] Run `go test ./internal/session -run TestSessionCancellationAndBounds -v` and `npm test`; observe the relevant failures before implementation.
- [ ] Implement per-session cancellation and bounded delivery. Initial validation limits: 1 MiB log queue and 1 MiB frontend log history per session; batches at most 64 KiB or every 50 ms. Watch queues hold at most 256 updates then request resync. Terminal output uses bounded 64 KiB chunks/queue with flow control; input backpressure rejects explicitly rather than dropping. These are trial transport bounds, not product performance promises.
- [ ] Wire connection close/generation invalidation to `StopConnection`. Prevent new session starts during close; callback ordering must not deadlock manager locks. Expose safe state/cancellation errors through the desktop service.
- [ ] Run focused checks and `scripts/check.sh`. Commit: `feat: isolate tab sessions and bound stream delivery`.

## Task 5: Local fixtures and actual Kubernetes streams

**Files:** create kube read/streams/terminal/forward files, `scripts/local-clusters.sh`, `tests/fixtures/phase1a.yaml`, `tests/integration/validation_test.go`, `frontend/src/Terminal.tsx`; modify App, desktop service and build dependencies.

**Interfaces:** `kube.NewService(connections *connection.Manager, sessions *session.Manager) *kube.Service` supplies:
- `Pods(ctx context.Context, ref model.ConnectionRef, namespace string) ([]corev1.Pod, error)` (adapter projects only needed fields to React).
- `WatchPods(ctx context.Context, target model.Target) (model.SessionRef, error)`.
- `Logs(ctx context.Context, target model.Target, previous bool, tail int64) (model.SessionRef, error)`.
- `Exec(ctx context.Context, target model.Target) (model.SessionRef, error)`; command fixed to interactive shell selection for this validation UI, not a local runner.
- `Input(ref model.SessionRef, bytes []byte) error`, `Resize(ref model.SessionRef, cols, rows uint16) error`.
- `Forward(ctx context.Context, target model.Target, local, remote uint16) (kube.ForwardResult, error)`; result includes SessionRef and actual local port, zero requests ephemeral local port.

- [ ] Write one opt-in `TestLocalValidation` (`//go:build integration`) with substeps: A/B identical names yield different markers, selected Pod logs contain expected markers, denied identity gets forbidden, and a forwarded endpoint returns the expected cluster marker. Require explicit `KUBESIFT_TEST_KUBECONFIG` and require the explicit A/B mapping and refuse missing/unrecognized fixture contexts; no ambient fallback. After the fixture setup below, run before the adapters are implemented to observe unsupported operations, not just missing environment.
- [ ] Implement `scripts/local-clusters.sh up|down` using [local development](../../engineering/local-development.html): A is existing `kind-dev-local` / `payments`; B is disposable `kubesift-b` / context `kind-kubesift-b`, namespace `payments`. Reuse A without recreating it. Create identical validation-owned fixture names with distinct markers on both, keeping existing checkout-api/payments-db workloads untouched. Preserve isolated ignored `.local/phase1a/` configuration and specify context/namespace on every call. Track ownership; teardown deletes only run-owned resources, and deletes B only if this run created it. Never delete A or its namespace. Unknown pre-existing B or fixture-name collisions fail setup clearly. Use pinned shell/log/HTTP fixture images and record their digests.
- [ ] Add limited RBAC fixture credentials for denial checks and local helper integration. Do not track generated kubeconfigs/tokens. Start local clusters only in the implementation session and only with the targeted fixture script.
- [ ] Implement adapters using explicit clients/configs and shared session ownership. Re-list after watch expiration; log readers stop on cancellation; remote exec handles TTY resize; port forward opens loopback and reports allocated port. Connection errors must be distinct from RBAC denial. No automatic write/exec replay on reconnect.
- [ ] Implement minimal tabbed controls and trust settings. Render untrusted strings as text. Use xterm.js byte input/output with explicit decoding and resize, disable unsafe escape-driven link actions. Retain keyboard focus and visible cluster/namespace/container labels. This is validation UI, not a design-system rollout.
- [ ] Run `scripts/local-clusters.sh up`, then `KUBESIFT_TEST_KUBECONFIG="$PWD/.local/phase1a/kubeconfig" go test -tags=integration ./tests/integration -run TestLocalValidation -v`. Expected pass with A/B identity, logs, forbidden and forwarding results. Run `scripts/check.sh`. Commit: `feat: validate Kubernetes streams on isolated clusters`.

## Task 6: Prepared mutation and wrong-target protection

**Files:** create operation file/test; extend desktop/App review action and existing integration scenario.

**Interfaces:** `operation.NewService(connections *connection.Manager, now func() time.Time) *operation.Service`:
- `Prepare(ctx context.Context, target model.Target, value string) (operation.Preview, error)` reads ConfigMap `ks-marker` and returns opaque token, target, old/new value, UID, resourceVersion and expiry.
- `Execute(ctx context.Context, token string) error` atomically consumes the token and updates only its stored target/payload; no frontend target override.
- Tokens expire after five minutes; attempts are single-use even if the outcome becomes uncertain. Explicit prepare is needed to retry.

- [ ] Write `TestPreparedMutationSafety` as a small table: active-tab switch has no target input to Execute; stale UID/version/generation rejected; token reused/expired rejected; simulated response loss yields `uncertain` and exactly one write attempt. Inject a fake Kubernetes client and clock. No live cluster in this unit test.
- [ ] Run `go test ./internal/operation -run TestPreparedMutationSafety -v`; expected red before implementation.
- [ ] Bind prepared target and payload in Go; recheck identity and trust at execution. Use ConfigMap UID/resourceVersion preconditions via supported update/patch semantics; surface conflict rather than forcing ownership. Consumption must be atomic across concurrent calls. Restrict the validation write UI to fixture ConfigMap targets selected from the local test configuration.
- [ ] Add review showing cluster, namespace, resource, old/new marker and expiry. Confirming after switching tabs still uses the original token; closing/replacing the originating connection invalidates it.
- [ ] Extend the single integration test to prepare A, switch presentation to B, execute A and verify B unchanged; edit/recreate the target and confirm rejection. Run focused Go test, integration scenario and `scripts/check.sh`. Commit: `feat: bind validation writes to reviewed targets`.

## Task 7: Packaged acceptance, load evidence and handoff

**Files:** create `docs/validation/phase-1a.html`, `measurements.json`; finalize `toolchain.html`, source README files and docs index. Modify adapters only for issues demonstrated by validation.

**Interfaces:** the report records spec cases A01–A10 as pass/fail/not-run with command or manual procedure, expected/actual result and evidence reference. `measurements.json` records host summary, exact versions, elapsed durations, startup/latency samples, memory samples, session/queue/drop counts and limitations; no fabricated measurements.

- [ ] Run `scripts/check.sh`, `go test -race ./internal/...` and the single integration scenario. Fix observed failures with the smallest regression case; do not expand test coverage merely to reach a percentage.
- [ ] Package with the pinned Wails tooling. Launch `.app` through Finder; verify local startup and explicit connection, helper approval/review/revocation, two clusters, container shell/resize, logs and port conflict. No automated native UI end-to-end framework is required.
- [ ] Run a ten-minute load: two clusters, one Pod watch and two log follows each, one terminal and one forward. Use 100 lines/sec × 256 bytes on each log source for baseline and a 60-second 2,000 lines/sec flood on one source. Record truncation and bounds. Repeat start/stop twenty times; compare workers/listeners and post-cleanup memory across cycles. Record ten packaged-start and twenty terminal round-trip samples; report median/p95 without inventing a pass budget.
- [ ] Manually exercise disconnect/reconnect on disposable B (never stop the user-owned primary cluster), denied RBAC, stale watch recovery, sleep/wake, closing a tab during stream setup, app exit, missing helper, config change and hostile annotations/logs. The unit tests already cover core identity/guard paths; use this checklist for platform behavior. No mutation or terminal command may replay. Verify no leaked forward listener remains after shutdown.
- [ ] Complete the A01–A10 matrix. Local architecture pass requires all acceptance outcomes; an unrun local case blocks a full pass. Record untested cloud/OpenShift authentication separately as compatibility follow-ups, never silently count them as passed. A correctness/security failure is revise, not conditional success.
- [ ] Write proceed/proceed-with-follow-ups/revise recommendation and measured proposed performance budgets. Keep signing/notarization/updater readiness explicitly separate from this local packaged check. Ensure all HTML/Markdown links resolve using `scripts/check-docs.py` and preserve license attribution in package assets.
- [ ] Obtain one correctness review, fix actionable findings and rerun only affected checks. Use repo review skill if available; otherwise use its prompt with one reviewer. This is a review of the implemented branch, not permission to merge or release.
- [ ] Commit: `docs: record Phase 1A validation evidence`. Return branch, commits, commands, report and remaining limitations for review. Stop before Phase 1B and do not create a release.

## Coverage and test size

| Acceptance | Primary evidence |
| --- | --- |
| A01 packaging | Task 1 Ping smoke; Task 7 packaged run |
| A02 config | Task 2 single test; Task 7 checklist |
| A03 helper lifecycle | Task 3 single test; Task 7 actual helper refresh |
| A04 targeting | Task 2 identity, Task 4 event routing, Task 5 integration |
| A05 concurrency | Task 5 adapters; Task 7 concurrent run |
| A06 forwarding | Task 5 integration; Task 7 conflict/cleanup |
| A07 mutation | Task 6 single safety test and integration substep |
| A08 recovery | Task 7 checklist; task-level regression only for discovered defects |
| A09 untrusted input | Task 3 trust test, Task 4 routing test, Task 7 hostile-data check |
| A10 cleanup/load | Task 4 lifecycle test; Task 7 measurement run |

Baseline automated suite: four focused Go test groups, one frontend reducer test, one opt-in integration scenario. No snapshot suite, broad mock matrix, live-provider CI or exhaustive permission tests. Retain manual evidence for acceptance outcomes rather than pretending unit tests prove them.

## Implementation-session prompt

> Implement Phase 1A only using `docs/superpowers/plans/2026-11-05-phase-1a-validation.md` and its approved specification. Read AGENTS.md and the engineering/Git workflow first. Use Superpowers executing-plans in a suitable implementation worktree and feature branch. Verify the latest reviewed planning commit before starting; do not overwrite existing work. Keep tests minimal as specified. Record actual local validation and packaged smoke results, and return a reviewable branch and HTML report. Do not start Phase 1B, publish a release or change remote rulesets. If framework compatibility or a safety boundary cannot be met, stop that dependent work and report the required design change.
