# Phase 1C.1 — Cluster entry and live connections

**Status:** Approved by the user on 2026-11-06. The [implementation plan](../plans/2026-11-06-phase-1c1-cluster-connections.md) is prepared for a separate session.
**Date:** 2026-11-06
**Scope:** Planning only in this session. Implementation takes place separately.

## Intent

Open KubeSift, choose an existing kubeconfig context, and enter a clearly identified live cluster workspace. Users should not need to understand development modes or credential internals to get started. Connection failures must explain what failed and support retry after the user fixes connectivity or renews credentials externally.

The user approved doing all entry/header/sidebar refactoring in 1C.1, placing connection configuration in Settings and removing the separate validation workspace from the product. Phase 1B is merged through PR #2. Its UI and Phase 1A services are the starting point, not a reason to preserve their temporary user journeys.

## Owning references

- [Roadmap](../../roadmap/roadmap.html#phase-1c): phase order and broader explorer coverage.
- [UX](../../ux/workflows.html#live-entry): approved cluster-first entry and connection status direction.
- [Architecture](../../architecture/overview.html): explicit identities and native service boundaries.
- [Security](../../security/security-model.html): helper trust, credential handling and RBAC.
- [Engineering](../../engineering/engineering.html) and [Git workflow](../../engineering/gitworkflow.md): source descriptions, minimal verification and publication.
- [Local development](../../engineering/local-development.html): only `kind-dev-local-a` and `kind-dev-local-b` for new routine development and validation.
- [1A results](../validation/phase-1a.html) and [1B results](../validation/phase-1b.html): actual validation and retained limitations.

This document specifies the increment's behavior and boundaries. The subsequent implementation plan owns exact file edits, signatures and task commands.

## Approach and alternatives

**Selected direction:** one product workspace, a context-list entry, and Settings for source/configuration management. Reuse the existing guarded Go connection, trust and session services; adapt the product UI to real connection state. Remove development-mode routing from normal startup.

Keeping a branded home page plus separate preview/live choices was rejected by the user as confusing. A separate 1B-2 visual patch would create another temporary flow; the user chose to deliver the corrections with actual connections in 1C.1. A generic resource explorer and all diagnostics at once would exceed this increment; the remainder stays in 1C.2/1C.3.

## Current implementation and required changes

- `frontend/src/App.tsx` currently switches between welcome, preview and retained validation UI. Product resource views use synthetic data. Settings links helper trust back to validation. Replace these product entry paths; do not relabel synthetic data as live.
- `internal/config/kubeconfig.go` reads kubeconfig without helper execution and preserves Kubernetes loading semantics. `LoadContexts` in the desktop service currently owns one replaceable snapshot. Multiple configured source groups need independently identified snapshots so refreshing source A cannot invalidate an unrelated context selection from B.
- `internal/connection/manager.go` issues explicit connection IDs/generations and guarded transports. `Open` creates an entry without proving API access. A returned reference must never alone cause a green Connected indicator.
- Existing Pod reads are a validation slice capped at 100 items. Reuse them only with honest limit handling in this increment, not as a complete explorer or silent pagination implementation.
- Existing preferences store only appearance/geometry in a strict version-1 schema. New source/label preferences need a reviewed migration or separate strict store; never put credentials or kubeconfig bodies in it.
- Keep existing session stop/cancel/ack/target safeguards. Do not let removal of the validation screen orphan hidden live sessions or accidentally expose its fixture mutation controls in the product.

## Product surfaces and journeys

### 1. Start and choose a cluster

Startup loads context summaries from configured sources locally. It does not open a cluster, execute a helper, probe every context or reconnect last-session tabs automatically. Show a useful searchable cluster list, not a slogan page. Default ordering is stable by context name with source as tie-breaker; favorites/recent ranking is deferred to avoid adding another persistence feature.

Each row shows context name, configured namespace, user-assigned environment label if any, and connection state. Source and sanitized endpoint are available in row detail to distinguish equal names. Show `Not connected` until this application has connection evidence; do not infer Local/Production from names or report cluster health from kubeconfig contents.

Selecting a row opens or focuses its tab and begins a connection attempt. The same `(source group, context name)` opens one tab, even on repeated clicks. Distinct source groups with matching context names remain independent. A connection failure stays visible in that tab/list entry with Retry and a safe explanation.

The cluster list remains reachable through a compact cluster selector beside tabs. This selector only chooses existing contexts. Close every tab to return to the list. With no contexts or a source error, give a clear empty/error state and a link to Settings; do not silently substitute fixtures.

### 2. Compact workspace header and navigation

Use two compact rows:

1. Small KubeSift brand, cluster tabs, existing-context selector, Settings and Ask AI/Hide AI.
2. Active context, connection indicator, namespace control and current resource location.

Preserve explicit target visibility on laptop and external displays, with tab overflow and scrolling instead of oversized padding or tiny type. Remove marketing copy, “Cluster workspace”, “Desktop ready”, raw preview IDs, inline label editor, phase labels and validation counters from normal UI. Technical connection IDs remain available only in useful detail, not as primary navigation.

Use distinct resource icons from one consistent icon set (or a small local SVG set selected during planning), paired with text. Do not use repeated diamonds or unexplained availability dots. Decorative icons are not announced by assistive technology; collapsed navigation has accessible names/tooltips. Preserve focus, theme and pane-size behavior established in 1B; do not repeat the user's closed-ended monitor-size investigation.

The AI button continues to toggle the same right-side panel on every product page. Until Phase 2, it honestly states that live investigation is unavailable; no synthetic diagnosis, provider key input, collection or model request is presented as working AI.

### 3. Settings owns configuration

Settings sections: Appearance; Kubernetes sources; Cluster labels; Credential-helper trust. Replace links to validation with actual settings controls. Returning from Settings preserves tabs, namespaces, selection and connection state.

Source behavior (design decision for this spec):

- A default source group uses Kubernetes default loading semantics, including `KUBECONFIG` when present and the conventional default file otherwise. Show the effective paths and explain the environment of a desktop launch; do not scrape interactive shell startup files.
- Additional files are added through a native file picker or explicit path in Settings. Each is a separately identified source group; within a group retain Kubernetes merge precedence. Do not merge unrelated added files into one ambiguous name list.
- Settings can add, disable/remove and reload sources. Removing a source never deletes or edits its kubeconfig file. The default group can be disabled and restored; it is not an editable token form.
- Show malformed/missing/unreadable sources separately; other valid source groups remain usable. A malformed source is not an empty successful list.
- Before disabling/removing a source with open tabs, show the affected contexts and require confirmation to close their owned connections/sessions. Cancel leaves them intact. No silent orphan tabs.
- Context names, endpoints, identities and credentials are read from kubeconfig. This increment does not create/edit kubeconfig contexts or accept pasted credentials. External CLI commands remain responsible for creating/updating them.
- Environment labels are user-assigned, persisted against stable source-group/context identity, and never affect authentication or server permissions. An endpoint/user identity change marks a stored label for review rather than implying the old environment classification is still reliable.

Persist only source references/enabled state and labels, alongside existing appearance/geometry through a strict versioned schema. Secrets, helper argument/environment values, certificates, tokens, resource data and connected state do not belong in preferences. Paths and labels stay local and are excluded from public fixtures. Preserve existing version-1 preferences during migration; invalid new settings get visible recovery with explicit reset, not silent overwrite.

### 4. Connect and report truthful status

Connection is an asynchronous, cancellable operation bound to source identity, selected context, configuration snapshot and attempt ID. A cancelled/older attempt cannot resurrect a closed tab or replace a newer result. Each established connection uses a backend-issued reference and generation; UI selection is never a backend routing mechanism.

| State | Indicator and behavior |
| --- | --- |
| Not connected | Gray dot and text; no current successful connection |
| Connecting | Amber progress and text; duplicate clicks do not start overlapping attempts |
| Review required | Amber review indicator; no helper executes until trust approval and explicit Connect |
| Connected | Green dot and text after a successful authorized resource read through the selected credential configuration; not merely `/version`, public discovery, client construction or kubeconfig load |
| Access restricted | Amber indicator when the initial read is forbidden and no successful authorized resource read is available; keep manual namespace choice and Retry available |
| Authentication required | Red indicator for an observed authentication rejection; do not assert expiry without evidence |
| Unreachable / Certificate error | Red indicator with the applicable network, DNS, timeout or TLS category; no suggestion to bypass certificate validation |
| Connection failed | Red indicator for unclassified failure, with safe detail and Retry |
| Stale / checking | Amber indicator when old connection evidence has expired or a resumed check is pending; show last successful check time |

A view-level forbidden response does not invalidate a previously proven connection or hide other accessible resources. A failing Pod does not change the connection indicator to red. Conversely, a connection-level authentication/transport failure does not leave the tab green because an older request succeeded.

Initial reads: attempt namespace listing using the selected identity. If denied, use the kubeconfig namespace (default when omitted) and allow manual namespace entry, then attempt a small Pod list there. Successful namespace or Pod reads can establish connection; if neither is permitted show Access restricted. Do not request permissions by attempting a mutation. An explicitly anonymous configuration cannot be described as an authenticated user: mark anonymous access visibly or unsupported rather than implying credentials were verified.

Proposed bounded defaults for implementation planning: a connection attempt has a 30-second overall deadline including helper acquisition; at most one status check per connection is in flight; opened connections without recent successful reads get a check at most every 30 seconds; evidence older than 90 seconds is stale. A periodic check repeats a previously successful bounded resource read, not an admin-only endpoint. Stop polling terminal authentication/trust failures until user Retry/review. Transport checks use bounded backoff, capped at 60 seconds. No checks for unopened contexts. These values are design defaults, not existing measured guarantees; validate their interaction with the owned helper timeout in the plan.

### 5. External login and Retry

The user runs `oc login`, AWS SSO authentication or Azure authentication in their own terminal. KubeSift does not run login commands, open an authentication terminal, accept a pasted login token or initiate browser/device login. Failure copy tells the user to refresh credentials externally, then select Retry. Do not construct executable login commands from untrusted context or helper strings.

Configured helpers such as credential-token retrieval commands may still execute under the existing remembered/revocable trust policy. A helper that requires interactive login fails with external-login guidance; no stdin prompting. An approved executable is not sandboxed, so the app must not promise to prevent every side effect of arbitrary trusted helper code. Review must make clear that a login command is not the intended helper configuration.

Retry reloads the selected source, resolves the context again and re-evaluates helper configuration. Changed endpoint, CA, user or credential configuration replaces the connection generation and invalidates pending work. The already-approved source policy governs additional files; Retry never searches unrelated contexts for working credentials. If the context disappeared, say so and return to selection/Settings.

Require a fresh helper review when the executable configuration changes; retain trust otherwise and expose revocation directly in Settings. Reuse the existing conservative revoke behavior (currently closes all live connections), explain that effect before confirmation, and reflect it in every affected tab. Narrowing revocation to a subset is not required here.

Only read state may resume after a successful reconnection. Never replay terminal input, commands or mutations, and do not automatically restore a previously stopped port forward. Reloading the source list alone does not silently change the target of an existing connection; changed active configuration must visibly invalidate/reconnect through the lifecycle above.

### 6. Namespace and minimal live content

1C.1 must end in a useful live workspace, not a green dot beside synthetic rows. Include real namespace selection and a basic live Pod list using the existing Pod-summary capability. Show actual names, namespace, readiness/phase and stable identity; display Loading/Empty/Forbidden/Disconnected distinctly. Selecting a Pod may show the basic fields the existing summary actually supplies.

When namespace listing is forbidden, manual namespace entry remains available and its read result establishes availability. Do not pretend an entered namespace has been discovered. Cluster-scoped navigation does not carry a misleading namespace; broader resource kinds can remain visibly unavailable until their increment.

Keep the current 100-resource validation limit visible: if the existing reader rejects a continued list, report that list capacity is not yet supported; never show 100 rows as the complete set. Production pagination, broad discovery, full Pod/Deployment details, YAML/events and integrated log browsing belong to 1C.2/1C.3. Product exec, forwarding and general mutation controls remain unavailable until their committed milestones; remove simulated operation controls from live views.

## Refactoring boundaries

Retire the normal App `welcome|preview|validation` mode choice in favor of product cluster-list/settings/workspace state. Move reusable connection/trust controls into the product components and native services. The live product must not mount `ValidationApp` invisibly or retain hidden fixture mutations, profile autorun or synthetic data behind normal navigation.

Preserve useful development diagnostics and prior tests in a development-only entry/harness excluded from the normal packaged route. Exact harness wiring is a builder decision; it must not create another product screen. Keep the existing session, guarded transport and prepared-operation services and their tests intact unless a concrete integration need requires change. Normal startup never activates profiling from a UI entry.

Use backend-owned source/snapshot records with bounded retention. A snapshot for B must survive a refresh of A; superseded snapshots for a source cannot be used for a new approval/connect request. Existing connections hold their own captured configuration until explicit replacement/invalidation. Frontend state uses stable source/tab identity plus operation attempt identity, never context name alone.

Secrets stay native. Return safe context summaries and structured error categories to the frontend, not REST config or raw helper output. Keep Go services independent of Wails. Exact service contracts, migration layout and component files are set in the implementation plan after this spec is approved.

## Local fixture work

Use only `kind-dev-local-a` and `kind-dev-local-b`; inspect existing resources before adding anything. Preserve current `shop` and `billing` resources. In 1C.1 extend the existing development fixture setup idempotently with same-named, differently marked Pods across the clusters and representative healthy/failing workloads to make connection/namespace/list behavior inspectable. Add the broader small inventory defined in the [local development guide](../../engineering/local-development.html#explorer-fixtures) as fixtures, not as new application capabilities.

Keep requests/replicas/log volume bounded; use synthetic Secret values; distinguish intentionally Pending storage or failed workloads from setup errors. CRDs and other cluster-scoped test objects need explicit collision checks and ownership. No extra clusters, monitoring stack, ingress controller or operator installation just to populate navigation. Creating fixture objects does not claim 1C.1 can yet browse every kind; later increments consume them. No fixture creation occurs in this planning session.

## Acceptance and minimal verification

| Case | Required evidence |
| --- | --- |
| C11 · Product entry | Packaged app starts with real local context list and no API connections/helper execution; no marketing/preview/validation choices; empty/error state points to Settings |
| C12 · Settings | Add/reload/remove a source; duplicate names remain distinct; valid sources survive another source's error; existing appearance survives migration; cancellation preserves open tabs |
| C13 · Explicit connection | Select A then B; independent tabs/status/namespace/basic Pod data; selecting A again focuses it; late A responses cannot overwrite B or reopen a closed tab |
| C14 · Auth and trust | Unapproved helper does not execute; approval requires explicit connect; changed helper reviewed again; revoke reflected across connections; synthetic rejected auth shows external renewal guidance; Retry reads refreshed config |
| C15 · Honest failures | Synthetic timeout/TLS/401/403 outcomes produce distinct safe states; forbidden namespace listing supports manual namespace; successful read required for green; loss/age invalidates old green state |
| C16 · UI | Compact header, meaningful icons, themes and keyboard selection remain usable at existing reviewed laptop/wide sizes; Settings/AI toggle preserve workspace; no new exhaustive size investigation |
| C17 · Cleanup and isolation | Cancelling connection, closing tab, removing source and app exit leave no owned requests/timers/sessions; old generation cannot update new tab; no commands/writes replay |
| C18 · Fixtures and boundaries | Both designated local clusters show distinct actual fixtures; resource limits/errors are honest; no simulated live data, login workflow, new cloud compatibility claim or AI traffic |

Retain existing tests. Add focused synthetic cases for source identity/reload, connection-state/late-result routing, settings migration and failure classification where the owning implementation changes. Prefer a handful of behavior tests at shared boundaries, not one test per control. Use one local two-cluster integration walkthrough and a short packaged-app checklist; no exhaustive UI/screenshot suite. Do not rerun the full Phase 1A load unless the change touches its stream/flow-control behavior. Keep public results synthetic and safe.

Real OpenShift/EKS/AKS login behavior remains separately unverified until an explicitly authorized environment is exercised. Tests with synthetic helpers establish contracts, not provider compatibility. Carry forward backgrounded WebKit log loss, sleep/wake, OS appearance transition and physical monitor-disconnect qualifications without relabeling them passed.

## Completion and handoff

After specification approval, use Superpowers writing-plans to create a detailed implementation plan. The separate implementation session builds only 1C.1, follows the source-description and minimal-test practices, and produces an HTML validation report with actual checks, screenshots, unrun environments and remaining limits. No root README progress tracking.

User review should demonstrate: launch → choose A → connected and actual Pods → choose B → independent workspace → explain an auth/network failure → Retry after correction. General explorer expansion remains 1C.2/1C.3; AI remains Phase 2. Planning documents are committed directly to develop; implementation uses a feature branch and the repository review workflow.

## User UAT deliverable · approved addition

After implementation, create `docs/superpowers/validation/phase-1c1-ui-uat.html` using the actual screens and labels. Include concise steps, expected results, Pass/Fail/Not tested/Blocked choices and notes for cluster selection, Settings, connection/error/Retry, tab and namespace isolation, real Pod data, navigation/icons/themes/keyboard, AI toggle and cleanup. User results start as Not tested and stay separate from the agent technical report. Provide safe local prerequisites and printable results; do not ask the user to disrupt normal credentials or shared clusters. User UAT remains pending until the user records it; leave the implementation PR unmerged for that review.
