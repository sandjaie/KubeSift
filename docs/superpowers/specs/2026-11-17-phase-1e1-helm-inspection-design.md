# Phase 1E.1 — Native Helm release inspection

**Status:** Spec and companion plan requested together; prepared for review, no application implementation authorized in this session.
**Date:** 2026-11-17

Owners: [roadmap](../../roadmap/roadmap.html#phase-1e), [architecture](../../architecture/overview.html), [UX](../../ux/workflows.html), [security](../../security/security-model.html), [engineering](../../engineering/engineering.html#agent-verification). Baseline: develop `b636125`, following the Phase 1D closure.

## Outcome and boundaries

Select Helm in the existing sidebar, choose a cluster/namespace, list its releases and inspect one release's status, retained revisions, values, stored manifests, hooks, notes and resource references. Preserve the current list/detail layout, themes, collapsible sidebar and independent cluster tabs. Kubernetes permissions govern reads; no extra approval system.

This is inspection only. Install, upgrade, rollback and uninstall are 1E.2. Metrics remain committed in 1E.3/1E.4 and node administration in 1E.5. No Helm command runner, chart download/render, plugins, hooks execution, tests execution, repository login, dependency installation or infrastructure deployment occurs through the shipped 1E.1 feature. Local test fixtures are separate development tooling.

## Native SDK decision and compatibility

The user selected Helm's Go SDK integrated with existing Go services. It needs no installed Helm CLI on the user's laptop. The alternative, invoking a CLI, adds executable/version/environment management and is not selected. Avoid a home-grown release engine; use SDK release/storage types and value coalescing, with a narrow read adapter for existing transport safeguards.

Start with pinned `helm.sh/helm/v4 v4.3.1`, verified against its [tagged go.mod](https://github.com/helm/helm/blob/v4.3.1/go.mod): Go 1.26.0 and Kubernetes v0.37.1, compared with this repo's Go 1.26.0/v0.37.0. Implementation may align the Kubernetes module family to v0.37.1 if required by dependency resolution; review the module diff and run existing checks. Do not silently downgrade libraries or upgrade Wails/React. Record the resolved versions, transitive/binary impact and actual test outcomes before claiming compatibility. Reference: [SDK examples](https://docs.helm.sh/docs/sdk/examples/).

Support Helm 3/4 Kubernetes-backed release records, verified with explicit fixtures for both. Do not claim compatibility with every historic chart schema merely from successful SDK compilation. Helm 2/Tiller and SQL-backed release storage are outside this increment and must be stated as unsupported; no SQL credentials/settings are added.

## Connection and storage

Use only the chosen connection generation and namespace through the existing credential/helper manager. Do not use Helm's default kubeconfig loader, global current context, inherited HELM_DRIVER, CLI plugins or independently executed credential helpers. A fresh owned REST configuration/client is injected into the reader; app disconnect and request cancellation propagate to network work.

Default storage is Secrets; offer an explicit Secret/ConfigMap storage selector in the Helm view, remembered per cluster tab in memory. Never fall back to ConfigMaps after a Secret Forbidden response or merge duplicate names across drivers. Release identity includes storage driver as well as connection/namespace/name. Ordinary Secret browsing remains metadata-only; entering Helm explicitly reads Helm-owned release records under the user's normal Kubernetes permissions. Values/manifests may contain sensitive content. See [Helm storage documentation](https://docs.helm.sh/docs/v3/topics/advanced/).

Only query Helm-labelled records in the selected namespace. List/get permissions remain actual server decisions; no trial writes, broad Secret dump or cross-namespace scan. Show denied/unavailable/storage-specific empty states accurately. If list is forbidden but get is allowed, explain that browsing requires list; do not guess release record names.

## Release list and bounded reading

Show release name, latest retained revision, Helm status, chart name/version, app version when recorded and last deployment time. Helm deployed status is not live workload health. Include all recorded status families, including pending, failed, superseded and uninstalled when history was retained; don't silently hide failures. Group by release name within namespace/driver, choosing the highest valid numeric retained revision, not creation time or status label. Compare revisions exactly, not through JS float conversion.

Use a bounded metadata snapshot: Kubernetes pages of 100, at most 1,000 Helm record metadata entries per namespace/filter, excluding annotations and bodies. Finish scanning before claiming each release's latest revision; if the cap is exceeded, show an explicit capacity message and an exact release-name filter rather than partial “latest” results. The exact-name filter is a server label selector, not substring search. Display summaries 100 releases/page from this bounded snapshot, sorting/filtering labelled as within that snapshot. Retain only metadata snapshot and current page, no full release cache.

Validate decoded record name/namespace/revision against its metadata identity; duplicate/malformed records produce visible unavailable/corrupt status and never select an arbitrary record or silently fall back to an older revision. List cards need only bounded summaries. Per-record failure does not erase unrelated valid summaries. Refresh rebuilds the snapshot; no background scan on hidden tabs. History is the same bounded record inventory filtered to one release, with exact revision source selection.

All requests have a 30-second deadline, cancellation and a maximum 16 MiB API response body. Each release payload must pass bounded base64/gzip preflight before SDK decoding: at most 8 MiB decompressed JSON per record and 32 MiB aggregate decoded payload processed per summary page/request. Fail oversized individual rows explicitly, not by silently truncating valid data. Selected section output and total bridge response are limited to 2 MiB. Keep payloads only for the active request/section; cap concurrent Helm reads to four app-wide and cancel on scope change. These are technical bounds, not permission rules.

Helm's [tagged Secret driver](https://github.com/helm/helm/blob/v4.3.1/pkg/storage/driver/secrets.go) uses background contexts internally, and its [decoder](https://github.com/helm/helm/blob/v4.3.1/pkg/storage/driver/util.go) expands gzip without the app's limits. Do not assume SDK action List pagination bounds the API read. Wrap SDK storage Get with request-bound, bounded typed Secret/ConfigMap adapters and perform metadata pagination separately. Keep adapter write methods unavailable in this read-only service. Never log SDK release objects or raw decode/server errors containing content.

## Detail sections

Use Overview, History, Values, Manifest, Hooks, Notes and Resources in the existing detail area; fetch the selected section only. The selected revision is visible throughout. Opening another section must not silently jump to the latest revision. Captured metadata includes record UID/resourceVersion, revision, storage and collection time. If the record changes/disappears, ask to Refresh/reselect; do not silently bind to a recreated release of the same name.

- **Overview:** recorded status/description, chart/app versions, revision and deployment timestamps. Description is inert text and may be sensitive. Missing values remain unknown. No inferred chart repository URL or live health verdict.
- **History:** retained revisions and their statuses/times/chart versions. Selecting a row inspects that explicit stored revision; no rollback action in 1E.1. Pruned/deleted history is not reconstructed.
- **Values:** user-supplied values by default; a clearly labelled Computed values toggle uses the selected stored chart defaults plus overrides through SDK coalescing. It is not live configuration and never re-renders templates or calls lookup. If the SDK cannot coalesce a record, preserve user-supplied values and explain computed unavailability. Search/select and explicit Copy displayed values use current text controls.
- **Manifest:** exact stored rendered manifest text for this revision, not current live YAML or original chart templates. Preserve multi-document text; no edit/apply button masquerading as a Helm upgrade. Search/select/copy using existing controls.
- **Hooks:** retained hook definitions, events/weights/delete policies and last recorded execution metadata where present; inspect stored hook manifests. Never run hooks or assume hook-created resources still exist.
- **Notes:** stored text rendered inertly. Do not execute commands, interpolate templates or activate links automatically.
- **Resources:** parse stored manifest document identities, resolve installed API scope through existing discovery and offer explicit navigation to the current live object. Namespace omitted uses the release namespace only for a discovered namespaced kind; cluster-scoped kinds use no namespace. Unknown API, missing name/generated-name only, malformed document or unresolved scope remains a non-navigable row with explanation. Do not fetch all live objects automatically. Cap identities to 500 per captured manifest and disclose truncation. Hooks are not silently merged into ordinary resource inventory.

Manifest identities do not contain historical live UIDs. Label links “Open current resource”; resolving a current name is not proof it is the same object or still managed by this release. Use existing exact-version navigation and Back behavior. Default live Secret navigation remains metadata-only. List/History source metadata and selected release content are not a transaction with live resources; label timestamps accordingly.

## Security, lifecycle and UI

Values/manifest selection is an intentional content read with existing credentials; no repeated reveal or approval gates. Do not promise complete redaction. Keep content local, memory-only and out of operation history, diagnostics, telemetry and AI. No download/export subsystem. Explicit copying is allowed; no automatic clipboard or browser actions.

Capture connection generation, namespace, driver, release name, revision and storage record UID/version in every detail request. Reject late mismatched responses in both native validation and frontend state. Tab/namespace/driver changes cancel reads and clear payloads; Settings/theme navigation must not cause uncontrolled duplicate scans. App exit clears memory. Do not persist release snapshots in preferences or add Helm reads to the existing resource health poll.

## Minimal validation and completion

One native table group covers driver/namespace isolation, latest retained grouping, corrupt/duplicate records, size/decompression bounds, cancelled calls, revision drift and forbidden reads. A small detail group verifies values versus computed values, exact stored manifests, inert notes and live-resource scope resolution. One frontend state scenario covers A/B/driver changes and late detail replies. Avoid per-tab/component suites or a chart catalog test matrix.

Use kind-dev-local-a/b only. Prepare a tiny local synthetic chart and retained revisions using development-only tooling, recording the exact Helm 3/4 fixture versions. Fixture tools may use SDK or an available Helm CLI; the shipped app must work without that executable. Test Secret and ConfigMap storage, actual release history/content and an RBAC denial. No remote charts/operators/monitoring stacks needed. Clean only owned fixtures.

Run repository checks, packaging and agent desktop verification of cluster → Helm → release → history → selected values/manifest → current resource → Back, both themes, namespace/driver isolation and error states. One independent whole-PR review and targeted fixes. Publish `docs/superpowers/validation/phase-1e1.html` with actual compatibility/limits; no duplicate user UAT checklist, release or 1E.2 implementation.

Acceptance: correct bounded release/revision evidence, native SDK with existing credentials, all inspection sections usable, truthful current-resource links, no writes/hooks/chart network operations, current UI preserved and representative local/native verification recorded.

## Planning handoff

Superpowers builder review found no important unanswered planning questions. The [five-task implementation plan](../plans/2026-11-17-phase-1e1-helm-inspection.md) was prepared alongside this spec at the user’s request. Review both before the separate implementation session. No application code, dependency installation or cluster mutation occurred during planning.
