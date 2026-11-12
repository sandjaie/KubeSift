# Phase 1C.2 Workload Investigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking. Implementation belongs in a separate session after plan review.

**Goal:** Investigate actual Pods and Deployments through overview, read-only YAML, events and bounded container logs in the existing workspace.

**Architecture:** Extend the native typed Kubernetes reader and reuse its connection manager and pending-session transport. Keep workload pages and section requests independent of connection-health polling. React owns per-tab navigation and one shared log store; Go enforces identities, permissions, bounds and cancellation.

**Tech Stack:** Existing pinned Wails/Go/client-go and React/TypeScript/Vitest toolchain. No framework upgrade or new UI library is required.

**Spec:** [Approved Phase 1C.2 design](../specs/2026-11-12-phase-1c2-workload-investigation-design.md), approved 2026-11-12. Planning baseline: `66e5739` on develop; incorporate subsequent planning commits before implementation.

## Global Constraints

- Read [engineering](../../engineering/engineering.html), [Git workflow](../../engineering/gitworkflow.md), [security](../../security/security-model.html) and [local development](../../engineering/local-development.html). Preserve the [workspace refinement](../specs/2026-11-12-dark-palette-workspace-controls-design.md); use its current code, not the synthetic 1B shell.
- Preserve neutral dark/Light/System, compact header, connection disclosure, searchable namespace selection, phase/readiness distinction and version-2 remembered splits. No visual redesign.
- Page size 100; at most 20 cursor-history entries per tab/kind. Auto-refresh off by default, at most every 15 seconds on visible page one, one request in flight.
- Detail/YAML representation maximum 2 MiB; request deadlines 30 seconds except active follows. Never persist workload payloads or log them diagnostically.
- Logs: default tail 200, maximum 1,000 lines; maximum eight retained sessions application-wide; 1 MiB retained UTF-8 text per session; 64 KiB display window. Previous never follows. No automatic stream reconnect.
- Only kind-dev-local-a/b for validation, explicit contexts everywhere, existing fixture scripts. No installed monitoring infrastructure or extra clusters.
- Tests remain small and behavioral. Reuse existing suites; no UI snapshots, per-component suites, coverage target or new native automation framework.
- Keep source descriptions and formatting current. Mark verified plan steps complete as work proceeds; ignored execution notes live under `.superpowers/sdd/phase-1c2/`.
- Work on `feature/phase-1c2-workload-investigation` from updated develop, in place. Preserve concurrent work. No application changes in this planning session; no next phase or release.

## Review Focus

- Reused resource names and delayed A/B responses must not display or resume another UID: task 1 service cases, task 4 identity checks and task 5 reducer case.
- Sparse status and zero-replica Deployments must remain truthful: task 2 service cases and task 7 native walkthrough.
- Expired pages or partial relationship permissions must not become a false empty collection: tasks 1–3 table cases.
- Oversized objects, split UTF-8 and newline-free logs must remain bounded without invalid YAML or silent data loss: tasks 2, 4 and 6 cases.
- Settings/theme changes, inactive tabs and late stopped-session events must not duplicate readers or leak sessions: task 6 lifecycle case and task 7 native cleanup.

---

## File and contract map

Existing `internal/kube/Service` remains the reader owner. Add focused files there rather than another connection manager. `internal/desktop/workloads.go` adds methods to existing `ExplorerService`, delegating through `s.core.kube`; register no parallel service. Generated bindings are regenerated, not handwritten.

| Files | Responsibility |
| --- | --- |
| `internal/kube/workload_types.go`, `workloads.go`, `bounded_reads.go` | Resource/page DTOs, typed list reads, bounded read-only client |
| `internal/kube/details.go`, `relationships.go`, `events.go` | UID-checked detail/YAML, Deployment relationships, normalized events |
| `internal/kube/logs.go`, existing `streams.go` | Product log API alongside compatible legacy validation API |
| `internal/desktop/workloads.go`, existing `service.go` | Thin cancellable read bridge and log lifecycle bridge |
| `frontend/src/workloads/api.ts`, `model.ts`, `state.ts`, `useWorkloads.ts` | Generated API seam, per-tab/kind memory, request ownership |
| Existing `workspace/Workspace.tsx`, `ResourceTable.tsx`, `ResourceDetail.tsx`; new `ResourceYaml.tsx`, `ResourceEvents.tsx`, `DeploymentPods.tsx` | Current workspace populated with live investigation sections |
| `frontend/src/sessions/logStore.ts`, `useLogSessions.ts`, `LogViewer.tsx`; existing `SessionsDock.tsx`, `bridge.ts` | One log subscription/store and bounded view shared by detail/dock |
| Existing `App.tsx`, `connections/useConnections.ts`, `connections/model.ts`, `internal/explorer/coordinator.go` | Compose stores and separate connection health from workload inventory |
| Existing tests plus new `internal/kube/workloads_test.go`, `frontend/src/workloads/state.test.ts`, `frontend/src/sessions/logStore.test.ts`, `tests/integration/workloads_test.go` | Compact representative behavior coverage |

