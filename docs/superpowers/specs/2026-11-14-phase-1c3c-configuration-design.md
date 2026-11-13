# Phase 1C.3c — Namespace policy and configuration

**Date:** 2026-11-14. **Status:** Planning package for separate-session implementation. No application changes authorized in this planning session. Use the existing checkout; no worktree.

## 1. Outcome and baseline

Inspect namespace constraints and application configuration without leaving the explorer: Namespaces, ResourceQuotas, LimitRanges, ConfigMaps and metadata-only Secrets. Follow an explicit Pod configuration reference and return to the originating Pod. Explain configured policy without claiming measured consumption, effective admission outcomes or live container configuration.

Phase 1C.3b is merged through PR #9 (`1279e6b`), with status recorded at `621d56f`. Planning starts from develop containing `87f024b` (whole-PR review policy). Reconcile the actual branch and current owning documents before execution. Preserve the approved compact tables, neutral theme, split panes, literal search inputs, YAML/Events components and existing Logs lifecycle. Reuse established typed readers and navigation rather than introducing a generic dynamic explorer now.

The roadmap now fixes this as the next increment. Storage, access control, availability resources and generic discovery follow separately. Alternatives considered: storage first provides volume investigation but leaves namespace constraints/configuration unavailable; generic discovery first increases scope and weakens dedicated evidence presentation. Configuration first extends existing navigation with a bounded, useful set of core APIs.

## 2. Resource and tab contract

All five kinds use core `v1`; keep the pinned Go/client dependencies unchanged. Each list and detail operation has independent authorization. No all-namespace inventories in this increment.

| Kind | Scope / navigation group | Inventory columns beyond Name | Detail tabs |
| --- | --- | --- | --- |
| Namespace | Cluster / Cluster & access | Phase, Age | Overview, YAML |
| ResourceQuota | Selected namespace / Configuration | Resource count, Scope summary, Age | Overview, YAML, Events |
| LimitRange | Selected namespace / Configuration | Item types, Age | Overview, YAML, Events |
| ConfigMap | Selected namespace / Configuration | Text keys, Binary keys, Immutable, Age | Overview, Data, YAML, Events |
| Secret | Selected namespace / Configuration | Age; persistent “Metadata only” context | Overview only |

Add Namespace to the cluster group and ResourceQuota/LimitRange to Configuration; activate existing ConfigMap/Secret placeholders. Retain all existing destinations. Namespace inventory is independent of the namespace chooser: inspecting a Namespace does not change the active filter. “Browse namespace” explicitly changes the saved namespace and opens ConfigMaps; policy links open ResourceQuotas or LimitRanges filtered to that namespace. Empty, forbidden, unsupported and failed are distinct states. Denied Namespace reads must not disable permitted namespaced reads.

Namespace selection retains the selected namespaced kind and clears old scoped selection/evidence. In cluster-scoped Namespace views disable the chooser, label scope “Cluster”, and retain the prior namespace for return, as for IngressClass. Histories remain per cluster tab and kind; use the existing eight-entry Back stack for referenced-object journeys. Never let a late inventory response clear an independently opened related object.

## 3. Evidence views

**Namespace:** show phase, creation time, labels/annotations, conditions with raw reason/message/timestamps, and spec finalizers as read-only configuration. Active/Terminating are observed states, not a cluster health score. No finalizer removal, deletion or namespace creation. Policy buttons list configuration; they do not infer that no policy applies from an empty captured page.

**ResourceQuota:** one table row per resource key in the union of spec.hard, status.hard and status.used. Columns: Resource, Configured hard, Reported hard, Reported used. Preserve exact Kubernetes quantity strings; missing is “Not reported”, zero remains zero. Do not sum unlike quantities, calculate utilization bars or substitute spec limits for absent status. Show scopes and scopeSelector expressions with their actual operators and grouping, without evaluating them against workloads. Explain that allocation/accounting is not observed CPU or memory use. Several quota objects remain independent; do not compute an “effective quota”.

