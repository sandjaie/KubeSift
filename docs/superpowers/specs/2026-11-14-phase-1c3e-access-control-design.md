# Phase 1C.3e — Read-only access control

**Date:** 2026-11-14. **Status:** Planning package for a separate implementation session. Existing checkout; no worktree. This session changes documentation only.

## 1. Outcome and baseline

Inspect ServiceAccounts, Roles, RoleBindings, ClusterRoles and ClusterRoleBindings, and follow explicit Pod → ServiceAccount and binding → role/ServiceAccount references. Explain recorded RBAC configuration without claiming complete effective access, identity membership or successful authorization. Use dedicated bounded views on the existing explorer; a generic YAML-only view would hide rule/subject scope, while a permission simulator would exceed this increment.

Storage merged as `eff5135` through PR #11; planning baseline is develop `94fd096`. Recheck actual source before implementation. Preserve current storage/configuration/networking/workload UI and navigation, Secret metadata-only handling and Logs lifecycle. In particular, preserve storage review fixes: expanded nested content mounts only on demand, and navigation presents the exact capture the resolver validated. The [storage report](../validation/phase-1c3d.html) records synthetic CSI coverage and unexercised native denial; these remain named compatibility gaps. Repeat overlapping state/permission journeys, not unrelated manual UAT.

Follow the roadmap's access-control scope. No product writes, token generation, impersonation, SubjectAccessReview/SelfSubjectAccessReview/SelfSubjectRulesReview calls, external directory lookup, permission simulation, reverse cluster-wide binding scans, user-account management, release or next phase. Use current dependencies without upgrades. Progress stays in the SDD checkpoint, task plan and validation report; the roadmap owns sequence/scope only.

## 2. Resource, scope and tab contract

Create an **Access control** navigation group with all five kinds. Rename the existing Cluster & access group to **Cluster**, retaining Namespaces and Nodes in place, and remove its combined Roles & bindings placeholder. This is a resource grouping extension, not a shell redesign.

As the user-requested small navigation refinement in this phase, move **ConfigMaps** and **Secrets** from Configuration to **Workloads**, after CronJobs in that order. **Configuration** retains ResourceQuotas and LimitRanges. Each destination appears once. This changes sidebar placement only: retain kind IDs, configuration reader/model routing, tabs, namespace selection, Back history and existing metadata-only Secret safeguards. A navigation group does not determine backend resource type; do not reclassify these kinds as workload controllers or expose workload-only tabs/actions. This supersedes their sidebar placement in the historical 1C.3c specification.

| Kind / API | Scope | Inventory columns after Name | Tabs |
| --- | --- | --- | --- |
| ServiceAccount / v1 | Selected namespace | Automount configuration, Image pull references, Age | Overview, YAML, Events |
| Role / rbac.authorization.k8s.io/v1 | Selected namespace | Rule count, Age | Overview, YAML, Events |
| RoleBinding / rbac.authorization.k8s.io/v1 | Selected namespace | Role kind/name, Subject count, Age | Overview, YAML, Events |
| ClusterRole / rbac.authorization.k8s.io/v1 | Cluster | Rule count, Aggregation configured, Age | Overview, YAML |
| ClusterRoleBinding / rbac.authorization.k8s.io/v1 | Cluster | Role name, Subject count, Age | Overview, YAML |

Use existing compact inventory, Overview, YAML and Events components and theme tokens. Disable the namespace chooser for cluster kinds while retaining the saved namespace; namespaced selection retains the selected kind. No all-namespace lists. An empty list, forbidden read, unsupported collection API and missing detail are distinct. Failure to list ClusterRoles must not disable allowed Role/ServiceAccount reads. Events on the three namespaced kinds use checked UID and existing API-family rules; cluster Events reject before I/O, with no all-namespace scan.

RBAC resource names use Kubernetes path-segment validation, not DNS validation: support colon-containing system names and retain literal case. Reject empty names, `.`/`..`, slash and percent via the pinned apimachinery validator. Use typed REST name encoding, never concatenate names into URLs. Apply this rule only to the four RBAC kinds; ServiceAccount and all existing kinds keep their current applicable validation. User/Group subject names are display strings, not resource addresses and not DNS names.

## 3. Overview and rule presentation

**ServiceAccount:** identity/metadata, automountServiceAccountToken true/false/not specified, imagePullSecrets names and legacy secrets references as captured configuration. References are text only: never GET Secret bodies/metadata automatically, request tokens, decode credentials or count usable credentials. List projection includes image-pull reference count, not referenced names or arbitrary metadata. Add a compact Access control section to Pod Overview showing the actual serviceAccountName and Pod automount setting; link only a nonempty captured serviceAccountName in the same namespace. Do not synthesize a default account link from an empty field or fetch the ServiceAccount to infer effective token mounting.

