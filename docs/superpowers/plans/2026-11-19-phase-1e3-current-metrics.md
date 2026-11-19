# Phase 1E.3 Current Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display current Pod and Node CPU/memory usage in the existing tables and Overview panels without installing infrastructure for the app.

**Architecture:** Add a bounded read-only metrics path to the existing kube service, using discovered Metrics API versions and owned connections. Keep metrics state independent of inventory/connection health, with one visible-workspace polling controller and UID-bound samples. Reuse current table, overview and error components.

**Tech Stack:** Existing Go/client-go v0.37.1/apimachinery/Wails/React/TypeScript. Existing dynamic/REST clients suffice; no metrics framework or charting dependency.

**Spec:** [Phase 1E.3 design](../specs/2026-11-19-phase-1e3-current-metrics-design.md). Baseline `e6dae08`; incorporate the planning commit before implementation. Implementation happens in a separate session.

## Global Constraints

- Read AGENTS.md, owning documents linked by the spec, the Kubernetes development skill and engineering/Git practices. Preserve current UI, explicit targets, source descriptions and minimal tests.
- Metrics API only, discover v1 then v1beta1; no version inference, kubelet/Prometheus fallback, ambient credentials or product installer.
- CPU in millicores, memory in binary units from working-set bytes. Container percentages explicitly name request/limit; Nodes use allocatable; Pod totals have no percentage. Missing values are not zero; >100% is allowed.
- Stale after 60 seconds; reject invalid timestamps/windows or >30-second future samples. Label retained values after refresh failure; clear on identity/auth/scope/disconnect changes.
- Poll 15 seconds after completion, only visible relevant workspace; no overlap. Discovery cache/cooldown 60 seconds, manual Refresh invalidates. Metrics failure does not block resource actions or alter cluster health.
- Batch at most 100 targets, two batches/four target workers app-wide, 30-second deadline. Existing 16 MiB core/discovery cap, metrics 256 KiB/128 containers, bridge 2 MiB. No silent subtotal/truncation presented as complete.
- Use `feature/phase-1e3-current-metrics` from updated develop, existing checkout. Test only kind-dev-local-a/b. Metrics Server provisioning is explicit dev-fixture setup, never product code or default startup.
- One independent whole-PR review after implementation, targeted rechecks, actual packaged desktop evidence, no duplicate user UAT gate or next-phase/release work.

## Review Focus

- Missing/partial quantities, zero denominators and huge/fractional values must not misrepresent usage: task 1.
- Name reuse, absent metric UID and a stale sample from before creation must not attach to the new object: task 2.
- Discovery and resource-specific 403/404 must not cause global outages, fallback credentials or request storms: task 2.
- Tab/page/window changes, wake and delayed responses must not leak polling or revive another target's sample: task 3.
- Live samples, configured denominators and kubectl output have different timestamps/rounding; validation must not claim exact simultaneity: task 4.

## File map and contracts

| Files | Responsibility |
| --- | --- |
| New `internal/kube/metrics_types.go`, `metrics_projection.go`, `metrics.go`, `metrics_test.go` | Narrow DTOs, quantities/coverage, discovery and named reads |
| Existing `internal/kube/read.go`, `bounded_reads.go` and connection cleanup | Own metrics concurrency/cache and reuse bounded transport without changing logs |
| New `internal/desktop/metrics.go`; generated explorer bindings | Cancellable read bridge, no write endpoints |
| New `frontend/src/metrics/api.ts`, `useMetrics.ts`, `MetricsView.tsx`, `metrics.test.tsx` | Typed reads, visible polling, formatted cells and detail |
| Existing `frontend/src/workspace/Workspace.tsx`, `ResourceTable.tsx`, `ResourceDetail.tsx`, `ResourceOverview.tsx`, `frontend/src/cluster/ClusterTable.tsx`, `ClusterOverview.tsx` | Add metrics at current list/detail boundaries |
| `frontend/src/App.tsx` only if needed for native window visibility | Forward existing visibility/lifecycle signal, do not duplicate connection state |
| New `tests/integration/metrics_test.go`, `tests/fixtures/metrics/README.html`, `scripts/metrics-fixtures.sh`; phase validation report | Opt-in provider setup, live evidence and cleanup |

Every owned source gets the repository plain-English description. Small files may be consolidated if responsibilities remain clear. Do not rewrite the existing useWorkloads state machine to put metrics into inventory results.

Native DTOs use camelCase JSON and existing kube.ResourceRef/model.Problem:

