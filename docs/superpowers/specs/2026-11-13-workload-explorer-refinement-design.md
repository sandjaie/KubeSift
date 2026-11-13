# Workload explorer refinement design

**Date:** 2026-11-13. **Status:** Requested planning package; scope and visual direction accepted in discussion, written specification awaiting final review. No implementation authorization.

## Intent and decisions

Make workload investigation readable, preserve the selected workload kind when changing namespace, and expose ReplicaSets as first-class read-only resources. The user accepted the compact Events and resource-detail mockups, requested the full ReplicaSets explorer, and assigned implementation to a separate session. Logs is being implemented concurrently; this package owns only the shared literal-search requirement for that surface.

This is a targeted follow-up to [Phase 1C.2](2026-11-12-phase-1c2-workload-investigation-design.md), not authorization for all of 1C.3. The [roadmap](../../roadmap/roadmap.html) owns phase placement; [UX workflows](../../ux/workflows.html) owns interaction rules; [security](../../security/security-model.html) owns safeguards. Existing constraints remain unless explicitly replaced here.

### Approaches considered

1. Compact tables, structured facts and expandable secondary details: selected by the user after reviewing interactive mockups. Best for scanning conditions, messages and counts without losing evidence.
2. A denser version of the current prose list: smaller change, but long messages and repeated metadata remain difficult to compare; not selected.
3. ReplicaSet links only within existing detail panes: smaller scope, but excludes independent browsing. The user explicitly selected the full sidebar explorer plus relationship navigation.

### Scope

- Pod and Deployment Overview, YAML, Events and related Pods redesigns.
- Workloads → ReplicaSets inventory; ReplicaSet Overview, YAML, Events and Pods.
- Deployment → ReplicaSets tab; Pod owner and related ReplicaSet links; bounded return navigation.
- Namespace changes preserve the selected kind.
- Technical search/filter inputs do not suggest spelling corrections or modify typed identifiers.
- Existing shell, neutral light/dark tokens, compact controls and remembered detail split remain.

Excluded: Logs redesign/collection changes, scale/delete/edit/apply/rollback, rollout-history claims, metrics, unrelated resource kinds, cluster infrastructure and release work. This session writes documentation and mockups only.

## 1. Shared detail presentation

