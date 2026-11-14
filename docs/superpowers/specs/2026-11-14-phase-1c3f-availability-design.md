# Phase 1C.3f — Availability and remaining workload resources

**Date:** 2026-11-14. **Status:** Planning package for separate-session implementation. Existing checkout; no worktree. No application changes in this session.

## 1. Outcome and baseline

Inspect HorizontalPodAutoscalers (HPAs), PodDisruptionBudgets (PDBs), ReplicationControllers (RCs) and ControllerRevisions. Follow HPA targets, PDB label-selected Pods, RC-owned Pods and StatefulSet/DaemonSet revision history without leaving the existing explorer. Show configured policy and reported observations without calculating live scaling decisions, eviction eligibility or rollout safety.

Access control merged through PR #12 as `dea9197`; planning baseline is develop `f276cdc`. Recheck actual source before execution. Preserve the approved compact UI, ConfigMaps/Secrets under Workloads, Access control grouping, resource-name validation, captured-target navigation, lazy nested rendering, Secret metadata boundary and Logs lifecycle. The [1C.3e report](../validation/phase-1c3e.html) leaves native restricted-denial coverage and earlier unrelated limitations open. Exercise overlapping navigation/state behavior here without requiring duplicate manual UAT or relabeling historical gaps.

The roadmap fixes this scope before cluster/API discovery. Dedicated typed views are preferable to generic YAML alone because ownership, label selection and policy observation mean different things. No dynamic discovery, custom scale targets, metrics infrastructure, live metrics API reads, scale/eviction/rollback actions, resource edits, release or next phase. Progress belongs in checkpoint/plan/report, not the roadmap.

## 2. Kinds and tabs

All four kinds are namespace-scoped and require a selected namespace. No all-namespace lists or API-version fallback.

| Kind / API | Inventory columns after Name | Tabs |
| --- | --- | --- |
| HorizontalPodAutoscaler / autoscaling/v2 | Target, Min/Max, Reported current/desired replicas, Observation, Age | Overview, YAML, Events |
| PodDisruptionBudget / policy/v1 | Min available / Max unavailable, Reported disruptions allowed, Observation, Age | Overview, YAML, Events, Selected Pods |
| ReplicationController / v1 | Desired, Reported current/ready/available, Age | Overview, YAML, Events, Pods |
| ControllerRevision / apps/v1 | Revision, Controller owner, Age | Overview, YAML, Events |

Add RC after ReplicaSets in Workloads. Add HPAs, PDBs and ControllerRevisions after CronJobs and before ConfigMaps/Secrets, retaining their Workloads placement. Sidebar grouping does not define reader type: RC joins the workload contract; the other three use dedicated availability projections and explicit tabs. StatefulSet and DaemonSet gain a **Revisions** tab. No new Logs tab beyond existing Pod Logs.

Retain namespace-kind selection, per-cluster tab state, page-local search/sort, explicit refresh, eight-entry Back and existing split controls. Empty, forbidden, unsupported LIST API and missing GET target remain distinct. Events use checked source UID and existing namespaced API-family behavior. A related target not in the current inventory page stays selected.

## 3. Evidence presentation

**HPA:** show scaleTargetRef, minReplicas/maxReplicas, behavior.scaleUp/scaleDown policies, windows and selectPolicy. Separate spec.metrics targets from status.currentMetrics observations; do not pair them by array index. Preserve type-specific fields for Resource, ContainerResource, Pods, Object and External metrics, including metric names/selectors, described-object reference, resource/container, Value/AverageValue/Utilization and raw quantities. Missing metrics are “Not reported”, never zero. Unknown metric types remain visibly uninterpreted configuration; no target metric fetching. Show current/desired replica observations, lastScaleTime, observedGeneration and all conditions with reason/message/time. Optional fields remain absent rather than receiving invented defaults. These are controller-reported observations, not a real-time measurement or recommendation to scale.

**PDB:** display minAvailable/maxUnavailable with original integer versus percentage form, selector and unhealthyPodEvictionPolicy. Missing policy may be described as “Not specified (API default: IfHealthyBudget)” separately from captured data; preserve unknown values. Distinguish unset/null selector (“Selects no Pods”) from `{}` (“Selects all Pods in this namespace”) for policy/v1. Show currentHealthy, desiredHealthy, expectedPods, disruptionsAllowed, conditions and disruptedPods timestamps as reported. disruptedPods is controller bookkeeping, not a navigable current-UID Pod list. Do not calculate allowances from selected rows, round percentages or combine overlapping PDBs.

For HPA/PDB, compare metadata.generation and status.observedGeneration natively without JavaScript precision loss: equal and present → “Observed current generation”; unequal → “Observation differs from current generation”; missing → “Observation not reported”. Missing values remain unknown; reported zeros stay zero. Display stale PDB allowances with the observation warning, never as permission to evict. Even current status is a snapshot, not authorization or a guarantee. Preserve raw field presence before typed decoding can erase missing values.

