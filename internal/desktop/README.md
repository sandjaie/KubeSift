# desktop

`desktop` is the thin Wails adapter. `app.go` wires the native application to domain services. `service.go` exposes the narrow API that generated bindings call. Policy for connections, trust, sessions, reads, and mutations stays in the framework-independent packages. This package translates typed requests and events.

`explorer.go` exposes independent sources, cancellable connections, and helper settings through one shared native core. Credentials remain in Go. Resetting corrupt source metadata requires an explicit user action. `workloads.go` adds a 16-entry cancellable read registry and thin methods for workloads, networking, and logs. A request id owns cancellation for that call. Domain bounds and UID policy remain in [`kube`](../kube/README.md).

Reviewed mutations are one bridge method per action: scale, restart, delete, rollback, edit, create, and apply. Each method forwards the explicit target and returns the domain result. Retained apply and create results carry the route, the identity, and the outcome. They do not carry draft text. Helm inspection and Helm management are separate methods. The bridge does not own Helm storage and does not establish release identity. Management credentials stay in the native session. An in-flight Helm action outlives navigation and ends when native connection ownership ends. `quit.go` asks before exit while a native Helm operation is still running. Approval lets the existing shutdown path cancel and join those workers.

`preferences.go` exposes appearance and window geometry. Background geometry saves do not overwrite an invalid preferences file. A reset is an explicit UI action.

`metrics.go` exposes cancellable current metrics for the visible workspace. The endpoint reads metrics. It does not install a metrics provider.

## Local validation only

`fixture.go` and `validation.go` exist for the explicit Phase 1A measurement profile. They are disabled unless the isolated fixture environment is set. Before a marker target is issued, `fixture.go` checks the isolated source, loopback endpoints, and the ownership journal. A context label or a namespace typed by the user does not authorize that write.

`validation.go` checks both owned contexts, marks readiness only after a real frontend Ping, and records numeric renderer evidence under `.local/phase1a`, with a 4 MiB file cap. It does not accept a frontend path, an executable, a payload blob, or an arbitrary local command.
