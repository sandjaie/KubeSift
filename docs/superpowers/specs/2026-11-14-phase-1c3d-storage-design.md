# Phase 1C.3d — Read-only storage investigation

**Date:** 2026-11-14. **Status:** Planning package for separate-session implementation. No application work in this session; existing checkout, no worktree.

## 1. Outcome and baseline

Follow Pod → PersistentVolumeClaim → PersistentVolume → StorageClass and inspect attachment/CSI configuration using the existing explorer. Show observed binding and attachment evidence without implying mounted, writable, healthy or available storage. Extend the established typed readers, compact tables, detail tabs and bounded Back history; do not build a dynamic explorer or redesign the shell.

Baseline: configuration Phase 1C.3c merged through PR #10 as `98801df`; planning begins on develop `ebed580`, which also contains planning-site/toolchain changes. Inspect the actual checkout before execution. Preserve Secret metadata-only behavior, configuration references and outgoing Data clearing, networking links, namespace-kind retention, Logs lifetime and existing themes. The [1C.3c report](../validation/phase-1c3c.html) identifies native rare-status/failure-injection gaps and links earlier networking gaps: these are compatibility limits, not reasons to repeat unrelated manual UAT. Exercise overlapping state/navigation paths in this phase.

The [roadmap](../../roadmap/roadmap.html#phase-1c3-sequence) fixes storage after configuration and before access control. Dedicated read-only views are the chosen approach: generic YAML alone cannot explain binding; installing CSI infrastructure to demonstrate operations would expand scope. Progress belongs in the checkpoint, plan and validation report, never the roadmap.

## 2. Kinds, scope and navigation

| Kind / API | Scope | Columns after Name | Tabs |
| --- | --- | --- | --- |
| PersistentVolumeClaim / v1 | Selected namespace | Phase, Requested storage, Reported capacity, Class, Volume, Age | Overview, YAML, Events |
| PersistentVolume / v1 | Cluster | Phase, Capacity, Access modes, Reclaim policy, Class, Claim, Age | Overview, YAML |
| StorageClass / storage.k8s.io/v1 | Cluster | Provisioner, Default annotation, Binding mode, Reclaim policy, Expansion, Age | Overview, YAML |
| VolumeAttachment / storage.k8s.io/v1 | Cluster | Attacher, Volume source, Node, Attached, Age | Overview, YAML |
| CSIDriver / storage.k8s.io/v1 | Cluster | Attach required, Lifecycle modes, Age | Overview, YAML |
| CSINode / storage.k8s.io/v1 | Cluster | Registered driver count, Age | Overview, YAML |

Place all six in Storage, labeled Volume claims, Persistent volumes, Storage classes, Volume attachments, CSI drivers and CSI nodes. Activate the existing PVC placeholder. Cluster-scoped kinds disable the namespace chooser and preserve its saved value. Selecting a namespace while browsing PVCs retains PVC kind. Denied cluster-scoped reads must not break allowed claim reads. No all-namespace PVC inventory or cluster-wide Events scan. Each resource list/get has independent authorization; no permission-probing mutations or fallback to another API on forbidden.

Keep page-local search/sort and existing split-pane, keyboard and refresh patterns. Related targets outside the current inventory page remain selected. Back restores originating scope, kind, section, filter/page/scroll where supported, within the eight-entry history. PV → claim navigation may change the selected namespace only to the namespace explicitly captured in claimRef; Back restores the previous cluster view and saved scope. No automatic background traversal or guessed default class.

## 3. Overview evidence

**PVC:** requested resources versus status capacity, requested/reported access modes, volume mode, phase, conditions, volumeName, selector and dataSource/dataSourceRef configuration. Preserve exact quantity strings and absent/zero distinction. For storageClassName, distinguish nil (“Not specified”) from explicit empty (“No class”) and a named class. Show resize/allocation fields when present as reported evidence, without usage percentages or a predicted completion time. Snapshot/clone/data-source references are readable but non-navigable in this phase. Pending is a phase, not automatically an error; do not diagnose missing provisioner or WaitForFirstConsumer without its evidence.

**PV:** capacity, access modes, volume mode, reclaim policy, class, claimRef, conditions/reason/message and transition time where served; source kind, node affinity and mount options. For CSI sources show driver, volumeHandle, fsType and readOnly; keep source attributes expandable and inert. Other typed volume sources remain inspectable in structured raw source fields and YAML, without a promise to interpret every legacy driver. Empty class means no class. Capacity is configured/reported provisioned space, not disk usage. Access modes and Bound state do not prove access from a particular Pod.

**StorageClass:** provisioner, reclaimPolicy, volumeBindingMode, allowVolumeExpansion, mountOptions, parameters and allowedTopologies. Show the stable and legacy default annotations separately when present, including conflicts; never choose a default from the current page. Missing optional fields say “Not reported”, not false. Expansion is configuration, not proof that a particular volume can resize. WaitForFirstConsumer describes binding policy, not an error diagnosis. Provisioner is text, not an assumed CSIDriver relationship.

**VolumeAttachment:** attacher, nodeName, persistentVolumeName or inlineVolumeSpec, status.attached, attach/detach error messages and timestamps, and expandable attachmentMetadata. Attached false, true and unavailable evidence stay distinguishable; an explicitly present attached=false means reported false, while missing status or attached field must not be invented as false. Preserve field presence before typed decoding erases it. Explain that attachment does not prove filesystem mount or application I/O. Inline migrated volume specs do not create a fabricated PV link. Node names are plain text until Node browsing is separately implemented.

**CSIDriver:** reported attachRequired, podInfoOnMount, storageCapacity, fsGroupPolicy, volumeLifecycleModes, requiresRepublish, seLinuxMount, tokenRequests and nodeAllocatableUpdatePeriodSeconds where present. Do not render omitted booleans as false or claim every server supports optional fields. Token requests describe configuration; do not request tokens or fetch Secrets. Driver registration/configuration is not health or reachability.

**CSINode:** each spec.drivers entry remains a separate row/section with name, nodeID, topologyKeys and allocatable.count. Missing count is “Not reported”; zero is zero; count is a published attachment limit, not free slots or current usage. Driver rows page locally at 100. Node identity is shown as text; do not derive topology values or health from topology key names. Link driver names only via the explicit resolver below.

YAML retains the existing typed server representation, managed-fields toggle, copy-exactly-displayed behavior and unknown-field/formatting limitations. Storage parameters, attributes, annotations and errors may contain sensitive strings: local ephemeral evidence only, no AI, telemetry or payload logging and no claim of universal redaction. Never follow a path, mount, network endpoint, plugin or Secret reference from displayed storage configuration.

## 4. Reference contract

Add a compact Storage section to Pod Overview, derived from captured spec.volumes. Show volume name/type, explicit PVC claimName/readOnly, and mount/device associations from regular/init/ephemeral containers. Only explicit persistentVolumeClaim references are navigable. Generic ephemeral volumeClaimTemplates, inline CSI, emptyDir, hostPath and other sources are labeled configuration; do not invent a standalone Volume resource, generated PVC name or inferred link. Existing ConfigMap/Secret references remain owned by their current section.

| Source field | Allowed target / scope |
| --- | --- |
| Pod spec.volumes[].persistentVolumeClaim.claimName | PVC, same namespace |
| PVC spec.volumeName | PV, cluster |
| PVC nonempty spec.storageClassName | StorageClass, cluster |
| PV spec.claimRef | PVC, exact declared namespace/name; check UID when supplied |
| PV nonempty spec.storageClassName | StorageClass, cluster |
| PV spec.csi.driver | CSIDriver, cluster |
| VolumeAttachment spec.source.persistentVolumeName | PV, cluster |
| VolumeAttachment spec.attacher | CSIDriver, cluster; named registration lookup, not proof it exists |
| CSINode spec.drivers[].name | CSIDriver, cluster |

Every click supplies captured source ResourceRef and nonempty resourceVersion. Backend checks source kind/API/scope/connection/generation/UID, re-reads it, rejects a changed version, verifies the allowlisted field, then GETs the expected target. Target kind/API/namespace must match this table; client-supplied targets are not arbitrary GET permission. A supplied target UID and any authoritative claimRef UID must match the returned object. No target I/O on malformed/stale source or forged reference. Return captured target UID with `name-resolved` unless a source UID reference established `uid-verified`; disclose name resolution at open time. Keep source evidence visible on missing/forbidden/replaced/conflicting targets.

For PVC↔PV, compare both captured ends. claimRef requires a name and namespace; allow absent kind/apiVersion as the core PVC reference convention, but reject an explicitly different kind or API. If a present claimRef identifies another namespace/name or a nonempty UID differs, reject the relation as conflicting. When opening PV→PVC, a nonempty PVC volumeName pointing elsewhere also conflicts. Matching PVC volumeName plus PV claimRef namespace/name/UID earns “Binding references agree”; missing counterpart or claimRef UID yields “Reference only; binding unconfirmed”. A source reservation with no UID is not historical identity proof. Observations are separate reads, not an atomic binding snapshot; neither badge promises mounted/usable storage. Preserve both source and target version context in the resolver result.

No reverse all-Pod/attachment scans, PV Attachments tab, default-class discovery, generic ephemeral ownership inference, CSI Node→Node navigation, snapshot navigation, arbitrary Secret access or storage writes. Missing CSIDriver registration remains a local link result, not evidence the volume is unusable.

## 5. Reader/state boundaries

Use a dedicated StorageRow/StoragePage projection, not workload replica fields. Integrate explicit kind/version dispatch into detail/YAML and scope validation. Core PVC is the only new namespaced kind; all five other kinds reject nonempty namespace. PVC Events use checked UID and existing API-family behavior; all cluster storage Events requests reject before I/O. Unsupported LIST APIs, empty collections, forbidden lists and missing GET targets remain distinct; no new dynamic discovery dependency or older-version fallback.

Reuse 100-object API pages, 20-entry cursor history, 16 KiB encoded cursor maximum, eight-entry Back history, 30-second read deadlines, 16 concurrent desktop reads, 16 MiB decoded HTTP body cap and 2 MiB serialized bridge/detail/YAML cap. All new list/detail/resolver outputs are capped. Local nested tables page at 100. Preserve continuations on empty pages; bind cursors to kind, scope and connection generation. No watches, accumulated all-page data or uncapped raw projections. Inventory is a deliberate summary: omit parameters, attributes, attachmentMetadata, arbitrary metadata and full volume sources until detail is selected.

Cancellation and stale-success/error suppression cover namespace, cluster tab, kind, section, refresh and Back changes. Scope changes invalidate outgoing evidence; Secret metadata isolation and ConfigMap Data clearing must remain intact. Do not broaden current resource validation into accepting an empty namespace for arbitrary kinds. Keep dependencies and generated binding workflow unchanged.

## 6. Verification and fixtures

| ID | Required evidence |
| --- | --- |
| S01 | Six kinds/APIs/scopes/tabs; list/get denial independent; absent API distinct from empty/missing target |
| S02 | PVC nil/empty/named class, Pending/Bound/Lost, missing/zero capacities, conditions/resize/data sources; no usage claims |
| S03 | PV source types, reclaim policy, affinity, claimRef absent/reserved/matching/conflicting/recreated; exact capacity/access mode evidence |
| S04 | Class optional fields/default annotations, Immediate/WaitForFirstConsumer, topology/parameters and inert hostile strings |
| S05 | Attachment false/true/missing status, error timestamps, inline spec, missing registered driver; CSI optional values and missing/zero allocatable count |
| S06 | All reference routes, no prefetch, source/version/UID changes, name-only disclosure, forged scopes, claimRef UID conflict and no unsupported link execution |
| S07 | Cursor expiry/misuse, empty continuing pages, HTTP/bridge caps and nested paging; independent cluster/namespace state |
| S08 | Pod→PVC→PV→class→Back; PV→claim cross-scope restoration, related targets outside inventory and stale responses after navigation |
| S09 | Existing configuration/Secret protections, workload/networking/Events/YAML/Logs regressions remain passing |
| S10 | Exact packaged desktop keyboard, narrow split, light/dark, 200% zoom, YAML copy and end-to-end journeys; honest unavailable CSI states |

Use only explicit kind-dev-local-a/b and owned synthetic objects. Reuse existing PVCs where suitable; inspect fixture ownership first. Plan a small isolated namespace plus collision-checked cluster objects: nondefault `kubernetes.io/no-provisioner` StorageClass, Retain static PV and explicitly prebound PVC for binding reads, plus an unbound claim. An unmounted local/hostPath PV may be used solely as API configuration evidence; no host directory creation, mounts or application I/O are needed. A suspended/unscheduled synthetic Pod can carry an explicit claim reference without running a storage consumer. Do not force finalizers or delete unrelated resources during cleanup; report retained owned objects if normal deletion cannot finish.

Do not install CSI/provisioners/snapshot controllers or create fake attachments against real nodes/volumes to manufacture a healthy display. Existing genuine CSI objects can provide read evidence. Use synthetic transport/unit fixtures for CSI statuses and edge cases absent on kind, explicitly separated from native/live proof. Test both clusters, restricted PVC-only versus cluster-storage-denied credentials, and missing/unsupported APIs without changing global context. No external cluster or cloud disk operations.

One report records automated, integration and agent-run packaged desktop results, failed/unrun cases and fixture cleanup; no duplicate mandatory manual UAT. Run one fresh whole-PR review after full implementation/tests, fix findings and obtain targeted rechecks, then ready PR/merge under the existing workflow. No release or next phase. Carry forward unrelated gaps without falsely closing them.

## 7. References and handoff

Official Kubernetes sources consulted through Context7 and official API documentation on 14 November 2026: [persistent volumes and binding](https://kubernetes.io/docs/concepts/storage/persistent-volumes/), [PV API](https://kubernetes.io/docs/reference/kubernetes-api/core/persistent-volume-v1/), [StorageClasses](https://kubernetes.io/docs/concepts/storage/storage-classes/), [ephemeral volumes](https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/), [VolumeAttachment](https://kubernetes.io/docs/reference/kubernetes-api/storage/volume-attachment-v1/), [CSIDriver](https://kubernetes.io/docs/reference/kubernetes-api/storage/csi-driver-v1/), [CSINode](https://kubernetes.io/docs/reference/kubernetes-api/storage/csi-node-v1/). Check optional fields against pinned clients and actual server capabilities; no upgrades implied.

Read the [five-task implementation plan](../plans/2026-11-14-phase-1c3d-storage.md) in the separate authorized session. The roadmap owns order/scope only; checkpoint and validation documents own progress.