- `MetricsRequest { Connection model.ConnectionRef; Targets []ResourceRef; Detail *ResourceRef; RefreshDiscovery bool }`: Detail must match one target exactly; all targets share Connection and kind Pod or Node. Reject inconsistent scope, duplicate/conflicting identities or >100 targets.
- `MetricValue { Amount *string; Request *string; Limit *string; Allocatable *string; RequestPercent *string; LimitPercent *string; AllocatablePercent *string }`: canonical decimal amounts/denominators in CPU millicores or memory bytes, decimal percentages; nil means unavailable, string "0" is valid. Do not pass arbitrary raw Kubernetes bodies.
- `ContainerUsage { Name, Category string; CPU, Memory MetricValue }`: category application/init/ephemeral based on current Pod spec. Containers only returned for Detail.
- `ResourceUsage { Resource ResourceRef; APIVersion, State string; SampledAt *time.Time; WindowSeconds string; FetchedAt time.Time; ConfiguredAt *time.Time; CPU, Memory MetricValue; CPUComplete, MemoryComplete bool; MissingContainers []string; Containers []ContainerUsage; Problem *model.Problem }`. State available/partial/stale/unavailable; refresh failure of a retained sample is frontend state. No age field that becomes silently frozen.
- `MetricsBatch { Items []ResourceUsage; FetchedAt time.Time; Problem *model.Problem }`. Batch Problem identifies shared failures; return matching per-target unavailable records so UI doesn't retain zero-looking holes.
- `(*kube.Service).ReadCurrentMetrics(ctx context.Context, req MetricsRequest) (MetricsBatch, error)`; desktop `(*ExplorerService).ReadCurrentMetrics(id string, req kube.MetricsRequest) (kube.MetricsBatch, error)` uses the existing cancellation/connection lifetime pattern.
- Frontend `useMetrics({ connection, targets, detail, visible })` returns `{ byIdentity, loading, refresh }`; identity includes connection ID/generation, kind, namespace, name and UID. `MetricsCell` and `CurrentUsage` consume that map, not separate readers.

## Task 1 — Quantities, coverage and display contracts

**Files:** metrics types/projection and `internal/kube/metrics_test.go`; no network behavior yet.

**Consumes:** bounded parsed sample plus current typed Pod/Node. **Produces:** ResourceUsage projection used by task 2.

- [ ] Add `TestMetricsProjection` table: CPU `125000000n` → `125` millicores; memory `80Mi` → `83886080` bytes; request `500m` → 25%; absent/zero denominator → nil; usage above request → >100 without clamp. Include tiny CPU, invalid/negative quantities and large exact sums.
- [ ] Add variants in the same group for per-metric missing values, multi-container sum, missing running container, completed init, running sidecar/ephemeral, duplicate/unknown container name, Pod-level budgets and Node missing allocatable. Assert partial totals remain labelled and no Pod percentage is manufactured.
- [ ] Run `go test ./internal/kube -run TestMetricsProjection -count=1`; establish failing behavior before implementation.
- [ ] Implement decimal quantity arithmetic with existing apimachinery/decimal support, nullable DTO values and per-container classification. Validate sample timestamp/window and supply pure clock-controlled stale classification; preserve source time/config capture separately. Percent formatting may round for display, but arithmetic must precede rounding.
- [ ] Run the targeted group and `go test ./internal/kube`; expect pass. Commit `feat: phase1e3-task1 define current metrics semantics`.

## Task 2 — Bounded native reads and bridge

**Files:** metrics.go, read.go/service cleanup, bounded_reads.go only if narrow configurable bound needed, metrics tests, desktop metrics.go and generated bindings.

**Consumes:** task 1 projection; existing owned connection configuration/discovery transport. **Produces:** ReadCurrentMetrics bridge contract.

- [ ] Add `TestCurrentMetricsReader` using existing HTTP test patterns: v1 preference/v1beta1 fallback, absent API, Forbidden discovery, resource-specific 404, 503/timeout, UID mismatch and sample-before-creation. Assert exact encoded target paths, read-only methods, no alternate credential/version fallback on 403 and no whole-cluster scans.
- [ ] Add cancellation/capacity cases to the same group: cancelled native requests release workers, 256 KiB response/128-container/100-target/2 MiB result limits reject explicitly; two batches share four workers, no per-target unbounded goroutines. Verify a scoped discovery error suppresses redundant requests while a single missing sample leaves others available.
- [ ] Run `go test ./internal/kube -run TestCurrentMetricsReader -count=1`; confirm new cases fail.
- [ ] Implement discovered-version selection and a 60-second connection-generation cache. Use named Metrics GET then current core GET/UID verification before projection. Cache only discovery, not samples or core bodies. Metrics reads preserve existing TLS/helper guards and body bounds; reject mismatched UID/scope/old interval without affecting inventory.
- [ ] Add batch deadline, shared worker capacity, per-target results, one rediscovery for endpoint removal and safe error mapping. Capture connection lifetime cancellation; clear cache on cleanup. Do not make absence depend on APIService list permission. Manual Refresh invalidates discovery/cooldown without creating parallel duplicate discovery requests.
- [ ] Add desktop endpoint under existing cancellable reads and generate bindings with `wails3 task bindings`. Run `go test ./internal/kube ./internal/desktop` and `go test -race ./internal/kube`; expect pass. Commit `feat: phase1e3-task2 read bounded Kubernetes metrics`.

