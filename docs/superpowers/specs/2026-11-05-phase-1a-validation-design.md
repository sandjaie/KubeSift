# Phase 1A — desktop integration validation specification

Date: 2026-11-05
Status: approved by the user on 2026-11-05; execution belongs in a separate implementation chat
Parent: [KubeSift design](2026-11-05-kubesift-design.md)
Sequence owner: [roadmap](../../roadmap/roadmap.html)

## Intent

Prove that the selected Wails + Go + React/TypeScript architecture can support authenticated, simultaneous Kubernetes sessions, interactive streams and correctly targeted operations in a packaged macOS application. This is a narrow integration milestone, not the full explorer or a polished product shell.

This chat owns discussion, architecture, specification and plan review. A separate implementation chat will execute the approved plan. Repository instructions apply to both; the planning-only constraint on this chat must not be misread as a permanent ban on authorized implementation elsewhere.

## Approaches and chosen direction

The user already selected Wails with an early validation milestone. Expand that milestone into a minimal end-to-end desktop integration, preserving testable Go domain boundaries.

- A mock-only desktop demo would test rendering but leave Kubernetes authentication, actual streaming and cleanup unproven; it is insufficient.
- A full explorer would delay discovery of architecture risks and duplicate Phase 1B scope; defer it.
- The selected integration approach uses a small validation interface, real local clusters and synthetic fixtures. Retain useful service code only if it satisfies the approved boundaries and checks; validation controls are not automatically production UX.

## Goals and non-goals

Goals: packaged GUI launch; explicit connection identity; two real cluster sessions; kubeconfig and credential-helper trust; logs/watch/exec/forward lifecycle; one controlled mutation; fault handling; measured performance and a proceed/revise report.

Non-goals: full resource navigation, complete RBAC UI, Helm, metrics dashboards, node drain, dedicated OpenShift workflows, AI, provider keys, auto-updates and public release. No monitoring agents or supporting components are installed by the application. Local fixture deployments are development infrastructure created by test tooling.

All permanent safeguards are owned by [the security model](../../security/security-model.html). This spec defines the Phase 1A slice and tests, not alternative security rules.

## Environment and prerequisite policy

Initial validation target: this development Mac's arm64 architecture. Host inspection found macOS 26.7.1, Node and a Docker executable; Go and kind/minikube/k3d were not on PATH. This does not establish a running container engine or minimum supported macOS version.

Local target update (2026-11-05): use the existing `kind-dev-local` context and `payments` namespace for primary development/validation. Treat its workloads as user-owned. Use a separate disposable second cluster for the two-server checks. [Local development](../../engineering/local-development.html) owns target details, setup and teardown rules. Do not recreate the primary cluster or namespace.

Use two genuinely different API servers, each containing an identically named namespace and workload. Give their synthetic responses different marker values so cross-cluster mistakes are observable. Store test kubeconfigs outside tracked paths and pass an explicit isolated kubeconfig to fixture tooling. Do not alter the user's default kubeconfig or current-context.

Use only local fixtures for mutation/fault injection. External authentication checks require explicit user-selected contexts and read-only scope; no automatic probing of every kubeconfig context. Real OpenShift/EKS/AKS results are reported separately from local architectural validation.

Approved visual direction: [UX specification](../../ux/workflows.html) and its single mockup gallery. This does not expand Phase 1A into the full explorer or AI implementation.

## Minimal validation interface

A small React interface needs only:

- Add/select a kubeconfig source and context, with errors and a credential-helper review.
- Two cluster tabs, persistent connection/namespace identity and selected test Pod/container.
- A resource summary/watch area, bounded log output, terminal and active-forward list.
- Explicit start/stop controls, session states and one prepared-operation confirmation.
- A trust settings view allowing review and revocation.

Visual polish and final component-system choices are deferred. Every control must be keyboard-operable and labels must identify the target without relying on color.

## Service and identity contracts

Implement framework-independent Go services behind a thin Wails adapter. Frontend requests use backend-issued connection IDs and explicit target references. A connection generation identifies the resolved endpoint/authentication configuration; a session ID identifies each watch, log stream, terminal or forward.

Logical operations (names are illustrative, not frozen APIs):

- Open/close connection; list accessible namespaces/Pods; inspect a chosen Pod.
- Review/approve/revoke helper configuration; approval itself must not execute it.
- Start/stop watch, logs, exec and port forward; send terminal input and resize.
- Prepare one local mutation; confirm/execute its immutable prepared target/payload.

Data events include connection generation, session ID, sequence and payload/type. Reject stale events and requests; no handler resolves its target from the currently selected UI tab. Cancellation must reach Go and Kubernetes network work, not merely unsubscribe React listeners.

A prepared action is single-use, expires and contains target generation, resource UID/resourceVersion where applicable and payload. Mutation errors distinguish known rejection from uncertain network outcome. Never retry an uncertain write automatically.

## Fixture and mutation design

Use synthetic Pods that generate predictable log lines, provide a shell and a simple network endpoint. Create a limited identity for denial tests, plus a permitted identity for normal tests. Choose resource/image versions and fixture implementation in the execution plan.

