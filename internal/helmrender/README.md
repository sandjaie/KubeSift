# helmrender

`helmrender` runs Helm template rendering inside a bounded native WebAssembly runtime. Helm preparation and execution call it through the action template adapter in [`helmmanager`](../helmmanager/README.md).

Only the trusted bundled worker runs. Chart code receives no filesystem, no network, and no process access. Host-side adapter templates stay in the host. Original chart templates stay in WebAssembly. Allocation and execution are capped by the runtime.

[`wire`](wire/README.md) is the private in-memory protocol between the host and that worker. Payloads from the protocol do not enter files, logs, or application result history.