**RC:** spec replicas/selector/template and reported total, fully labeled, ready, available, observedGeneration and conditions. Keep desired versus observed counters distinct using existing workload observation semantics. Selector narrows reads; controller owner UID proves ownership. Labels alone never populate the owned Pods tab. Missing/empty invalid controller selectors produce an explicit unavailable relationship rather than an unrestricted scan. Existing captured Pod navigation and Logs remain available.

**ControllerRevision:** exact revision number, captured ownerReferences, metadata and a collapsed read-only data disclosure. Data is opaque historical controller content/patch; it is not necessarily a full current manifest. Do not apply, merge, execute, compare as a guaranteed rollout diff, follow embedded references or expose rollback. Keep revision integers exact as decimal text in summaries and structured display, including values above JavaScript's safe integer range. YAML remains the captured representation; no frontend JSON stringify round-trip for exact copy. Unknown owner kinds stay readable without invented navigation.

**Revisions tab:** show retained ControllerRevisions whose controlling owner API/kind/name/UID match the selected StatefulSet or DaemonSet. No latest/global ordering claim from a partial page. For StatefulSet only, mark rows whose names equal captured status.currentRevision/updateRevision as “Referenced by captured currentRevision/updateRevision”; this does not mean every Pod runs that revision. DaemonSet revisions have no inferred current badge. A deleted history entry is not recoverable from the application. Retention is not complete deployment/audit history.

Keep list summaries small: no complete metrics, selectors, templates, revision data or nested conditions in inventories. Lazy disclosures and local pages of 100 apply to metrics, policies, disruptedPods and nested revision content; collapsed content is not mounted. Treat all values as inert, local ephemeral text. Configuration/history can contain sensitive literals: no telemetry/payload logging, AI sharing, credential lookup or universal redaction claim.

## 4. Relationships and reference validation

**PDB Selected Pods:** use a separate label-association reader, not the owned-Pod DTO/UI. Re-read and validate PDB UID for each request. Unset selector returns a complete empty result without a Pod LIST; `{}` deliberately lists the selected namespace. Compile matchLabels/matchExpressions using Kubernetes selector semantics, rejecting invalid selectors. List one bounded Pod API page, filter/validate returned namespace and selector membership, retain continuation even if no rows remain. Label every row “Selector match”; show selected source version and selector with the result. Counts are captured-page counts, not protected/healthy/global totals. Bind cursor to connection, namespace, PDB UID and canonical selector (including a null-versus-empty discriminator) so selector changes invalidate continuation. Validate any supplied cursor before returning the no-selection result. A denied Pod list is unavailable evidence, not zero matches.

**Owned revisions:** validate parent UID on each request; list ControllerRevisions in its namespace and filter controlling owner API/kind/name/UID. Owner fields must agree, not just labels or name prefix. Traverse at most eight API pages per call to skip unrelated entries; stop on a nonempty matching page, end or budget. Return continuation/Complete accurately, without accumulation or claiming complete history on budget exhaustion. Bind cursor to parent identity. Capture parent sourceVersion and StatefulSet revision-name markers in each response.

**Click resolver allowlist:** HPA→Deployment/StatefulSet/ReplicaSet (apps/v1) or RC (v1) from exact scaleTargetRef; PDB→Pod from current selector membership; StatefulSet/DaemonSet→owned ControllerRevision; ControllerRevision→controlling StatefulSet/DaemonSet owner. All stay in the same namespace and connection. HPA custom/unknown version/unsupported targets are readable but non-navigable; never GET a /scale subresource or discover a target API. Multiple ambiguous controlling owners are non-navigable. RC-owned Pod clicks retain the existing UID-checked Pod opening contract.

Resolver validates source ResourceRef and captured nonempty resourceVersion, re-reads source, rejects changed UID/version, then verifies the allowed reference/relationship before returning the exact capped target capture. A PDB→Pod GET is allowed only after source/selector validation and with the captured row's nonempty target UID; after GET, require UID and current labels to match. Never use the PDB's disruptedPods names as such a route. Revision clicks require captured target UID and matching controller owner; reverse owner navigation requires the source's authoritative owner UID. HPA name-only target refs disclose resolution at open time and capture target UID without claiming ownership. A caller-provided UID must match on every route. Forged kind/API/namespace/connection requests fail before target I/O.

Return source/target versions, association/identity evidence and the exact ResourceDetail read during validation. Open it without a second initial GET. On Refresh/reentry, clear old relationship badges if a new capture replaces it; Back preserves source view and position. Errors leave source evidence visible. No reverse all-HPA scans, PDB overlap analysis, generic revision owner discovery or cross-namespace targets.