Use JSON camelCase and generated TypeScript DTOs. Define these public contracts in `workload_types.go` (Go context is native only):

- `ResourceRef { Connection model.ConnectionRef; APIVersion, Kind, Namespace, Name, UID string }`; allow only Pod/v1 and Deployment/apps/v1 for detail endpoints.
- `PageRequest { Connection model.ConnectionRef; Kind, Namespace, Cursor string }`; opaque cursor scope binds generation/kind/namespace/query. `PageInfo { NextCursor, ResourceVersion string; FetchedAt time.Time }`.
- `WorkloadRow { Resource ResourceRef; Phase, Reason, Node string; Ready, Desired, Updated, Available, Restarts *int32; CreatedAt *time.Time }`; pointers preserve unknown versus zero. `WorkloadPage { Items []WorkloadRow; Page PageInfo }`.
- `DetailRequest { Resource ResourceRef }`; `ResourceDetail { Resource ResourceRef; ResourceVersion string; FetchedAt time.Time; ObjectJSON string }`. ObjectJSON is a capped typed-object serialization used only for local presentation, not arbitrary HTML; project overview fields in the frontend by kind. Do not retrieve referenced Secrets/ConfigMaps.
- `YAMLDocument { Resource ResourceRef; ResourceVersion string; FetchedAt time.Time; WithoutManagedFields, WithManagedFields string }`; both forms come from one captured object and combined bridge response must fit 2 MiB.
- `RelatedRequest { Resource ResourceRef; Cursor string }`; `RelatedPod { Pod WorkloadRow; ReplicaSet ResourceRef }`; `RelatedPage { Items []RelatedPod; Page PageInfo; Complete bool; Problem *model.Problem }`.
- `EventRequest { Resource ResourceRef; Cursor string }`; `ResourceEvent { UID, Type, Reason, Message, Reporter string; Count *int32; FirstAt, LastAt *time.Time }`; `EventPage { Items []ResourceEvent; Page PageInfo }`.
- `LogRequest { Resource ResourceRef; Container string; Previous, Follow bool; TailLines int64 }`. Keep `model.Target` unchanged for existing terminal/watch/forward callers. Native product methods convert a validated LogRequest to the existing transport target.

For read calls, `ExplorerService` methods take `(requestID string, req <Request>)` and return `(result <DTO>, error)`; method names are `ReadWorkloads`, `ReadResource`, `ReadResourceYAML`, `ReadRelatedPods`, `ReadResourceEvents`. `CancelWorkloadRead(requestID string) error` cancels only that request. IDs are unique per invocation. Bound concurrent native read entries to 16, remove on all completion paths, reject excess with a safe busy error. Namespace/tab/generation/section changes cancel the old request and also discard late responses by epoch.

Native reader methods use the same names and DTOs with `ctx context.Context` instead of requestID. The bridge derives a 30-second context and the reader validates connection generation before and after completion. For UI state, keys are tab ID + generation + namespace + kind, with section/selection/request epoch added for detail responses. Retain only current pages and the selected detail per kind, plus one captured Deployment back-reference; no growing object cache.

## Task 1: Bounded Pod/Deployment inventories and native read bridge

**Files:** Create `workload_types.go`, `workloads.go`, `bounded_reads.go`, `workloads_test.go` under `internal/kube/`; create `internal/desktop/workloads.go`. Modify `internal/kube/read.go` only for shared initialization; update generated bindings through the build task.

**Interfaces:** Produces `ReadWorkloads(ctx context.Context, req PageRequest) (WorkloadPage, error)` and its bridge method plus cancellation. Subsequent tasks use the same bounded client and immutable ResourceRef.

