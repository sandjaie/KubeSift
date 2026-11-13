# Superpowers plans

Acceptance follows [agent verification](../../engineering/engineering.html#agent-verification): plans require automated and agent-run desktop checks in one validation report, not a duplicate manual user UAT checklist. This supersedes older user-checklist requirements below without altering historical results. Ask the user only for subjective decisions or a specific check the agent cannot complete.

## Phase 1C.3c · Namespace policy and configuration (next; planned)

- [Specification](../specs/2026-11-14-phase-1c3c-configuration-design.md): five core kinds, strict metadata-only Secrets, namespace policy evidence and explicit Pod configuration links.
- [Five-task plan](2026-11-14-phase-1c3c-configuration.md): bounded readers, reference validation, views, shared state/navigation and complete verification/delivery.
- Separate authorized implementation session; existing checkout, no worktree. Follow the [fixed roadmap sequence](../../roadmap/roadmap.html#phase-1c3-sequence). Run one fresh whole-PR review after implementation and tests, with targeted fix follow-ups; no routine per-task reviews or duplicate user UAT.

## Phase 1C.3b · Networking

- [Specification](../specs/2026-11-13-phase-1c3b-networking-design.md): five networking kinds, correct cluster scope, bounded association and safe reference resolution. Scope accepted; written package for review.
- [Five-task implementation plan](2026-11-13-phase-1c3b-networking.md): typed reads, references, structured views, shared navigation and agent desktop validation.
- Execute only in a separately authorized session, in the existing checkout without a worktree. The merged 1C.3a baseline is required. No duplicate manual user UAT, mutations, probes or installed infrastructure.

## Phase 1C.3a · Remaining workloads

- [Specification](../specs/2026-11-13-phase-1c3a-remaining-workloads-design.md): StatefulSets, DaemonSets, Jobs and CronJobs with direct Pod ownership and CronJob → Jobs navigation. Scope accepted; written package for review.
- [Five-task implementation plan](2026-11-13-phase-1c3a-remaining-workloads.md): typed readers, bounded relationships, existing UI extension, navigation and explicit validation/user UAT.
- Implementation belongs in a separate authorized session. Use the existing checkout; no worktree. The merged Phase 1C.2 baseline is required; prior user UAT gaps remain separate.

## Workload refinement · 2026-11-13

The user accepted the visual direction and full ReplicaSets scope and requested a planning-only handoff. The written package awaits review; implementation requires a separate explicit request.

- [Design specification](../specs/2026-11-13-workload-explorer-refinement-design.md).
- [UI/navigation refinement plan](2026-11-13-workload-ui-refinement.md): Overview, Events, YAML, related Pods, namespace-kind retention and literal technical inputs.
- [ReplicaSets explorer plan](2026-11-13-replicasets-explorer.md): follows the UI plan and reviewed Phase 1C.2 baseline.
- [Accepted mockups](../../ux/mocks/index.html#workload-refinement).

Logs redesign is separately owned. Planning develop currently precedes the feature implementation; reconcile the implementation/Logs baseline before execution. This package does not authorize a merge, release or the remaining 1C.3 scope.

The [Phase 1A specification](../specs/2026-11-05-phase-1a-validation-design.md) was approved on 2026-11-05.

- [Phase 1A implementation plan](2026-11-05-phase-1a-validation.md): detailed tasks, minimal tests and separate-session handoff; completed with qualified local validation; see its report.

The [Phase 1B UI specification](../specs/2026-11-06-phase-1b-ui-design.md) was approved on 2026-11-06.

- [Phase 1B UI foundation implementation plan](2026-11-06-phase-1b-ui-foundation.md): six tasks, minimal verification and a separate-session implementation prompt. Merged through PR #2; see the Phase 1B validation report for actual checks and limitations.

The [Phase 1C.1 specification](../specs/2026-11-06-phase-1c1-cluster-connections-design.md) was approved on 2026-11-06.

- [Phase 1C.1 implementation plan](2026-11-06-phase-1c1-cluster-connections.md): seven tasks, focused tests, two-cluster validation and a post-implementation user UI UAT checklist. Merged through PR #3 after qualified agent UAT and maintainer authorization; see its report for coverage limits.

The [workspace refinement specification](../specs/2026-11-12-dark-palette-workspace-controls-design.md) was approved on 2026-11-12.

- [Workspace theme and controls plan](2026-11-12-dark-palette-workspace-controls.md): neutral dark palette, searchable namespaces and remembered detail splits, implemented in the same session at the user’s request. See the [validation report](../validation/2026-11-12-workspace-refinement.html). Further UI/UX discussion stays in this plan.

Plan completion tracking follows the [engineering documentation guidance](../../engineering/engineering.html#documentation-layout): mark verified completed steps `[x]` in the permanent plan as work finishes.

The [roadmap](../../roadmap/roadmap.html) owns phase order. This directory owns execution tasks. Future plans follow the Superpowers writing-plans workflow after the relevant design is approved. Keep scope, safeguards and permanent instructions in their owning documents and link to them.

Implementation requires an explicit request after plan review; the user may authorize execution in the same session. Later milestones are not authorized by the Phase 1A plan.

The [Phase 1C.2 workload investigation specification](../specs/2026-11-12-phase-1c2-workload-investigation-design.md) was approved on 2026-11-12.

- [Phase 1C.2 implementation plan](2026-11-12-phase-1c2-workload-investigation.md): seven tasks for bounded workloads/details/events/logs, focused tests and a separate HTML user UI UAT checklist. Ready for review and implementation in a separate session; no implementation performed by this planning update.