Accepted density and hierarchy are preserved in the [durable mockup index](../../ux/mocks/index.html#workload-refinement), including interactive Pod/Deployment tabs and Events. Mockup behavior and sample values are illustrative; this specification governs implementation and edge states. ReplicaSets extends the same pattern without claiming a separately accepted screenshot.

Keep resource name, context, namespace and kind visible. Use the existing tab order; add ReplicaSets after Pods for Deployments. Refresh the active section explicitly. Loading, empty, forbidden, unavailable, failed and retained-stale evidence remain distinct; failure in one section must not erase permitted evidence in another.

Use compact section headings, aligned tables, subtle separators and disclosures. Colour supplements text and icons. Keyboard focus must remain visible, expansion controls must be keyboard operable, and long names/messages must be fully available through expansion or selectable text. At narrow split widths, wrap toolbars and use local table scrolling; never force the whole workspace wider or hide essential actions. Validate both themes, 200% zoom and keyboard use.

Move capture time and resourceVersion into subdued evidence metadata. Label resourceVersion literally, not as an application version. Show human-readable local dates/times with timezone; expose exact timestamps in selectable expanded metadata. Unknown/invalid timestamps remain explicitly unknown. Display formatting must not modify copied YAML or event messages.

## 2. Overview

### Pod

Lead with phase, ready containers, restarts and QoS. Running remains blue and must not imply readiness. Show termination lifecycle when deletionTimestamp exists without inventing a Pod phase. Structured facts include node, all Pod IPs, creation/start/deletion timestamps when present. Absent optional metadata is omitted or marked unavailable, never fabricated.

Conditions are rows containing type, actual status (True/False/Unknown), reason and message, with transition timestamps available in expanded details. Do not infer an overall diagnosis or apply a universal green=True rule to arbitrary conditions.

Group regular, init and ephemeral containers explicitly. Each row shows name/image, current state/reason, readiness and restarts. Expand for full image, started status, waiting/running/terminated fields, last termination reason/exit code/times, requests/limits and configured readiness/liveness/startup probes. Missing configuration says not configured; unavailable observations remain unknown. Requests/limits are configuration, not usage. Keep volumes, owners, labels and annotations in structured disclosures; do not reduce arbitrary structured data to only the illustrative mock's fields. References never trigger Secret reads.

### Deployment and ReplicaSet

Deployment summary: desired, updated, ready, available and unavailable; desired/observed generation stays visible in facts. Include conditions, selector, strategy, template containers/resources/probes, volumes and owners. Preserve zero replicas as a legitimate state. Lagging generation prevents a false completed-rollout interpretation.

ReplicaSet summary: desired, current, ready and available, plus fully labeled count if returned; generation, conditions, selector and template configuration follow the same patterns. It has no Deployment rollout strategy or direct Logs tab. Missing counts must not reuse Deployment-specific normalization without verifying ReplicaSet semantics. No invented availability conditions when the API returns none.

## 3. Events

Use Type, Reason/message, Count and Last seen columns. Default is All. Warnings filters only the loaded page; label counts as page-local. Counts in the toolbar count event records, while each row retains its occurrence/series count. Use an amber warning icon/text. Keep backend ordering; filters do not reorder events.

Collapsed rows show a readable message preview. Expand for the complete selectable message, reporter, exact first/last times and unknown values. One or multiple rows may be expanded; reset expansion on resource UID change. Do not automatically expand warnings in production merely because the illustrative mock does.

Footer: “Newest first on this page”, captured time and page navigation when available. Show end of results only after complete enumeration is established. Preserve existing UID scoping, deduplication, optional timestamps and API-family fallback rules. Empty results do not establish absence of failures; retain a concise retention/permissions explanation. No background event stream.

## 4. YAML

Read-only code surface with compact search, previous/next match actions, match position/count, Show managed fields and Copy displayed YAML. Enter moves forward, Shift+Enter backward, wrapping with clear feedback. Empty query and no-match states disable navigation appropriately. Searches operate on exactly the displayed representation; toggling managed fields recalculates matches without another fetch.

Use monospace, line numbers and restrained syntax highlighting rendered as inert text; never interpret cluster strings as HTML. Line numbers are not included in copied/selected source. Keep indentation and horizontal scrolling by default; optional wrapping is not required. The viewer fills available detail-pane height rather than a small manually resizable textarea. Avoid expensive highlighting or full DOM duplication near the existing 2 MiB payload limit; a plain-text fallback must be visibly labeled and keep search/copy correct. Prefer existing tooling or a small bounded presentation implementation; no editor dependency without a documented need.

Keep capture time/resourceVersion and server-representation explanation in a compact footer or information disclosure. Keep a brief sensitive-literal-values notice. Managed fields are omitted by default and the control shows whether they are included. Copy means exactly the displayed YAML, with success/failure feedback. No automatic export, persistence, redaction claim or fetching of referenced Secrets. Reset query, selection and notices on resource UID change; do not leave stale search positions after refresh.

## 5. Related Pods and ReplicaSets

Replace the current button-and-prose Pods list with rows: Pod name, phase, ready/desired containers, restarts and ReplicaSet. Pod and ReplicaSet names are separate explicit navigation targets. Preserve the direct Deployment → Pods route. ReplicaSet → Pods uses the same table; its owner column may be omitted when identical for every row and identified by the parent header.

Deployment → ReplicaSets shows name, desired/current/ready/available and age. Include zero-replica owned ReplicaSets without labeling them failed or claiming a rollout history. Owner links in Overview navigate only to supported kinds; unsupported owners remain readable metadata.

Labels may narrow requests; controller ownerReference UIDs establish relationships. Verify the selected parent UID before reading related children and verify the linked target UID on opening it. Same name/new UID must show replacement, not silently navigate to the replacement. Standalone ReplicaSets without a Deployment owner remain browseable.

Related views retain bounded pagination and partial-coverage messages. Load more replaces the current bounded batch; do not accumulate unbounded pages. Labels distinguish “N shown” from a total. Empty intermediate pages with continuation are not an empty relationship. Reuse the existing eight-step traversal bound and 100-object API page limit; no unbounded namespace scan. Permission denial for related Pods or a parent must not prevent permitted ReplicaSet inventory/details.

## 6. ReplicaSets inventory and navigation

Place ReplicaSets directly after Deployments under Workloads. Inventory columns: name, desired/current/ready/available, age; owner available in detail. Use the existing page-local name filtering, sort, refresh, optional visible-inventory auto-refresh, page size 100 and bounded cursor history of 20 pages. ReplicaSet API identity is apps/v1, namespace-scoped; no generic-discovery expansion is required.

Support Overview, YAML, Events, Pods using the same immutable ResourceRef contract as other kinds. Deployment additionally has ReplicaSets. Back navigation stores explicit source resource, section and scope; preserve the origin inventory filters/page/scroll while valid. A Deployment → ReplicaSet → Pod path supports Back to ReplicaSet then Back to Deployment. Cap relation navigation at eight entries, drop the oldest when exceeded, avoid duplicate entries for the same resource, and clear it on explicit sidebar selection or namespace/connection-scope change. Back validates UID before restoring detail; replaced, deleted or forbidden targets get an actionable message. Never infer target identity from mutable global context.

## 7. Namespace retention defect

Inspected `frontend/src/workloads/state.ts`: a changed scope reconstructs state with `kind: "Pod"`. `useWorkloads.ts` also falls back to Pod while the new scope is not yet installed. These are source-level findings, not a completed runtime fix.

On namespace change, preserve the current supported kind in that cluster tab (Pod, Deployment or ReplicaSet). Clear old pages/cursors, selection/details, query filter, scroll and relation history; restart the selected kind at page one. Keep the existing tab's auto-refresh preference for each kind, while obeying its visibility and in-flight rules. Default to Pods only when the tab has never selected a kind. A connection-generation change may retain the navigation preference but must discard all old evidence and requests. Empty/forbidden namespaces still stay on the selected kind.

No transient Pods read or visible Pods flash is acceptable during scope reconciliation. Cancel old reads; scope+epoch checks reject late responses. Other cluster tabs remain independent. Re-selecting the same namespace is a no-op.

## 8. Literal technical inputs

The screenshot's “Bootstrap” bubble is consistent with macOS text correction; the inspected LogViewer search input has no explicit spelling/correction opt-outs. Native reproduction is still required. Apple documents per-app automatic correction in its [typing guidance](https://support.apple.com/en-bh/guide/mac-help/mchlp2299/mac).

Technical search/filter inputs (logs, YAML, resource names and namespace query) must preserve exact entered text: no spellcheck suggestions, automatic correction, capitalization or browser autocomplete/history suggestions. Keep the intentional namespace result list and command matching; these are application navigation, not unwanted linguistic suggestions. Preserve IME composition, normal selection, paste and accessibility. Do not change system-wide macOS settings or suppress all keyboard events.

Start with field-level platform settings and verify the packaged macOS app with OS correction enabled. Escalate to a narrowly scoped WebView solution only if native evidence proves field settings insufficient. Logs integration is a coordination task after the concurrent work lands; do not rewrite its search/stream implementation.

## 9. Verification and delivery

Use synthetic data and only the documented kind-dev-local-a/b contexts with explicit context. Acceptance must cover failed and healthy Pods, unknown values, init/ephemeral containers, zero replicas, multiple generations, long strings, absent conditions, partial permissions, replaced UIDs, multiple pages, rapid namespace changes and two cluster tabs.

Deliver separate automated evidence and user UI UAT. Native checks include correction disabled in technical fields with OS correction enabled, YAML search/copy fidelity, narrow panes/zoom, keyboard focus, detail navigation and no Logs regression. No claim that browser-only checks prove macOS behavior.

Plans: [UI/navigation refinement](../plans/2026-11-13-workload-ui-refinement.md), then [ReplicaSets explorer](../plans/2026-11-13-replicasets-explorer.md). Implementation must start from the reviewed Phase 1C.2 baseline with concurrent Logs work reconciled. Current develop lacks that implementation; do not implement this package directly on that older baseline. No merge/release is authorized by this document.
