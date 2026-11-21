# Phase 1D.3 — Reviewed workload deletion

Planning only; implementation in a separate session. Owners: [roadmap](../../roadmap/roadmap.html#phase-1d), [security](../../security/security-model.html), [UX](../../ux/workflows.html), [restart/shared operation contract](2026-11-15-phase-1d2-reviewed-restart-design.md), [verification](../../engineering/engineering.html#agent-verification).

## Outcome and scope

Delete one inspected workload through the existing native review/operation coordinator. Explain target identity, controller consequences, background garbage collection and uncertainty without promising immediate removal. Extend the current inspector and mixed Operation results; preserve the reviewed scaling/restart and PR #16 detail refinements.

Supported fixed APIs: core/v1 Pod and ReplicationController; apps/v1 Deployment, ReplicaSet, StatefulSet and DaemonSet; batch/v1 Job and CronJob. All require an explicit namespace and nonempty captured UID/resourceVersion. No generic/discovered resource writes, namespaces, storage, configuration, Secrets, RBAC, CRDs, APIService or other resource deletion. No deletecollection, bulk actions, Pod eviction, drain, force/zero-grace deletion, finalizer removal, orphan/foreground selection, undo, or automatic recreation.

Chosen policy: explicit Background propagation and server-selected grace (omit gracePeriodSeconds entirely). This makes the first deletion increment bounded and avoids exposing force/propagation combinations before their own design. Every supported kind requires the exact resource name typed during confirmation. There is no weaker one-click Pod delete path.

Block terminating targets (deletionTimestamp already set), Pods with the mirror/static annotation `kubernetes.io/config.mirror`, malformed identity/evidence and StatefulSets whose effective persistentVolumeClaimRetentionPolicy.whenDeleted is Delete or unknown. Missing retention policy/whenDeleted uses the API's Retain default with provenance; explicit Retain is eligible. An unknown/malformed policy is not presumed safe. This does not promise all PVCs or application data will survive: existing ownership and other controllers may have effects the app has not enumerated. No scans for storage or dependent objects and no option to override the storage restriction.

## Review experience and impact evidence

Offer **Delete…** as a visually destructive action in the supported resource Overview toolbar. Native eligibility is authoritative. A disabled action has an accessible reason. Review uses existing modal styling/focus behavior and the frozen connection/generation, cluster label, namespace, kind/name, UID and source version, never the current mutable selection.

The user selects Review deletion, sees the fresh impact summary and mandatory server dry-run result, types the name, then selects **Confirm deletion**. Preview contains:

- Exact target and a clear irreversible action statement, Background propagation and “Server default” grace. For Pods show captured terminationGracePeriodSeconds if present/defaulted as an indication, not a guaranteed removal deadline. Do not send that observed value as an override.
- Direct controlling owner API/kind/name/UID, when present, clearly labelled observed ownership. Deleting a controller-owned Pod/ReplicaSet/Job may cause recreation by its owner; a standalone Pod will not be recreated by a controller merely because this UI deleted it. Do not resolve owner chains or infer ownership from labels.
- Workload-kind consequence: deleting a controller schedules garbage collection of its dependents; Deployment/ReplicaSet/RC/DaemonSet/StatefulSet can remove managed Pods, Job deletion can remove its Pods, and CronJob deletion stops future schedules and can remove retained owned Jobs/Pods through garbage collection. These are scope warnings, not an enumerated or complete dependency list.
- Captured desired/current/ready or job activity counts where meaningful, with missing values Unknown. No count is a deletion preview of all dependent objects.
- Bounded finalizer names/count with a partial-display indication, if necessary. Finalizers and grace can keep the target/dependents present after acceptance. Do not offer to clear them.
- For direct Pod deletion, state that it is not eviction and does not obtain PDB admission protection; controller deletion also carries no zero-downtime/PDB guarantee. External automation may recreate resources. No automatic retry or undo.

Only safe bounded projections cross the bridge: no parent manifest, Pod environment, arbitrary annotation values, Secret references or raw error bodies. Mirror annotation is a native presence check; expose only the blocking reason. Cap displayed owner/finalizer lists at 100 entries and 8 KiB total with explicit truncation; owner controller ambiguity/malformed owner entries must yield Unknown ownership with a warning, not a false standalone claim. Keep identity and typed-name controls exact, not truncated.

Closing the dialog, source refresh or resource/namespace/tab/generation changes revoke pending review. Late prepare responses are revoked/discarded. Native modal focus prevents ordinary background clicks; mounted-state tests still cover programmatic changes. Escape revokes pending review and restores focus. Once dispatched, deletion is operation-owned and cannot be cancelled as though unsent; results remain available across navigation. Duplicate confirmation never resends.

## Native preparation and execution

Use action-specific DeleteService with the existing Coordinator and operation HTTP helper. No new client/credential path, generic operation URL or separate per-action capacity budget.

| Step | Contract |
| --- | --- |
| Inspect | Exact supported parent GET; require displayed UID/version, namespace/name/GVK, non-terminating state and eligible policy |
| Permission hint | SelfSubjectAccessReview, verb delete, exact API group/resource/namespace/name, no subresource |
| Preview | DELETE exact parent route with JSON metav1.DeleteOptions: preconditions.uid and resourceVersion, propagationPolicy Background, dryRun [All]; omit gracePeriodSeconds, orphanDependents, ignoreStoreReadErrorWithClusterBreakingPotential and all other overrides |
| Execute | Re-read and validate the same parent/preconditions/eligibility; one DELETE with the same options except dryRun omitted |

Request-level permission-review failure/403, evaluation error or undecided response is Unknown; a successful explicit deny blocks. Actual GET and mandatory dry-run are authoritative at preparation time, not a grant for later execution. No bypass for unsupported dry-run. Denial at execution is still possible.

Encode DeleteOptions natively with apiVersion v1 and kind DeleteOptions. Put dryRun in the DeleteOptions body as specified by this API; do not accidentally copy the patch helper's query-only behavior. The real-server integration must demonstrate that preview leaves the object UID/version/deletionTimestamp and dependents unchanged. Server/third-party unrelated activity may change versions independently; use stable fixtures to make that assertion meaningful.

Preparation validates the deletion response shape: Kubernetes may return a matching resource object or a v1 Status with status Success. Validate identity on an object response; for Status, validate details against the target when supplied, but do not require optional details. Reject malformed/failure payloads and unexpected success codes. Accept documented HTTP 200/202 successful responses only. Do not mistake a dry-run object's simulated deletionTimestamp for an actual deletion.

Issue a five-minute token bound to action delete, generation, exact target UID/version, typed-name requirement, safe impact and serialized options. Retain no full object. Execute consumes through the shared coordinator, checks exact typed name natively, re-reads the parent and validates unchanged UID/version and eligibility. The UID AND resourceVersion inside DeleteOptions enforce the race after that GET. Never refresh the version automatically. A wrong-action token must neither dispatch nor consume another action's review.

Conditional deletion uses the same per-target lock as scale/restart. The lock does not make a whole owner tree atomic: a child operation and parent deletion can race in the cluster, so each still enforces its own identity/version. Do not claim a hierarchy lock or introduce dependency scans. Preserve aggregate limits: 128 pending tokens, 16 prepares, four executes, one execute per exact target, 100 terminal records with active records protected, ten-minute completed token lookup. All operation requests retain 30-second deadlines, decoded-body cap 16 MiB, bridge output 2 MiB and warning cap 8 KiB. Add DeleteOptions request cap 16 KiB. Connection close/expiry/shutdown cleanup remains shared.

Send a non-replayable DELETE body through the guarded bounded transport. Assert wire request counts despite DELETE's HTTP idempotency: no retry on 429/5xx, redirect, transport/auth response, timeout, lost bridge reply or repeated Execute. Dry-run and real DELETE are counted separately. Definite authorization/validation rejection stays Rejected, missing/replaced/conflicting target is Stale, and explicit precondition failure may be classified Stale only with verified evidence. Preserve the existing HTTP 412 handling unless staleness is independently established. Post-dispatch connection loss, timeout, unreadable/oversize/malformed success or ambiguous response is Unknown, not “failed, retry”. No automatic rollback/recreation.

## Results and post-delete inspection

Extend OperationResult with action delete and exactly one Delete result payload. Accepted means deletion was accepted, not that the object or dependents have disappeared. A valid v1 Status Success or matching resource at 200/202 may establish acceptance; failure/malformed/inconsistent responses must never assert success. Preserve original target and outcomes across navigation; do not optimistically remove the selected row before confirmation.

Provide **Check target** as an explicit bounded GET of the original fixed route. It returns a separate observation with checkedAt and one of present, terminating, absent, replaced, unavailable; terminating requires same UID plus deletionTimestamp, absent requires an object-specific HTTP 404/v1 Status NotFound whose safe details match the requested name and canonical group/resource, replaced means the name now has another UID. Denied/transport errors, missing API routes, ambiguous 404s and malformed/inconsistent responses are unavailable, never absent. Parse only bounded safe Status fields (apiVersion, kind, reason, code and details name/group/kind) natively to distinguish object NotFound; do not forward/log raw status messages or bodies. Status details.kind is the resource plural, not the object Kind. This observation does not overwrite accepted/unknown or attribute removal to this client. Readback never re-executes deletion and never follows the replacement automatically. Bind checking to a retained delete operation ID resolved natively, not arbitrary frontend route input.

Keep accepted feedback even if the subsequent read fails. If the original detail is open, refresh it after acceptance through the existing captured identity path and show an absent/terminating state rather than opening a same-name replacement. Offer Back to inventory; explicit user inventory selection may open a replacement as a new resource. No automatic watch/poll of cluster resources or claim that children are gone when the parent is absent. Existing result-list polling may continue only to recover native in-flight results. No persistent audit, result restoration after app restart or undo capability is introduced.

## Acceptance and verification

| ID | Required evidence |
| --- | --- |
| D01 | Eight fixed workload kinds only, exact APIs/scope/identity; blocked terminating/mirror-Pod/unsafe StatefulSet policy and malformed evidence |
| D02 | Review explains Background/default grace, owners/recreation, finalizers, cascading impact, unknown counts and typed-name requirement without sensitive payload leakage |
| D03 | Correct delete permission hint and mandatory dry-run body; preview does not persist; Status/object responses and denial handling |
| D04 | Real server UID/version delete preconditions, race after re-read, recreation, expiry, wrong-action token, duplicate submit and no wire replay |
| D05 | Shared three-action locks/budgets/retention and lifecycle, accepted versus unknown versus removal observation; name replacement cannot be retargeted |
| D06 | Check target present/terminating/absent/replaced/unavailable; no retry, inferred causality, accidental replacement open or lost result |
| D07 | Both approved local clusters, final packaged UI, scaling/restart/detail regressions and one independent whole-PR review with targeted fixes |

Integration uses only explicit kind-dev-local-a/b, task-owned fixture namespaces and known local fixture images. Cover all eight API mappings, typed confirmation/no-preview mutation, role permitting get but denying delete, and unavailable permission hint with permitted dry-run. Create StatefulSets without PVC templates: Retain/default allowed; Delete policy fixture blocked without creating/deleting claims. Mirror-Pod classification can use fake HTTP data; never remove system static Pods. Use one task-owned finalizer-bearing object to verify terminating/readback, then remove only that known test finalizer with explicit-context test cleanup (not a product feature). Verify owned child cleanup for representative Deployment and CronJob cascades; never claim all dependent types are inventoried. Race tests assert server UID/version behavior after final re-read and controlled same-name recreation. Unknown-response tests use counting transport, not cluster outages. No force delete, cluster installs or external context mutations.

Packaged computer-use checks cover standalone Pod and controller review/cancel/confirm, retained finalizer state, stale target/denied outcome, typed-name mismatch, mixed operation results, Check target and Back, both contexts, focus/Escape, keyboard, light/dark and narrow split. Test other API mappings through integration and clearly label native coverage limits. Preserve prior reports as historical evidence; no duplicate manual UAT. Clean task-owned objects/dependents/credentials and verify both-context cleanup. Finish with one whole-PR review, fixes/rechecks, ready PR and authorized merge into develop. No release or next phase.

## References

Current official references retrieved through Context7: [API concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/), [garbage collection](https://kubernetes.io/docs/concepts/architecture/garbage-collection/), [finalizers](https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/), [StatefulSet PVC retention](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/). Shared helper layout may follow the current implementation; deletion options, safety restrictions, identity and outcome contracts may not be weakened silently.