**LimitRange:** one section per spec.limits item, retaining item type and boundaries. Rows are resource keys; columns Min, Max, Default limit, Default request, Max limit/request ratio. Preserve exact quantities and absent versus zero. Do not merge Pod, Container and PersistentVolumeClaim items or several LimitRange objects. Display configured defaults/constraints, not predictions of admission or retroactive changes to existing Pods.

**ConfigMap:** Overview shows identity/metadata, immutable true/false/not reported, and text/binary key counts. Inventory DTOs never include values. Data is fetched only when selected (no detail prefetch from inventory); alphabetical key table, Text/Binary type and byte length, with local pages of 100 keys. Selecting a text key opens escaped, selectable monospace text with wrap and explicit Copy value; preserve empty strings and multiline content exactly. Binary values display canonical base64 with “Base64” labeling and Copy base64; do not execute, render or download content. Text search applies only to the current captured key list, not all resources. YAML is the existing typed server representation with managed-fields toggle and copy exactly as displayed. ConfigMaps and metadata can contain sensitive values; do not claim redaction, persist values, log payloads or send them to AI. Clear selected values on scope/resource change, refresh invalidation and tab close.

**Secret:** deliberately limited metadata, not a masked full-resource reader. Request only PartialObjectMetadata/List with strict content negotiation, no full-object fallback. Before crossing the bridge, project only name, namespace, UID, resourceVersion and creation time; omit labels, annotations, managedFields, ownerReferences and arbitrary metadata. In particular, last-applied annotations can embed full resource content. No type, immutable flag, key names/counts, data sizes, values, decode/reveal, copy-values, YAML or Events tab. Explain “Only identity metadata is requested. Secret contents are not available in this phase.” Metadata-only access still requires ordinary Secret get/list permissions; it is not a separate RBAC privilege.

Secret response kind/group must match the negotiated partial representation. A 406 or unexpected full representation produces an explicit unsupported-metadata error without retrying a full Secret read. Error messages must not include response bodies. Kubernetes metadata itself can contain sensitive strings: strict projection prevents arbitrary metadata crossing the bridge, but is not a claim that no sensitive text can ever reach native memory. Never call typed Secrets().Get/List through the generic detail, YAML, event or reference paths. Even direct bridge calls for unsupported Secret sections must reject before I/O.

## 4. References and namespace navigation

Add a compact Configuration references section to Pod Overview. Read only the captured Pod's explicit references: ConfigMap/Secret env.valueFrom keys, envFrom sources in regular/init/ephemeral containers; direct and projected ConfigMap/Secret volumes; and imagePullSecrets. Retain container/volume location, key (if present), and optional flag. Optional is configuration, not proof that a target exists. Exclude CSI/plugin Secret references, ServiceAccounts, controller templates, reverse “used by” scans, and arbitrary reference fields. No background target fetches.

Clicking a reference uses a dedicated backend resolver: validate captured source connection/generation, Pod UID and resourceVersion; re-read it and verify the requested reference occurs in the allowlisted fields, then GET the target in the same namespace. Reject wrong kind/API/scope, cross-connection targets, missing reference or changed source before target I/O. ConfigMap uses the typed bounded read; Secret uses strict metadata only. Return captured target UID and “Resolved by name at open time”; do not imply that the running container uses these current values or that an optional key exists. A supplied target UID must match. Absent/forbidden/replaced targets leave the source visible with a local error. Reuse Back and source-section restoration.

Namespace navigation is explicit scope navigation, not ownership: opening “Browse namespace” or a namespace policy link validates the captured Namespace UID before routing, then starts a new scoped inventory. Do not combine individually fetched objects into an atomic namespace snapshot. This phase does not add workload-to-Namespace identity links or change Ingress TLS Secret navigation.

## 5. Read architecture and limits

Use a dedicated configuration inventory projection, sharing ResourceRef, PageRequest/PageInfo, detail/YAML and cancellation infrastructure where safe. Expand kind/scope validation explicitly: Namespace requires empty namespace; every new namespaced kind requires a nonempty namespace. Do not weaken existing kinds to permit all-namespace requests.

