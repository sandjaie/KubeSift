# Phase 1E.4 historical metrics implementation plan

> For the separate implementation chat, use superpowers:executing-plans and complete the tasks in order. Check boxes only against verified results.

**Goal.** Show on-demand historical CPU and working-set memory charts for a selected Pod container or Node.

**Architecture.** A connection-owned source and native Go reader perform verified core reads and bounded HTTPS range queries. The Wails bridge returns a typed projection to the existing Overview. Current metrics keep their existing lifecycle.

**Stack.** Existing client-go, Go net/http and encoding/json, Wails bindings, React/TypeScript and native chart primitives. Use a real Prometheus engine for query validation. Product dependencies should remain unchanged.

**Specification.** Read the [design](../specs/2026-11-21-phase-1e4-historical-metrics-design.md) including query contracts, limits and acceptance. Planning baseline is `8a7f927`. Start from current develop containing this planning package. Use `feature/phase-1e4-historical-metrics` in the existing checkout.

## Global constraints

Keep credentials and evidence local and ephemeral. Read only the selected resource and approved provider templates. History runs on demand and remains independent of current metrics. Use explicit kind-dev-local-a and kind-dev-local-b for cluster tests. Keep the app free of provider installation. Preserve current layout, access controls, source descriptions, generated bindings layout and the [Git workflow](../../engineering/gitworkflow.md).

## Review focus

| Failure | Proof |
| --- | --- |
| Editing a source sends an old credential to a new origin, or a redirect does so | Task 1 uses two TLS servers with request counters |
| A provider permits access after Kubernetes access was revoked, or returns another cluster's labels | Task 2 verifies before and after query and validates each series |
| A reset, replica, pre-creation sample or missing metric yields a plausible false chart | Tasks 2 and 4 cover matrix responses and real PromQL evaluation |
| Collapse, disconnect or a range change lets a late response restore old data | Tasks 2 and 3 prove native cancellation and UI invalidation separately |
| A compatibility fixture is mistaken for a live OpenShift test | Task 4 records endpoint, server and test type separately |

## Task 1 — Select and bind the provider

- [x] Trace connection lifetimes in `internal/kube/read.go`, current reads in `internal/kube/metrics.go`, the adapter in `internal/desktop/metrics.go` and generated `frontend/bindings/internal/desktop`. Add focused native source code/tests alongside the kube reader. Extend the existing cleanup path, including quit, and document responsibilities in the owning package README.
- [x] Define bridge operations to suggest the fixed Route host, set/replace, describe and clear the source for a ConnectionRef. Return only an opaque source revision/ID, validated origin, scope mode, cluster binding and whether auth is configured. Keep credentials write-only. Require the caller's expected source revision on edits/removal so a stale tab cannot replace a newer binding.
- [x] Write and run failing `TestHistorySource` cases covering absent/denied Route, unadmitted or malformed host, accepted manual origin, failed replacement leaving the valid binding intact, redirect across two TLS servers, mismatched connection generations, concurrent replacement, token disposal and zero probes before confirmation. Use a temporary private CA and test source-scoped PEM trust, hostname mismatch, invalid PEM, empty shared-cluster labels and stale edits after removal. Follow the bounded CA file pattern in `internal/helmmanager/sources.go`. Test project mode and shared-cluster scope independently.
- [x] Implement source ownership, origin validation, native client with redirect refusal, and named Route suggestion via the existing owned Kubernetes transport. Disable environment HTTP proxies in the provider client. Cancel affected reads before replacement completes. Retain neither the credential nor the client after owning connection cleanup. Verify unknown/stale source IDs cannot dispatch traffic.
- [x] Run `go test -race ./internal/kube ./internal/desktop -run TestHistorySource -count=1`. Confirm all intended cases executed, then commit the source lifecycle increment.

## Task 2 — Read and project a bounded history range

- [x] Add `internal/kube/history.go`, DTOs and targeted tests next to the existing metrics files. Expose `(*Service).ReadHistoricalMetrics(ctx, req)` and `(*ExplorerService).ReadHistoricalMetrics(id, req)` using the existing beginWorkloadRead/cancel path. Request fields are ResourceRef (with Connection), source ID, preset and optional Pod container. Return source ID, target, requested/effective interval, step, CPU window, fetch time, and separate CPU/memory states with bounded timestamp/value pairs, coverage and safe problems.
- [x] Start with failing `TestHistoricalMetrics` cases using a fixed clock, a fake core server and a TLS range endpoint. Assert an exact namespace/container or Node root matcher, optional exact cluster matcher, explicit project parameter where required, fixed metric names, CPU total filtering and five-minute windows. Assert no provider I/O on disabled/denied/mismatched targets. For a Pod younger than five minutes expect no eligible points.
- [x] Implement named core reads and UID guards, then build preset-derived queries and send them sequentially. Apply deadlines, native cancellation and shared capacity. Reject opaque target swaps, replaced sources and changed core identity/access before returning either metric. No fallback after an auth failure. Run local tests for a mid-read 403, replacement, slow body, cancellation and capacity release.
- [x] Decode only bounded matrix results. Exercise real zero, holes, non-finite values, warnings, wrong labels, duplicate series, oversize body/result, off-grid/unsorted timestamps and missing CPU with valid memory. Include a provider that ignores the series limit. Project safe output without reflecting untrusted strings. Use the exact numeric limits in the spec.
- [x] Run `go test -race ./internal/kube ./internal/desktop`, regenerate with `wails3 task bindings` and inspect the diff for secret-bearing return fields, generated layout drift or unrelated changes. Commit the bounded reader and bridge.

