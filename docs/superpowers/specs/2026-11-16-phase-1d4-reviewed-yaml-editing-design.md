# Phase 1D.4 — Reviewed YAML editing

**Status:** Design for review; approach approved, implementation not authorized.
**Date:** 2026-11-16

Owners: [roadmap](../../roadmap/roadmap.html#phase-1d), [security](../../security/security-model.html), [UX](../../ux/workflows.html), [engineering](../../engineering/engineering.html#agent-verification). Baseline: develop `dbfd356`, following deletion merge `cc28d2e`. Preserve the approved current UI and detail refinements.

## Intent and scope

An engineer edits one existing Kubernetes object in its current YAML view, understands the intended and server-effective changes, and submits exactly the reviewed conditional patch. Support every discovered ordinary resource type that exposes the required read/patch API; no workload-only or built-in-kind allowlist. Include workloads, configuration, networking, storage, RBAC, cluster resources, CRDs and installed custom/OpenShift resources. Secret reveal/edit is included through an explicit sensitive-data flow.

Broad editing support is not a promise that every field is mutable or every endpoint permits patch. RBAC, schema, admission, immutability and controller reconciliation remain authoritative. Unsupported routes or denied operations show a specific reason, not an invisible feature. Do not reinterpret generic edit authorization as generic delete/create authorization.

Excluded: creation, multi-document manifests, server-side apply/field ownership takeover, batch changes, arbitrary API URLs, editing subresources (status, scale, approval, eviction, binding), forced replacement, force conflict resolution, automatic retries, persistence/export of drafts and AI requests. No terminal, forwarding or rollback implementation here.

## Chosen approach

The user selected conditional patching of existing resources, including explicit Secret reveal/edit. Compare a captured complete source object with the edited document; generate a native JSON Patch whose first conditions test metadata.uid and metadata.resourceVersion. Reuse the reviewed-operation coordinator, target locks, bounded reviews/results and single-dispatch semantics from Scale/Restart/Delete.

Server-side apply was considered and deferred: declarative ownership/create behavior is a separate product workflow. Whole-object replacement was considered but does not match the selected changes-only approach and increases the chance of losing fields absent from a presentation projection.

Use lossless JSON/YAML transformations: preserve numeric precision, unknown fields, nulls and intentional removals. Never derive an edit baseline from a summary DTO or the read-only viewer's truncated/omitted representation. Kubernetes conditional patch and dry-run semantics are described in the [official API concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/).

## Resource identity, eligibility and field boundaries

Native discovery resolves a connection-bound descriptor to group/version/resource, kind and scope. Fixed-kind and generic views converge on this route, using the existing connection generation, namespace (empty only for cluster scope), name, UID and resourceVersion. Never accept a frontend-supplied arbitrary route. Revalidate stale descriptors and identities before preparation and execution. New or unavailable API versions require reopening the resource; never silently convert an edited document to another version.

Require get and patch support, a durable metadata UID/resourceVersion and successful bounded reads. RBAC hints are advisory; actual reads, dry-run and execution determine authorization. A forbidden response is not an expired-login diagnosis. A discovery failure is not an empty resource list. Unsupported JSON Patch/dry-run is explained; there is no fallback to unconditional writes.

Keep apiVersion, kind, metadata.name, namespace, UID and resourceVersion fixed. Exclude server-managed metadata (creation/deletion timestamps, deletion grace, generation, managedFields, selfLink) and status from the editable document. Native enforcement rejects attempts to inject or change those paths; simply omitting them must never generate removal operations. Preserve them on the server. All other ordinary resource content is editable subject to the server, including ownerReferences and finalizers: highlight these as lifecycle-impact changes in review rather than silently exempting their resource types. Clearing finalizers or changing owners requires typed-name confirmation and a specific explanation of cleanup/garbage-collection consequences. This is an explicit YAML-edit capability, not a change to the deletion action's defaults.

Terminating resources may be inspected and reviewed for permitted metadata edits; the server decides what changes it accepts. Never offer force-delete as conflict recovery. Special API review objects or subresources without persistent identity are not existing editable objects in this workflow.

## Edit capture and draft lifecycle

Select Edit YAML from the current YAML section. Native Get verifies the displayed UID and captures a fresh full object; visibly show if its version changed since inspection before starting the draft. Never quietly start from stale display text. Capture the resource route and connection in the draft header; switching another tab cannot retarget it.

Use a single document with clear read-only identity context and an explicit note that server-owned fields are excluded. Provide text editing, syntax/parse feedback, search and Cancel/Review changes using existing tokens, layout, focus and modal patterns. No new home screen, editor workspace or theme. A lightweight editor is sufficient; package choice belongs to the plan and needs justification before adding a dependency.

One active draft application-wide for this increment, bounded to 2 MiB UTF-8 source and edited documents independently. New edit offers return to the existing draft or explicit discard; never evict it. Leaving the resource with unsaved edits requires keep editing/discard, while cluster-tab closure, disconnect, helper revocation and generation changes invalidate native review eligibility. A disconnected draft can remain labelled local unsaved text, but reconnection requires a fresh capture and review; never auto-rebase or submit it. App exit does not persist drafts. Cancel/close releases native captures and frontend references; no promise of secure zeroization of JavaScript memory.

Parse in Go as well as frontend feedback. Reject duplicate mapping keys, multiple documents, non-object roots, custom YAML tags, excessive nesting and alias expansion; use bounded parser work, not just a post-parse size check. Accept standard scalar values without float64 conversion. No external file/environment interpolation or template evaluation. Empty/no-op edits produce “No changes to review” and no mutation request.

## Diff, server dry-run and confirmation

1. Native preparation compares edited content with the immutable baseline and constructs the minimal deterministic JSON Patch. Escape JSON Pointer keys correctly; arrays preserve order and can be replaced as a whole to keep the first implementation predictable. Never send omitted protected paths as deletes.
2. Read the current object and require the captured UID/version. On change show “Resource changed; reload and review again”; retain the draft until explicit discard, but require refreshed user reconciliation. Do not retry, merge or rebase automatically.
3. Send the conditional patch with dryRun=All, strict field validation where supported, bounded response handling and normal authorization. Unknown fields/pruning or server warnings must be surfaced; do not silently fall back to ignoring invalid input. An API that cannot provide the required validation/dry-run fails preparation clearly.
4. Review separates **Your changes** (baseline to proposed object) from **Server preview** (baseline to dry-run result), showing admission/defaulting differences and warnings. Omit protected server bookkeeping from the readable diff but retain it natively for identity checks. Do not imply that a dry-run preview guarantees identical later admission/controller behavior.
5. Confirmation binds the target, baseline, exact patch bytes and preview digest to a short-lived, single-use native review. Editing any character invalidates that review. Display cluster, namespace/cluster scope, kind/name and impact. Confirm once; do not submit on keyboard navigation or mere modal dismissal.

All edits require explicit confirmation. Require typed exact resource name for Secrets, RBAC, cluster-scoped objects and ownerReferences/finalizers changes; other edits use the existing confirmation interaction. Flag potential workload restart, traffic, storage or privilege effects only when identifiable from changed paths; unknown custom-resource effects remain unknown, not labelled harmless.

Use existing coordinator expiry/capacity and locks shared across edit/scale/restart/delete, keyed to immutable resource identity. Bound native captures, patch and preview to 2 MiB each and at most one edit review; no copied manifests in retained operation records. Read/dry-run/write requests use the existing bounded operation deadlines. Do not hold locks indefinitely while a draft is being typed.

Execution rechecks binding, expiry, descriptor and generation, consumes the review token before dispatch and sends exactly one real conditional patch with the reviewed body. The server UID/version tests remain the final race protection. Never rerun dry-run and silently substitute a new patch or preview after confirmation. Late conflicts require a new review.

Results retain safe target identity/action/timing and accepted/rejected/unknown outcome, not YAML, Secret data, patch bodies or sensitive errors. Acceptance means the API accepted the request; it does not promise rollout health, unchanged webhook output or controller convergence. A separate explicit Refresh reads actual current state and explains possible controller reconciliation. Unknown network outcomes are inspected, never automatically replayed. An unreviewed response difference cannot be undone automatically.

## Explicit Secret reveal and editing

Default Secret lists/details remain metadata-only, including generic discovery routes. Opening an ordinary list or YAML section never fetches Secret bodies. An explicit **Reveal / edit Secret YAML** action explains local sensitive display, requires normal Secret get permission and captures only the selected Secret. It does not grant access to referenced Secrets, tokens or credential stores.

After that action, expose standard Kubernetes YAML with `data` as base64 and optional `stringData` input; explain base64 is not encryption. Do not silently decode arbitrary binary data. Normalize stringData into data in the native proposed object using Kubernetes precedence (stringData wins for matching keys); compare against the captured data, and never patch a synthetic persistent stringData field. Reveal/edit includes type and immutable fields subject to server validation; an immutable Secret may reject data edits without offering delete/recreate.

Secret review initially masks values on both sides, showing keys and added/changed/removed indicators. Mask the entire potentially sensitive value surfaces, including annotations, and sanitize server error details; raw admission messages can echo values. Explicit **Reveal values in this review** exposes the exact local proposed/server-preview content, with an obvious Hide action. Redaction is presentation only: placeholder text is never part of the submitted patch. Clipboard copy is explicit only, with a sensitive-data notice; no automatic copy or download.

Never retain source, drafts, diff text, hashes intended as public identifiers, request/response bodies or raw server errors in operation history, diagnostics, telemetry, public evidence, local storage or AI context. Only safe error category and non-sensitive field paths cross ordinary error paths; Secret key names stay inside the sensitive view. On hiding/cancelling clear revealed presentations; on close/disconnect clear Secret bodies and drafts rather than offering disconnected retention. Do not persist sensitive values in confirmation state after execution. Apply memory-only/no-diagnostic-payload rules to non-Secret YAML as well because arbitrary resources may contain credentials.

## Architecture and compatibility

Go owns descriptor resolution, bounded capture/parser, protected paths, diff/patch generation, Secret normalization, dry-run, review binding and execution. React owns draft presentation and diff navigation but cannot authorize writes or choose endpoints. Extend the current operation coordinator and bounded API transport; avoid a parallel connection manager or mutation subsystem. Keep generic read restrictions intact outside the explicit edit/reveal path.

Frontend and native stale-result checks include draft ID, target identity, connection generation and review revision. Register operation results before dispatch feedback can arrive. Preserve independent cluster tabs, namespace scope, current themes, keyboard accessibility and detail layout. A Secret reveal state must not leak into another resource after navigation.

The implementation plan settles concrete file/DTO decomposition, deterministic diff rendering, parser limits, deadline values by reference to the current coordinator, dynamic endpoint compatibility and focused integration fixtures. It must prove native enforcement independent of frontend controls and inspect pinned dependencies before selecting APIs. Broader resource editing requires updating security/UX owners; old phase-specific read-only restrictions remain accurate history, not a permanent ban.

## Minimal verification and acceptance

Reuse existing operation and resource fakes. One compact table-driven native group covers protected/unknown fields, precision, duplicate YAML, pointer escaping/arrays, no-op, same-name replacement and version conflicts, dry-run changes, consumed-token retry prevention, Secret normalization/masking and denial. Extend a small frontend state group for draft/review invalidation, A/B targeting and explicit Secret reveal cleanup. Do not create per-resource mirrored suites or snapshots.

One opt-in integration group on only kind-dev-local-a/b uses synthetic workload, ConfigMap, Secret, networking, namespaced custom resource and cluster-scoped fixture examples. Include a representative immutable-field rejection, RBAC denial, late version/UID race and simulated ambiguous response. Use owned disposable fixtures and scoped identities; never loosen real roles to make the suite pass. Existing kind APIs need no extra monitoring infrastructure; test unsupported API behavior with fakes rather than installing an operator. Review storage/RBAC/finalizer impact using isolated fixtures or fakes, without modifying shared system objects.

Agent desktop verification covers edit → both diffs → dry-run → confirmation → refreshed evidence; unsaved navigation; conflict recovery; masked/revealed Secret review; both themes and existing laptop layout. Keep checks minimal and risk-focused, run normal repository checks/package and one independent whole-PR review with targeted fixes. Follow current engineering policy: no duplicate manual user UAT checklist by default.

| Acceptance | Evidence |
| --- | --- |
| E1 | Common editor reaches fixed and discovered existing resources in namespace and cluster scope; unsupported/denied endpoints explain why |
| E2 | Protected fields preserved; exact numbers, unknown fields and intended additions/removals survive parsing and patch generation |
| E3 | Review separates intended diff and server preview; failed dry-run cannot enable confirmation |
| E4 | UID/version/generation changes, altered draft, expired/consumed token and concurrent actions cannot silently retarget or overwrite |
| E5 | Secret bodies appear only after explicit reveal, review masking is real, patches contain values rather than placeholders, cleanup and safe errors verified |
| E6 | Accepted/rejected/unknown results remain distinct; no automatic retries, creates, applies or subresource edits |
| E7 | Current UI preserved and representative local/native checks plus independent review recorded |

After implementation create `docs/superpowers/validation/phase-1d4.html` with actual evidence, unsupported API/provider limits and checks not run. Update the portable checkpoint and verified plan checkboxes. Follow the current Git workflow for ready PR/review/authorized merge; no release or next increment is implied.

## Next step

Superpowers builder check found no important unanswered planning questions. Review this specification, then create the Superpowers implementation plan after approval. Planning creates no application changes, fixtures or cluster mutations.
