# Phase 1D.1 — Reviewed workload scaling

Planning only; implementation in a separate session. Owners: [roadmap](../../roadmap/roadmap.html#phase-1d), [security](../../security/security-model.html), [UX](../../ux/workflows.html), [verification](../../engineering/engineering.html#agent-verification).

## Outcome and choice

Scale one inspected Deployment or StatefulSet through an explicit review, without retargeting, stale writes or automatic retries. Establish reusable operation boundaries by delivering one complete operation rather than a generic mutation framework with no product journey. Existing `internal/operation` provides a five-minute, single-use ConfigMap marker experiment; preserve that validation-only boundary while adding a separate product ScaleService.

Chosen first slice: reviewed scaling for apps/v1 Deployments and StatefulSets. YAML edit/apply introduces field ownership and diff semantics; delete introduces propagation/grace choices; exec and forwarding introduce session ownership. Those remain later 1D specifications, alongside restart and rollback. ReplicaSets, ReplicationControllers, custom `/scale` resources, bulk actions, Secret reveal, metrics, automatic rollback and release are outside this increment. Generic API browsing stays read-only.

## Product flow

1. The detail header for either supported kind offers **Scale…**. Opening it captures connection ID/generation, cluster label, namespace, API/kind/name, UID and displayed resourceVersion. No action is inferred from current mutable selection after capture.
2. A compact dialog shows the immutable target, current desired count and an integer input. Input accepts 0 through 2,147,483,647; reject fractions, negatives, exponent notation, blank and overflow in native code too. Send the requested count as decimal text across the bridge; native code accepts only ASCII digits and parses the bounded integer. Do not pass a JSON number that could normalize exponent notation before validation. This is an API range, not a capacity recommendation. Unchanged desired count is a no-op with no dry-run/write or confirmation token.
3. **Review change** requests native preparation. Show before→after count, target, expiry, dry-run result and impact. All changes require **Confirm scale**. Zero additionally requires typing the resource name. Scale-down warns that Pods may terminate and PDBs do not establish protection for controller replica reduction. StatefulSet scale-down warns that configured PVC retention policy may delete claims; show the captured policy (including default when omitted), do not promise storage preservation. A universal concise notice explains that autoscalers, GitOps and other controllers may overwrite this value; this slice does not scan HPAs or claim absence of automation.
4. Confirmation sends only the opaque token and required acknowledgement. Editing the count invalidates the review. Changing resource, namespace, cluster tab or connection, closing the dialog, or refreshing the source revokes the pending review and returns to inspection; late prepare responses are discarded and revoked. Focus returns to the action trigger where it still exists.
5. After dispatch, show the operation's frozen target and result. Navigation may continue but must not cancel tracking or change the target. A small app-level result surface remains accessible across tabs. Do not add a new Sessions dock just for scaling. App close does not imply rollback; pending in-flight outcomes may be unknown on the next launch.
6. **Accepted** means the API server accepted the request, not that Pods are ready or the desired count will persist. Update the original resource only if still selected; otherwise retain the target-bound result with an explicit Open target link that revalidates UID. **Refresh target** is read-only. No automatic rollout watch or success inferred from a later matching count.

Reuse current dialog/button/table tokens, appearances and keyboard conventions. Trap dialog focus; Escape revokes a pending review, not an already-dispatched write. Disable duplicate Confirm while submitting. Errors are selectable plain text and use existing safe error styling. No redundant user UAT checklist.

## Operation and permissions matrix

| Operation | API and RBAC attributes | Review |
| --- | --- | --- |
| Inspect target | GET apps/v1 deployments or statefulsets, explicit namespace/name | Existing captured identity plus fresh validation |
| Inspect scale | GET same resource `/scale` | Must return matching UID/name/namespace and resourceVersion |
| Permission hint | POST authorization.k8s.io/v1 SelfSubjectAccessReview, verb update, group apps, resource deployments/statefulsets, subresource scale, namespace/name | Advisory; never a grant |
| Preview | PUT same `/scale`, autoscaling/v1 Scale, dryRun=All | Exact requested replicas, UID and version preconditions |
| Execute | One PUT same `/scale`, same reviewed replicas/preconditions, no dryRun | Single-use token plus acknowledgement; server authorization final |

An explicit permission denial disables preparation with a reason. Failed/unavailable permission review is Unknown, not Allowed: preparation may continue through actual reads and mandatory server dry-run. An HTTP 403 creating the SelfSubjectAccessReview is an unavailable hint (Unknown), not a successful review denying scale. A successful response with allowed=false and denied=true blocks; allowed=false and denied=false is Unknown. Evaluation errors or contradictory allowed/denied fields are Unknown; only an unambiguous successful allowed=true review yields Allowed. No product RBAC edits, probing trial mutations or authorization review for arbitrary user input. Permission hints are bound to the exact target and not cached across generation/review changes.

Dry-run is required for this first operation. If unsupported or rejected, explain and do not offer a bypass. Dry-run invokes normal server validation/admission and is not a permission guarantee for later execution. Reject a dry-run response whose identity or desired replicas differs from the proposed operation; no silent admission rewrite. Surface bounded sanitized server warnings in review. Do not reuse the dry-run response's resourceVersion as a new write baseline: preserve the original captured live precondition.

## Native prepare/execute contract

Implement in `internal/operation` using the connection manager's guarded REST configuration and bounded transport. Share transport bounds through a focused helper if necessary; do not create a second unguarded credential/client path. Keep the marker experiment's fixture restrictions unchanged.

Preparation validates the fixed kind/API/scope, nonempty UID/version and desired count before network I/O. GET parent and Scale; require the displayed parent UID/version still matches and Scale matches parent UID/name/namespace/version. Missing preconditions or mismatches demand refresh and a new review. Inspect parent deletionTimestamp and reject terminating resources. Capture StatefulSet retention policy from this same parent. Send only a minimal Scale update with identity/resourceVersion and desired replicas, never a whole parent manifest, status update, force flag or arbitrary patch. Verify UID/version enforcement with the real API server; if the server cannot enforce these preconditions, block delivery rather than silently substituting unconditional writes.

A successful dry-run creates an unguessable native token bound to action, connection generation, target UID, original resourceVersion, before/after replicas, reviewed impact evidence and five-minute expiry. At most 128 pending tokens process-wide, cleanup on preparation, expiry and connection close. No token/payload persistence. Issuing tokens does not mutate persisted cluster state.

Execute atomically consumes the token before network work, including failed acknowledgement or expired requests. Validate zero-name acknowledgement natively. One in-flight operation per exact target (connection/generation/API/namespace/name/UID); other attempts fail Busy and require fresh review. Revalidate connection and GET parent/Scale, require unchanged identity/version and no deletion timestamp, then send precisely the prepared minimal update. Optimistic resourceVersion/UID in the write itself must cover the race after the final GET. Do not retry conflicts by refreshing the version. No operation can supply a new target or count to Execute.

Use 30-second request deadlines, decoded HTTP body cap 16 MiB, bridge output cap 2 MiB, at most 16 simultaneous operation read/prepare requests and at most four dispatched executions. A prepare uses bounded reads, one permission hint and at most one dry-run; it never scans namespaces/resources. Preserve the repository's total concurrency controls when sharing read infrastructure. Native warning text is bounded to 8 KiB total and rendered as text. Errors never include kubeconfig, credentials, arbitrary response bodies or raw request payloads.

Disable application/client HTTP retries for dry-run and execution (`MaxRetries(0)` or equivalent); inspect transport behavior and test actual requests, not just a mocked service invocation count. Never automatically resubmit a real write after timeout, cancellation, transport failure, 5xx or ambiguous response. Connection closure before dispatch prevents a write; closure after dispatch cannot prove it did not happen.

## Outcomes, cancellation and retention

Return target-bound operation ID/time and one of: `accepted`, `rejected`, `stale`, `cancelled-before-send`, `unknown`. API 403/401 and explicit validation rejection are rejected; 409/404 are stale. A known server rejection is distinguishable from a connection loss after dispatch. A success response that cannot be decoded/validated is unknown; do not claim rejection after a potentially committed request. Matching UID/desired count in a later read is an observation, not proof that this client performed the write. If a valid success response reports a different admitted count, show that reported count distinctly and do not describe the requested value as achieved.

Only pending review can be cancelled with certainty. Revocation racing execution has atomic first-wins semantics: revocation winning prevents dispatch; execution winning retains tracked result and cannot promise cancellation. The dispatched operation uses an operation-owned bounded context so changing tabs/dialogs does not abort it. Shutdown closes transport and reports uncertainty where possible, with no promise of persistence.

Retain up to 100 terminal result summaries in memory for the running app, dropping oldest first; never evict an in-flight record. Results contain target, counts, time and outcome only, no manifest, secrets or credentials. Add list/get-result bridge methods so UI remount or a lost bridge response can retrieve an outcome without calling Execute again. Repeated token submission never resends: it returns the existing operation/result while retained, otherwise an already-used/unknown-token error. Token lookup/tombstones must also be bounded; expire completed lookup entries after ten minutes and evict with their result record. This is session feedback, not a durable audit log. Durable audit retention/export remains a separately reviewed prerequisite for release under the security model.

## Acceptance

| ID | Required proof |
| --- | --- |
| S01 | Fixed Deployment/StatefulSet routes only; malformed target/count/generic resource fails before I/O; no-op has no dry-run/write. |
| S02 | Permission Allowed/Denied/Unknown, denied actual GET/update, mandatory dry-run failure and admission differences; no persisted change during preview. |
| S03 | UID recreation, changed resourceVersion/count/policy, terminating target, expiry, tampering, double-click, concurrent execution and revocation races never write an unreviewed target. |
| S04 | Real server enforces UID/version during race after re-read; one real dispatch despite timeout/5xx/429/bridge loss; unknown remains unknown without retry. |
| S05 | Review displays immutable context/count/impact; zero-name acknowledgement, StatefulSet retention and controller notice; namespace/tab changes invalidate pending reviews and never retarget a dispatched action. |
| S06 | Accepted versus readiness distinction, result retrieval after remount, bounded retention, explicit read-only refresh and UID-safe Open target. |
| S07 | Automated safety tests, both approved kind clusters, packaged desktop scaling/denial/stale/zero/Back journeys, keyboard/themes/narrow split and independent whole-PR review. |

Integration fixtures are task-owned Deployments and StatefulSets with no PVC templates, using only explicit kind-dev-local-a/b. Test 1→2→1 and 1→0→1 on these fixtures; no changes to system workloads. Restricted credentials permit parent/scale get but deny scale update; separate test covers permission-review denial with actual dry-run permitted. Inject uncertain writes with a local test transport against owned fixtures; never disrupt cluster connectivity or install admission infrastructure. Fake HTTP tests cover PVC policy warnings and unexpected admission mutations; do not delete real PVCs for verification. Verify cleanup by recorded identities on both contexts. Preserve 1C.3g limits (optional extension coverage, clipboard and native frame gaps) as historical evidence, not newly passed tests.

Implementation finishes with one independent whole-PR review, targeted fix rechecks and ready-PR merge per [Git workflow](../../engineering/gitworkflow.md). No release, bulk operations or subsequent-phase implementation.

## References and open implementation choices

Current references retrieved through Context7: [Deployment scale API](https://kubernetes.io/docs/reference/kubernetes-api/apps/deployment-v1/), [StatefulSet scale API](https://kubernetes.io/docs/reference/kubernetes-api/apps/stateful-set-v1/), [API concurrency/dry-run](https://kubernetes.io/docs/reference/using-api/api-concepts/). Transport helper placement and dialog component names may follow existing architecture; identity, confirmation, no-retry and bounded-result contracts may not be relaxed without updating this specification.