- [x] Add table cases to `TestWorkloadReads`: Pod/Deployment namespace list returns at most 100 rows and a continuation; changed namespace/generation rejects the old cursor; server 410 yields `list_changed`; missing status does not become zero. Use existing client fakes and an HTTP test server for response bounds; no live cluster.
- [x] Run `go test ./internal/kube -run TestWorkloadReads -count=1`; confirm the new assertions fail before implementation.
- [x] Implement `ReadWorkloads` with typed list calls, explicit namespace and Limit 100. Cursor is a versioned opaque envelope containing API token and full request scope; validate all fields and cap encoded length at 16 KiB. It is not an authorization token. No unbounded fallback. Return selected page only.
- [x] Implement `boundedReadClient(ctx context.Context, ref model.ConnectionRef) (kubernetes.Interface, error)` using a copied owned REST config and a response-body limiter that errors above 16 MiB per read response, including chunked/compressed responses. Preserve existing transport/auth behavior; never apply this limit to log streams. Reject excessive bodies during reading, not after unbounded decoding. Detail output has the stricter 2 MiB limit.
- [x] Add cancellable desktop read registry, complete/cancel cleanup and safe error normalization. Add one bridge lifecycle case in `internal/desktop/workloads_test.go`: cancelled request releases its slot and cannot cancel a different request. Ensure disconnect/generation validation still owns clients.
- [x] Run `go test ./internal/kube ./internal/desktop`, then `wails3 task bindings`; expect pass and generated read contracts. Inspect generated diff.
- [x] Commit task files only: `feat: add bounded workload inventory reads`.

## Task 2: UID-checked overview, YAML and Deployment relationships

**Files:** Create `internal/kube/details.go`, `relationships.go`; extend `workloads_test.go` and `internal/desktop/workloads.go`.

**Interfaces:** Produces `ReadResource(ctx, DetailRequest) (ResourceDetail, error)`, `ReadResourceYAML(ctx, DetailRequest) (YAMLDocument, error)`, `ReadRelatedPods(ctx, RelatedRequest) (RelatedPage, error)` with concrete `context.Context` parameters; consumes task 1 resource/cursor/client contracts.

- [x] Extend `TestWorkloadReads` with same-name replacement UID rejection; sparse Pod/container fields and scaled-to-zero Deployment preserved; managedFields forms share resourceVersion; oversized YAML fails rather than truncates; equal-label non-owned Pod excluded; forbidden relationship read preserves a usable Deployment and marks coverage incomplete. Run the targeted test and confirm new cases fail.
- [x] Implement one UID-checked typed Get helper used by every detail/relationship endpoint. Include expected kind/version validation. Return safe `resource_replaced` or `not_found`; never automatically select the replacement. Preserve optional API fields in capped ObjectJSON and render them locally without diagnostic logging.
- [x] Serialize YAML from one checked object using the repository's existing YAML dependency. Produce with/without-managedFields variants without mutating the captured source. Check object, each representation and combined response bounds before bridge return. Reject oversize explicitly. No mutation/export endpoint.
- [x] Implement bounded relationship traversal: cursor carries stage, ReplicaSet continuation, current ReplicaSet UID/name and Pod continuation plus Deployment scope. Each call reads at most one 100-item ReplicaSet page or one 100-item Pod page; process owned ReplicaSets incrementally, not an unbounded UID set. Narrow Pod requests using the ReplicaSet's full label selector, then require matching controller owner UID. Capture only bounded pending ReplicaSet identities in the cursor and enforce the cursor cap; return explicit capacity failure if exceeded. Empty intermediate pages remain incomplete with NextCursor; forbidden errors never mean no Pods. Validate Deployment UID on every call.
- [x] Run `go test ./internal/kube ./internal/desktop`; expect all detail/ownership cases pass. Regenerate bindings after adding bridge methods.
- [x] Commit: `feat: add workload details and owned Pod navigation`.

## Task 3: Resource-scoped event evidence

**Files:** Create `internal/kube/events.go`; extend `workloads_test.go`, `internal/desktop/workloads.go`.

**Interfaces:** Produces `ReadResourceEvents(ctx context.Context, req EventRequest) (EventPage, error)` and bridge counterpart; consumes UID-checked Get and bounded client.

