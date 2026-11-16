# Phase 1D.5 — Workload history and rollback

**Status:** Design for review; scope approved for specification, implementation not authorized.
**Date:** 2026-11-16

Owners: [roadmap](../../roadmap/roadmap.html#phase-1d), [UX](../../ux/workflows.html), [security](../../security/security-model.html), [engineering](../../engineering/engineering.html#agent-verification). Baseline: develop `36f795a`, after YAML editing merge `87c7ce8`.

## Outcome and scope

An engineer opens a workload, sees its retained revisions, selects one, reviews the template changes and server dry-run, and confirms rollback. Keep the current UI and reuse existing operation machinery. Kubernetes permissions and API validation decide authorization; no extra permission system or typed-name confirmations.

Include apps/v1 Deployments, StatefulSets and DaemonSets. These are the standard [Kubernetes rollout resource types](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_rollout/). Applicability means an actual controller-supported historical configuration, not a rollback button on every object:

| Resource | History and action |
| --- | --- |
| Deployment | Owned ReplicaSets with deployment revision annotations; restore the selected retained Pod template |
| StatefulSet / DaemonSet | Owned ControllerRevisions; restore the selected retained Pod template |
| ReplicaSet / ControllerRevision | History evidence; navigate to its captured owning workload to roll back, not an independent rollback action |
| Pods, Jobs/CronJobs, ReplicationControllers, Services, configuration, RBAC, storage and arbitrary custom resources | No common retained-revision rollback workflow in this increment; existing YAML editing remains available according to permissions |
| Helm releases | Their actual release-history rollback remains Phase 1E |
| OpenShift DeploymentConfigs and operator-specific rollback | Separate controller-specific workflows in Phase 1F where supported; do not emulate them through Kubernetes Deployment assumptions |

Do not install controllers or store application-owned history. Missing/pruned history is unavailable, not reconstructable from timestamps, event messages or same-named objects. No backup restore, database rollback, Secret/ConfigMap value rollback, creation, batch rollback, automatic Pod deletion, strategy changes or automatic rollback on failure.

## Approach

Use existing history readers, typed workload reads and native reviewed operations. Prepare a conditional JSON Patch restoring only `spec.template`, with UID/resourceVersion tests on the current parent. Read and interpret the selected historical source natively. Reuse diff, dry-run, single-use confirmation and result presentation from 1D.4 without forcing a user to edit generated YAML.

Alternatives: invoking kubectl rollout undo delegates behavior to an external executable and does not naturally bind the existing native review to exactly one write; restoring entire historical manifests risks changing replicas, strategy and unrelated configuration. The proposed native template-only approach keeps the review small and predictable. Label it clearly as a workload template rollback, not restoration of the complete resource at a past time.

The plan must inspect pinned Kubernetes dependencies and reference behavior when decoding revision data; no generic application of opaque ControllerRevision patches to arbitrary parent fields. Use typed decoding to extract the historical template and reject missing/unrecognized data with an explanation. Do not silently offer a partial or guessed restore.

## History in the current detail view

Reuse the existing Revisions tab for StatefulSets/DaemonSets and add a consistent Revisions tab for Deployments. Keep current tables, tokens, keyboard selection, split layout and modal patterns. No new dashboard or timeline framework.

Show revision number, captured source name, creation time, images where available, optional change-cause annotation, and controller-reported current/update references when present. Revision numbers are exact decimal strings, never JavaScript floating-point identities. Do not label the largest number as healthy or assume creation time is revision order. A current-template match may be labelled matching current template; do not conflate it with all Pods being updated.

History must belong to the current workload UID through its controller owner reference, not labels or name alone. Deployment candidates are owned ReplicaSets with valid positive revision annotation; same-label lookalikes are excluded. StatefulSet/DaemonSet candidates are owned ControllerRevisions. Keep a retained source's name/UID/resourceVersion and revision number together; duplicate revision numbers never choose an arbitrary source. Users select an explicit source row.

Reuse existing bounded pagination and cancellation. Add missing Deployment history paging using the same page size 100 and existing bounded scan/cursor limits. Sort the visible page by exact revision number; disclose additional pages and partial coverage. No unbounded namespace scans, invented total, or global latest/previous shortcut. Refresh invalidates an in-progress selection/review if its captured identities change.

Empty, forbidden, unsupported and failed reads remain distinct. Partial permissions do not blank the workload Overview. No automatic fallback from forbidden history to guessed rollback. No retained revisions means an explanatory empty view. The history source may be inspected without granting permission to patch the parent.

## Review and restore semantics

Select a retained row and choose Roll back. Fetch the current parent and selected historical source using their captured identities and verify ownership. If either UID/version differs from the selection, require refresh and a new review. Capture the resulting patch natively; the frontend cannot supply arbitrary source data as a trusted revision.

For Deployment, copy the historical ReplicaSet's Pod template while removing only the controller-generated `pod-template-hash` label. Preserve other historical template labels and annotations. Preserve the live parent selector, replicas, strategy and top-level metadata. Do not copy ReplicaSet replica counts or annotations into the Deployment.

For StatefulSet/DaemonSet, reconstruct the historical `spec.template` from recognized controller-produced ControllerRevision data. Keep live replicas, selectors, service name, claim templates, retention policy, scheduling/update strategy and other parent fields unchanged. Secret and ConfigMap references in the template remain references: rollback does not restore their previous contents or guarantee an unchanged image behind a mutable tag.

Show one review with current versus selected template diff, source revision/name and target cluster/namespace/workload. Run required server dry-run on the conditional parent patch, show its outcome and expandable server changes/warnings, then enable ordinary Confirm. If the selected template already matches, show No template changes and do not write. Malformed history, admission rejection or denied access produces a useful error, not another permission gate.

Use the current five-minute review expiry, bounded 2 MiB documents and 30-second operation deadline. Existing coordinator locks must serialize rollback with edit/scale/restart/delete against the same resource identity. Confirm submits a token bound to parent identity/version, selected source identity/version, exact normalized template and patch. Execution rereads the source to verify it still matches before consuming/dispatching the write; token handling must still prevent duplicate executions. Recheck parent and connection generation and preserve the server UID/version tests for the final race.

The source and parent cannot be updated atomically across two Kubernetes objects. A source change after the final check cannot alter the already captured reviewed patch; it also must not be described as an atomic snapshot of live history. Missing/changed source before dispatch invalidates the review. Do not auto-select another revision or silently reprepare after confirmation.

Send at most one real parent patch. Conflicts require refreshed review. Ambiguous network outcomes remain unknown and are never automatically retried. Store only safe target/source identifiers, operation state and timing in retained results, not historical manifests or diffs; all preview payloads remain local and memory-only, without logs or AI submission.

## Update strategy and result meaning

Restoring a template and updating running Pods are separate facts. Do not block a template rollback solely because its workload uses OnDelete, partitions, paused rollout or zero replicas; disclose its current behavior, let server validation decide and never silently change the strategy. The action does not bypass RBAC or admission.

- RollingUpdate: explain that the controller applies the restored template under its current availability and ordering settings.
- StatefulSet partition: preserve the partition and explain that only eligible ordinals update automatically; no partition override.
- OnDelete: explain that existing Pods are not automatically replaced; the user may use separately available Pod operations if appropriate. Rollback does not delete them.
- Paused Deployment: preserve pause; explain rollout is paused. If API/admission rejects the template change, show that rejection; do not resume the Deployment automatically.
- A stuck StatefulSet can need additional Pod intervention even after its template is reverted. Explain this when relevant; do not bundle deletion or claim rollback repaired the workload.

Use existing operation results for accepted/rejected/unknown. Accepted means template patch accepted, not successful rollout. An explicit Refresh/Check workload rereads the original UID and shows current generation, conditions and ready/updated counts using existing detail evidence. Keep a replacement workload separate. No new polling framework, indefinite waiter, automatic remediation or invented success based solely on matching template.

The [Deployment guide](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/), [StatefulSet guide](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) and [DaemonSet rollback guide](https://kubernetes.io/docs/tasks/manage-daemon/rollback-daemon-set/) explain controller history and rollout behavior. Check current pinned versions while planning implementation; optional missing fields remain unknown.

## Minimal verification

Reuse existing history, operation and frontend state suites. Add a small table-driven native group for exact ownership, historical template extraction, Deployment hash removal, unchanged non-template fields, no-op, source/parent changes and dry-run denial. One execution group verifies single dispatch/conflict/unknown outcome and shared locks. One UI state case covers stale selection and a fixed target across tab switches. No per-field or per-component suite expansion, screenshot snapshots or coverage target.

Validate the real flow on only kind-dev-local-a/b with synthetic owned Deployment, StatefulSet and DaemonSet fixtures retaining two known templates. Confirm selected template restoration and unchanged replicas/strategy; include a denied/conflict case using existing scoped test identities or fakes. Cover OnDelete/partition/paused messaging and decoding through representative tests rather than building a large live matrix. Do not create extra clusters, install operators or mutate system workloads. Restore/clean only owned fixtures.

The implementing agent verifies the packaged app's history → diff/dry-run → Confirm → refreshed workload journey, no-op and error presentation, existing themes/layout and cross-cluster target retention. Run repository checks/package and one independent whole-PR review; no duplicate manual user UAT checklist. Record actual results and limitations in `docs/superpowers/validation/phase-1d5.html`, update verified plan checkboxes/checkpoint and follow the current Git workflow through authorized ready PR/merge. No release or next increment.

Acceptance:

- All three controller types expose real owned retained revisions with truthful partial/empty/denied states.
- A selected historical template yields the correct diff and dry-run without reverting unrelated parent fields.
- Ordinary confirmation sends one conditional write to the original target; stale parent/source and ambiguous outcomes are handled without replay.
- Strategy behavior and template acceptance are distinct from rollout completion; no automatic deletion or forced strategy change.
- Current UI and permissions model remain intact, with minimal representative tests and native verification.

## Next step

Superpowers builder review found no important unanswered planning questions. Review this specification, then create the Superpowers implementation plan after approval. No application implementation or cluster mutation occurs while authoring this spec.
