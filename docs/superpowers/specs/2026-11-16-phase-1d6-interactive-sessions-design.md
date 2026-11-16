# Phase 1D.6 — Pod terminals and port forwarding

**Status:** Spec and companion plan prepared at the user's request; no implementation authorized in this planning session.
**Date:** 2026-11-16

Owners: [roadmap](../../roadmap/roadmap.html#phase-1d), [UX](../../ux/workflows.html), [security](../../security/security-model.html), [engineering](../../engineering/engineering.html#agent-verification). Baseline: develop `4380d03`, after rollback merge `13b94f6`.

## Outcome and approach

Open a terminal inside a selected Pod container or forward a Pod/Service TCP port to the laptop, then manage it in the current Sessions dock. Kubernetes permissions remain authoritative. Start is the explicit user action; no dry-run, typed confirmation or approval for each shell command. An interactive shell can change the container; it is not read-only.

Reuse the Phase 1A Go exec/forward transports, session manager and existing xterm renderer. Extend their explicit identity and product lifecycle rather than launch external kubectl processes or build another session subsystem. External terminals would fragment the workspace; a local shell runner would introduce unrelated laptop command execution.

Scope: Pod-container terminals and Pod/Service forwarding, multiple sessions, input/resize, start/stop/close and original-target display. Laptop/local-shell terminals are excluded in this increment, as are node shells, injected debug containers, file transfer, UDP, non-loopback listeners, SSH, persisted sessions and automatic reconnection. The unanswered local-shell option is recorded here as a scope assumption, not a permanent product exclusion.

## Pod terminal

Use Terminal from an existing Pod detail or selected Pod action. Show cluster, namespace, Pod and container. Select the sole running container automatically; otherwise let the user choose among regular/init/existing ephemeral containers currently running. Do not inject a container, install a shell or silently select another Pod when the selected container is unavailable.

Default executable is `/bin/sh`, editable before opening. An optional arguments field accepts a JSON string array (default empty), described plainly with an example; the native request carries executable plus argument vector without local shell parsing. This permits `/bin/bash`, PowerShell or another installed program without command fallback/replay. Reject empty/NUL-containing command arguments and bound the total to 8 KiB/32 arguments. Failed shell launch shows the error and lets the user change the command manually; no automatic second execution.

Use TTY input/output and the existing xterm renderer for typing, normal paste, selection/copy, resize and standard control keys. Resize follows the dock dimensions. Use existing theme tokens and 1,000-line scrollback. Keep output native/bridge bounded and input queued at the existing 64 KiB limit; show rejected input rather than dropping it silently. Escape-driven clipboard and link execution remain disabled; intentional user copy/paste stays available. No automatic command logging or transcript persistence.

Exit and errors show ended/failed states. Stop closes the local exec transport; it cannot promise that every remote child process exits. Start again is a new explicit session after identity validation, never a replay of past commands.

## Port forwarding

From Pod detail select/supply a remote TCP port and local port. Offer declared ports as suggestions but allow a numeric remote port even if not declared. Local port defaults to automatic allocation (0); manual values 1–65535 are supported. Show the actual allocated endpoint only after successful binding. Bind to `127.0.0.1`, one mapping per session. A requested occupied port fails clearly; do not silently choose another port.

From Service detail select a TCP Service port. Resolve the selected Service UID/version and selector to a bounded page of running Pods in the same namespace. Show the actual backend Pod before starting: choose it automatically only if exactly one candidate is known and discovery is complete; otherwise require a selection. Reuse 100-item paging; never fetch an unbounded candidate list. Label selector matches as candidates, not proof of healthy routing.

Resolve numeric targetPort directly; when omitted use Service port; resolve a named targetPort against the chosen Pod's declared TCP container ports, rejecting missing/conflicting mappings. Validate the Service UID/version, selector, chosen Pod identity and selected port again on Start. A Service forward is one fixed Pod connection, not a load balancer. Display both Service and Pod. Do not follow EndpointSlice IPs or arbitrary external endpoints in this increment. ExternalName and selectorless Services explain that this workflow has no selectable Pod; users can forward an actual Pod directly if available. UDP and SCTP are explicitly unsupported.

The ready view shows local address, remote port, target and Stop/Close; provide explicit Copy address. Do not assume HTTP, open a browser automatically or proxy network payload through React. Startup is cancellable and bounded by the existing 20-second timeout. Show connecting, listening, stopped and failed states, with port conflicts, RBAC and remote failures distinguishable where known.

## Identity and lifetime

Capture connection generation, namespace, Pod name/UID, container for exec, and optional Service name/UID/version for forwarding. Native Get checks the Pod UID before activation. Recheck Pod identity after startup and every 15 seconds while active; terminate on replacement, absence or failed verification with an explanation. Exec/forward endpoints are name-addressed and cannot enforce an atomic UID precondition: this monitoring reduces risk but does not remove the API race. Container restart/exit ends exec naturally; no automatic container restart tracking or reattachment. Service changes after startup do not migrate an already fixed Pod forward; the dock describes it as the captured mapping.

Use pending-session/register-before-activate for product starts. Return a session ID before transport output, then emit bounded ready/error/end events. Refactor the existing forward path's synchronous readiness only as needed for this lifecycle; preserve legacy validation callers through a wrapper if necessary. Cancel/close before activation must release all pending resources and cannot race into a late listener.

Keep native data delivery and acknowledgements independent of the selected cluster/view. Sessions remain owned and accessible through Settings navigation, theme changes, resource selection and dock collapse. Keep xterm instances mounted while their sessions exist; pause expensive offscreen layout without interrupting acknowledgement or growing buffers. Retain at most eight terminal/forward entries combined, including ended terminals, alongside the existing separate log limit. Starting a ninth asks the user to close one; no silent eviction. Native transport cap remains the existing 128-session safety ceiling.

Stop cancels native work but retains the bounded session entry/terminal display. Close also frees UI state/listeners; forward ports must be released on either. Closing a cluster tab uses one confirmation listing its active sessions, then stops and closes them. Disconnect/helper revocation/generation change stops native work and marks entries disconnected; app exit cleans everything. Restarting the app restores no shells/listeners. User Retry/Open again creates a new explicit target-bound session; no automatic replay, retarget or service failover.

Do not introduce a second dock: add terminal and forwarding entries to the current Sessions presentation with clear type, cluster/namespace/resource and lifecycle status. Keyboard shortcuts act only on the selected eligible resource, and must not steal terminal control keys or typing in inputs. Preserve current palette, geometry, split state and navigation.

## Permissions and evidence

Reuse existing credentials and trusted helper behavior. Actual Pod reads, exec and port-forward calls determine permission; unknown advisory permission hints are not denial. Forbidden does not mean expired credentials. Successful read does not promise exec access. Never test permission with a trial shell or hidden port forward.

Only content intentionally displayed in the terminal stays in bounded memory. Do not log terminal input/output, credentials or forwarded traffic. Ended-session metadata is session feedback, not audit history. Retain known Phase 1A background WebKit limits and report actual bounded behavior; no lossless transcript or all-platform claim.

References: [container shells](https://kubernetes.io/docs/tasks/debug/debug-application/get-shell-running-container/), [port forwarding](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_port-forward/), [Service ports](https://kubernetes.io/docs/concepts/services-networking/service/). Check pinned client transport support during implementation; no automatic replaying fallback once a stream may have started.

## Minimal verification and delivery

Extend existing session tests with representative UID replacement, cancellation-before-ready, input/resize, listener cleanup, service named-port resolution and permission failure cases. Add a small frontend lifecycle case for tab/theme changes and late events. No per-component suites, new E2E framework or full repeat of historical 1A load experiments.

Use only kind-dev-local-a/b and owned synthetic shell/TCP fixtures. Verify one terminal per cluster, resize/input, Service and direct Pod forwarding, port conflict, unavailable shell/container, stop and tab/exit cleanup. Test race conditions with fakes and actual listener release with a local integration check. Record unsupported/unrun cloud/provider paths honestly.

Agent desktop verification and repository checks/package feed one `docs/superpowers/validation/phase-1d6.html` report. Follow one independent whole-PR review and current authorized PR/merge workflow; no duplicate user UAT checklist, release or next-phase work.

Acceptance: target-correct terminal input/resize; fixed-Pod Service/Pod TCP forwarding; useful errors; sessions survive navigation without duplicate readers; stop/close/disconnect/exit release streams and ports; current UI and Kubernetes permissions respected.

## Planning handoff

Superpowers builder review found no important unanswered planning questions. The [five-task implementation plan](../plans/2026-11-16-phase-1d6-interactive-sessions.md) was prepared alongside this spec at the user’s request. Review both before starting the separate implementation session.
