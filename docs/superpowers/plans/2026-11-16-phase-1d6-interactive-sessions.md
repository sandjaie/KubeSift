# Phase 1D.6 Interactive Sessions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Open Pod-container terminals and Pod/Service port forwards from existing resource views and manage them in the current Sessions dock.

**Architecture:** Extend Phase 1A native exec/forward adapters with UID-bound product contracts and pending activation. Reuse the session manager and xterm renderer, with one product owner that survives navigation. Service selection resolves to an explicitly chosen fixed Pod; it is not a continuously re-resolved proxy.

**Tech Stack:** Existing pinned Go/client-go, Wails, React/TypeScript, xterm/FitAddon and Vitest. No new dependencies or local shell runner.

**Spec:** [1D.6 interactive sessions design](../specs/2026-11-16-phase-1d6-interactive-sessions-design.md). Spec and plan requested together; both prepared for review, no implementation in this planning session. Baseline `4380d03`.

## Global Constraints

- Preserve current UI, Sessions geometry, themes and keyboard conventions. Kubernetes permissions are authoritative; Start is the user action, with no dry-run or command-by-command confirmation.
- Pod-container terminals only; local laptop terminal excluded. No debug-container injection, automatic command fallback, automatic reconnect/failover, UDP or public listening addresses.
- Reuse native session flow control. Eight terminal/forward entries combined, existing independent log limit unchanged; native total cap 128. Terminal scrollback 1,000 lines, input 64 KiB, command 8 KiB/32 arguments; page size 100, forward startup 20 seconds, identity checks every 15 seconds.
- Loopback is `127.0.0.1`; one TCP port pair per session; local 0 requests allocation. Ended sessions retained until Close count toward the product cap.
- Follow [engineering](../../engineering/engineering.html), [Git workflow](../../engineering/gitworkflow.md), [Kubernetes references](../../engineering/kubernetes-development.html) and [local development](../../engineering/local-development.html). Source descriptions, scoped formatting and verified checkbox updates are required.
- Implement in the existing checkout on `feature/phase-1d6-interactive-sessions` from updated develop. No worktree, release or next phase; only explicit kind-dev-local-a/b for tests.
- Keep tests representative, reuse suites and run one independent whole-PR review. No new native automation framework or duplicate user UAT checklist.

## Review Focus

- Reused Pod names and delayed startup must not retarget sessions: task 1 native and task 4 state cases.
- Service named ports/partial candidate pages must not pick an arbitrary destination: task 3 resolver cases.
- Cancellation and failed startup must release listeners, pending slots and streams: tasks 1–3 lifecycle cases.
- Hidden/theme-switched terminals must not duplicate execution or deadlock acknowledgements: task 4 lifecycle case and task 5 desktop check.
- Port conflict, missing shell and denied subresources must remain distinct from success or automatic retry: tasks 2/3 and local walkthrough.

---

## File map and contracts

| Files | Responsibility |
| --- | --- |
| New `internal/kube/interactive_types.go`, `interactive.go` | Product targets, bounded ownership, validation/cleanup |
| Existing `internal/kube/terminal.go`, `forward.go`; new `service_forward.go` | Reusable transports, explicit command, pending forward readiness and Service resolution |
| New `internal/desktop/interactive.go`; existing service/connection cleanup wiring | Thin bridge, cancellation and generated contract composition |
| Existing `frontend/src/Terminal.tsx`, `bridge.ts`; new `sessions/useInteractiveSessions.ts`, `interactiveState.ts` | Owned xterm and product lifecycle, no validation-only state in product path |
| New `sessions/TerminalStart.tsx`, `ForwardStart.tsx`, `InteractiveSessions.tsx`; existing App/resource detail/current Sessions owner | Simple start controls and integration into the existing dock |
| Focused native/state tests plus `tests/integration/interactive_test.go` | Target/lifecycle/port resolution and actual local cleanup evidence |

Before edits locate the current dock owner through App and its children; extend it rather than create a second Sessions surface. Preserve the existing log store/subscription behavior. Legacy Phase 1A APIs may delegate to shared transport helpers without forcing their old response shapes onto the product.

Add concrete Go DTOs, camelCase JSON and generated TS types:

- `PodSessionTarget { Resource ResourceRef; Container string }`, requiring core/v1 Pod UID; container required only for terminal. ResourceRef already includes connection generation.
- `TerminalRequest { Target PodSessionTarget; Command []string }`; first command element is executable, remaining elements arguments. Default is applied by UI as `["/bin/sh"]`, not automatic retry.
- `ForwardRequest { Target PodSessionTarget; LocalPort, RemotePort uint16; Service *ServiceForwardSelection }`.
- `ServiceForwardSelection { Resource ResourceRef; ResourceVersion, PortName string; Port int32 }`; native Start validates Service selection, resolved port and chosen Pod again.
- `ForwardCandidateRequest { Service ResourceRef; ServiceVersion string; PortName string; Port int32; Cursor string }`, `ForwardCandidate { Pod ResourceRef; PodVersion string; RemotePort uint16 }`, `ForwardCandidatePage { Items []ForwardCandidate; Page PageInfo; Complete bool }`.
- `InteractiveEntry { Session model.SessionRef; Kind string; Target PodSessionTarget; Service *ServiceForwardSelection; LocalPort, RemotePort uint16; State string; Problem *model.Problem }`. No command transcript or traffic payload in metadata.