- [x] Extend the service table with same-name/different-UID event exclusion, event UID deduplication, optional timestamps, page continuation, and forbidden modern Events API causing no core API request. Run targeted tests; confirm new cases fail.
- [x] Implement modern event API request with namespace and `regarding.uid` selector; use core `involvedObject.uid` only when discovery/endpoint positively indicates unsupported modern API. Unknown discovery/403/network failure is surfaced, not fallback authorization. Bind event-family choice into continuation scope. Validate selected resource UID on every request and filter returned event identities defensively.
- [x] Normalize first time from deprecated firstTimestamp then eventTime then metadata.creationTimestamp; last time from series.lastObservedTime then deprecated lastTimestamp then eventTime then first time. Use analogous core fields. Missing remains unknown; series count precedes deprecated count. Deduplicate event UID on page, stable last-time descending sort with UID tie-breaker. Return page metadata; do not promise globally newest results.
- [x] Run `go test ./internal/kube ./internal/desktop`; expect pass. Regenerate bindings and commit: `feat: add UID-scoped workload events`.

## Task 4: Product log lifecycle with explicit identity

**Files:** Create `internal/kube/logs.go`; modify `internal/kube/streams.go`, `streams_test.go`, `internal/desktop/workloads.go`, `service.go`; extend existing `internal/session` tests only if transport behavior changes.

**Interfaces:** Produces `OpenPodLogs(ctx context.Context, req LogRequest) (model.SessionRef, error)`, `ClosePodLogs(ref model.SessionRef) error` on kube.Service and bridge `OpenPodLogs(req LogRequest)` / `ClosePodLogs(ref)`. Existing ActivateSession/StopSession/AcknowledgeSession remain the transport controls. Product reservations retain stopped/completed sessions until Close, with at most eight. Legacy `Logs` remains compatible for validation callers and must not bypass product reservations through the new UI.

- [x] Extend `TestPodLogs` in `streams_test.go`: exact UID/container/current/previous options, previous+follow rejected, replacement on post-open or periodic check stops stream, snapshot EOF completes, unexpected follow EOF records gap, eight retained slots reject ninth until Close. Run `go test ./internal/kube -run TestPodLogs -count=1`; expect failure before implementation.
- [x] Validate target UID and container (regular/init/ephemeral) before reserving/opening. Reject invalid tail; default zero to 200 for product API. Snapshot requests use Follow false, timestamps true, tail and LimitBytes 1 MiB; detect cap and disclose possible truncation. Follow uses no lifetime byte limit. Apply 30-second snapshot deadline; follow owns an explicitly cancelled context.
- [x] Use StartPending and existing native queue/ack lifecycle. Recheck UID immediately after opening before delivering bytes and every 15 seconds during follow; a failed check cancels the stream. Do not claim atomic UID protection on Kubernetes' name-addressed logs API. Emit safe distinguishable complete/error/gap/loss states using existing event transport conventions.
- [x] Make reservation creation/close atomic; release on failed open, retain on stop/EOF, release all on tab closure/disconnect/app shutdown. Stream cancellation must unblock Read. Explicit Resume creates a new native segment after fresh validation; UI closes the old reservation as part of replacement, retaining its text within the same logical session. If generation changed, require explicit user confirmation of the newly connected target and validate UID; never automatically resume.
- [x] Run `go test ./internal/kube ./internal/session ./internal/desktop` and `go test -race ./internal/kube ./internal/session ./internal/desktop`; expect pass. Regenerate bindings. No repeat of unrelated historical load experiments.
- [x] Commit: `feat: bind product logs to Pod identity and bounded sessions`.

## Task 5: Live inventory and investigation sections in the current UI

**Files:** Create `frontend/src/workloads/{api.ts,model.ts,state.ts,useWorkloads.ts,state.test.ts}` and `workspace/{ResourceYaml.tsx,ResourceEvents.tsx,DeploymentPods.tsx}`. Modify existing Workspace, ResourceTable, ResourceDetail, App and connection state/hook; adjust coordinator only to separate health reads. Use existing style files for scoped additions.

**Interfaces:** `useWorkloads(tabID: string, connection: ConnectionRef | null, namespace: string, visible: boolean): WorkloadController`; controller exposes kind, inventory, selection, activeSection, section states, selectKind/selectResource/selectSection/refresh/next/previous/backToDeployment. Types live in workload model and consume generated DTOs, not handwritten RPC shapes. State keyed per tab/generation/namespace/kind; one owning provider in App survives Workspace remounts.