## Task 3 — Show history in the existing detail view

- [x] Extend `frontend/src/workspace/ResourceDetail.tsx` where it mounts CurrentUsage. Reuse active/visible connection signals from `frontend/src/workspace/Workspace.tsx`, cancellation conventions in `frontend/src/metrics/api.ts` and existing disclosure/field styles. Add a focused `frontend/src/metrics/HistoricalUsage.tsx` and `history.test.tsx`. Keep list polling and table columns unchanged.
- [x] Start with a mounted test using deferred responses. Opening the disclosure exposes source selection if unconfigured. After confirmation it queries the default range/container once. Exercise range change, manual refresh, origin replacement, forbidden, hidden window, collapse, disconnect and a delayed A response after switching to B or to another container in the same Pod. Assert native cancellation and suppressed obsolete data. Closing clears the chart; reopening performs a fresh read. Source persists only for the active connection generation.
- [x] Implement the source form, OpenShift suggestion, labelled source/range/container controls, status messages, two charts with visible gaps, and accessible values. The read identity includes source revision, full resource identity, container and range. Test empty, partial, stale coverage and zero with independent metric states. Reuse a single formatter for chart and accessible values and preserve keyboard focus, both themes and narrow detail layout.
- [x] Run `npm test --workspace frontend -- src/metrics/history.test.tsx src/metrics/metrics.test.tsx`, `npm run lint`, `npm run typecheck` and `npm run build`. Commit the UI increment.

## Task 4 — Verify the workflow and deliver

- [x] Add an opt-in `TestHistoricalMetricsIntegration` in `tests/integration/history_test.go`. Use named synthetic Pods and actual Node targets on the approved kind contexts. For the provider, use a disposable pinned local Prometheus process/container with synthetic labelled series, TLS and private ephemeral credentials. A small test-only TLS gateway may enforce tenancy/denial and supply warning responses. Keep it separate from product code. Bind local fixture listeners to loopback. Record fixture ownership and clean up only owned resources.
- [x] Seed distinct values for same-name resources and a counter reset; use an isolated test database or generated input blocks with deterministic timestamps. Set up the resource early enough for the five-minute lifetime guard to admit samples. Query the real engine and compare returned points to an independently calculated rate/gauge expectation. Demonstrate both targets, cluster binding, duplicate series, missing data and separate template/tenancy endpoint tests. Record synthetic series versus actual scraped telemetry and generic Prometheus versus Red Hat contract coverage.
- [x] Run `KUBESIFT_HISTORY_INTEGRATION=1 go test -tags=integration ./tests/integration -run '^TestHistoricalMetricsIntegration$' -count=1 -v`, `wails3 task bindings`, `scripts/check.sh`, `go test -race ./internal/kube ./internal/desktop`, `npm run build` and `wails3 task package`. Keep the fixtures available for the packaged walkthrough.
- [ ] Before dismantling the desktop fixture, exercise the packaged app through unconfigured → select source → view both charts → change range/container → refresh → disconnect. Check a provider denial, same-name cross-connection switch, no effect on Current usage/actions, keyboard operation, themes and narrow layout. Observe cancellation where possible and identify deterministic-only coverage. Use the fixture CA through the product source form. Capture only sanitized evidence. Then remove task-owned fixtures and verify cleanup on both named contexts, including local listeners, provider data and temporary credentials.
- [ ] Publish actual results, versions, limitations and cleanup in `docs/superpowers/validation/phase-1e4.html` and link its index. Update owning capability/UX docs, checkboxes and portable checkpoint only from verified results. Keep operational logs ignored. Complete one fresh independent whole-PR review with the repository review skill, fix findings, obtain targeted rechecks and complete the ready PR and authorized squash merge into develop. Verify merge and record it. Release and Phase 1E.5 require separate authorization.

Validation detail and remaining desktop checks are in the [phase report](../validation/phase-1e4.html). Reader tests use a fixed clock, synthetic creation timestamps and fixed-grid matrix inputs. UI tests cover the configured and unconfigured panels, cancellation and late replies.