**Role/ClusterRole:** preserve every rules[] boundary. One expandable rule block shows Verbs, API groups, Resources/subresources, Resource names and Non-resource URLs. Display empty API-group string as “Core API group” while retaining raw values for copy/YAML. Display `*` explicitly as a wildcard; show `pods/log`, `pods/exec` and other subresources literally. Missing/empty resourceNames means “No name restriction in this rule”, not a deny rule or all-resource permission independent of verbs/resources. Non-resource paths are inert text and never opened as URLs. Unknown/malformed combinations remain visible with “Unrecognized rule shape”, without silently dropping fields or inventing permission conclusions.

RBAC grants are additive; an empty rules array is “No rules captured”, not “No access”. No privilege scores, risk verdicts or flattened matrix that joins fields across separate rules. Explain that a ClusterRole referenced by a RoleBinding is bound within that RoleBinding's namespace; do not label that relationship cluster-wide merely because the role object is cluster-scoped. Non-resource URL rules apply through ClusterRoleBindings; viewing a ClusterRole alone does not prove that any subject has those permissions.

**Aggregated ClusterRole:** show aggregationRule.clusterRoleSelectors separately from captured rules. Preserve selector boundaries, matchLabels and matchExpressions/operators/values; multiple selectors are alternatives, while requirements within each selector remain grouped. Label rules as captured controller-managed rules when aggregation is configured. Do not enumerate matching ClusterRoles, recompute aggregation or assume selectors and rules were reconciled synchronously. A zero-rule capture is not proof that no roles match.

**RoleBinding/ClusterRoleBinding:** show roleRef API group/kind/name and explicit binding scope, then a paged subject table with Kind, Name, API group, Captured namespace and Resolution namespace. RoleBinding can reference Role in its own namespace or ClusterRole; ClusterRoleBinding can reference only ClusterRole. Preserve malformed references as non-navigable evidence with a reason. User and Group rows are external identity strings, not clickable Kubernetes objects; do not split names, infer group membership or treat the `system:serviceaccount:` prefix as a ServiceAccount object reference.

ServiceAccount subjects require core API group (empty). For a RoleBinding, an omitted subject namespace resolves to the binding's namespace and is visibly labeled “Binding namespace”; an explicit subject namespace may differ from the binding's namespace. For a ClusterRoleBinding, missing ServiceAccount namespace remains unresolved/non-navigable, not the current chooser value. User/Group API-group defaults may be explained as RBAC conventions, but no object lookup is permitted. Subject namespace never changes the binding's permission scope. Duplicate subjects remain separate captured entries.

Rules, subjects and selector arrays page locally at 100 entries. Mount expanded nested values only on demand; cap/paginate long lists inside a rule too, not just the outer rule list. Show captured counts without claiming global completeness. Strings remain escaped/selectable; YAML uses the existing server representation, managed-fields toggle and copy-exactly-displayed behavior, with the existing serialization limitations. Identity strings, annotations and rule content stay local/ephemeral, outside logs, telemetry and AI payloads. No universal redaction claim.

## 4. Explicit reference resolution

| Source | Target | Namespace |
| --- | --- | --- |
| Pod spec.serviceAccountName | ServiceAccount | Pod namespace |
| RoleBinding roleRef.kind=Role | Role | Binding namespace |
| RoleBinding roleRef.kind=ClusterRole | ClusterRole | Cluster |
| ClusterRoleBinding roleRef.kind=ClusterRole | ClusterRole | Cluster |
| RoleBinding ServiceAccount subject | ServiceAccount | Subject namespace, or binding namespace if omitted |
| ClusterRoleBinding ServiceAccount subject | ServiceAccount | Explicit subject namespace only |

No other reference routes are introduced. roleRef.apiGroup must equal `rbac.authorization.k8s.io`; ServiceAccount subject apiGroup must be empty. Unknown kinds/groups or invalid names stay readable but reject navigation before target I/O. A binding's roleRef immutability does not eliminate source UID/resourceVersion validation: the binding can be deleted/recreated or its subjects changed.

On click, backend validates captured source connection/generation/kind/API/scope/UID and nonempty version, re-reads source and compares version, verifies the requested target occurs in an allowlisted field with the exact effective namespace, then performs a bounded GET. Client targets are not arbitrary GET authorization. Reject forged cross-connection/scope/kind requests and stale source before target I/O. Optional client target UID must match the capture. These references have no authoritative target UID in the source; label successful navigation “Resolved by name at open time”, never ownership or historical identity proof.

Return the same capped ResourceDetail capture used to validate the target, with source/target versions and captured target UID. The frontend opens that capture without a second initial GET; later explicit Refresh may capture a new version and clears prior resolution badges. Back retains bounded history and saved scope/position. Binding→ServiceAccount can change namespace only to the validated subject namespace; RoleBinding→ClusterRole retains the saved namespace. Errors remain local with source evidence visible. No prefetch, reverse subject membership listing, derived “used by” inventory or automatic role expansion on binding selection.

## 5. Bounds and safeguards