`OpenTerminal(ctx context.Context, req TerminalRequest) (InteractiveEntry, error)` and `OpenForward(ctx context.Context, req ForwardRequest) (InteractiveEntry, error)` return pending identities, not ready success. `ReadForwardCandidates(ctx context.Context, req ForwardCandidateRequest) (ForwardCandidatePage, error)` is cancellable. `StopInteractive(ref model.SessionRef) error` stops work but retains the entry; `CloseInteractive(ref) error` releases it. Use existing ActivateSession, TerminalInput, ResizeTerminal and acknowledgement methods. Reuse session event envelopes: terminal data remains bytes; state events carry bounded JSON with lifecycle, actual local port and safe problem. Forward traffic never crosses the bridge.

Desktop Open calls take a requestID through the existing cancellable request adapter. A cancellation that wins before return must stop/release the new entry; a late response discarded by frontend must also Close it. Native session lifetime is rooted in the connection/app, not the short setup request context after returning. Product owner registers the identity/listener before activation. Do not start transports before ownership is recorded.

## Task 1: UID-bound interactive ownership

**Files:** Create interactive types/owner and `internal/kube/interactive_test.go`; add desktop bridge scaffold with actual validation/cleanup, not empty APIs.

**Interfaces:** Produces target validation, product reservation/state lifecycle and Stop/Close; later tasks plug transports into this owner.

- [x] Add `TestInteractiveOwnership`: replacement UID rejected, eight retained entries reject ninth, stopped remains retained, Close frees slot, generation mismatch and cancel-before-activation clean up. Run `go test ./internal/kube -run TestInteractiveOwnership -count=1` and confirm new assertions fail.
- [x] Implement reservation under existing kube.Service synchronization, using the established product log lifecycle as a pattern without combining independent caps. Validate explicit Pod identity/container from a fresh read; pass a transport target only after validation.
- [x] Add cancellable pre-start/post-start and 15-second active Pod UID checks. A failed check stops work with a safe reason; no claim of atomic name-addressed exec/forward identity. Reuse connection cleanup fanout so adding this owner never replaces logs/terminals cleanup.
- [x] Implement pending cancellation and idempotent Stop/Close/connection shutdown, including retained-entry cleanup. Return bounded metadata only. Run `go test ./internal/kube ./internal/session`; expect pass. Commit `feat: own UID-bound interactive sessions`.

## Task 2: Product Pod terminal transport

**Files:** Modify `internal/kube/terminal.go`; extend interactive tests or current stream tests and desktop `interactive.go`.

**Interfaces:** Implements OpenTerminal; existing input/resize methods remain SessionRef-addressed. Preserve compatible legacy Exec wrapper with `/bin/sh` default for validation.

- [x] Add representative terminal cases: explicit argument vector reaches the transport unchanged, unavailable container/Forbidden is reported, bounded input rejects overflow, resize targets the session, cancellation unblocks reader/writer. Run targeted native tests before implementation and confirm failure.
- [x] Extract a shared exec helper accepting command vector and owned session context. Use current pinned remotecommand transport without automatic command fallback/replay. Bound argument count/bytes, reject NUL and empty executable; perform no local shell execution. Send running/ended/failed state so UI does not equate pending with connected.
- [x] Keep existing input queue and size coalescing, pending activation and cleanup. Post-start checks must not create a second exec. Successful shell exit is ended; nonzero/error is described without credential output. A lost stream is not retried.
- [x] Run `go test ./internal/kube ./internal/session ./internal/desktop`; expect pass; regenerate bindings as needed. Commit `feat: expose product Pod container terminals`.

## Task 3: Pod and Service forwarding

**Files:** Modify `internal/kube/forward.go`; create `service_forward.go`, `service_forward_test.go`; extend lifecycle tests and desktop bridge.

**Interfaces:** Implements ReadForwardCandidates and OpenForward. Native resolution binds one Service mapping to one selected Pod; UI cannot supply an unchecked arbitrary endpoint.

- [x] Add `TestServiceForward`: numeric/omitted/named targetPort, conflicting named port, UDP, selectorless/ExternalName, partial candidate page, changed Service version and same-name replacement Pod. Add one transport case for occupied local port and cancellation-before-ready. Run targeted tests and confirm failure.
- [x] Resolve Service and selector in its namespace using bounded 100-item Pod pages. Include only running candidates with a valid mapping; do not infer readiness or healthy Service routing. Bind cursors to Service identity/version and selected port. Named port matching must use TCP declarations and reject conflicting numeric resolutions.
- [x] Revalidate Service version/selector/port and selected Pod at OpenForward. Direct Pod forwarding permits any valid numeric remote TCP port. Refactor forward transport to StartPending, then emit ready with actual allocated port. Use existing 20-second startup timeout, loopback listener and explicit conflict reporting; cancellation/error always closes listener/transport. Legacy synchronous Forward can wrap readiness if validation still needs it.
- [x] Expose candidate read cancellation and product Open/Stop/Close in bridge; inspect generated DTOs after `wails3 task bindings`. Run `go test ./internal/kube ./internal/session ./internal/desktop` and `go test -race ./internal/kube ./internal/session`; expect pass. Commit `feat: add owned Pod and Service port forwarding`.

