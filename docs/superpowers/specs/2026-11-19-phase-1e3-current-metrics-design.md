# Phase 1E.3 — Current CPU and memory metrics

Date: 19 November 2026. Baseline `e6dae08` on develop, after Phase 1E.2 / PR #26. Specification and plan requested together; implementation is a separate session.

## Intent and approach

Show current Pod and Node CPU/memory usage where engineers already inspect those resources. Use the existing cluster's Metrics API through the owned Kubernetes connection, with clear units, timestamps and unavailable states. Preserve the current UI and Kubernetes RBAC. Do not install monitoring infrastructure or require Prometheus.

Choose bounded reads for visible resources through existing client-go transport. Direct kubelet scraping needs different access and transport; Prometheus introduces source/query configuration and belongs in 1E.4. Neither is a fallback in this phase. Use the existing dynamic client or a narrow REST wire DTO with apimachinery quantities; no new metrics framework or module is required for these two read shapes.

Owners: [architecture](../../architecture/overview.html), [UX](../../ux/workflows.html), [security](../../security/security-model.html), [engineering](../../engineering/engineering.html), [local testing](../../engineering/local-development.html), [roadmap](../../roadmap/roadmap.html#phase-1e-sequence). Follow the repository Kubernetes development skill.

## Scope and UI

- Add CPU and Memory columns to existing Pod and Node inventory tables. Pod columns display absolute usage; Node cells also identify percent of allocatable in their secondary text or accessible detail. Keep existing columns, page controls, selection and layout conventions.
- Add a Current usage section to Pod and Node Overview. Pod detail includes per-container CPU, memory, request and limit comparisons. Node detail shows absolute usage and percent of allocatable. Show source API version, sample timestamp, collection window and last fetch time.
- Reuse current table/detail typography, tokens, responsive scrolling and status/error presentation. Text accompanies color; no new chart library, dashboard, sidebar entry, settings page or percentage-mode selector.
- Automatically sample while the relevant view is visible. Existing Refresh also refreshes metrics. Metrics loading/errors never block inventory, YAML, events, logs or operational actions, and never change the cluster connection indicator by themselves.

```text
Pods                 CPU         Memory
checkout-api         125m        80 MiB

Overview · Current usage
Metrics API · sampled 10 seconds ago · window 30 seconds
app      CPU 125m · 25% of request · 12.5% of limit
         Memory 80 MiB · 62.5% of request · 31.3% of limit
```

These are illustrative values, not mock data shipped in the app. Other workload tables continue to show their existing status; do not invent Deployment/HPA/namespace totals from an incomplete Pod page. Workload-level aggregation, historical graphs, alerts, disk/network/custom metrics and Prometheus integration are excluded from 1E.3.

## API access and identity

Discover `metrics.k8s.io` using owned credentials, prefer served `v1`, otherwise served `v1beta1`. Verify the selected API resource advertises the correct scope and `get` verb. Never infer availability from the Kubernetes server version, require permission to list APIService objects, or fall back on Forbidden to another credential/API version. On a removed endpoint, invalidate discovery and rediscover once; distinguish a missing sample from a missing group/version using discovery evidence. Report unauthorized, forbidden, unavailable API, missing sample, timeout and malformed/capacity errors separately. API access is read-only and uses no direct kubelet, custom endpoint or third-party network request.

Read named PodMetrics/NodeMetrics only for the current inventory page and selected Pod/Node detail. This avoids relying on Metrics API list pagination or whole-cluster listing permissions. The tradeoff is more small GETs; bound concurrency and cadence below. Requests carry explicit connection generation and captured resource UID; frontend cannot supply a URL or arbitrary API path. Node namespace must be empty. Use exact path segment encoding and validate kind/name/scope.

After reading each sample, fetch the current core Pod/Node through the same owned connection. Require its UID to match the requested target before attaching the sample or computing denominators. If metrics metadata supplies a UID, require it to match too. Reject a sample interval that starts before the current object's creation time; do not attach previous-object evidence to a replacement. Return unavailable when that core read is denied or identity changed, without hiding the resource itself.

Metrics commonly identify objects by name, not container runtime ID. These guards reduce name-reuse errors but do not prove that a same-Pod container restart has a wholly new sample. Display source time/window and do not claim per-restart identity or instantaneous usage. Unknown container names, duplicate names and inconsistent scope invalidate the Pod sample rather than double-counting it.

## Quantity and percentage meaning

- CPU is average usage over the reported window, displayed in millicores (`m`); 1,000m equals one core. Memory is working-set bytes, displayed using binary KiB/MiB/GiB. Neither is cumulative consumption nor a peak.
- Parse resource.Quantity in Go. Carry canonical nonnegative decimal quantities across the bridge as strings, preserving zero versus missing. Reject invalid, negative or non-finite conversions. Do arithmetic before display rounding; do not use integer MilliValue rounding for tiny CPU values or JavaScript integers for exact byte sums.
- Pod absolute usage is the sum of returned known containers for each metric. Include application, init/sidecar and ephemeral entries only when present in the sample and current Pod specification; never assume completed init containers should have samples. Compare coverage to currently running containers from all corresponding status lists. Missing running-container usage is partial: show the subtotal labelled Partial and missing names, never a complete Pod total or zero. With no usable entries show unavailable. CPU and memory completeness are independent.
- Container percentages use that container's positive configured request or limit for the corresponding resource: `usage / denominator × 100`. Show each denominator label/value; absent or zero means Not set, never infinity or zero percent. Requests are not a utilization ceiling; percentages may exceed 100 and must not be clamped.
- Do not manufacture a Pod-level percentage by summing scheduling requests (init maxima, overhead and Pod-level resources have different semantics). Pod totals are absolute only. For containers lacking individual settings when Pod-level budgets exist, show Not set at container level and explain the Pod-level budget is not allocated per container. Keep current configuration details available.
- Node percentages use current `status.allocatable`, with no fallback to capacity. Missing/nonpositive allocatable means unavailable percentage while absolute usage remains visible. Node usage can include system processes and exceed allocatable; do not present it as the sum of Pod usage or clamp it.
- In-place resource changes can make current configuration differ from the sample interval. Label denominator capture time; the percentage is a comparison with current configured resources, not proof of historical allocation or HPA utilization.

## Freshness and errors

A sample older than 60 seconds is stale. Missing/invalid timestamps, nonpositive windows or timestamps over 30 seconds in the future are invalid; small future skew is displayed with the timestamp and no negative age. Use sample timestamp, not fetch completion, for freshness. Re-evaluate age while visible even if repeated fetches return the same sample.

While refreshing, retain the last value for the same full target identity with its original timestamp. On failure label it Last sample / refresh failed; it is not current even if under 60 seconds old. On Forbidden/Unauthorized, identity change, scope change or disconnect clear retained metrics immediately. Never convert a failed or absent sample to numeric zero. Missing/stale metrics do not imply the workload or node is unhealthy. Errors use safe classification, not raw response bodies.

## Polling, bounds and lifecycle

Implementation choices for this phase:

- Fetch immediately on visible Pod/Node entry, then 15 seconds after the previous batch finishes. No overlapping polling batches per visible workspace; Refresh cancels the obsolete read and starts a new one. Merge/deduplicate page and selected target by full identity; if more than 100 targets, split into sequential batches, prioritizing selection.
- Pause requests and cancel pending work for hidden app/window, inactive cluster tab, irrelevant screen, scope change and disconnect. Re-entry/wake triggers one fresh batch, not a backlog. YAML/logs still visible alongside a Pod list may keep list metrics active; a hidden list/detail must not poll. Cancel native work as well as suppressing stale responses.
- At most 100 explicit targets per native batch, two active batches and four target-read workers app-wide, 30-second batch deadline. A worker performs named metrics/core reads sequentially and respects inherited transport rate limits. Return per-target timeout/busy state for unfinished work; no unbounded queue or goroutine per resource.
- Reuse existing 16 MiB core/discovery response cap; metrics response cap 256 KiB before decode, at most 128 container entries per Pod. Summary batch response cap 2 MiB. Include container breakdown only for the requested selected detail target, not every row. Capacity failures are explicit; never silently truncate a Pod total.
- Cache only Metrics API discovery for 60 seconds per connection generation (including absence); explicit Refresh clears it. Shared discovery failure/Forbidden stops redundant metric requests for that batch/scope; automatic retries use a 60-second cooldown, with manual Refresh allowed immediately. Resource-specific missing/denied samples do not suppress other resources.
- Keep only the latest samples for the active page/selection; no disk cache, history buffer, background all-cluster collection, telemetry or AI submission. Clear native connection discovery state and frontend samples on connection cleanup. Do not add a Metrics Server installer to product settings.

## Acceptance and minimal tests

1. Existing Pod/Node pages remain usable with no Metrics API and with denied metrics access. Show unavailable, not zero and not a disconnected-cluster badge.
2. Existing API supplies real CPU/memory in tables and selected details; sample window/time are visible. A multi-container Pod sums correctly, missing container data is partial, zero remains zero, and request/limit/allocatable percentages have the right labels.
3. Both API versions are covered through deterministic responses; test version selection and error classification. Record the version actually exercised live instead of claiming both live.
4. Same-name objects across kind-dev-local-a/b stay isolated; replaced UID/old timestamp and delayed responses after tab/namespace changes cannot attach to the new target.
5. Stale samples, missing denominators, >100% use, absent API, Forbidden, timeout and cancellation behave truthfully; native workers/timers stop when hidden/disconnected.
6. Packaged macOS walkthrough preserves current themes/layout, keyboard selection and all non-metrics features. Agent owns automated and desktop verification; no duplicate user UAT checklist.

Keep one table-driven native semantics group, one reader/transport group, one focused frontend polling/presentation test group, and one opt-in integration walkthrough. Reuse current source-description, formatting and independent whole-PR review practices.

## Development fixture boundary

Only kind-dev-local-a and kind-dev-local-b are permitted. First inspect existing API availability; reuse any installed provider. If absent, implementation may explicitly install a version/digest-pinned upstream Metrics Server as a development fixture on kind-dev-local-a using a checked-in opt-in fixture script and explicit context. This is test setup outside KubeSift, never invoked by app code or default cluster startup. Do not overwrite an existing provider or change its APIService registration. Record all newly created fixture resources for cleanup; leave pre-existing providers intact. If kind requires fixture-only kubelet TLS relaxation, document it prominently and never change the application's TLS policy.

Use the other cluster for isolation and missing-provider behavior only if it is actually absent; otherwise use deterministic HTTP fixtures for absence rather than uninstalling user infrastructure. Create restricted fixture identity for real Forbidden behavior without trial mutations. Compare samples to raw Metrics API responses and kubectl top, allowing different sample times/rounding. Fake tests do not replace one successful live provider and packaged-app reading. Do not deploy Prometheus or another cluster.

## Sources and implementation discretion

Reviewed 19 November 2026: [resource metrics pipeline](https://kubernetes.io/docs/tasks/debug/debug-cluster/resource-metrics-pipeline/), [v1beta1 schema](https://kubernetes.io/docs/reference/external-api/metrics.v1beta1/), [v1 graduation and matching schema](https://kubernetes.io/blog/2026/08/27/kubernetes-v1-37-metrics-api-ga/), [quantity API](https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource). Live support depends on the provider's served APIs, not just server version.

Builder chooses narrow wire DTO versus existing dynamic client, decimal helper implementation and component extraction based on current code. No broad reader/state rewrite or dependency upgrade is required. Preserve these semantics and limits; record actual version and UI evidence in the phase validation report.