The single write experiment updates a synthetic ConfigMap marker using a version precondition. This is enough to test review and correct target routing without implementing all Phase 1C operations. Also delete/recreate or concurrently update that ConfigMap to prove stale UID/version rejection. A UI tab switch must not affect the prepared target; a connection-generation change must invalidate it.

## Credential-helper slice

Use a synthetic local exec helper to validate protocol integration, approval persistence, expiry/failure responses and re-review without live provider credentials. It must never be executed before trust approval. Bind trust to resolved executable path, arguments and configured environment overrides as specified in the security model.

Exercise unchanged configuration, changed arguments/environment/path, revoked trust, missing executable and failed/expired authentication. Do not include resolved secret values in UI or diagnostics. Helper-config review cannot leak secrets embedded in arguments.

Synthetic helper success is not proof of AWS SSO/Azure/OpenShift compatibility. After local validation, record real-provider smoke tests as passed, failed or not run. A local architecture pass may proceed to 1B with named provider-compatibility gaps; it must not claim complete support or satisfy Phase 1F compatibility gates.

## Session behavior

- Watches re-list after expired resource versions and expose stale state during outages; overflow resynchronizes instead of silently losing state.
- Logs support container selection, bounded initial tail and follow. Batching/ring buffers prevent unbounded growth; dropped display history is labeled.
- Exec supports TTY input/output and resize; input is never silently discarded. A disconnect ends the session visibly; reconnect does not replay commands.
- Port forwarding binds loopback by default and exposes local port, target and stop control. Port conflict and remote termination are visible. No payload proxy through React.
- Switching tabs preserves sessions. Confirmed tab closure cancels owned sessions; app shutdown releases all resources. Listener removal is not sufficient evidence of cleanup.

No arbitrary local command runner is exposed to React. Cluster logs/annotations are untrusted data and must not execute HTML/scripts or invoke native services.

## Verification approach

The user requested minimal tests on 2026-11-05. Use a small representative safety suite, one opt-in local integration scenario and a packaged-app manual checklist. Preserve the acceptance outcomes below without adding an exhaustive test matrix or a test for every implementation detail.

## Acceptance matrix

| ID | Scenario | Pass condition |
| --- | --- | --- |
| A01 | Build and Finder launch | Packaged app launches without an interactive shell's environment; narrow Go call succeeds; missing dependencies produce useful errors |
| A02 | Kubeconfig loading | Default and explicit sources work; explicit test config leaves user files/current-context unchanged |
| A03 | Helper lifecycle | No execution before review; trust persists; executable configuration changes re-prompt; revocation blocks next execution |
| A04 | Two clusters | Identical resource names return their distinct markers; UI selection cannot change backend targets |
| A05 | Concurrent sessions | Watches/logs in both clusters and an interactive terminal operate together; terminal resize and stop work |
| A06 | Port forwarding | Endpoint reachable via loopback, conflict explained, stop/close releases listener |
| A07 | Prepared mutation | Switching to B still changes only original A; duplicate confirmation and stale identity/version rejected |
| A08 | Failure recovery | Network loss, expired watch state and RBAC denial are visible; reads recover without write/command replay |
| A09 | Untrusted content | Synthetic hostile strings render inert; malformed/stale session requests are rejected in Go |
| A10 | Cleanup and load | Repeated start/stop leaves no growing session/goroutine/listener leak; flood remains bounded with visible degradation |

## Measurement protocol and decision gate

Proposed initial load profile: two clusters, one Pod watch and two log-follow sessions per cluster, one interactive terminal and one port forward; run for at least ten minutes and repeat session start/stop twenty times. Include a higher-rate log fixture to exercise configured buffer bounds. The exact rates and buffer limits must be fixed in the execution plan before measurement.

Measure packaged startup, terminal round-trip latency, process memory after warm-up/during load/after cleanup, active sessions/goroutines/listeners and delivered/truncated log volume. Record host/resources and raw synthetic measurements. Do not infer a memory leak from one snapshot; inspect repeated growth and retained sessions.

Functional pass requires A01–A10 evidence with no unresolved cross-target operation, unauthorized helper execution, credential leakage or unbounded resource accumulation. Quantitative results establish proposed product budgets, not retrospective invented success thresholds. Report outcome as proceed, proceed with explicit compatibility/performance follow-ups, or revise architecture. Material correctness/security failures cannot receive a conditional pass.

First assess batching, cancellation and flow control when performance is inadequate. Adding another transport or changing frameworks requires an architecture decision update and review. Phase 1A does not establish production-scale or cross-platform readiness.

## Deliverables for the implementation chat

An approved execution plan, minimal integration source/tests/fixtures, reproducible local verification instructions and a validation report under docs/validation/ (HTML summary with synthetic machine-readable measurements as appropriate). The report must list actual versions, pass/fail/not-run cases, measured results, limitations and recommendation. Use synthetic evidence in the report.

The user approved this specification. Create and review its detailed Superpowers plan in this planning chat before implementation. The implementation chat should receive links to this spec, the approved plan, parent design and AGENTS.md; it should verify the checked-out commit and execute only Phase 1A. Decisions that change scope return to this planning chat rather than being silently expanded.
