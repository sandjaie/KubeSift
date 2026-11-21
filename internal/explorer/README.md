# explorer

`explorer` coordinates product connection attempts. `ExplorerService` in the desktop adapter, and the local integration tests, call `coordinator.go`.

The coordinator serializes changes to a source, guards attempt generations, and schedules bounded resource checks. Each worker holds an explicit connection target. A result that arrives after cancellation, or after a newer attempt has replaced it, cannot restore the earlier connection. Tests hold probes at barriers so that late-result rejection is visible without a cluster or a credential process.

The package does not import Wails and does not route work through a global current context.
