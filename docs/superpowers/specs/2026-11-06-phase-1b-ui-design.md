# Phase 1B — UI foundation

**Status:** Draft for user review. No implementation authorization.
**Date:** 2026-11-06
**Prerequisite:** reviewed Phase 1A architecture gate before implementation; planning can proceed while its final task finishes.

## Intent

Build the desktop workspace before expanding the live Kubernetes explorer. An engineer should understand the current cluster, namespace, selected resource and available actions at a glance, on a MacBook or an external display. Preserve the approved explorer visual direction and right-side AI interaction. Avoid a dashboard crowded with unrelated panels.

The [roadmap](../../roadmap/roadmap.html#phase-1b) owns sequence: UI is now 1B, explorer 1C, operations 1D, administration 1E, OpenShift 1F and release readiness 1G. Historical labels are mapped there.

## Owning references

- [Approved mockups](../../ux/mocks/index.html): wide dark and laptop light references; one shared mock folder.
- [UX behavior](../../ux/workflows.html): interaction requirements, themes, target identity, sessions and future AI panel.
- [Architecture](../../architecture/overview.html) and [security](../../security/security-model.html): service boundaries and safeguards.
- [Engineering](../../engineering/engineering.html): source descriptions, layout and minimal meaningful checks.
- [Actual 1A validation](../validation/phase-1a.html): implementation readiness, not a screenshot-based assumption.

## Approach and tradeoffs

The user selected UI work before the next live explorer phase. This draft proposes a reusable shell with an explicit synthetic preview mode. It lets the user review real selection, resizing, keyboard and theme behavior before API expansion.

A static mock-only milestone would not validate those interactions. Building the full live explorer together with the UI would combine the two milestones the user asked to separate. Replacing the existing validation services would discard tested integration work. The proposed approach keeps stable 1A service interfaces and isolates preview data from live adapters.

**Proposal awaiting approval:** use synthetic previews to cover future screens, including AI, while connecting only already-validated services where that does not expand 1B. The detailed plan must list every live versus preview surface. Demo operation controls never invoke live mutation/session tools. Do not mix fixture rows into a live cluster list or silently substitute demo data on errors.

## Deliverable

A working desktop shell and reusable components, reviewed in the packaged app, with an explicitly labeled UI preview entry point for unfinished workflows. This is product UI code in the later implementation session, not a replacement set of static images. This document itself is planning only.

### Shell and navigation

- Independent cluster tabs, cluster/context and namespace identity, resource navigation, list/detail workspace and bottom Sessions dock.
- Distinguish all namespaces, one namespace and cluster scope. User-assigned environment labels are independent of connection/health status; no inferred production labels or fabricated alerts.
- Preserve selection, filters, detail tab and scroll per cluster tab. Settings is reachable without discarding workspace state.
- Organize navigation for workloads, networking, configuration, storage, cluster/RBAC resources, custom resources, Helm and OpenShift. The exact group labels/order are builder choices subject to user review. Unimplemented destinations identify their status clearly.

### Resource and evidence surfaces

- Reusable table with selected-row identity, filter presentation, status, empty/loading/error states and detail navigation.
- Pod preview: Overview, Events, Logs and YAML; expose future Terminal and supported action affordances through explicitly labeled previews. Use synthetic failing-Pod evidence consistent across views.
- Log layout supports container/previous selection, follow state and error gutter styling. Fixtures demonstrate states; real stream lifecycle remains owned by services.
- Generic resource detail/YAML pattern for kinds without a dedicated page. Show resource identity without relying on truncated table cells.
- Review-dialog component shows the frozen action target and intended change. In preview mode, confirm/cancel only changes synthetic UI state and clearly reports simulation.

### Sessions and keyboard

- Bottom dock supports log, terminal and forward session presentations with explicit targets, state and stop controls. Collapse preserves sessions; confirmed tab closure follows the owning UX contract.
- Reuse 1A session adapters only where ready; future previews must not pretend to start a real terminal/forward.
- Arrow-key row selection, command palette and shortcut legend operate on the selected resource. Inputs/editors/terminals keep their keystrokes. Destructive action shortcuts still open review.
- The plan selects exact keybindings after checking OS and editor conflicts; do not blindly copy mock shortcut labels.

### Themes and adaptive layout

- System default with persistent Light/Dark overrides, changed only in Settings. System mode reacts to OS changes; appearance does not reset workspace state.
- Use shared semantic tokens for surfaces, typography, spacing, borders, selection, focus and status. Status meaning is conveyed with text/icons as well as color.
- Wide displays support side-by-side list, details and future AI. Laptop widths can stack list/detail within the left workspace; AI remains on the right. Users control navigation/dock collapse.
- Proposed validation sizes: 1440×900 and 1920×1080, plus a candidate minimum 1100×700 logical window size. These are review targets, not a proven minimum. Measure usable pane widths before finalizing the minimum; revise the spec if the required layout cannot fit legibly.
- Clamp restored window bounds and pane sizes after monitor changes. Preserve selection, sessions and focus through resizing; use scrolling rather than tiny text.

### AI design preview, integration in Phase 2

- In the labeled preview, Ask AI is available on every page and opens the same resizable right-side panel without replacing the current screen. In normal live mode, it clearly identifies Phase 2 availability and makes no provider requests.
- Demonstrate target/no-target states, provider/model presentation, investigation bound to its original target across navigation, timestamped citations and evidence inspection using synthetic data.
- Demonstrate redacted-payload preview and sent-payload layout with explicit synthetic labels; do not claim data was sent or actually sanitized by a functioning AI pipeline.
- Store preview conversation state only for the preview session. Do not introduce real API-key setup, model SDKs or outbound AI requests in 1B.

## State and integration boundaries

Keep presentation state separate from domain services and the Wails bridge. Per-tab navigation state must refer to explicit connection/resource identity; callbacks must not read a mutable global current context to choose a target. Prepared actions retain their original identity after a tab switch.

The implementation plan should inventory existing frontend components and bridge/session contracts before proposing file changes. Retain useful 1A code, adapt it behind presentation interfaces and avoid parallel client implementations. Preview adapters must be explicit and isolated so 1C replaces data sources without rebuilding the shell.

Persist user preferences such as theme and valid window/pane geometry. Transient fixture selections and AI preview content are not production preferences. Do not introduce new credential persistence or silently restore terminal/forward connections from UI state.

## Non-goals

Full live resource discovery/browsing, new Kubernetes mutation workflows, Helm/Operator integrations, real metrics queries, cloud-auth compatibility claims, AI requests, cross-platform packaging and a comprehensive end-to-end suite. Later milestones own these capabilities. Do not turn unavailable controls into apparently successful operations.

## Acceptance and minimal verification

1. User can navigate the synthetic failing-Pod workflow in both themes and recognize the approved mock hierarchy.
2. Two tabs with identical resource names preserve independent namespace/selection state; switching tabs while a review is open does not change its target.
3. Settings theme follows the OS when System is selected and preserves state on theme changes.
4. Laptop/wide/minimum-candidate layouts remain readable, keyboard accessible and scrollable; resizing and monitor-bound restoration do not strand controls.
5. Sessions collapse/restore without losing their UI state; explicit close/stop behavior identifies the correct session. Live adapter cleanup remains covered by applicable existing service checks.
6. AI preview opens on the right from resource and settings pages; navigation does not silently retarget the sample investigation. No real AI request is made.
7. Preview mode and unavailable features are unmistakable; forbidden, disconnected, stale, unsupported, empty and loading states have distinct presentations.

Use existing type/build checks, a small state test covering tab/target isolation and a focused manual packaged-app walkthrough with screenshots at recorded sizes. Add another automated check only for a material behavior risk not already covered. No screenshot suite or exhaustive component tests are required merely to match fixtures.

## Approval and subsequent planning

Review the proposed preview boundary and candidate window sizes. After approval, create the detailed Superpowers implementation plan here in the planning session or in a later planning session; no need to rediscover the design in the implementation session. The plan should sequence tokens/components → shell/state → resource/session previews → settings/adaptive behavior → AI preview → focused validation, with reviewable tasks and explicit files after inspecting the current tree.

The implementation session must receive the approved spec, plan, current 1A report and engineering instructions. Completion produces a UI validation report with tested dimensions, screenshots, live/preview boundaries and remaining issues. User UI review is the handoff gate to 1C.
