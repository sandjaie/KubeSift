# sources

`sources` keeps independently configured kubeconfig files and the latest snapshot of each one. The explorer coordinator and the source settings screen use it.

`registry.go` loads a safe catalog and enforces snapshot identity, so one source's snapshot cannot be applied as another source. `store.go` persists source references and user labels in a file separate from appearance preferences. Malformed metadata is rejected. Saves are atomic and bounded.

Catalog loading is local. It does not resolve credential helpers and it does not contact Kubernetes. Registry tests cover snapshot isolation and non-destructive strict loading.
