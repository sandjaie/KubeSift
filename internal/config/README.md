# config

`config` reads Kubernetes configuration for a named context and returns a private snapshot. The desktop source picker, the connection manager, and helper review all consume those snapshots. Loading follows Kubernetes kubeconfig semantics and keeps supported extensions. Safety-sensitive fields are checked. Extensible Kubernetes resources are not rejected for carrying extra data.

The package does not write kubeconfig and does not execute credential helpers. A preview is local information for the user to inspect. It is not permission to run a helper. Resolved credentials stay in Go. They are not copied into events or frontend payloads.
