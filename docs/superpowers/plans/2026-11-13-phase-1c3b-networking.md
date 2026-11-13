# Phase 1C.3b networking implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Use superpowers:subagent-driven-development only if the user selects delegated execution. Steps use checkbox syntax for tracking.

**Status:** Implemented in [PR #9](https://github.com/sandjaie/KubeSift/pull/9). See the [validation report](../validation/phase-1c3b.html) for open acceptance checks. Existing checkout only; no worktree.

**Goal:** Deliver read-only networking inspection and Ingress → Service → EndpointSlice → Pod investigation using the approved UI.

**Architecture:** Add typed networking list/relationship reads to the existing Go service and share ResourceRef, checked detail/YAML/events and navigation lifecycle. Extend frontend kind/scope routing while keeping workload and networking inventory payloads distinct. Resolve name-only references explicitly and validate their captured source before navigation.

**Tech Stack:** Existing Go/client-go v0.37.0, Wails bindings, React/TypeScript, shared CSS and Vitest. No new package or client version required.

**Spec:** [Networking design](../specs/2026-11-13-phase-1c3b-networking-design.md).

## Global constraints

- Existing checkout; no worktree. Preserve unrelated changes. No implementation on develop; follow repository feature-branch rules after execution is authorized.
- Read-only; no probes, Secret reads, installed controllers/CNI, arbitrary URL fetching or write operations.
- API pages 100; history 20; navigation stack eight; empty relationship traversal eight; cursor 16 KiB; read deadline 30 seconds; object/YAML and networking response caps 2 MiB; local endpoint rendering pages 100.
- Preserve the approved shell, palettes and workload/Logs behavior. Only IngressClass has cluster scope; namespaced requests never use empty namespace as all namespaces.
- Follow [agent verification](../../engineering/engineering.html#agent-verification): one report, agent-run desktop checks, no mandatory duplicate user UAT.

## Review focus

- Empty namespace accidentally broadening reads or confusing Back navigation after IngressClass: Tasks 1 and 4.
- Manually managed slices and reused Service names: association labels must not become owner-UID claims: Task 2.
- Name-only references changed since capture or forged targets: source-version and reference validation before GET/navigation: Task 2.
- Policy empty values and AND/OR grouping causing a false connectivity verdict: Task 3.
- Large nested endpoints, untrusted strings and stale responses overwhelming or contaminating the pane: Tasks 1, 3–5.

## Entry gate and source map

Baseline inspected: develop at `a980966`, after Phase 1C.3a merge `ccc47ca`. Check current status and owning documents, refresh safely from origin, and create the implementation feature branch in place only after the user requests implementation. Do not discard/stash another session's work or create a worktree to evade a conflict.

Backend existing files: `internal/kube/{workload_types.go,workloads.go,details.go,events.go,bounded_reads.go}`, `internal/desktop/workloads.go`. Create `internal/kube/{networking_types.go,networking.go,networking_relationships.go,networking_test.go}` for scoped networking payloads/readers. Keep domain logic outside the desktop bridge.

Frontend existing files: `frontend/src/workloads/{api.ts,model.ts,state.ts,useWorkloads.ts}`, `frontend/src/workspace/{Workspace.tsx,ResourceNavigation.tsx,ResourceDetail.tsx,ResourceTable.tsx,NamespaceControl.tsx}`, namespace control owner in `App.tsx`, and icon/styles. Create `frontend/src/networking/{model.ts,NetworkTable.tsx,NetworkOverview.tsx,ServiceEndpointSlices.tsx}` and focused tests. Reuse shared YAML/Events and detail layout; do not rename the entire workload subsystem merely because it now hosts a broader controller.

### Task 1: Explicit kind/scope contracts and networking reads

**Interfaces:** New `Service.ReadNetworking(ctx context.Context, req PageRequest) (NetworkPage, error)`. `NetworkPage { Items []NetworkRow; Page PageInfo }`; `NetworkRow { Resource ResourceRef; CreatedAt *time.Time; ObjectJSON string }`, lower-camel JSON. ObjectJSON contains only the allowlisted inventory projection fields specified below, not an arbitrary whole object. Continue using existing ResourceDetail/YAMLDocument for details. Desktop adapter: `ExplorerService.ReadNetworking(id string, req kube.PageRequest) (kube.NetworkPage, error)` with existing request cancellation pattern.

Projection fields: Service spec.type/clusterIP/clusterIPs/externalName/externalIPs/ports plus status.loadBalancer; EndpointSlice metadata.labels/addressType and endpointCount; Ingress spec.ingressClassName/rules host values plus status.loadBalancer; IngressClass spec.controller/parameters and default annotation; NetworkPolicy spec.podSelector/policyTypes plus ingressRuleCount/egressRuleCount. Include raw configured values, not HTML or health labels. Each frontend projection type is explicit by ResourceRef.Kind; full detail remains a separate read.

- [x] Add failing table-driven tests for all five kinds, correct APIs, namespaced empty-namespace rejection, IngressClass nonempty-namespace rejection, invalid kind/version, generation mismatch, denied/unavailable endpoints, known empty fields and cursor scope mismatch. Assert no API call on invalid target.
- [x] Implement typed lists with 100 limit, projection/cursor scope and 2 MiB response check. Extend `validResource`/`checkedObject` for exact new kind/API/scope pairs without weakening workload validation. Keep cluster-scoped GET handling separate from namespaced helpers; explicitly reject cluster-scoped Events requests.
- [x] Extend shared checked detail/YAML and namespaced Events. Add oversized/malicious-value regression cases and preserve typed-YAML limitation, managed-fields behavior and no external reads.
- [x] Expose desktop method, regenerate `wails3 task bindings`, run `go test ./internal/kube ./internal/desktop`, `go vet ./...`, `npm run typecheck`; commit the bounded read contracts.

### Task 2: Service association and safe reference resolution

**Interfaces:** `ReadServiceEndpointSlices(ctx context.Context, req RelatedRequest) (ServiceSlicePage, error)`; page has `Items []ServiceSliceRow`, `Page PageInfo`, `Complete bool`, `Problem *model.Problem`. A row contains `Slice NetworkRow`, `Association string` constrained to `owner-confirmed`, `label-only`, `owner-conflict`. Do not attach fabricated controller refs to this relationship.

`ResolveNetworkReference(ctx context.Context, req NetworkReferenceRequest) (ResolvedNetworkReference, error)` where request contains `Source ResourceRef`, `SourceVersion string`, `Target ResourceRef`; target UID can be empty only for an actual name-only source reference. Response contains `Resource ResourceRef`, `IdentityEvidence string` (`uid-verified` or `name-resolved`). Desktop methods use the same names with `(id string, req ...)` and existing cancellation. The backend validates source kind/version/UID/resourceVersion, then allowlisted reference existence, expected target scope/API and optional target UID; no generic arbitrary-target GET facility.

- [x] Test label-only manual slices, matching owner UID, conflicting Service owner UID, Service replacement, forbidden/partial lists and expired/mixed relation cursors. A conflict row must stay readable but not become an ownership-confirmed navigation route.
- [x] Implement Service label-selector listing and typed association evidence. Validate parent each request, preserve bounded continuation and response cap. Do not enumerate selected Pods or calculate global readiness.
- [x] Test each permitted reference: Ingress backend Service and explicit class; slice Pod targetRef and associated Service. Cover source version changed, removed reference, wrong namespace/connection/API, missing target, forbidden GET, reused name with UID mismatch, absent UID and cross-namespace Pod rejection. No target GET before source/reference validation.
- [x] Implement resolution with current source snapshot validation and response UID capture. Name-only references disclose resolution time; labels stay associations. Bind slice→Service owner confirmation to the Service UID when the owner provides one. Unsupported targets and no class reference do not trigger discovery or default guessing.
- [x] Regenerate bindings and update API wrappers; run Go unit/race tests and frontend typecheck. Commit relationships and reference contracts together.

### Task 3: Structured networking views

**Interfaces:** `NetworkTable({ rows: NetworkRow[], kind, selected, onSelect })` uses the same selection/keyboard contract as ResourceTable. `NetworkOverview({ data: ResourceDetail, onOpenReference: (target: ResourceRef) => void })` uses the captured resource/version retained by its parent. `ServiceEndpointSlices({ data: ServiceSlicePage, onNext, onSelect })` displays explicit association state. Keep source raw values separate from formatting.

- [x] Add rendering/projection tests for N02–N06: headless/ExternalName/selectorless, named target ports, dual-stack, null endpoint conditions, manual/conflicting slices, ingress default/resource backends, class annotation versus explicit name, and TLS Secret names with no fetch handler.
- [x] Implement inventory columns and Overview content exactly as the spec tables. Service tabs include EndpointSlices; slice endpoints paginate locally at 100 with captured counts. Render ports separately from endpoint addresses; never infer Pods from IPs. Provide full selectable long-value details without injecting HTML.
- [x] Implement NetworkPolicy rules with explicit peer/rule boundaries and AND/OR labels. Test empty policyTypes default interpretation, zero rules in selected direction, empty rule, absent ports/peers, namespace+Pod selector combination, CIDR exceptions and endPort. Do not flatten into an effective allow/deny verdict.
- [x] Keep class parameters/resource backends and unsupported targets readable but non-navigable. Conflicting Service ownership stays visible. Cluster content remains inert and no URL/Secret lookup is introduced.
- [ ] Run frontend tests/lint/typecheck/build. Compare dark/light and narrow layouts with approved resource tables; commit views without new design tokens.

### Task 4: Integrate kind routing, cluster scope and journeys

**Interfaces:** Add `networkKinds` and `ResourceKind = Kind | NetworkKind` in the existing navigation model while retaining workload-only checks for workload API dispatch. Inventory page state accepts WorkloadPage or NetworkPage; both rows expose resource/createdAt. Add `EndpointSlices` section and networking page evidence to section union. Use one existing controller/history lifecycle, not a second independent back stack.

- [ ] Test kind initialization/scope reset for every networking kind, previous namespace preservation for IngressClass, no namespaced fetch during cluster-kind reconciliation and no refetch on a namespace-only change while IngressClass is active. Confirm separate cluster tabs and generation invalidation.
- [x] Extend active inventory API dispatch, ResourceNavigation destinations, icons, Workspace table routing and explicit detail tab sets. Disable namespace chooser for IngressClass via its owning App control; show Cluster-scoped in header and inventory. Returning to namespaced kinds restores the saved namespace, never empty namespace.
- [x] Wire onOpenReference to Task 2 resolution and then existing openRelated/Back validation using captured resolved UID. Keep the origin and its source-version errors visible until navigation succeeds. Display identity/association evidence without claiming lossless historical identity for name-only refs.
- [ ] Test Ingress→Service→slice→Pod→Logs→Back, slice→class-invalid rejection, direct Ingress→IngressClass→Back, eight-entry cap, origin filters/section restoration, old request arrival and outside-page targets. Verify no workload-kind guard silently blocks cross-domain navigation.
- [ ] Run complete frontend and Go regression suites; verify existing Pod/Deployment/ReplicaSet/StatefulSet/DaemonSet/Job/CronJob routes and Logs cleanup. Commit integrated navigation.

### Task 5: Agent desktop validation and delivery

- [x] Inspect existing owned fixtures and reuse Services/Ingresses/NetworkPolicies first. Add only small owned EndpointSlice/IngressClass and policy fixtures needed for gaps on explicit kind-dev-local-a/b. Inventory cluster-scoped fixture names and cleanup ownership. No ingress controller, CNI, cloud load balancer, extra cluster or live traffic probe. Use fake data for difficult status/size cases and identify those tests separately.
- [x] Add `tests/integration/networking_test.go` under the existing integration build tag/config pattern: `TestNetworkingInvestigation` covers list/detail/YAML/events and relationships with same-named distinct objects on both local contexts and restricted read credentials. Reject external targets. Denied-rights tests use read requests only.
- [x] Run `scripts/check.sh`, `wails3 task bindings`, `npm run build`, `go test -race ./internal/kube ./internal/desktop`, then `KUBESIFT_EXPLORER_INTEGRATION=1 KUBESIFT_EXPLORER_CONFIG_DIR=<absolute-owned-config-dir> go test -tags=integration ./tests/integration -run 'TestNetworkingInvestigation|TestWorkloadInvestigation' -count=1`. Build the native package with the documented workflow.
- [ ] Use desktop computer-use tools on that package to verify N01–N10, including both cluster tabs, scope transitions, reference failures, policy readability, exact YAML clipboard contents, keyboard, theme, narrow split and 200% zoom. Record passed, failed and unrun cases with precise build/fixtures. Do not ask the user to repeat completed agent tests; explain any specific agent blocker or subjective question instead.
- [x] Create one report `docs/superpowers/validation/phase-1c3b.html`; do not create a mandatory user UAT checklist. Keep logs/working screenshots under ignored `.superpowers/sdd/2026-11-13-phase-1c3b/`, curate only necessary durable evidence. Update source descriptions, fixture READMEs, index, UX, roadmap, capability coverage and status after verification.
- [x] Obtain independent correctness/hygiene review, resolve issues, rerun affected checks, commit/push the feature branch and prepare a PR. Do not convert outstanding gaps to passes, merge/release or start the next increment without authorization.

Unchecked steps have partial or unrun checks recorded in the validation report. The temporary retained Jobs closed the integration prerequisite. The temporary Ingress and IngressClass closed the native class journey.

## Separate-session prompt

“Implement Phase 1C.3b from `docs/superpowers/plans/2026-11-13-phase-1c3b-networking.md` and its linked spec. This authorizes the scoped implementation only. Use `/Users/sandjaieravi/dev/KubeSift`, the existing checkout, and no worktree. Verify current develop and preserve unrelated work before making the feature branch. Add read-only Services, EndpointSlices, Ingresses, IngressClasses and NetworkPolicies with safe Ingress→Service→EndpointSlice→Pod navigation and correct cluster scope. Preserve all workload and Logs behavior. Run automated and agent desktop verification, record one honest validation report, and prepare the PR. No duplicate manual user UAT, mutations, probes, infrastructure installation, later phase, merge or release.”
