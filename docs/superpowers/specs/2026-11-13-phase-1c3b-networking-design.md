# Phase 1C.3b — Read-only networking investigation

**Date:** 2026-11-13. **Status:** Scope accepted; written package for review. Implementation requires a separate authorized session. Use the existing checkout; no worktree.

## Intent and approach

Make Service → EndpointSlice → Pod and Ingress → Service investigation possible without leaving the cluster workspace. Show routing and NetworkPolicy configuration honestly, without claiming packet delivery, controller reconciliation or CNI enforcement. Extend the merged Phase 1C.3a baseline; preserve the [approved detail UI](../../ux/mocks/index.html#workload-refinement), [UX ownership](../../ux/workflows.html), [architecture](../../architecture/overview.html) and [security model](../../security/security-model.html).

Selected approach: typed networking reads and small kind-specific views integrated into the existing navigation/evidence lifecycle. A generic dynamic-resource browser is deferred; a separate networking workspace would duplicate target and back-navigation logic. No new design system, dependency upgrade or graph visualization is needed.

Included: Services, EndpointSlices, Ingresses, IngressClasses, NetworkPolicies, structured Overview, read-only YAML, namespaced resource Events, and explicit supported-reference navigation. Excluded: writes, port forwarding, connectivity/DNS/TLS probes, endpoint health checks, traffic metrics, CNI analysis, controller installation, Secret reveal, legacy Endpoints fallback, Gateway API-specific views, Routes, load-balancer infrastructure and generic discovery. Legacy Endpoints remains a later coverage obligation; an unavailable EndpointSlice API is disclosed, not silently replaced.

## 1. Navigation and scope

Networking order: Services, EndpointSlices, Ingresses, IngressClasses, NetworkPolicies. Other deferred destinations stay unchanged.

| Kind | API / scope | Tabs | Inventory columns after name |
| --- | --- | --- | --- |
| Service | v1 / namespace | Overview, YAML, Events, EndpointSlices | Type, cluster IPs, external address/name, ports, age |
| EndpointSlice | discovery.k8s.io/v1 / namespace | Overview, YAML, Events | Associated Service label, address type, endpoint count, managed-by label, age |
| Ingress | networking.k8s.io/v1 / namespace | Overview, YAML, Events | Class reference, hosts, reported addresses, age |
| IngressClass | networking.k8s.io/v1 / cluster | Overview, YAML | Controller identifier, default-class annotation, parameter reference, age |
| NetworkPolicy | networking.k8s.io/v1 / namespace | Overview, YAML, Events | Pod selector, policy types, ingress/egress rule counts, age |

Reuse page-local name filtering/sorting, refresh, optional visible-inventory auto-refresh, keyboard selection and remembered detail split. Unknown, empty, forbidden, API unavailable, loading and stale evidence stay distinct. Large lists do not invent totals. Keep full strings accessible when previews truncate.

IngressClass requests have an empty namespace and a visible “Cluster-scoped” label. Disable the namespace control while this kind is active, preserving the last selected namespace for returning to namespaced kinds. Cluster-scoped state/request identity excludes namespace but includes connection ID/generation. A programmatic namespace change must not invalidate/refetch an otherwise unchanged IngressClass view; connection changes must. Namespaced destinations preserve their selected kind across namespace changes and clear old evidence under existing rules. Never interpret an empty namespace as all namespaces for namespaced endpoints.

## 2. Service and endpoint evidence

### Service Overview

Show type, clusterIPs/IP families, headless state, externalName, declared externalIPs, reported loadBalancer ingress, selector, session affinity, traffic policies and publishNotReadyAddresses where present. Keep declared externalIPs separate from status-reported load-balancer addresses. Ports table: name, protocol, port, targetPort (string or integer), nodePort and appProtocol; missing optional values remain absent. Show health-check node port, traffic distribution and allocation settings as configuration when supported by the pinned types.

Headless (`clusterIP: None`), selectorless and ExternalName are valid configurations, not failed Services. ExternalName is a DNS alias reference, not an endpoint health assertion. Do not expand selectors into a supposed backing-Pod set. Service Overview has no ready/healthy aggregate derived from a partial page of endpoints. No addresses are fetched or opened automatically.

### Service → EndpointSlices

Validate Service UID before each bounded list, then list slices in its namespace using kubernetes.io/service-name. This label is **association**, not proof of controller ownership. Include label-associated manually managed slices and display managed-by/owner evidence. If a slice has a Service ownerReference with a different UID from the selected Service, show a relationship conflict and do not navigate through it as verified ownership; retain readable conflict evidence. Missing owner UID means “Label-associated; ownership unverified”, not a fabricated mismatch or omission.

Rows show slice name, address type, endpoint count, association/ownership status and manager. Open the slice for endpoints. Counts apply to the shown batch; no global ready-endpoint totals. Empty association results do not prove a Service cannot route, particularly for ExternalName or selectorless Services. Refresh starts a fresh association snapshot.

### EndpointSlice Overview

Show association labels, address type, manager, owners and slice-wide ports. Render endpoints as rows containing all addresses, hostname, ready/serving/terminating, node, zone and target reference. Expand for hints/topology and supported optional fields. Keep ready, serving and terminating separate, with unknown when omitted; do not paint omitted ready as confirmed Ready. Explain that omitted ready is interpreted as ready by consumers but is not a positive observed readiness claim. publishNotReadyAddresses and terminating conditions prevent a simple green traffic-health badge.

Multiple ports or address families stay explicit; do not flatten slices into an assumed one-address/one-Pod mapping. Endpoint count counts entries, not addresses or distinct Pods. Do not infer Pods from IP/name/node matching. A targetRef to a supported Pod can navigate after target resolution; external/non-Pod targets remain readable references. Cross-namespace Pod refs are displayed but not navigable in this increment. Missing ref namespace is normalized to the slice namespace, never to a mutable UI namespace.

## 3. Ingress and IngressClass

Ingress Overview shows ingressClassName, legacy class annotation separately, reported status addresses, default backend, and a route table: host, path, exact pathType, backend Service and named/numeric port. Preserve wildcard hosts and ImplementationSpecific literally; do not simulate precedence or controller-specific rewrites. Service backend links resolve the referenced same-namespace Service on explicit click. Resource backends remain readable configuration, without fetching arbitrary kinds. Unsupported or malformed backend combinations show unknown/invalid evidence rather than a guessed Service.

TLS shows configured hosts and Secret reference names only. No Secret reads, certificate validation or TLS-success badge. Controller annotations remain inert/selectable data. No external URL fetching or implicit probes.

Explicit ingressClassName links to a cluster-scoped IngressClass. An absent class says “Not specified”; do not guess from a default-class annotation or translate a legacy class annotation into a named IngressClass. IngressClass Overview shows controller string, default annotation value and parameters reference with its explicit API group/kind/name/scope/namespace. Parameters are configuration-only; do not fetch arbitrary custom/cluster resources. Denied IngressClass access must not block permitted Ingress or Service views.

## 4. NetworkPolicy

Show podSelector, policyTypes and direction-specific rules. Preserve matchLabels and matchExpressions, including operators and values. Empty podSelector selects all Pods in the policy namespace; label it explicitly. Present rule boundaries: alternatives across rules/peers are OR; namespaceSelector and podSelector in the same peer are AND. A podSelector-only peer refers to Pods in the policy namespace. Empty namespaceSelector means all namespaces. Show ipBlock CIDR and exceptions without network probing.

Distinguish a direction absent from policyTypes (“Not selected by this policy”), a selected direction with zero rules (“No allow rules in this policy”), and an empty rule (“All peers and ports for this rule”). Omitted/empty peers allow all peers within that rule; omitted/empty ports allow all ports. Ports preserve protocol, named/numeric values and endPort where present. If policyTypes is omitted in the captured object, display API-defaulted interpretation with that label: Ingress, plus Egress when egress rules are present; do not silently claim an explicitly returned value.

Keep a concise note that policies combine additively and enforcement depends on the cluster network implementation. No cluster-wide effective-permission verdict, connectivity proof, policy simulator, selected-Pods expansion or CNI detection in this increment. Unknown/new fields remain visible only to the extent the pinned typed reader preserves them; retain the YAML serialization limitation.

## 5. Reference resolution and back navigation

Reuse ResourceRef and the existing eight-entry Back history, extended across networking and workload kinds. Preserve originating section, inventory position and scope; opening a referenced object outside the current list page must not be cleared by inventory responses. Service → slice → Pod → Logs and Ingress → Service → slice → Pod can return through explicit Back links. Entering/leaving Pod Logs retains its current lifetime rules.

References often contain names without UIDs. Resolve them only after the user clicks, using the captured source identity and source resourceVersion. The backend re-reads and validates the source UID/version and verifies the requested reference is present in the allowlisted source field before GET of the target. If the source version changed, request refresh rather than following stale configuration. The target GET captures its current UID; where the source carried a target UID, require an exact match. With no source UID, disclose “Resolved by name at open time”; do not imply historical identity proof. Back then uses the captured target UID as usual.

Allowed references: Ingress Service backends, explicit ingressClassName, EndpointSlice Pod targetRefs, EndpointSlice associated Service label/owner. Same connection throughout; backend enforces expected target kind/API/scope and ref identity, not arbitrary client-supplied targets. A Service label link is visibly label-associated, with mismatch handling above. No cross-namespace or arbitrary parameter/resource backend traversal. Missing/deleted/forbidden/replaced targets fail locally; keep the source evidence visible. Never follow labels or names as if they were owner UIDs.

## 6. Data, security and limits

Use existing Go typed clients, bounded reads, request cancellation and connection generation validation. Add a networking list path rather than stuffing unrelated fields into workload replica DTOs; share evidence/target/navigation contracts. Expand checked-object validation for explicit networking kind/version/scope combinations; preserve rejection of empty namespace for all namespaced existing kinds. No dynamic API discovery prerequisite, privilege probes or fallback on forbidden.

API inventory/related page size 100, inventory cursor history 20, empty-association traversal at most eight API pages, encoded cursor at most 16 KiB, existing 30-second read deadlines and 2 MiB object/YAML limits remain. Add a 2 MiB serialized networking list/relationship response cap before crossing the bridge; oversized responses return a clear size-limit error, not silently truncated address/route data. Endpoint rows within one captured slice use local pages of 100, with counts “shown / captured”; rendering all nested endpoints is not required. No accumulated relationship history or watches. Cursors bind connection generation, kind, resource scope and association/source UID as applicable; they are not authorization.

Namespaced Events use checked UID and existing API-family behavior. IngressClass deliberately has no Events tab; no all-namespace event scan is introduced. Raw text is escaped; YAML copy means exactly displayed representation, not original manifest or guaranteed redaction. Referenced Secrets, URLs and arbitrary custom resources are never fetched.

## 7. Acceptance and delivery

| ID | Required verification |
| --- | --- |
| N01 | All five inventories/allowed tabs; correct namespace versus cluster scope and independent permissions |
| N02 | ClusterIP, headless, ExternalName, selectorless, dual-stack, named/numeric ports and pending load-balancer evidence |
| N03 | Label-associated manual slices, owner mismatch/recreated Service, all condition combinations, multiple ports/addresses and bounded endpoint rendering |
| N04 | Verified/missing/name-only Pod refs, no IP matching, cross-namespace rejection and accurate back navigation |
| N05 | Ingress default/rule/resource backends, named ports, TLS references, class absence/default/legacy distinction and denied class |
| N06 | Policy AND/OR grouping, empty selectors/rules/directions, CIDR exceptions and named ports without enforcement claims |
| N07 | Source-version change, deleted/replaced targets, cursor expiry/misuse, partial RBAC and cancelled/late reads |
| N08 | Two cluster tabs and namespace changes preserve kind, cluster scope and target isolation |
| N09 | Existing workload YAML/Events/Logs and all prior kind navigation remain correct |
| N10 | Packaged desktop keyboard, light/dark, narrow split, 200% zoom, long/hostile strings, exact copy and full investigation journeys |

Follow [agent verification and acceptance](../../engineering/engineering.html#agent-verification). The implementing agent runs automated checks and desktop computer-use verification, records one report with passed/failed/unrun outcomes, and does not create a mandatory duplicate user UAT checklist. Ask the user only for subjective decisions or specific checks the agent cannot complete. Unrun checks remain open; browser observations do not prove native behavior.

Reuse synthetic fixtures on explicit kind-dev-local-a/b. Create only small, owned fixture objects if needed; do not install an ingress controller, CNI, DNS/proxy or load balancer for this phase. Objects with no active controller or enforced policy are valid configuration-view fixtures and must be labeled accordingly. Status edge cases may use unit fixtures, separately identified from real-cluster/native evidence. No external contexts or traffic tests.

## Sources and handoff

Current official Kubernetes docs were consulted through Context7 on 13 November 2026: [EndpointSlices](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/), [Services](https://kubernetes.io/docs/concepts/services-networking/service/), [Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/), [NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/), [API v1.37 reference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/). Optional API fields remain optional across server versions; existing pinned clients are unchanged.

Use the [implementation plan](../plans/2026-11-13-phase-1c3b-networking.md) in the separate authorized implementation session. No application changes, merge, release or later increment is authorized by this planning package.
