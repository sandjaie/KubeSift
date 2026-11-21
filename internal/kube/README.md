# kube

`kube` is the Kubernetes domain for reads and streams. It touches only the resources a caller names, on the connection generation that caller passes. The desktop adapter and the opt-in integration tests are the callers. The package does not import Wails and does not keep a mutable active context.

Reads are bounded. `status.go` classifies safe failures for resource-read evidence. `bounded_reads.go` pages workload inventories. Details, relationships, and events bind evidence to UIDs before two objects are treated as the same one. `logs.go` keeps product log reservations for an explicit Pod UID and container. The Kubernetes log API is name-addressed, so the UID check narrows a race. It does not remove that race.

Legacy summaries omit full specs. Investigation reads use a separate cap on local payloads. Credentials stay in the native client.

## Inventories and references

`controller_workloads.go` projects controller-specific counts and Job status for inventory and detail.

`networking.go` projects the five typed networking inventories. `networking_relationships.go` validates Service associations and explicit captured references before a target is opened. `service_forward.go` resolves a captured Service TCP port to one selected Pod. It does not dial an EndpointSlice address.

`configuration.go` projects configuration inventories and captures policy quantities. `secret_metadata.go` negotiates partial metadata and projects Secret identity. A full Secret object is read only from `edit_capture.go`, after the user deliberately opens that object for a reviewed edit. `configuration_references.go` validates Pod fields before those explicit reads.

`storage.go` captures bounded typed storage reads and preserves reported quantities and field presence. `storage_references.go` verifies explicit storage links and checks both sides of a binding reference.

`access_control.go` reads ServiceAccount and RBAC inventories. `access_control_references.go` validates an explicit reference against the captured source, then captures that one target.

`availability.go` captures reported availability and exact revisions. Selector association stays in `availability_relationships.go`. Controller ownership stays in `availability_references.go`. A label match and a controller reference answer different questions and stay separate.

`discovery.go` lists advertised API paths on the selected connection. It does not follow server-supplied URLs or subresource paths. Discovery hints do not establish read permission. `generic_resources.go` reads one page, or captures one object, from an installed read-only API. A list does not export object contents. Detail decoding does not send numbers through `float64`. `manifest_target.go` resolves an exact manifest address with live discovery for reviewed YAML create and edit. A plural, a preferred version, or a caller-supplied URL is not a route.

`metrics.go` reads current CPU and memory for the Pods and Nodes the workspace names. Amounts stay decimal strings: CPU in millicores, memory in bytes. Pod totals have no percentage, and incomplete coverage is labelled. The service does not install a metrics provider and does not cache samples.

## Interactive sessions

`interactive.go` owns pending terminal and port-forward slots and verifies the exact Pod identity before a stream starts. `terminal.go` and `forward.go` use the pinned SPDY protocols, with one cancellable HTTP upgrade per session. Session lifetime, backpressure, and cancellation live in [`session`](../session/README.md). This package verifies the Kubernetes target and opens the stream.