Add dedicated AccessControlRow/AccessControlPage projections and explicit version/scope dispatch, reusing bounded reads, cancellation, ResourceRef, ResourceDetail and YAMLDocument. Inventory summaries contain only spec-defined columns; no full rule/subject arrays or metadata payloads. Separate resource-specific scope/name validation from display subject strings. Do not weaken existing Secret or resource guards while extending shared details.

Retain API page 100, cursor history 20, encoded cursor maximum 16 KiB, Back history eight, request deadline 30 seconds, 16 concurrent desktop reads, decoded HTTP-body cap 16 MiB and serialized page/detail/YAML/resolver cap 2 MiB. Local nested pages are 100 entries. Empty pages with continuation remain navigable; cursors bind kind/scope/connection generation. Search/sort applies only to captured inventory pages. No watches or unbounded accumulated data.

Read-only operation allowlist is resource GET/LIST plus existing namespaced Events. Never call TokenRequest, TokenReview, access-review APIs, impersonation or write APIs as a product feature or permission probe. Fixture setup may create narrowly scoped synthetic RBAC only in the approved local clusters under existing development authorization. Keep product and setup privileges distinct. No new RBAC bindings to real users, built-in system groups, existing workload identities or default ServiceAccounts.

## 6. Acceptance and delivery

| ID | Required evidence |
| --- | --- |
| A01 | Five kinds/APIs/scopes/tabs, independent list/get permissions, forbidden/empty/unsupported distinction and retained kind |
| A02 | `system:` RBAC names open correctly; path-invalid names reject before I/O; existing DNS validation unchanged |
| A03 | Rule boundaries, core group, wildcards, subresources, resourceNames, non-resource paths, empty and malformed rules without effective-access claims |
| A04 | Aggregation selector boundaries and captured rules, missing/empty rules, no selector-driven lookups |
| A05 | Binding role kinds/scopes; User/Group strings; SA subject explicit/defaulted/missing namespace; duplicates and mismatched API groups |
| A06 | ServiceAccount/Pod optional automount settings, empty Pod account name, inert Secret references and zero token/review/Secret API requests |
| A07 | Every allowed reference and forged target, source UID/version changes, reused target UID, same validated capture on open, local errors |
| A08 | Cursor/cap tests, nested lazy rendering, >100 items, cancellation/stale responses and independent cluster/namespace state |
| A09 | Existing storage/configuration/Secret/networking/workload/YAML/Events/Logs and Back regressions |
| A10 | Packaged desktop Pod→SA and binding→role/SA→Back; cross-scope restoration, keyboard, light/dark, narrow split, 200% zoom and YAML copy |

Use explicit kind-dev-local-a/b only. Reuse fixture conventions with owned namespace(s), dedicated unprivileged ServiceAccounts, narrow read-only Roles/ClusterRoles and their bindings, colon-containing names, and an unscheduled synthetic Pod with automount disabled. Cluster-scoped fixtures require collision checks/ownership inventory; bind narrow rules only to dedicated test ServiceAccounts. Do not modify `system:` built-ins or bind admin/escalate/impersonate/token privileges. For cross-namespace subject tests use a second owned namespace. Aggregation fixtures use unique labels and synthetic ClusterRoles so they cannot select or contribute to built-in roles. Use transport/rendering fixtures for malformed API objects or edge conditions that real admission disallows.

Test restricted identities with allowed namespaced access but denied cluster RBAC, and a binding readable while its target role GET is denied. Isolated fixture credentials stay untracked; do not use trial writes to probe privileges. Cleanup only owned fixtures, preserve unrelated rules and report cleanup failures. Unit fakes do not prove wire encoding, RBAC enforcement or native behavior; use HTTP tests, real integration and packaged desktop evidence for their respective claims.

Implementer records one report of actual automated/integration/native results and open limits, with exact package identity and cleanup. No duplicate manual user UAT. If desktop tools are unavailable, record that unmet gate instead of treating browser evidence as native acceptance. After full implementation/testing, perform one fresh whole-PR review with the repository skill, fix findings and obtain targeted rechecks, then ready PR and merge under standing authorization. No per-task review gates, release or 1C.3f implementation.

## 7. Sources and handoff

Current official docs checked through Context7 on 14 November 2026: [RBAC concepts, naming and scope](https://kubernetes.io/docs/reference/access-authn-authz/rbac/), [v1.37 API definitions](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/). Namespace fallback was cross-checked against [upstream RBAC subject resolution](https://github.com/kubernetes/kubernetes/blob/v1.37.0/pkg/registry/rbac/validation/rule.go). Pinned k8s.io/api v0.37.0 Subject/RoleRef and apimachinery path-segment validation were inspected locally; keep those version assumptions explicit.

Use the [five-task plan](../plans/2026-11-14-phase-1c3e-access-control.md) in the separate authorized session. Current completion evidence remains in the checkpoint and phase reports, not the roadmap.