## 5. Limits and architecture

Use typed APIs on the existing bounded transport. RC extends ReadWorkloads/WorkloadRow and owned-Pod paths. HPA/PDB/ControllerRevision use AvailabilityRow/Page; preserve raw numeric/optional field evidence for these captures as the storage reader does. Add an optional availability summary to ResourceDetail for exact revision text and native generation-observation state; no changes to other kind semantics. YAML must retain captured values and existing managed-fields/copy limits.

Retain API page 100; inventory cursor history 20; encoded cursor maximum 16 KiB; Back eight; deadline 30 seconds; active desktop reads 16; decoded body cap 16 MiB; serialized page/detail/YAML/resolver cap 2 MiB. Owned-revision scans at most eight pages per request; PDB selected Pods one page. Cap complete responses and source/target snapshots; reject oversize with an explicit error, never silently truncate policy or history data. No watches/unbounded accumulation. Cancel and suppress late successes/errors across kind/namespace/tab/section/refresh/Back changes. Do not broaden scope or RBAC-name validation to unrelated kinds.

## 6. Acceptance

| ID | Required evidence |
| --- | --- |
| F01 | Four APIs/kinds/tabs, namespace retention, independent list/get/Events denial and absent API handling |
| F02 | HPA all five metric types, absent/zero values, separate targets/observations, behaviors/conditions and supported/unsupported scaleTargetRef |
| F03 | PDB null/empty/expression selectors, integer/percent fields, unknown eviction policy, stale/missing observedGeneration and allowances without eviction claims |
| F04 | Selected Pods label-only evidence, cursor selector changes, zero matching continuing page, denied lists, source/Pod replacement and no disruptedPods-name links |
| F05 | RC zero/missing counters, selector versus controlling UID, orphan/mismatched Pods and existing Pod→Logs path |
| F06 | Revision exact int64 display, opaque/lazy data, owner API/kind/UID mismatch, bounded partial history, captured current/update markers and unsupported owners |
| F07 | All resolver routes, source/version changes, forged scope/UID, validated capture used once, same-name replacement and local target errors |
| F08 | Caps, cursor expiry/misuse, eight-page budget, nested paging, null collections, stale response/cancellation and independent tabs |
| F09 | Existing access/storage/configuration/Secret/networking/workload behavior, ConfigMaps/Secrets placement and Back regressions |
| F10 | Exact packaged desktop HPA→target, PDB→selected Pod→Logs, RC→owned Pod and StatefulSet/DaemonSet→revision→owner journeys; keyboard, both themes, narrow split, 200% zoom and YAML copy |

Only explicit kind-dev-local-a/b and owned synthetic fixtures. Create an isolated namespace, a small RC/Pod and labeled PDB cases, and an HPA with min=max against a dedicated disposable target (or nonexistent target for missing-reference evidence); never attach an HPA to existing sample/user workloads. Reuse owned StatefulSet/DaemonSet history where suitable or create small dedicated controllers under existing fixture policy. No metrics-server/custom-metrics adapters, eviction/drain, manual scale, rollback or production contexts. Revision/metric/status edge cases may use synthetic transport/rendering fixtures, clearly distinguished from live/native evidence. No controller-managed history edits merely to make the UI appear healthy.

Test both clusters and restricted identity where source list/get succeeds but target/Pod/revision access fails. Do not probe permissions by mutations. Cleanup owned fixtures only, remove HPA/controller dependents in safe order and report leftovers. Write one permanent validation report with actual checks, exact package identity, cleanup and open limits; no duplicate mandatory manual UAT. Missing desktop tooling leaves a named unmet native gate, not a browser substitute.

After the whole implementation/tests, one fresh independent whole-PR review using the repo skill; fix/recheck affected changes, create ready PR and merge when existing gates pass without another instruction. Do not release or start 1C.3g.

## 7. Sources and handoff

Official sources checked through Context7 and Kubernetes documentation on 14 November 2026: [HPA v2 API](https://kubernetes.io/docs/reference/kubernetes-api/autoscaling/horizontal-pod-autoscaler-v2/), [PDB v1 API](https://kubernetes.io/docs/reference/kubernetes-api/policy/pod-disruption-budget-v1/), [StatefulSet revision status](https://kubernetes.io/docs/reference/kubernetes-api/apps/stateful-set-v1/), [ControllerRevision API](https://kubernetes.io/docs/reference/kubernetes-api/apps/controller-revision-v1/), [ReplicationController](https://kubernetes.io/docs/concepts/workloads/controllers/replicationcontroller/). Pinned v0.37.0 PDB selector and ControllerRevision int64 definitions were inspected locally. No dependency/server upgrade implied.

Execute the [five-task plan](../plans/2026-11-14-phase-1c3f-availability.md) only in the separate authorized implementation session.
