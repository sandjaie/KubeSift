# helmmanager

`helmmanager` owns native Helm chart sources and reviewed release actions. Desktop management requests are the caller. The package keeps ephemeral chart bytes and source credentials in memory. `Close` cancels native work and drops those references. Credentials do not outlive the session, and chart content is not written into preferences.

`sources.go` resolves bounded HTTPS and OCI chart locations without ambient Helm state. A redirect does not receive credentials meant for another origin, and an HTTP downgrade is rejected. `charts.go` bounds a packaged chart before any SDK decoder runs. Nothing is extracted to disk, and dependencies are not fetched implicitly.

`configuration.go` builds Helm SDK clients from one captured native connection. Discovery, storage, and the action client share that connection's guarded transport. `storage.go` bounds SDK storage reads. The owned transport controls cancellation and preview writes. Embedded clients keep the SDK's normal write semantics for a confirmed action.

`review.go` freezes the inputs for one install or upgrade and produces a bounded review. A preview pins those inputs. It does not pin later admission results or output that depends on lookup or time. `impact.go` describes the scoped change and previews newly declared custom resources. Objects the preview defers stay visible in the review. Execution uses the normal SDK client.

`render.go` sends both action renders through the isolated engine in [`helmrender`](../helmrender/README.md). `execution.go` runs one reviewed action under native lifecycle ownership. Desktop confirmation, cancellation, and result polling call it. Execute accepts the native review token. It does not accept a fresh set of inputs in place of that token. Cancelling does not release the worker lock until the SDK worker has actually stopped.

`logging.go` stops Helm's package-global diagnostics from printing chart values. Action-local discard loggers do not cover every SDK schema and dependency helper, which is why the package-global path is closed as well.

[`helmreader`](../helmreader/README.md) remains the read-only inspection path. Management reuses its bounded stored-evidence helpers for identity checks. Those helpers do not gain a write implementation.
