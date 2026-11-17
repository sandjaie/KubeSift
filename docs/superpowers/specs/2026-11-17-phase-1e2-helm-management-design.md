# Phase 1E.2 — Helm release management

Date: 17 November 2026. Baseline: `8ec0327` on develop, after Phase 1E.1 / PR #24. The maintainer requested the specification and plans together and delegated routine decisions. This authorizes planning; implementation remains a separate session.

## Intent and scope

Install, upgrade, roll back and uninstall Helm releases from the existing workspace using the native Helm SDK. Preserve the current UI, explicit cluster targets, Kubernetes authorization and one review/confirmation. Support local packaged charts, HTTPS chart repositories and OCI registries, including private sources. Do not add infrastructure for KubeSift itself.

Owners: [architecture](../../architecture/overview.html#native-helm), [security](../../security/security-model.html), [UX](../../ux/workflows.html), [engineering](../../engineering/engineering.html), [compatibility](../../engineering/toolchain.html#compatibility), [phase order](../../roadmap/roadmap.html#phase-1e-sequence). The [1E.1 design](2026-11-17-phase-1e1-helm-inspection-design.md) continues to own inspection limits and evidence semantics.

## Approach

Extend the pinned Helm v4.3.1 SDK integration. Shelling out to the user's Helm would add executable/version/configuration dependencies; independently applying chart YAML would lose Helm lifecycle semantics. Neither fits the selected native approach. Reuse reader identities, existing diff/dialog presentation and operation result conventions. Add a focused native Helm management service; keep inspection read-only.

A release is identified by connection generation, namespace, driver and name, with current storage record UID/version/revision captured for review. Do not pretend a release is a Kubernetes object with one UID. Resource-operation locks cannot represent all resources a chart or its hooks might touch; use a release-specific lock, disclose the concurrency limit and preserve Helm's pending-state conflict checks.

## User experience

```text
Helm releases                        [Install]
Selected release                     [Upgrade] [Roll back] [Uninstall]
Prepare → Review → Confirm action → Running / Result
          cluster · namespace · release · storage · chart version
          values / manifest diff · hooks · CRDs · affected namespaces
```

Keep the existing list/detail split, tabs, themes, keyboard/accessibility conventions and Ask AI behavior. Add actions to the Helm view and a dialog using existing editor/diff controls. Rollback starts with a retained revision selection. Inspection of an old revision does not make it the current upgrade/uninstall target: resolve the current head and display both identities where relevant.

Settings contains a small Helm sources section: named HTTPS repository URLs or OCI registry/path prefixes, optional custom CA and client certificate/key file references, and Forget credentials. Local `.tgz` selection happens in the install/upgrade dialog through a native file picker. No chart marketplace, additional home screen or registry-wide catalog. HTTPS sources offer bounded chart/version search from their index; OCI accepts the exact chart reference plus version or digest. Show clear authentication, TLS, missing-version and capacity errors.

### Action behavior

- **Install:** select source/chart/exact version, release name and target namespace. Existing namespace by default; optional Create namespace is explicit and included in review. Never silently turn an install conflict into upgrade or reuse an uninstalled name/history. Use the selected Secret/ConfigMap driver, with no fallback.
- **Upgrade:** explicitly select the chart source/version; stored chart metadata does not prove repository origin. Prefill the values editor with the current release's user overrides. Treat the edited mapping as the complete override set on the new chart's defaults (SDK ResetValues semantics); omitted old override keys are removed. Provide Reset to chart defaults. No implicit ReuseValues or silent chart/version selection.
- **Rollback:** select a retained earlier revision, show its chart/values and stored manifest difference from the current revision. Use the Helm rollback action, creating a new revision. Explain this restores Kubernetes release configuration, not database contents or external side effects.
- **Uninstall:** list the release resources and delete hooks, retained CRDs/keep-policy resources and potentially retained hook resources. Keep history is an explicit toggle, off by default. Use normal background deletion; no force, adoption or ownership bypass. Completion does not guarantee every controller-created resource or external dependency has disappeared.

Values are an editable YAML mapping with optional file import. No `--set` parser, arbitrary Helm flags or plugin command UI. Use normal schema validation. Secrets/values are locally viewable without repeated approval; they can be stored by Helm in cluster release records as normal. Do not send them to AI, diagnostic logs or persisted app drafts.

Implementation defaults: hooks enabled with a visible disable toggle; wait for readiness and Jobs enabled; timeout 5 minutes, editable 1–30 minutes; hard overall action deadline equals the selected timeout. Preserve SDK apply behavior (install SDK default, upgrade/rollback `auto`), show the selected behavior in review, and disable force-conflicts/replacement/take-ownership. Automatic rollback/cleanup-on-failure is off: failure may have effects and a subsequent recovery is a separate reviewed action. Preserve history with MaxHistory=0 in this phase rather than silently pruning retained revisions. These are implementation choices, not extra app authorization rules.

## Chart sources and credentials

Resolve sources using SDK-compatible HTTP/OCI clients without loading ambient Helm plugins, registry credential helpers or mutable global HELM settings. Kubernetes authentication remains the existing owned connection; it is never reused for chart downloads.

Support anonymous or username/password/token authentication entered for the selected source; use password/token as the registry password where appropriate. Credentials stay in native session memory after submission, are scoped to that source/origin, and are cleared on Forget, source deletion or app exit. Do not save secrets in preferences, URLs, temporary registry files or logs. Authentication is not remembered across launches in this increment. Users obtain cloud registry tokens in their terminal; no cloud login automation or external helper execution is added. Persist only source labels, addresses and TLS file references. Never return stored credentials to the frontend.

Use HTTPS with system trust or an explicitly selected CA; optional client certificate/key supports private TLS endpoints. Reject embedded URL credentials and insecure TLS switches. Use standard OCI challenge/token exchange with origin-scoped credentials; do not forward registry/repository Authorization to unrelated chart download redirects. Handle a different download origin anonymously or request separate credentials for that origin. Reject HTTPS-to-HTTP downgrade. Local development registries use a trusted test CA, not a shipped insecure bypass.

Local charts must be packaged `.tgz` files; no arbitrary directory traversal, symlink extraction, plugins or post-renderer executable. Dependencies must be included in the chart package; explain missing dependencies and ask for a complete package instead of implicitly fetching more sources. HTTPS downloads verify the index digest when supplied. OCI resolves and verifies content digests. Keep an immutable native chart snapshot/hash for review and execution; never refetch a mutable tag after confirmation. A digest identifies content, not publisher trust; do not label unsigned charts verified/authentic.

## Preparation, review and execution

Preparation performs no cluster writes and executes no hooks. Use Helm's supported dry-run path with read-only cluster access where available. Guard the Kubernetes transport during preparation to reject writes, allowing only read operations and narrowly recognized non-mutating review APIs if needed. Do not use trial mutations to test RBAC. Required read failures stop preparation; Kubernetes remains authoritative at execution. Permission hints cannot guarantee hook permissions or future writes.

Show the resolved source/version/digest, target, override values difference, stored-current versus proposed manifest difference, resource names/scopes, CRDs, hooks and action settings. Label this as a Helm preview, not an exact Kubernetes admission result. Hooks, template `lookup`, time/random functions, admission and concurrent changes can make execution differ. Helm re-renders during execution; pinning inputs does not pin live lookup results. No per-resource dry-run engine or false promise of atomic multi-resource execution.

For rollback/uninstall use SDK dry-run if supported and supplement its limited output with stored release evidence; no fabricated server validation. CRDs and their dependent resources may be unverifiable before CRD installation. Display that limitation alongside their impact rather than blocking all CRD-bearing charts or silently skipping them. Keep Helm's CRD lifecycle semantics; do not auto-upgrade/delete CRDs independently.

Capture a short-lived single-use native review token binding action, connection generation, namespace, driver, name, current head identity or install absence, rollback revision identity, resolved chart bytes, values and options. Any changed input invalidates the token. Before starting, revalidate connection/head and rollback record (or install absence); changed/replaced/pruned/pending release requires refresh/review. Repeated confirmation returns the same operation ID. No frontend request can substitute new inputs at execute time.

This preflight is not a distributed release lock: external Helm clients can race after validation. Rely on SDK release-state checks and report conflicts/partial outcomes; do not retry mutations automatically. Same-process operations serialize by connection generation/namespace/driver/name. Do not hold a fake lock over every resource or claim to prevent direct kubectl edits.

Execution returns an operation ID promptly and continues natively across navigation, namespace changes and Settings. UI polls bounded status using the fixed target. Cancel requests stop future local work/requests where possible; already sent writes and running hooks cannot be undone. Tab disconnect/app exit cancels owned work; warn once if active operations are being stopped. Never release the native lock just because the frontend disconnected or a timeout response was displayed: release it when the worker actually terminates.

Install/upgrade have context-aware SDK entry points; rollback/uninstall currently use Run. Bind all SDK Kubernetes clients/storage and waits to the owned operation context and deadline, including SDK calls using background contexts. Suppress SDK logs and hook output containing payloads. Do not detach a runaway goroutine and call it cancelled. Verify cancellation/cleanup explicitly during implementation.

Results distinguish running, succeeded, failed and unknown outcome, with cancellation/timeout explanation, safe SDK error category and latest observed release state/time. A failed action may have changed resources; transport loss after submission is unknown until inspected. Retain bounded metadata results and offer Refresh release/history; do not automatically retry, purge pending records or clean up partial installations. Payloads and credentials do not belong in retained result history.

## Bounds and lifecycle

Reuse 1E.1 bounded release decoding, but permit the necessary SDK release writes in a separate operation adapter. Never replace the read-only adapter with a permissive global client. Bounds are explicit capacity errors, not partial successful reviews:

- Chart resolution 120 seconds; review 60 seconds; review token TTL 5 minutes.
- Chart archive 20 MiB, total expanded chart 64 MiB, 4,096 entries and nesting depth 32 including embedded dependencies; reject traversal/symlinks and enforce limits before SDK allocation/decode.
- Repository index 10 MiB, 10,000 chart/version entries, page size 100; OCI exact reference avoids an unbounded registry scan.
- Values YAML 1 MiB; rendered manifest plus hooks/CRDs 8 MiB; at most 1,000 resource identities per review. Preserve inspection's 8 MiB expanded release limit and reject larger management input records explicitly.
- Bridge response 2 MiB per page/chunk. Large review content uses bounded section/chunk reads, never silent truncation or one oversized DTO; no confirmation until review generation is complete.
- Four retained preparations/drafts and two executing Helm operations app-wide, one per release key; reserve capacity atomically. Review expiry clears charts/values; execution retains its snapshot until termination. Completed metadata results cap 128; no persistent content cache or background chart refresh.

Use native app-private temporary files only where SDK integration requires them, permissions 0700 directories/0600 files, no credential persistence; clean on success/failure/cancel/expiry and remove abandoned owned temp files on next startup. Native file selection returns an opaque handle, not an arbitrary frontend-supplied filesystem path.

## Acceptance and minimal verification

1. Install a local chart, upgrade from HTTPS, roll back and uninstall; inspect history/results in the existing UI.
2. Resolve/install an OCI chart with private authentication and show precise auth/TLS errors without credential leakage. Exercise private HTTPS auth as well.
3. Both approved kind contexts contain the same release name without cross-target actions. Changed head/expired token/double confirm and delayed UI responses are safe.
4. Review causes zero cluster mutations or hook execution; confirmation runs the reviewed hook setting. Test a CRD/keep-policy impact case without promising full dry-run validation.
5. A denied action or failed hook reports the actual partial outcome; timeout/cancel cleans local workers/temp data and retains truthful status. Test rollback/uninstall cancellation, not just context-aware actions.
6. Agent verifies packaged macOS flows, keyboard/focus, both themes and current inspection regressions. Follow the engineering acceptance policy; no duplicate user UAT checklist.

Use a small native source table, one native operation lifecycle test group, one frontend mounted workflow test, and a shared opt-in local integration walkthrough. Reuse existing chart fixtures. Host test repositories/registry on the development machine with synthetic credentials; install only test releases into kind-dev-local-a/b with explicit context and owned fixture namespaces. No new clusters or production tests. One independent whole-PR review after implementation, no routine per-task reviews.

## Exclusions and later work

No application runtime Helm CLI, arbitrary plugins/post-renderers, chart publishing, automatic dependency downloads, signature trust UI, cross-launch credential storage, SQL/Helm 2 storage, chart marketplace, automatic repair, metrics or node actions in this increment. Local, HTTPS and OCI source support and all four management actions are required here. Other roadmap scope remains unchanged.

## Evidence and builder decisions

Verified the pinned SDK source locally: `pkg/action/install.go` and `upgrade.go` provide RunWithContext; `rollback.go` and `uninstall.go` use Run; rollback has DryRunStrategy and uninstall DryRun. Upstream references: [SDK](https://helm.sh/docs/sdk/), [hooks](https://helm.sh/docs/topics/charts_hooks/), [OCI](https://helm.sh/docs/topics/registries/), [v4.3.1 actions](https://github.com/helm/helm/tree/v4.3.1/pkg/action). Some OCI documentation still notes incomplete Helm 4 coverage; verify tagged implementation rather than copying older examples.

Builder may choose concrete SDK adapter internals, reuse/split existing presentation components and select a focused credential transport library if the pinned SDK cannot supply in-memory credentials directly. Preserve these contracts and document any dependency reason. No SDK fork or broad operation-framework rewrite is prescribed.
