# Phase 1C.2 — Workload investigation

**Status:** Approved by the user on 2026-11-12; implementation belongs in a separate session.
**Date:** 2026-11-12
**Scope:** Pods, Deployments, resource details, read-only YAML, events and container logs.

## Intent and acceptance journey

An engineer selects a connected cluster and namespace, finds a failing Pod or Deployment, and reads its actual state, configuration, events and relevant container logs without leaving the workspace. Present evidence clearly; do not invent an AI diagnosis or require access to every resource type before showing permitted data.

The first complete journey is: choose cluster → choose namespace → select a failing Pod → inspect conditions/container failure → inspect YAML and events → read current or previous container logs. A second journey starts at a Deployment, follows its owned Pods and reaches the same Pod investigation. The result should let the user explain what they observed, not claim every failure has a confirmed root cause.

## Owners and implementation baseline

Read the [roadmap](../../roadmap/roadmap.html#phase-1c), [UX](../../ux/workflows.html), [architecture](../../architecture/overview.html), [security model](../../security/security-model.html), [engineering practices](../../engineering/engineering.html) and [local development guide](../../engineering/local-development.html). These own enduring requirements; this spec defines the increment.

Repository baseline inspected: `develop` at `c690a30`, after Phase 1C.1 merge `395ef7a`, workspace refinement PR #4 merge `d73f9c4`, and version 0.3.0 release history. The current macOS app name is KubeSift. Do not plan from the old synthetic UI or interpret earlier checkpoints saying PR #4 is unmerged as current truth.

Preserve the [approved workspace refinement](2026-11-12-dark-palette-workspace-controls-design.md) and its [qualified validation](../validation/2026-11-12-workspace-refinement.html):

- Neutral dark/System-dark palette and current Light appearance, semantic phase colours, accessible text/focus contrast and flat surfaces.
- Compact two-row navigation; connection disclosure with source/context/last read and Reconnect/Disconnect; directly available Cancel/Retry when needed.
- Search inside the namespace popover, explicit selection and manual namespace entry when discovery is incomplete. Typing alone never sends requests.
- Remembered draggable list/detail split independently for side-by-side and stacked layouts; preserve version-2 appearance/geometry preferences.
- Refresh beside the resource heading, sticky table headings, compact detail rows and expandable Identity & source. Do not reintroduce repeated namespace subtitles in single-namespace rows.

Current product code has live basic Pod summaries and single-namespace connection state. `Workspace.tsx` still keeps the selected kind in component state; move kind/detail/navigation state into its owning cluster tab so A/B switches do not mix or reset it. `internal/kube/read.go` rejects lists beyond 100 Pods. `internal/kube/streams.go` and the existing session manager already supply guarded, bounded log/watch infrastructure, but product log sessions are not wired and log targets currently lack UID protection. The product Sessions placeholder is not a real log controller. The connection coordinator's Pod polling must not overwrite new independently owned workload pages/details.

## Approach

**Approved approach:** extend the current typed Pod services, add typed Deployment/ReplicaSet reads, and reuse the owned session transport for logs. Preserve current presentation components and explicit connection generations. For inventories/details use bounded page reads, manual Refresh and an optional visible-page auto-refresh; do not introduce a new cluster-wide cache or generic informer framework in this increment.

Alternatives considered: building generic discovery first delays the requested investigation journey and belongs to 1C.3; building cluster-wide watches plus cache infrastructure now increases synchronization/large-list complexity without being required to inspect one workload. The chosen scope is the user's requested Pod/Deployment investigation; exact bounded defaults below are approved design decisions.

## Scope boundaries

**Included:** Pod/Deployment inventories and filters, single explicit namespace, basic page navigation, detailed overview, Deployment-owned Pod navigation, read-only YAML, resource-specific events, current/previous container log snapshots/follow, real log sessions in the existing dock, UI UAT deliverable.

**Not included:** editing/applying YAML, restart/scale/delete/rollback, terminal or forwarding product workflows, aggregated Deployment/multi-Pod log streams, AI requests/diagnosis, metrics, arbitrary custom-resource discovery, all-namespace browsing, persistent evidence history, downloading/exporting entire resources/logs, or another visual redesign. Existing backend operation capabilities may remain for tests but are not exposed as shortcuts in these views.

## 1. Pod and Deployment inventories

Switch between Pods and Deployments in the existing resource navigation. Preserve kind, filter, page, selected UID, detail tab and scroll independently per connection tab and kind. Changing namespace explicitly invalidates outstanding inventory/detail requests, resets the page and selection for the new scope, and leaves explicitly opened log sessions attached to their original targets in Sessions. Connection-generation changes invalidate all data/request ownership for that connection.

Pod columns: name, Phase, Ready count, restarts, age and node where space permits. Distinguish the API phase from container waiting/termination reason: a Running Pod with CrashLoopBackOff must not look healthy solely because of its phase. Retain phase dot colours (Running blue, Succeeded green, Pending amber, Failed red, Unknown gray), separate readiness and show an actionable waiting/termination reason as a text status. Represent missing/unavailable data as such, not zero.

Deployment columns: name, ready/desired, updated, available and age. Preserve zero desired replicas as a valid scaled-to-zero state, not failure. Interpret conditions in Overview; do not label a Deployment complete from ready count alone when its observed generation lags its desired generation.

Use page size 100, with explicit Next and a page count/current range rather than an invented total. Retain only the current page and bounded cursor history (20 pages per tab/kind); a history limit offers restart pagination, not a false end of collection. Cursor identity includes connection generation, kind, namespace and server-side query. Expired cursors produce “List changed; refresh” and restart from page one only through explicit Refresh. Do not append incompatible snapshots or fall back to unbounded list calls. Kubernetes continuation behavior is described in the [API concepts reference](https://kubernetes.io/docs/reference/using-api/api-concepts/).

Name text filtering and sorting operate on the displayed page and are labeled “Filter this page”; do not imply that zero matches means no matches in the namespace. Server-wide name substring search and broad selector building are deferred. Refresh begins a fresh page-one snapshot. Preserve selected resource by UID when still present; if not present on the refreshed page, clear selection without claiming deletion unless a targeted read confirms it.

Auto-refresh is off by default. When enabled for the visible inventory, refresh page one at most every 15 seconds, with one request in flight. Pause auto-refresh on later pages with a visible explanation so rows do not jump under the user; hidden cluster tabs do not poll their inventory/details. This is separate from existing lightweight connection status checks. Detail tabs have explicit Refresh and collection time; they are not silently claimed as a coherent transaction with every list refresh.

## 2. Pod details

Replace the “More resource details are not available yet” placeholder with Overview, YAML, Events and Logs tabs. Fetch only the active section; a forbidden section does not erase permitted sections. Keep context/namespace/resource name visible and keep object UID and metadata available in YAML; omit the separate Identity & source disclosure per UAT feedback.

Overview contains:

- Phase, readiness, restart counts, node, Pod IPs, QoS, start/creation/deletion time and actual conditions/reasons/messages.
- Regular, init and existing ephemeral containers, explicitly grouped. Show images, ready/started state, restart count, current waiting/running/terminated state and last termination reason/exit code/time.
- Requests/limits, configured probes, volumes and owner references as configuration, not measured usage. Secret/ConfigMap references show reference names/keys without fetching their values.
- Labels and annotations in expandable sections; render all cluster content as inert text. Avoid automatically expanding enormous metadata.

Missing optional fields remain absent/unknown. Terminating is a lifecycle indication from deletion metadata, not a new Pod phase. A completed Pod with no available logs still has useful status. Keep container selection scoped to the current Pod UID; never carry a same-named container to a different Pod implicitly.

## 3. Deployment details and owned workloads

Deployment detail offers Overview, YAML, Events and Pods. There is no direct Deployment log endpoint in this product: the Pods tab selects an actual Pod and then its container logs. Do not pretend one Pod's logs represent all replicas.

Overview shows desired/updated/ready/available/unavailable counts, observed versus desired generation, conditions and reasons, selector, template images, rollout strategy, and relevant resource/probe configuration. Rollout history and rollback remain later operations work.

Resolve relationships through controller ownerReferences: Deployment UID → ReplicaSet controller owner UID → Pod controller owner UID. Label matches may narrow candidates but must not prove ownership. Reuse typed ReplicaSet reads only for this relationship, not a new standalone ReplicaSet explorer commitment. Filter out lookalikes owned by another Deployment, including equal labels or reused names.

Bound related-object pages. Automatically advance through up to eight empty ownership pages per request so opening Pods reaches actual Pod rows without a discovery-only click. Stop at a nonempty Pod page, completion, error or that bound; offer Load more Pods only when continuation remains, without silently scanning an entire namespace. A Pods row identifies its ReplicaSet and phase/readiness. If ReplicaSet/Pod access is forbidden, keep the Deployment overview and show that relationship evidence is unavailable. Empty partial results are not “Deployment has no Pods.” Open a related Pod in the same cluster tab with an explicit Back to Deployment breadcrumb; preserve its original namespace/UID, and return to the Deployment only if that captured identity remains valid.

## 4. Read-only YAML

Retrieve Pod or Deployment from the selected connection and namespace and validate its UID before presenting YAML. Display the current server object, not the original local manifest; server serialization cannot promise original comments/key ordering. Include resourceVersion and fetched-at metadata outside the YAML viewer.

Default presentation omits managedFields for readability, with an explicit Show managed fields toggle. Label this omission; the toggle must not fetch another target or silently mix versions. Provide selectable text, syntax highlighting if safely supported by existing tooling, search within the displayed text and an explicit Copy displayed YAML action. No Apply/Edit/Save controls. Copy means exactly the displayed representation; never claim it is the original manifest or a secret-free export.

YAML remains local and subject to Kubernetes authorization. Workload objects may contain literal environment values or sensitive annotations: do not automatically fetch referenced Secrets, transmit YAML externally, persist it to disk or put it into diagnostic logs. Explain locally that the view may contain sensitive values; do not falsely promise complete redaction. Limit a detail object/YAML representation to 2 MiB before crossing the bridge; oversized data yields an explicit size-limit response, not silently truncated or invalid YAML. Additional server-response decode limits must be bounded during implementation; do not allocate unbounded objects then merely truncate display.

## 5. Resource events

Events are attached to the selected object's UID and namespace, not just its name. Use one normalized event representation, preferring events.k8s.io/v1 where supported and falling back to core/v1 only for unavailable API support; never turn forbidden into an automatic attempt to bypass RBAC through another endpoint. Avoid querying both families and displaying duplicates.

Show type/severity, reason, message, count/series count, first/last observed time and reporting component where present. Normalize optional timestamps with a documented fallback; unknown values remain unknown. Deduplicate updates by event UID, not message text. Order the displayed page by most recent observed timestamp, with a stable tie-breaker.

Fetch bounded UID-filtered pages (100), clearly label current-page sorting and additional-page availability. Do not claim the first page contains the globally latest events when the API did not guarantee that ordering. Show captured time and Refresh; no background event stream is required here. A Deployment Events tab contains its own events; related Pod events are inspected on that Pod, avoiding a misleading mixed timeline.

“No events returned” is not “no failure”: retention and permissions can leave gaps. Distinguish empty, forbidden, unavailable API, stale and request failure. Log lines and Kubernetes events remain different evidence types.

## 6. Container logs and Sessions

Open Logs for an explicit Pod/container. Offer regular/init/existing ephemeral containers with their current state. Select the only eligible container automatically; when several exist, show a clear selector and retain the user's choice only for that Pod UID. No command execution, container creation or shell access is introduced.

Controls: container, Current/Previous instance, initial tail (default 200, maximum 1,000 lines), server timestamps, Follow toggle, Stop, Clear view and search within retained text. Initial current logs load as a bounded snapshot with Follow off; enabling Follow creates an owned stream. Previous always disables Follow and explains when previous logs do not exist. Current/previous are server availability, not an archival history; see [Kubernetes logging](https://kubernetes.io/docs/concepts/cluster-administration/logging/).

Every opened request/stream has a native session ID and captured connection generation, namespace, Pod name, Pod UID, container and current/previous mode. The existing `model.Target` lacks UID: the implementation plan must extend the log-facing contract and validate selected UID/container against a fresh Pod read before opening or restarting a stream. Reconnect/restart must never silently attach to a replacement Pod with the same name. Kubernetes log requests address Pods by name, so a pre-read cannot create an atomic UID precondition; recheck after stream opening and periodically while following, terminate if identity changed, and label any identity-verification failure honestly. Do not claim this removes all races in the server API.

Use the existing pending-session/register-before-activate, credit/acknowledgement and bounded-buffer lifecycle. Create no second log reader for the same visible session. The Logs detail tab and dock reference the same session/store; switching detail tabs must not duplicate subscriptions. Selecting a different Pod does not retarget an existing log session. Namespace/kind/tab navigation keeps explicit sessions visible in the global dock with their original identities.

Approved bounds: maximum eight log sessions application-wide (including retained stopped snapshots); 1 MiB retained UTF-8 text per session; bounded native queue using existing session limits; a 64 KiB display chunk/window with navigation/search over retained text rather than an unbounded DOM. Bound incomplete lines and decode split UTF-8 sequences correctly. Initial snapshots request a byte cap as well as line tail; follow uses stream backpressure/retention rather than a total-lifetime byte cap. Surface truncation/dropped bytes and time gaps; never silently imply complete logs. Adding a ninth session asks the user to close one, never evicts another cluster's session without consent.

Stopping cancels native work while retaining bounded displayed text; closing releases both native ownership and retained text. Clear view empties display history without restarting the stream or resetting the target, and does not erase loss indicators. Pause auto-scroll is a presentation action, not Stop. Current snapshot EOF is a normal completed state; permission/server errors are not “no logs.”

If Follow ends unexpectedly, retain content and error/gap information and offer explicit Resume after connection recovery; no automatic reconnect of log streams in this increment. Resume starts a new explicitly identified segment after UID/config validation and warns that overlap/gaps are possible. Changed credentials require the existing external-login and Retry workflow. Closing a cluster tab with log sessions confirms the affected sessions and cancels them; Settings navigation, theme changes and dock collapse do not stop or recreate them. App exit releases every native stream and listener. Streams may continue in inactive cluster tabs, but heavy offscreen rendering pauses without breaking native flow control.

The Phase 1A backgrounded WebKit log-delivery/drop limitation remains a known risk. Validate the new product path for visible loss and bounded recovery; do not claim lossless capture, archival storage or a monitoring backend.

## Data ownership, errors and architecture

Use immutable resource references: source/context through issued connection ID+generation, API group/version/kind, namespace, name and UID. A request also captures section/query/page and request epoch. Late responses cannot overwrite a different selection, namespace, kind, generation or replaced UID. Backend UID validation applies to details, YAML, events and log starts, not just React checks.

A resource deleted/recreated under the same name produces “Resource replaced; select again.” Retained data is explicitly stale with fetched time. RBAC denial is section-specific; it does not discard the rest of the workspace. Transport/auth failures use the existing classified connection states. No native credentials or raw helper output cross the bridge.

Go owns typed API calls, bounded response handling, ownership validation and session lifetime; React owns navigation and display. Extend existing services/DTOs rather than introducing a second connection manager. Detail/YAML/event caches are session-memory only with bounded retained entries; closing a tab clears its resource cache. Request deadlines default to 30 seconds except explicitly stopped/cancelled log follows. Inactive detail requests are cancelled; no polling per resource on every cluster.

Separate connection-health evidence from workload inventory: `ConnectionView.pods` must not repeatedly replace a manually paged Pod/Deployment list or a selected detail object. Preserve current status/menu/source/trust behavior while introducing per-kind inventory state. A selected detail section has its own loading/error/fetched-at state so a failure in logs cannot blank Overview/YAML.

Implementation planning must settle exact DTO/file decomposition, backward-compatible changes to session targets and safe event API detection using pinned dependencies. UI/component organization remains builder discretion; changing the compact layout, palette, namespace semantics or stored split behavior is outside scope without a concrete need and review.

## Fixtures and minimal verification

Reuse `scripts/dev-clusters.sh --explorer` and the two approved clusters. Inspect existing fixtures first; add only what is missing: healthy and crashing Pods, multi-container/init-container evidence, a Deployment with owned ReplicaSet/Pods, a same-label non-owned Pod, same-named resources with different UIDs/markers on A/B, and a safe restricted-reader identity. Keep existing shop/billing intact. No new clusters, metrics stacks or cluster-wide role grants just to make tests pass. Ephemeral-container absence can be tested with synthetic objects; no need to inject one into a live workload.

Keep automated tests representative and behavior-focused:

- One table-driven service suite for UID ownership, Deployment relationships, partial RBAC, page/cursor identity, event normalization and bounded YAML; reuse existing fakes.
- One focused log-lifecycle regression group for fixed target/container, previous-without-follow, bounded decoding/retention, stop/close and identity change. Extend existing session tests rather than mirror them.
- A small frontend state test for A/B isolation, late detail responses, per-kind state and one log session shared by detail/dock.
- One opt-in two-cluster investigation walkthrough plus a short packaged-app manual matrix. Keep fixtures synthetic; no snapshots, per-component test explosion, coverage target or new native UI automation framework.

Use additional regression tests only for actual defects or a material uncovered risk. Run existing applicable repository checks and packaging; repeat race/load checks when the changed stream/ownership path justifies them. No need to reproduce all historical 1A performance experiments to validate unrelated presentation changes.

## Acceptance and UI UAT deliverable

| ID | Acceptance evidence |
| --- | --- |
| C21 | Pod and Deployment pages show real namespace-scoped data, truthful readiness/reasons, page bounds and page-local filtering; A/B navigation state is independent |
| C22 | Pod overview exposes conditions/container state/last termination and references without retrieving Secret values; partial RBAC does not erase usable evidence |
| C23 | Deployment overview and owned-Pod navigation follow UIDs, exclude label lookalikes and disclose incomplete relationship access |
| C24 | YAML is read-only, version/time labeled, selectable/searchable, managedFields omission explicit; Copy copies displayed representation only |
| C25 | Events match selected UID, normalize/deduplicate correctly and distinguish missing/forbidden/stale data; no false globally sorted claim |
| C26 | Current/previous multi-container logs work where available; Follow/Stop/Clear/scroll have distinct semantics and visible bounded loss |
| C27 | Sessions survive theme/Settings/navigation without retargeting; close/tab-close/exit clean up; replacement Pod or changed generation never silently rebinds |
| C28 | Compact header, current palette, namespace popover, phase/readiness distinction and saved split sizes remain intact at existing reviewed sizes |
| C29 | Failing-Pod and Deployment→Pod investigations complete using actual fixture evidence, with no AI or mutation calls |

After implementation create `docs/superpowers/validation/phase-1c2.html` for agent technical results and `docs/superpowers/validation/phase-1c2-ui-uat.html` for user acceptance. Use the actual implemented labels and navigation. The UAT document is HTML, readable/printable, with approximately 12–15 concise cases: prerequisites, steps, expected visible result, Pass/Fail/Not tested/Blocked and notes; include tested app commit/version/date/OS. Default all user results to Not tested and keep agent results separate. Provide print/save-as-PDF instructions and disclose whether results persist.

Core UAT cases use only the local fixtures, with exact namespaces and instructions to discover current Pod names. Optional restricted-access/previous-log/deletion-race cases need safe prepared prerequisites or Blocked, not instructions to break normal credentials or delete shared workloads. Include current UI regression checks and cleanup, not a new exhaustive screen-size study. No full resource/log payloads in public evidence; curated synthetic screenshots only.

The implementation session commits/pushes its feature branch and opens the PR to develop, leaving it unmerged pending user UAT unless the maintainer explicitly authorizes a qualified merge. Record remaining platform/provider/stream limits. No 1C.3 work or release is authorized by this spec.

## Approval and next step

The user approved this specification on 2026-11-12. The [implementation plan](../plans/2026-11-12-phase-1c2-workload-investigation.md) defines seven tasks, minimal behavior-focused checks and the separate UI UAT deliverable. Implementation occurs in a separate session; no application code, cluster mutation or new fixture installation occurred during planning.

## UAT refinement · 12 November 2026

User requested desktop-work-area maximization at launch (not macOS full screen), aligned inventory pagination/Auto-refresh and removal of the inventory capture timestamp. Follow the [owning UX behavior](../../ux/workflows.html#detail-divider) and its Themes and window sizes section. Saved display placement and pane proportions remain; detail evidence timestamps and stale-data warnings remain. These are Phase 1C.2 UAT fixes, not Phase 1C.3 scope.

Follow-up UAT: use explicit work-area dimensions on first native show instead of native zoom; remove the separate Identity & source disclosure and internal relationship timestamp/completion copy. Ownership/UID validation remains native and unchanged.

Logs UAT refinement: prioritize output with compact controls, conditional Stop/Resume, expandable Search logs and conditional Earlier/Later. Remove visible UUID/generation, repeated detail resource path and routine buffer statistics. Keep concise target labels in the cross-target Sessions dock and retain actual errors, omitted-text and gap warnings. Internal immutable targets and bounds are unchanged.
