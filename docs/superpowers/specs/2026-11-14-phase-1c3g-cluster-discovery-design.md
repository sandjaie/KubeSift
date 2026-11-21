# Phase 1C.3g — Cluster resources and API discovery

Planning specification. Implementation belongs in a separate session. Follow [UX](../../ux/workflows.html), [security](../../security/security-model.html), [Kubernetes references](../../engineering/kubernetes-development.html), and [verification](../../engineering/engineering.html#agent-verification).

## Outcome and boundaries

Investigate cluster infrastructure and browse installed APIs without installing controllers, expanding privileges or adding mutations. Extend the approved split explorer, captured detail, YAML, error and Back patterns. This phase implements the remaining 1C.3g roadmap slice; it does not establish complete Lens/K9s parity or authorize Phase 1D.

Dedicated destinations:

| Destination | API | Scope | List evidence |
| --- | --- | --- | --- |
| Nodes | v1 | Cluster | Name, Ready condition, scheduling disabled, version, age |
| Events | events.k8s.io/v1; core v1 fallback on API absence only | Selected namespace | Type, reason, message, regarding, occurrences, observed time |
| PriorityClasses | scheduling.k8s.io/v1 | Cluster | Name, exact priority value, global default, preemption policy, age |
| RuntimeClasses | node.k8s.io/v1 | Cluster | Name, handler, age |
| Leases | coordination.k8s.io/v1 | Selected namespace | Name, holder, renew time, duration, age |
| CustomResourceDefinitions | apiextensions.k8s.io/v1 | Cluster | Name, group, kind, scope, Established condition, age |
| APIServices | apiregistration.k8s.io/v1 | Cluster | Name, group/version, service or local, Available condition, age |

All dedicated kinds expose Overview and captured read-only YAML. Reuse resource Events where the existing reader supports the scope safely; do not introduce a cluster-wide Event scan or an Events-on-Event tab. A cluster-scoped resource may have namespaced events: this phase does not promise complete cluster-resource event history.

API resources covers top-level installed resources, including legacy Endpoints, Gateway API kinds, snapshot APIs, admission registrations and custom resources. These have metadata inventory, generic captured Overview and YAML; no specialized topology, interpretation, editors or relationship graph. Availability depends on discovery, supported verbs, API serving and actual read permission. No product-side installation of extensions, metrics servers, controllers or webhooks.

## Navigation and interaction

- Cluster: Namespaces, Nodes, Events, PriorityClasses, RuntimeClasses, Leases. Events and Leases clearly show the selected namespace despite placement under Cluster.
- Extensions: CustomResourceDefinitions, APIServices, API resources. Replace the disabled Custom resources placeholder; leave Helm/OpenShift later-phase placeholders unchanged.
- Preserve Workloads/Networking/Configuration/Storage/Access control placement. ConfigMaps and Secrets remain under Workloads.
- API resources starts with searchable group names, then version and resource selectors. Default version is the advertised preferred version if present, otherwise require selection; never silently substitute on failure. Display scope and group/version alongside kind to disambiguate duplicate names.
- Search filters the loaded catalog or current inventory page, not all objects in the cluster. Group filtering does not imply resource names in unopened group versions have been searched. One selected group-version is loaded on demand; no automatic all-group fan-out.
- Changing namespace retains destination and selected group/version/resource. Namespaced inventory reloads, detail/continuations clear; cluster-scoped view stays selected and keeps its capture. Switching connections retains independent navigation but never shares discovery caches or objects. Back restores the prior route, namespace and valid capture, bounded to eight entries.
- Refresh resources invalidates the selected discovery document and its handles; Refresh object updates only the object capture. Catalog expiry or removed API gives a recoverable refresh state, never a fallback to Pods. Explain denied, missing API, empty inventory, incomplete discovery, expired cursor and oversize distinctly.
- Dedicated kinds selected in API resources open the existing dedicated destination. Include a small explanation for protected/unsupported entries; do not show a second generic version of existing views.
- Use shared tables, surface/colour tokens, icons, keyboard focus and loading states. Long values wrap or expand without forcing panel width. Copy/YAML reflect the displayed capture. Generic values are escaped text, never HTML, links to execute, forms or manifests to apply. Search fields disable spellcheck/autocorrect/autocapitalization as existing code-oriented controls do.

## Dedicated evidence

Nodes show conditions with reasons/times, scheduling flag, taints, capacity and allocatable quantities, addresses and node information. Capacity/allocatable are reported quantities, not current utilization. Missing Ready is Unknown, not healthy. Scheduling disabled is independent of Ready. Do not infer workload ownership or automatically scan all namespaces for Pods or node Leases; no cordon/drain/exec/metrics.

PriorityClasses retain exact numeric value, description and presence of optional default/preemption fields. RuntimeClasses show handler, overhead quantities and scheduling selectors/tolerations; none establishes handler availability on a particular node. Leases retain holder identity, acquire/renew times, duration, transitions and any returned coordination fields as evidence. Do not infer current leadership, expiry/death, clock agreement or availability from timestamps.

CRDs show group, accepted/spec names distinctly, scope, served/storage flags per version, storedVersions and conditions. Schema and conversion configuration remain collapsed bounded evidence; show complete captured YAML on request. A served flag is not proof of read access. Browse instances uses the selected served version only after matching a current discovery descriptor's group, plural, kind and scope; missing/disagreeing discovery disables navigation with an explanation.

APIServices show group/version, priorities, condition status/reason/message, and local versus service-backed registration. Display service/TLS configuration as returned, without interpreting a missing condition as healthy. No direct request to the backing Service, URL, certificate endpoint or webhook; browsing goes through the selected connection's API server. View resources opens the matching catalog group/version after discovery validation.

Events use the existing normalization and modern-to-core absence fallback rules. Select one API family per inventory/cursor; do not combine or double-count families. Preserve the actual Event API/UID for details/YAML. Display relative time with exact timestamp available, series/count evidence, reporting component and full message on expansion. Sort within the fetched page only and label that limit; no globally latest guarantee. A missing timestamp/count remains unknown, not now/zero. Event target navigation is allowed only for already supported dedicated kinds with complete regarding API/kind/name/namespace/UID, matching scope and the same connection. Re-read source and validate its version; read target once, verify UID and return that exact detail capture. Missing UID, unsupported targets, replaced objects and forbidden reads remain explanatory text/errors. No kind-name guessing or cross-namespace inventory scan.

## Discovery and generic-read contracts

Use native Go readers over the existing connection REST configuration and bounded transport; desktop methods remain thin. Existing dedicated readers must work without discovery. Do not add API server infrastructure or new Kubernetes dependency families merely to decode CRDs/APIServices: bounded raw JSON with explicit fixed GVR mappings is sufficient.

1. Fetch unaggregated JSON discovery at fixed `/api` and `/apis` routes independently. Fetch `/api/<version>` or `/apis/<group>/<version>` only for a selected advertised version. Do not follow response-provided URLs or fetch OpenAPI schemas. A failed root/group document leaves other successful documents usable with a partial/error indicator. An empty successful response differs from a failed one.
2. Validate document group/version consistency and every resource path segment, scope and verb. Resource identifiers are canonical group/version/resource tuples (GVR); never pluralize Kind or use short names as REST paths. Exclude subresources containing `/` and connection/proxy endpoints. Require advertised list and get for generic browsing; discovery verbs are capability hints, never RBAC authorization.
3. Native cache keys include connection ID AND generation. Cache root/group-version documents for five minutes, bounded to eight group-version documents per connection using LRU. Root documents allow at most 512 groups and 32 versions/group; each group-version document at most 1,024 resource descriptors. Reject oversize documents with an explicit capacity error; do not silently truncate a supposedly complete catalog. No disk persistence. Invalidate on refresh, generation replacement and connection closure. Enforce existing session limits and release cache allocations on close.
4. Return an opaque native descriptor handle bound to connection generation, catalog epoch and canonical GVR/scope. Every generic read resolves the handle from that cache and rechecks validity. Expired/evicted handles require reselect/refresh; never reconstruct a route from arbitrary frontend GVR/path input. Object address accepts only validated name and, for namespaced resources, explicit namespace. Cluster resources reject a namespace. No caller-selected URL, headers, arbitrary query, field selector or HTTP verb.
5. Exact canonical supported GVRs route to dedicated readers. Block the entire core `secrets` resource from generic full-object GET/LIST before I/O; metadata-only dedicated Secret reads remain the sole route. Alternate versions/aliases/mismatched scope or kind must not bypass this boundary. For other already-supported group/resource pairs, unsupported versions are explanatory disabled entries rather than a generic fallback that bypasses specialized safeguards. A custom resource with a colliding Kind is distinct by group/resource and never dispatches by Kind alone.
6. Generic inventory returns metadata only: name, namespace, UID, creation time and actual GVR. The server may return standard full list objects natively, but never forward whole list items, spec/status, Secret material or raw discovery documents through list DTOs. Discard non-metadata content after bounded decoding. For non-Secret generic detail, provide bounded raw-preserving captured JSON/YAML and metadata; custom resources may contain sensitive literals, so apply existing local-only handling and avoid object values in logs, telemetry, errors or fixture artifacts. This is not a claim that all custom-resource sensitive values can be recognized/redacted.
7. Validate successful responses against expected GVK, scope, object name, namespace and requested UID before returning captures; reject malformed metadata and inconsistent list items. Generic records use a separate GVR-aware identity, not a broadened fixed-kind ResourceRef allowlist. Route transitions store descriptor identity and source capture; cache refresh cannot silently reinterpret an old route as another resource.
8. Raw detail preserves unknown fields and numbers without float64/JavaScript-number round trips. Parse only metadata needed for identity, preserving raw JSON tokens with UseNumber/equivalent native handling; generate captured YAML without precision loss. Overview has identity/source metadata and a bounded, collapsed raw field viewer, not an eager recursive schema renderer. Exact large integer and YAML-copy tests are mandatory. Server representation is not original manifest formatting/comments/order.
9. List uses one API page per call, explicit Next/Previous, opaque continuation bound to connection generation/GVR/namespace/limit. Reject mismatch before I/O. Handle expired continuations by explicit restart; never silently concatenate history. Generic object views expose Overview/YAML only; no guessed Events/Logs/owners or arbitrary subresource reads.

Existing limits apply: page 100, cursor history 20, cursor 16 KiB, Back eight, deadline 30 seconds, active reads 16, decoded HTTP response 16 MiB, serialized bridge output/detail/YAML 2 MiB and nested view pages 100. Discovery cache also respects decoded-body/output caps above; bounded descriptor projections must fit the bridge cap. Cancel on departure and discard late responses using connection generation, route and request identity. Caps apply before serialization as well as transport. Never treat truncated evidence as complete.

## Acceptance and evidence

| ID | Required behavior and proof |
| --- | --- |
| G01 | Seven dedicated destinations enforce fixed API/scope, bounded reads and correct denied/missing/empty states; existing kinds work when all discovery is denied. |
| G02 | Node conditions/scheduling/quantities, Lease times, PriorityClass numbers, RuntimeClass evidence and CRD/APIService fields preserve missing/false/zero distinctions without inferred health. |
| G03 | Event family selection, count/time semantics, page-only ordering and UID-validated target capture; recreated target cannot open as historical object. |
| G04 | Lazy, partial, capped discovery; independent root errors; removed/expired/evicted descriptors and stale-generation responses; no all-group request burst. |
| G05 | Negative-I/O tests prove forged handle/GVR/URL/scope/subresource/namespace/cursor and core Secret routes cannot reach an unintended endpoint; no product writes or credential/body logs. |
| G06 | Generic namespaced and cluster-scoped custom fixtures, duplicate Kind across groups, missing APIs, forbidden LIST/GET and metadata-only inventory. Exact unknown fields/large numbers survive detail/YAML and copy. |
| G07 | CRD→instances and APIService→catalog use matching live descriptors. Generic Endpoints works; installed Gateway/snapshot/admission APIs use the same generic path. Absence is explicit, never fabricated. |
| G08 | Namespace/destination retention, multi-connection isolation, bounded Back, refresh/cancellation, keyboard, narrow split, both appearances and zoom. Reentry never retains stale target evidence badges. |
| G09 | Existing workload/network/configuration/storage/access/availability and strict Secret flows regress successfully. Agent records actual automated, two-kind-cluster and packaged native evidence; one independent whole-PR review and targeted fix checks. |

Unit/fake HTTP tests cover malformed and oversize responses, conversion-webhook-related API errors, partial discovery, denial, boundary bypasses and numeric fidelity. Integration uses only explicit kind-dev-local-a/b and task-owned synthetic fixtures, including namespace- and cluster-scoped CRDs/custom objects without webhooks/controllers, restricted read credentials, core Endpoints and existing admission registrations where available. Synthetic CRDs test generic plumbing; they do not prove actual Gateway/snapshot semantics. Do not install Gateway/snapshot stacks solely for testing. Report optional API native coverage as unrun if absent. Delete owned objects before owned CRDs, remove only task-owned fixtures and verify cleanup on both contexts.

Native verification must cover Node, Event→Pod→Back, CRD→instances→generic object→YAML copy→Back, APIService→catalog, namespace retention and second connection. Unit evidence is not a substitute for claimed desktop observations. Document blockers precisely, do not create a repeat manual UAT checklist by default. Finish under the existing ready-PR/review/merge policy; no release or later-phase implementation.

## Decisions and references

Chosen: dedicated semantic views plus a bounded generic browser. A generic-only implementation would lose infrastructure evidence and existing safeguards; dedicated readers for every installed extension would make the scope unbounded. Chosen lazy unaggregated discovery gives explicit partial failures and bounded fan-out; aggregated-discovery optimization is deferred.

Current official references consulted through Context7: [API discovery](https://kubernetes.io/docs/concepts/overview/kubernetes-api/), [API concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/), [CRD versioning](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/). Exact dependency pins and commands remain in the [toolchain](../../engineering/toolchain.html); recheck API fields during implementation.
