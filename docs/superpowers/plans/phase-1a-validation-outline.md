# Phase 1A — desktop integration validation outline

Status: historical outline; expanded by [Phase 1A specification](../specs/2026-11-05-phase-1a-validation-design.md). Not an executable implementation plan.
Owner of phase order: [roadmap](../../roadmap/roadmap.html)
Design dependency: [specification](../specs/2026-11-05-kubesift-design.md)

## Purpose

Demonstrate that the selected Wails/Go integration can support KubeSift's real authentication and concurrent-session needs before expanding application features. Keep the Kubernetes services independent of the bridge so failure can lead to a focused architecture revision.

## Prerequisites before execution

- Written design approved and implementation explicitly requested.
- Select/pin toolchain and framework versions; select kind, minikube or k3d based on the host and test requirements.
- Identify supported initial macOS version/CPU architecture and packaged-build method.
- Create disposable local targets for mutations and failure injection. Use explicit fixture targets.
- For real authentication checks, obtain explicit contexts and read-only permissions; use synthetic evidence artifacts.

## Validation work packages

| Package | Experiment | Required evidence |
| --- | --- | --- |
| Desktop/packaging | Launch from Finder, render UI, invoke a narrow Go service, close app | Packaged build works independently of terminal shell environment; shutdown is clean |
| Kubeconfig/auth | Default/additional files, helper approval, configured helpers, expiry and missing-tool paths | Correct context resolution; first-use review; repeat trust; changed configuration re-review; revocation blocks subsequent runs |
| Concurrent identity | Two genuinely distinct cluster connections/tabs | Reads/events retain target identity; switching tabs cannot redirect operations; kubeconfig current-context unchanged |
| Streams | Concurrent watches/logs plus interactive container terminal with resize | Responsiveness and memory measured; bounded buffering; cancellation stops backend work; target/session IDs checked |
| Forwarding | Open/stop forward; conflict; tab close/app shutdown | Loopback binding and visible target; listener released; no orphan sessions |
| Controlled mutation | Prepare/review a change on local cluster A, switch tab to B, then execute | Only original intended resource changes; resource identity/version conflicts are handled; no mutation replay |
| Failure behavior | Drop connectivity, expire watch version, deny RBAC, sleep/wake, flood logs | Partial failures remain visible; bounded retries; reads recover; writes/exec are not automatically replayed |
| Security boundary | Synthetic hostile annotations/logs and invalid requests | Content rendered as data; backend validates scope; no credential output or unrestricted native command exposure |

## Measurements and decision

Record startup time, concurrent-stream throughput, memory growth/plateau, terminal interaction latency and cleanup behavior for the tested host and workload. Propose concrete budgets from those measurements and review them before scale acceptance. Passing a small demo is not proof of production-scale performance.

Pass requires the functional evidence above and no unresolved target-isolation, credential-disclosure or unbounded-resource failure. If transport throughput is inadequate, assess batching/flow control before adding another local transport. If integration remains unsuitable, update the architecture decision and compare alternatives before feature expansion.

Local Kubernetes validates generic behavior, not OpenShift-specific APIs or all cloud authentication configurations. Record each untested environment explicitly. External write tests and cluster-wide operations are outside this outline unless separately targeted and authorized.

## Deliverable when executed

A short validation report with tested versions/environment, synthetic fixtures, pass/fail evidence, measurements, limitations and the proceed/revise recommendation. Use synthetic evidence.
