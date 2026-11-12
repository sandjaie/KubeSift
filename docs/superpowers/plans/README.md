# Superpowers plans

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