## Task 4: Current UI and persistent Sessions integration

**Files:** Add useInteractiveSessions/state/start forms/InteractiveSessions listed above; modify Terminal.tsx, bridge, App, actual dock owner, Pod/Service detail controls. Add `frontend/src/sessions/interactiveState.test.ts`.

**Interfaces:** App owns `useInteractiveSessions()` exposing entries, openTerminal/openForward/stop/close and closeConnection. Start forms use generated request types. One xterm instance per terminal entry; one metadata listener in owner, terminal data listener acknowledges only its own frames.

- [x] Add one fake-bridge lifecycle scenario: pending Start → tab/theme switch → late ready → Stop/Close; assert one activation, same target, no duplicate reader, late cancelled Open closed and entry limits respected. Include ended state and stale generation. Run `npm test --workspace frontend -- src/sessions/interactiveState.test.ts`; confirm new cases fail.
- [x] Add Terminal and Port forward actions where applicable. Terminal form has container, executable default `/bin/sh`, optional JSON argument array and Open. Forward form has Pod/Service mapping, local auto/manual port and Start. Multiple/incomplete Service candidates require explicit Pod selection; one complete candidate may default. Show native errors without a permission wizard.
- [x] Reuse xterm while removing product dependence on validation callbacks and hardcoded theme. Keep instances mounted across Settings, dock collapse, tabs and theme changes; apply theme/fit in place. Preserve user copy/paste and disabled escape-driven links/clipboard. Continue parse acknowledgements offscreen; do not route input to a hidden previously focused terminal after focus changes.
- [x] Add type/target/state labels and Stop/Close to the current dock alongside logs. Show allocated address only on ready and explicit Copy address. Stop retains bounded terminal output; Close disposes it. Tab-close confirms all affected session types once; disconnect stops work and labels entries, app exit cleans up. No automatic reconnect, browser launch or command replay. Shortcuts must not steal terminal/input keystrokes.
- [x] Run targeted Vitest, `npm run lint`, `npm run typecheck`; expect pass. Commit `feat: integrate terminals and forwards into Sessions`.

## Task 5: Local validation and delivery

**Files:** Create `tests/integration/interactive_test.go`, `docs/superpowers/validation/phase-1d6.html`; extend only missing owned fixtures and integration README, validation index and checkpoint.

**Interfaces:** One opt-in `TestInteractiveSessions` group uses existing isolated config/context guards from tests/integration/README.html, only kind-dev-local-a/b. Reuse Phase 1A shell/TCP fixtures where possible.

- [ ] Inspect fixtures; add only missing Service named-port and multi-container examples in owned namespaces. Document exact opt-in environment/command. No extra clusters, shared system changes or debug-container injection.
- [ ] Run a bounded two-context walkthrough: actual terminal input/output/resize, Pod and Service forward requests, allocated endpoint request/response, conflict and Stop/Close. Verify ports cannot be connected after cleanup and native session counts return to baseline. Cover UID/start races with fakes rather than a large live matrix. Restore owned fixtures.
- [ ] Run `wails3 task bindings`, `npm run build`, `scripts/check.sh`, `wails3 task package`; expect success. Inspect generated diffs, source headers and scoped formatting. Repeat short pressure/cleanup checks only for changed transport risks, not all historical 1A benchmarks.
- [ ] Verify packaged UI input/resize/paste, Service backend choice, actual forwarding, missing shell/error, Settings/theme/tab/dock persistence, tab-close cancellation and app-exit listener release. Briefly background/restore to record actual WebKit behavior; do not claim lossless terminal capture or untested cloud compatibility.
- [ ] Record actual results/limits in one technical report, link the validation index, update portable status and verified plan checkboxes. Keep execution logs ignored and payloads out of public evidence. Commit `test: validate interactive sessions and cleanup`.
- [ ] Obtain one independent whole-PR review, fix blocking findings and rerun affected checks. Push feature branch and open a ready PR, then complete the authorized merge into develop under current Git workflow. Report final checks/PR/merge and any genuine blockers. No release or next increment.

## Self-review and handoff

Spec coverage maps terminal to task 2, forwarding to task 3, shared identity/cleanup to task 1, UI/navigation to task 4 and evidence/delivery to task 5. Five risk classes have focused tests. Contracts preserve the existing transport and log behavior rather than introducing a replacement. All steps remain unchecked until implementation verification.

Read this plan and its spec together in the separate implementation session. Laptop terminals remain outside 1D.6; no application changes were made while authoring these documents.
