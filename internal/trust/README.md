# trust

`trust` decides whether a credential helper may run, and then runs it. Connection authentication, stream startup, and the desktop helper-review flow call `guard.go` at the moment of execution. An earlier preview does not authorize a later run.

`store.go` keeps approval records in owner-readable files. The schema is strict and versioned. Fingerprints are HMAC-protected. The record stores the approval. It does not store helper arguments, environment values, tokens, or certificate keys.

Before the first run, the guard resolves the helper executable, its arguments, the kubeconfig source, and configured environment overrides. A change to that configuration requires a new approval. Preview ids are single-use. Revocation and process start share one authorization lock, so a revoke cannot race a start. The helper process is owned by the guard and can be cancelled. Native client-go exec refresh stays disabled. The package does not start a helper merely to preview an untrusted kubeconfig.