## Task 3 — Current UI and visible polling

**Files:** frontend metrics files and existing table/detail composition from the file map; current styles only if needed.

**Consumes:** generated ReadCurrentMetrics; existing visible page/selection and connection signals. **Produces:** CPU/memory columns and Current usage panels, one shared polling controller.

- [ ] Add one `metrics.test.tsx` group with fake timers and deferred API responses: page/selection deduplication, no overlapping polls, 15-second completion interval, hidden/inactive cancellation, wake refresh and A→B→late A isolation. Check age changes without a new sample and 60-second error cooldown with immediate manual refresh.
- [ ] Include mounted presentation assertions for real zero, unavailable, stale, partial subtotal, request/limit/allocatable labels and >100%. Forbidden/replacement clears old values; transient refresh failure retains them only as Last sample. Verify normal resource controls stay enabled when metrics fail.
- [ ] Run `npm test --workspace frontend -- src/metrics/metrics.test.tsx`; establish failure before implementation.
- [ ] Implement useMetrics with latest-only state, request epochs/native cancellation and bounded batch splitting. Feed full page targets plus selected detail once; no per-cell requests. Link existing Refresh and visibility/native-window lifecycle signals. Recompute staleness while visible, discard obsolete targets and never queue timer backlogs after sleep.
- [ ] Extend current Pod/Node tables and Overview with shared cells/detail. Show CPU in m, memory binary units, source/time/window and configured denominator capture time. Only selected Pod detail receives container breakdown. Reuse existing overflow/column conventions, focus and accessible status text; no graph or global settings. Keep CPU/memory display formatting centralized.
- [ ] Run targeted tests, `npm run lint`, `npm run typecheck` and `npm run build`; expect pass. Commit `feat: phase1e3-task3 show current Pod and Node metrics`.

## Task 4 — Local validation, documentation and delivery

**Files:** fixture script/README, integration test, `docs/superpowers/validation/phase-1e3.html` and validation index; verified plan/checkpoint and relevant capability docs.

**Consumes:** completed reader/UI. **Produces:** one real provider walkthrough, packaged proof and reviewed merge.

- [ ] Inspect API availability on kind-dev-local-a/b with explicit contexts. Reuse existing providers. Only if absent on A, pin an upstream compatible Metrics Server manifest/image digest in the opt-in fixture script; fail rather than overwrite pre-existing resources. Record owned resource inventory and cleanup, including RBAC/APIService resources. No extra clusters or app installation code.
- [ ] Add `TestCurrentMetrics` opt-in integration using `KUBESIFT_METRICS_INTEGRATION=1`. Verify live Pod/Node reads and same-name isolation, exact sample quantities against raw API for that captured sample, and restricted metrics RBAC with a fixture identity. Where the provider cannot supply an edge case, cite deterministic test coverage rather than claiming it was live. The other cluster needs no provider installation just to demonstrate absent-state behavior.
- [ ] Add an assert-based fixture-script check if new script branching is introduced: unsupported context and collision refuse changes; cleanup deletes only recorded owned resources. Pin versions during implementation after checking the existing server version; never use an unpinned latest URL. Explain fixture-only TLS relaxation if required by kind.
- [ ] Run `KUBESIFT_METRICS_INTEGRATION=1 go test -tags=integration ./tests/integration -run '^TestCurrentMetrics$' -count=1 -v`, `wails3 task bindings`, `npm run build`, `scripts/check.sh`, `go test -race ./internal/kube` and `wails3 task package`. Record real failures/skips, not inferred success.
- [ ] Agent-run packaged walkthrough: Pods/Nodes, selected container breakdown, both themes and window sizes, Refresh, tab/namespace changes, no-API/Forbidden states and non-metrics actions still usable. Check actual native request cancellation on hide/disconnect/wake; do not claim JavaScript timer tests alone prove WebKit lifecycle behavior. Compare kubectl top with allowance for different sample timestamps/rounding.
- [ ] Clean owned workload/identity/provider fixtures; preserve pre-existing providers. Record actual server/provider/API version, commands, screenshots, cleanup and gaps in one validation report. Update tracked checkpoint/plan only from verified work; keep detailed logs ignored. Commit `test: phase1e3-task4 validate current resource metrics`.
- [ ] Run one independent whole-PR review using the repository review skill, fix blockers and do targeted rechecks. Push a ready PR and finish the authorized merge into develop. No release/main merge or Phase 1E.4 implementation.

## Self-review and separate-session handoff

Semantics/coverage/units → task 1; transport/version/identity/bounds → task 2; UI/freshness/poll lifecycle → task 3; provider/live/macOS/RBAC/cleanup/review → task 4. Each Review Focus risk has a named check. No separate progress ledger or duplicate UAT checklist.

Start from updated develop containing this plan, use Superpowers executing-plans and the current engineering practices. Implement on feature/phase-1e3-current-metrics in the existing checkout. Historical metrics stays Phase 1E.4 after this increment.
