# Internal Go services

These packages are the KubeSift backend. `cmd/kubesift` starts the desktop process. `frontend/src` owns presentation. Code under `internal` owns Kubernetes access, session lifecycle, reviewed mutations, Helm inspection and management, and local settings.

Only [`desktop`](desktop/README.md) imports Wails. The other packages stay framework-independent, so the desktop adapter and the tests share one policy. A Kubernetes call carries an explicit connection id, generation, and resource identity. These packages do not read a mutable global current context, and they do not change kubeconfig `current-context`.

| Package | What it owns |
| --- | --- |
| [model](model/README.md) | Shared identities, events, and safe errors |
| [config](config/README.md) | Read-only kubeconfig loading and local context previews |
| [trust](trust/README.md) | Helper approval records and the moment a helper may run |
| [connection](connection/README.md) | Client generations and guarded request transports |
| [sources](sources/README.md) | Independent local kubeconfig catalogs and labels |
| [explorer](explorer/README.md) | Product connection attempts and source-change coordination |
| [session](session/README.md) | Bounded, cancellable background streams |
| [kube](kube/README.md) | Explicit-target reads, watches, logs, forwards, shells, and metrics |
| [operation](operation/README.md) | Reviewed workload and YAML mutations |
| [helmreader](helmreader/README.md) | Read-only Helm release inspection |
| [helmmanager](helmmanager/README.md) | Native Helm chart sources and reviewed release actions |
| [helmrender](helmrender/README.md) | Bounded WebAssembly Helm template execution |
| [preferences](preferences/README.md) | Non-sensitive appearance and window settings |
| [desktop](desktop/README.md) | Translation of Wails calls into the services above |

The comment at the top of each Go file is the map for that file: Purpose, Used by, Responsibilities, and Important. These READMEs describe the package boundary. They do not replace those headers.
