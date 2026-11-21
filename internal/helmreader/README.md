# helmreader

`helmreader` inspects Helm releases. The desktop explorer calls it for the release list and the detail pane. Each request uses the selected connection generation and shares the app-wide read cap. Inspection reads Kubernetes through that connection. It does not run a Helm executable and it does not use the Helm action engine.

Inventory scans Helm-labeled metadata in one namespace and one storage driver. Incomplete metadata does not establish a latest revision. `decode.go` preflights storage data for size and shape. The pinned SDK decodes the captured record only after those checks, and only from a verified copy. The SDK v1 release decoder uses Go JSON numeric semantics. The storage adapter given to the SDK has no write implementation.

Detail projects one selected section from a captured revision. Stored chart defaults and overrides are what computed values use. A record whose dependency charts are unavailable, or whose value table is incompatible, still keeps its stored override view. Section text and chart values are not persisted and are not written to logs. Stored values and manifests belong only to the section that was requested.

`resources.go` extracts resource links from a stored manifest for the Resources section and for an explicit "open this resource" action. A current name match does not establish historical identity or Helm ownership.

`management.go` shares bounded stored evidence with native release management so a review can recheck identity. Those helpers stay on the read-only adapter. Writes belong to [`helmmanager`](../helmmanager/README.md).