Keep API page size 100, cursor history 20, cursor size at most 16 KiB, Back history eight, read deadline 30 seconds, active desktop reads 16, decoded HTTP body cap 16 MiB and serialized bridge response/object/YAML cap 2 MiB. These apply to metadata reads and every new detail/list/resolver response. ConfigMap nested key tables page locally at 100; all content remains within the captured-object cap. Oversized resources fail clearly rather than silently truncating values. Bind continuation to connection generation, kind and namespace; preserve empty pages with continuation and expiry errors. Search/sort is within the captured page, not a globally complete inventory claim.

Namespace, quota, limit and ConfigMap details capture resourceVersion and validate UID. Secret paths need a separate metadata reader sharing the bounded authenticated transport, generation validation, cancellation and sanitized errors; do not install or upgrade dependencies. Namespaced quota/limit/ConfigMap Events use the existing UID-filtered API-family behavior. Namespace and Secret Events are intentionally unavailable; no all-namespace event scan or full Secret validation read. No watches, writes, probes, external contexts or installed infrastructure.

## 6. Acceptance

| ID | Required evidence |
| --- | --- |
| C01 | All five inventories/tabs and explicit scope validation; namespace selection retains kind; cluster tab isolation |
| C02 | Namespace Active/Terminating/unknown conditions, explicit browse/policy routing, denied Namespace list with allowed ConfigMap access |
| C03 | Quota absent/zero quantities, distinct spec/status hard, arbitrary resource names, scopes and multiple quotas without utilization claims |
| C04 | LimitRange item boundaries, defaults/defaultRequest, ratios, missing values and multiple objects without admission predictions |
| C05 | ConfigMap empty/multiline/Unicode/hostile text, binary values, immutable states, exact copy and >100-key paging; no values in inventory |
| C06 | Secret request Accept and partial response validation, 406/no-fallback, full-body rejection, projection stripping annotations, sanitized errors, forbidden direct YAML/Events before I/O |
| C07 | Every allowlisted Pod reference, duplicates/optional/missing keys, same-name replacement, source-version change, no target prefetch, malicious cross-scope request and no unsupported reference traversal |
| C08 | Empty continuing pages, expired/mixed cursors, body/bridge caps, cancellation and stale success/error suppression across namespace, tab and resource changes |
| C09 | Existing workload/networking YAML/Events/Logs/Back remain correct; no Secret full fetch through shared handlers |
| C10 | Packaged desktop complete Pod→ConfigMap/Secret→Back and Namespace→policy journeys; keyboard, narrow split, light/dark, 200% zoom and copy behavior |

Implementer owns automated and desktop verification in one permanent report; no duplicate mandatory user UAT. Real API tests use only explicit kind-dev-local-a/b and owned synthetic fixtures. Use transport fixtures for noncompliant metadata replies and status cases not reproducible on kind; fake-client tests alone do not prove content negotiation or server pagination. Create disposable quota/limit fixtures in an isolated owned namespace so admission rules do not disturb existing explorer workloads. Metadata Secret fixtures contain synthetic sentinels only; never capture real credentials in evidence. Clean up only owned fixtures. Existing 1C.3b unrun desktop edge cases remain named compatibility gaps; rerun overlapping journeys here and do not relabel unrelated unrun cases as passed.

After the entire implementation and applicable tests, run one fresh independent whole-PR review using the repository skill, fix findings and obtain targeted rechecks; follow the ready-PR/merge gate without per-task reviews or another merge instruction. No release or next increment is authorized.

## 7. Sources and implementation handoff

Current Kubernetes documentation checked through Context7 on 14 November 2026: [metadata-only API negotiation](https://kubernetes.io/docs/reference/using-api/api-concepts/), [LimitRange API](https://kubernetes.io/docs/reference/kubernetes-api/core/limit-range-v1/), [ConfigMap API](https://kubernetes.io/docs/reference/kubernetes-api/core/config-map-v1/), [quota allocation example](https://kubernetes.io/docs/tasks/administer-cluster/manage-resources/quota-memory-cpu-namespace/). Strict metadata negotiation intentionally omits the documented full-object fallback. Validate the pinned client's actual wire behavior during implementation.

Execute the [five-task plan](../plans/2026-11-14-phase-1c3c-configuration.md) only in the separate authorized implementation session. Product scope and safeguards remain in their owning roadmap, UX and security documents.
