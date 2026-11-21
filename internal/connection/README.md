# connection

`connection` owns Kubernetes clients for connections the user has explicitly selected. The desktop adapter and the Kubernetes service adapters call `manager.go` with a connection id and generation. The manager issues a new generation when that client is replaced, and it rejects a request whose generation no longer matches.

Each client is tied to the caller's cancellation. Credential material is attached through KubeSift's own transport, and only after [`trust`](../trust/README.md) has allowed the helper to run. The package loads its configuration from [`config`](../config/README.md). An unknown, replaced, or cancelled generation fails closed: the connection is closed or replaced, and the manager does not retarget that request onto a newer client. A replaced configuration preview is rejected by the desktop service before it calls `Open`.

The manager does not choose a target from the active UI tab or from kubeconfig `current-context`. It does not install client-go's native exec authentication plugin.