- [x] Add one reducer scenario in `state.test.ts`: A Pod selection → B Deployment → delayed A response → namespace/generation change. Assert selection/page/section isolation, cancelled response ignored, 20-cursor bound, health update cannot overwrite inventory, page-two auto-refresh paused. Run `npm test --workspace frontend -- src/workloads/state.test.ts`; expect new assertions to fail.
- [x] Implement reducer/controller and cancellable request adapter; remove workload dependency on `ConnectionView.pods` after connection setup. Keep health/source/trust and namespace discovery intact. Cancel hidden detail reads; no hidden inventory polling. Persist navigation in memory per tab/kind, not settings or disk. Clear on tab close; namespace/generation changes invalidate it.
- [x] Populate Pod and Deployment columns/overview per spec, with optional values, lifecycle separate from phase, container groups and collapsed metadata. Add page-local filter/sort, Next/Previous within bounded history, explicit refresh-to-page-one and opt-in 15-second visible-page-one refresh. Retain existing split geometry and keyboard row selection.
- [x] Wire active-section-only reads, independent loading/error/time states and inert text rendering. YAML has managedFields toggle/search/select/copy exact displayed representation; no editor or hidden write action. Events show page-local ordering, coverage/error explanations and manual refresh. Deployment Pods traverses bounded pages and preserves captured Back to Deployment identity; revalidate on return. A forbidden section never blanks another.
- [x] Run `npm test --workspace frontend -- src/workloads/state.test.ts`, `npm run lint`, `npm run typecheck`; expect pass. Inspect both themes with real local data during task 7; no new component snapshots.
- [x] Commit: `feat: connect workload investigation views`.

## Task 6: Shared Logs view and Sessions dock

**Files:** Create `frontend/src/sessions/{logStore.ts,useLogSessions.ts,LogViewer.tsx,logStore.test.ts}`; modify SessionsDock, bridge, ResourceDetail and App. Retire synthetic Sessions presentation on the product path without deleting unrelated validation infrastructure.

**Interfaces:** `useLogSessions(): LogSessionsController` is mounted once in App. Controller exposes `open(req: LogRequest)`, `stop(id)`, `close(id)`, `clear(id)`, `resume(id, confirmedConnection?)`, and immutable session snapshots. Logical ID retains text across explicit native segments; every segment has its own SessionRef and fixed target. Both detail and dock consume the same controller. Subscription registration precedes activation.

- [x] Add one table-driven `logStore.test.ts` group for split UTF-8/newline-free input, 1 MiB retention/64 KiB window, stale-segment rejection, clear preserving loss metadata, ninth-session rejection and stop-versus-close. Add one lifecycle case using a fake bridge: detail/dock mount changes cause one subscription/open, acknowledgements continue offscreen, late stopped events do not restore a session. Run targeted Vitest; expect failure.
- [x] Implement streaming decoder with bounded incomplete bytes/lines, byte-counted retention and explicit dropped/truncated counters. Bound rendered text to 64 KiB; navigate/search retained content without expanding DOM to full history. Acknowledge after bounded ingestion, independently of visible rendering. Do not reset loss on Clear or silently call truncated output complete.
- [x] Wire container selector/current/previous/tail/follow, loading/complete/error/gap status, timestamps, Stop, Clear and autoscroll. Only auto-select a sole eligible container. Detail and dock show original cluster/namespace/Pod/container/mode; no duplicated native reader. Explicit Resume warns overlap/gap and respects UID validation and eight-slot limit.
- [x] Compose tab-close confirmation with existing connection cleanup; release native and frontend ownership on close/exit. Settings/theme/navigation/dock collapse retain sessions. Disconnect stops native readers, retains labelled bounded evidence, and requires explicit validated resume after recovery. Pausing autoscroll never stops transport.
- [x] Run `npm test --workspace frontend -- src/sessions/logStore.test.ts`, lint and typecheck; expect pass. Verify no raw logs enter settings/diagnostic output. Commit: `feat: add shared workload logs and Sessions controls`.

## Task 7: Local walkthrough, minimal regression gate and user UAT deliverable

**Files:** Extend `scripts/explorer-fixtures.py` and `tests/fixtures/explorer/inventory.json` only for missing fixtures; add `tests/integration/workloads_test.go`. Create `docs/superpowers/validation/phase-1c2.html` and `phase-1c2-ui-uat.html`; update docs index/roadmap/status with actual results. Keep operational outputs ignored.

**Interfaces:** Opt-in test `TestWorkloadInvestigation`, gated by `KUBESIFT_EXPLORER_INTEGRATION=1` and existing `KUBESIFT_EXPLORER_CONFIG_DIR` absolute isolated directory. Reuse the existing integration guard that rejects nonlocal endpoints and contexts other than kind-dev-local-a/b.

