# Phase 1C.3a — Remaining workloads

**Date:** 2026-11-13. **Status:** Scope accepted; written specification prepared for review. Planning only; implementation requires a separate session and explicit authorization. Work in the existing checkout; do not create a worktree.

## Intent and approach

Extend the read-only investigation journey to StatefulSets, DaemonSets, Jobs and CronJobs, using the merged Phase 1C.2 workload readers and approved compact detail UI. The user chose this increment before networking. The existing [refinement design](2026-11-13-workload-explorer-refinement-design.md), [UX workflows](../../ux/workflows.html), [architecture](../../architecture/overview.html) and [security model](../../security/security-model.html) remain authoritative for shared behavior.

Approaches considered: extend typed readers and existing tables (selected; kind-specific semantics without a second framework); generic discovery-first views (deferred to the generic-resource increment; insufficient investigation semantics alone); separate viewers per kind (rejected because they duplicate navigation and evidence controls). No redesign of the shell or new UI library is needed. Existing [visual references](../../ux/mocks/index.html#workload-refinement) govern spacing, hierarchy, colours and disclosures; the tables below define new content.

### Included and excluded

Included: four namespace-scoped inventories, Overview/YAML/Events, directly owned Pods for StatefulSet/DaemonSet/Job, CronJob-owned Jobs, owner/back navigation, partial-permission states, bounded reads and separate user UAT.

Excluded: mutations (run now, suspend/resume, scale, restart, delete, edit, rollback), controller-level log aggregation, all-namespace mode, watches, networking/storage explorers, ControllerRevision history, HPA/PDB/ReplicationController views, generic discovery, metrics and extra clusters. Show configured service/PVC references without fetching those resources or implying their existence. Preserve existing Pod Logs and Secret-reference boundaries.

## Coverage and navigation

Workloads order: Pods, Deployments, ReplicaSets, StatefulSets, DaemonSets, Jobs, CronJobs. Enable only these implemented destinations; retain other deferred navigation unchanged. Namespace changes preserve the selected kind and invalidate old-scope evidence under the existing rules.

| Kind | API | Detail tabs | Primary relationship |
| --- | --- | --- | --- |
| StatefulSet | apps/v1 | Overview, YAML, Events, Pods | StatefulSet → directly owned Pods |
| DaemonSet | apps/v1 | Overview, YAML, Events, Pods | DaemonSet → directly owned Pods |
| Job | batch/v1 | Overview, YAML, Events, Pods | Job → directly owned Pods; owner CronJob when present |
| CronJob | batch/v1 | Overview, YAML, Events, Jobs | CronJob → Jobs → Pods |

Every row has name and age. Kind-specific inventory columns:

| Kind | Other visible columns |
| --- | --- |
| StatefulSet | Ready / desired, total Pods, updated Pods |
| DaemonSet | Ready / desired nodes, currently scheduled nodes, updated nodes |
| Job | Status, succeeded / target completions, active Pods, failed attempts |
| CronJob | Schedule, time zone, suspended, active Job references, last scheduled |

Narrow panes use local horizontal scrolling, not silently discarded essential columns. Full names and values remain accessible. Reuse page-local name filter/sort and existing keyboard selection. Do not introduce kind-specific query builders.

## Overview semantics

All kinds retain readable conditions/reasons/messages, labels, annotations, owners and creation/deletion metadata. Keep capture time and resourceVersion in evidence metadata; render cluster data as inert text. Missing optional values remain unknown/not configured; do not infer success from missing errors or absent conditions. A deletion timestamp is a lifecycle marker, not a fabricated API phase.

### StatefulSet

Lead with ready/desired, total Pods, updated Pods and available Pods where observed. Distinguish status.replicas (total) from status.currentReplicas (Pods at currentRevision); never label currentReplicas as total. Details show current-revision count, currentRevision/updateRevision, desired/observed generation, serviceName, podManagementPolicy, ordinal start when present, update strategy/partition and minReadySeconds. Display volumeClaimTemplates and retention policy as configuration only. Ready=desired does not prove all Pods use the update revision, especially with partitioned or OnDelete updates; do not invent a rollout-complete badge.

### DaemonSet

Counters are eligible/scheduled **nodes**, not replicas requested by a spec.replicas field. Show desiredNumberScheduled, currentNumberScheduled, updatedNumberScheduled, numberReady, numberAvailable, numberUnavailable and numberMisscheduled, with desired/observed generation. Zero eligible nodes is not automatically failure. Show update strategy, minReadySeconds, node selectors, affinity and tolerations as configured eligibility evidence, without claiming to evaluate the scheduler or checking every Node.

### Job

Show active/succeeded/failed Pod counts, configured completions and parallelism, suspend setting, start/completion time, completion mode, backoff settings, active deadline and TTL when configured. Preserve indexed completion/failure ranges as bounded selectable text; do not expand large index ranges. Failure/success policies remain readable configuration, including optional fields supported by the pinned API types. Preserve template metadata and Pod/container configuration.

Status uses true conditions, in this precedence: conflicting terminal Complete and Failed → “Conflicting status”; Failed → “Failed”; Complete → “Complete”; FailureTarget → “Failure target reached”; SuccessCriteriaMet → “Success criteria met”; Suspended or spec.suspend=true → “Suspended”; observed active>0 → “Active”; otherwise → “No terminal condition reported”. Show all conditions beneath the summary, including conflicts. Intermediate target/criteria conditions do not claim all Pods have terminated. A nonzero failed count alone means failed attempts, not a terminally failed Job. Missing completions is “Not specified”, not an invented one or a false succeeded/0 ratio. Completion time is not fabricated for failed Jobs. Unknown observation is not labelled Pending or healthy.

### CronJob

Show exact schedule, spec.timeZone when present, suspend, concurrencyPolicy, active references count, lastScheduleTime and lastSuccessfulTime, with history limits and starting deadline in details. When timeZone is absent, show “Controller default (unknown)” rather than the user's local timezone. Captured execution timestamps may use the existing local-time formatter, clearly distinguished from the schedule's zone. No next-run prediction or cron-expression parser in this increment.

Suspension prevents future scheduling; it does not mean existing Jobs stopped. Keep jobTemplate configuration (including nested Pod template) visible. Active references are reported evidence, not proof every referenced Job still exists. Do not synthesize a health condition for a CronJob that has none. Retained owned Jobs are not a complete execution history: deletion, TTL and retention settings can leave gaps.

### Counter presence

Typed decoding collapses absent scalar status counters to zero. The implementation must use a documented per-kind observation policy: StatefulSet/DaemonSet counts become known with nonzero observedGeneration, any condition or any nonzero status counter; otherwise display unknown. Job counts become known with a start/completion timestamp, any condition or any nonzero counter; otherwise unknown. CronJob active count is exactly the length of returned status.active (zero is an empty reported reference list, not proof of no running work). Spec pointer values preserve absent versus explicit zero; server defaults must not be invented client-side. Pointer-valued optional status counts remain unknown when absent even when other status is observed. No universal Deployment counter helper for these kinds.

## Relationships and navigation

Controller ownerReferences with controller=true and exact UID establish ownership; labels and names never prove it. StatefulSet, DaemonSet and Job Pods link directly to their validated parent. CronJob Jobs are selected by exact owner UID, not status.active alone: include retained completed/failed Jobs and show their actual status. Do not add a flattened CronJob Pods tab. The Jobs tab is sorted newest creation first **within the displayed batch**, not globally newest history.

Reuse the Pod table; generalize its ReplicaSet-specific owner field to a controller ResourceRef. Deployment Pods still show their actual ReplicaSet; other parent Pods may hide the repeated owner column, with parent identity visible in the header. Preserve node and phase/reason evidence in related Pod rows. Pod owner links support StatefulSet, DaemonSet and Job; Job owner supports CronJob. Unsupported owner kinds remain readable text.

Reuse explicit openRelated/back navigation: CronJob → Job → Pod → Back to Job → Back to CronJob restores originating section and valid inventory position. Keep the eight-entry history bound and existing scope/UID invalidation. Validate referenced identities on open/back; deletion, replacement and forbidden targets produce actionable local errors, not navigation to a same-name replacement. A related object outside the current inventory page must remain selected.

## Bounds, permissions and compatibility

Extend existing typed list/GET methods for apps/v1 and batch/v1 only. Preserve explicit connection generation, namespace, API identity and UID; reject incorrect kind/version combinations before a request. No API probing through mutations, no deprecated-version permission fallback. A denied kind or relationship must not hide accessible kinds/sections. Unavailable API differs from empty collection.

Reuse page size 100, inventory history 20, 16 KiB encoded cursor limit, existing request deadlines/cancellation and 2 MiB object/YAML limits. Related reads may skip at most eight empty ownership-filtered pages per request, then return continuation. Keep the existing smaller internal Deployment traversal batch where needed for cursor bounds. Load more replaces the bounded batch. Cursor scope binds relation type, parent kind/UID, namespace and connection generation; treat cursors as untrusted input, not authorization or signed tokens.

On every related call, validate the parent, filter controller ownership and return partial/complete state truthfully. Invalid/missing selectors produce an explicit error; never silently broaden to an unbounded Pod scan. CronJob Job enumeration uses bounded namespace pages and owner filtering, as no label association is required. Cursor expiry produces explicit refresh guidance. Generation change/cancellation must prevent stale data publication.

Shared YAML/search/copy and UID-scoped Events work for every new kind. Events remain local to the selected controller; related Pod failures are inspected on the Pod. No controller Logs tab: open a real Pod and select its container. Fields supported by the pinned Kubernetes types remain available in YAML even when not summarized in Overview. The current typed reader can omit fields unknown to those types; do not claim lossless raw-server YAML. Carry a concise limitation in the shared YAML information disclosure. A raw/unstructured reader is outside this increment; do not require feature-gated optional fields or change Kubernetes dependencies for this phase.

## Acceptance and delivery

| ID | Required evidence |
| --- | --- |
| A01 | All four inventories and details use correct API/scope; supported existing kinds regressions pass |
| A02 | StatefulSet total/current-revision distinction, zero replicas, partition/OnDelete and generation lag are honest |
| A03 | DaemonSet node counters, misscheduled and zero eligible nodes are honest |
| A04 | Job active, complete, terminal failure, failed retries, suspension, indexed and intermediate/conflicting conditions are distinct |
| A05 | CronJob custom/absent timezone, suspended with active refs, never scheduled and retained history gaps are clear |
| A06 | Correct direct ownership; equal-label nonowners excluded; CronJob→Job→Pod and Back preserve identity |
| A07 | Partial RBAC/API unavailable, empty intermediate pages, expiry, scope changes and replaced UIDs do not leak or misrepresent evidence |
| A08 | Namespace kind retention and same-named resources across two cluster tabs remain isolated |
| A09 | Shared YAML exact copy/search, Events scope and Pod Logs departure/return continue working |
| A10 | Native light/dark, narrow split, 200% zoom, keyboard and large/hostile text remain usable |

Use existing synthetic fixtures first, only documented kind-dev-local-a/b with explicit contexts. Inspect fixture ownership before additions; no extra clusters, storage controllers or monitoring infrastructure. Use fake/unit fixtures for statuses impractical to induce safely; label that evidence separately from native integration. Keep CronJobs suspended except a deliberately authorized bounded local test with cleanup; prefer owned static fixture objects for retained-Job navigation. App runtime makes no writes.

Deliver a technical validation report and separate user UAT checklist, with user results initially Not tested. Prior Phase 1C.2 UAT/soak/accessibility gaps remain tracked separately. No implementation, merge or release is authorized by this planning document.

## Verified documentation and handoff

Context7 official Kubernetes documentation was consulted on 2026-11-13 against the existing v0.37.0 Kubernetes dependencies: [StatefulSet API](https://kubernetes.io/docs/reference/kubernetes-api/apps/stateful-set-v1/), [DaemonSet API](https://kubernetes.io/docs/reference/kubernetes-api/apps/daemon-set-v1/), [Job API](https://kubernetes.io/docs/reference/kubernetes-api/batch/job-v1/), [Job lifecycle](https://kubernetes.io/docs/concepts/workloads/controllers/job/), [CronJob scheduling](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/). These explain the distinctions above; real server capabilities may differ and optional fields remain optional.

Execute the [implementation plan](../plans/2026-11-13-phase-1c3a-remaining-workloads.md) in a separately authorized session, in the existing checkout without creating a worktree.