- [ ] Inspect fixture inventory first. Add only missing healthy/failing multi/init-container workloads, Deployment ownership/lookalike scenario and restricted-reader fixtures in the existing test namespace. Keep shop/billing intact. Use `scripts/dev-clusters.sh --explorer`; never create another cluster or grant cluster-wide permissions for convenience.
- [ ] Implement one opt-in walkthrough: connect A/B, list Pods/Deployments, inspect one failing Pod overview/YAML/events, follow its Deployment relationship, request current/previous logs where fixture supports them, acknowledge/stop/close and verify cleanup. Assert different UIDs for same names and no Secret reads/writes. Use service fakes for large-page/oversize/ephemeral variants instead of installing hundreds of resources.
- [ ] Run `KUBESIFT_EXPLORER_INTEGRATION=1 KUBESIFT_EXPLORER_CONFIG_DIR="$PWD/.local/explorer" go test -tags=integration ./tests/integration -run TestWorkloadInvestigation -count=1` after creating `.local/explorer/a.kubeconfig` and `b.kubeconfig` with `kubectl --context kind-dev-local-a config view --minify --flatten --raw` and the corresponding explicit B context, redirecting directly to those ignored files with owner-only permissions. Never print or commit their contents. The fixture apply script does not generate these files. A skipped test is not success. Record exact contexts and outcomes without credentials.
- [ ] Run `wails3 task bindings`, `npm run build`, `scripts/check.sh`, then `wails3 task package`; expect exit zero. Check generated/source descriptions and scoped formatting. Fix actual regressions; do not broaden tests without a reason.
- [ ] Launch the packaged macOS app and exercise both investigation journeys, two tabs with matching names, keyboard selection, current/previous/follow controls, Settings/theme/split preservation, reconnect, tab/session close and exit. Use the existing reviewed laptop/external-monitor sizes. Briefly background and restore a live log stream; record visible gaps/loss and cleanup, not a lossless claim. Record untested cases honestly.
- [ ] Write technical report mapped to C21–C29, actual commit/version/OS, checks, screenshots only where useful, remaining provider/platform limits and independent review outcomes. Request repository-prescribed independent review, resolve blocking findings and rerun affected checks before phase completion.
- [ ] Create the separate HTML UI UAT checklist using the actual implemented labels. Include tester/date/OS/app version/commit, prerequisites, exact fixture namespaces and how to find current Pod names, steps, expected result, Pass/Fail/Not tested/Blocked and notes. All user results start **Not tested**; agent results remain in the technical report. Provide print/save-as-PDF directions and explicitly state whether entered results persist.
- [ ] Use these 15 concise UAT cases: (1) cluster/namespace A/B isolation; (2) Pod phase/readiness/reason; (3) Deployment counts and zero replicas; (4) page/filter/Refresh behavior; (5) Pod conditions and container groups; (6) owned Pods/Back navigation; (7) read-only YAML/toggle/search/copy; (8) UID events and empty/error presentation; (9) current container snapshot; (10) previous logs and multi-container selection; (11) Follow/Stop/Clear/autoscroll; (12) shared Sessions across Settings/theme/navigation; (13) bounds/loss/capacity; (14) layout/keyboard/saved splits; (15) tab-close/exit cleanup. Restricted access, replacement and unavailable previous logs require safe prepared fixtures or Blocked; no instructions to damage normal credentials or shared workloads.
- [ ] Link both reports from the docs index, record remaining user UAT in the portable checkpoint, mark only verified plan steps complete and commit: `test: validate workload investigation and add UI UAT checklist`.
- [ ] Push the feature branch and open the PR to develop with actual checks, review outcomes, limitations and UAT link. Leave it unmerged pending user UI UAT unless the maintainer explicitly authorizes a qualified merge. No release or 1C.3 implementation.

## Handoff and completion boundary

The implementation session reads this plan and its approved spec together, follows engineering practices and preserves the current refined UI. It reports verified task completion, blocked/unrun checks, commit/PR links and where the user performs UAT. Planning completion does not mark any implementation checkbox complete.

Coverage: C21 → tasks 1/5; C22–C24 → 2/5; C25 → 3/5; C26–C27 → 4/6; C28 → 5–7; C29 and separate user checklist → 7. Self-review checked scope coverage, cross-task contracts, the five failure classes and proportion; no application code or cluster changes were made to author this plan.
