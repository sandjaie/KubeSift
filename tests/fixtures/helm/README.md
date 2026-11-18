# Helm inspection fixture

Use one disposable namespace per approved context and pass explicit context and namespace flags to Helm. The chart uses only ConfigMaps, with one inert hook definition and synthetic values and notes. Create two revisions of release `phase1e1` for each storage driver. Set `identity` to its driver and `marker` to `one` for the first install and `two` for the upgrade. Use `--no-hooks` for both revisions. The historical and live values intentionally differ after upgrade.

The integration test reads fixture namespaces from `KUBESIFT_HELM_NAMESPACE_A` and `KUBESIFT_HELM_NAMESPACE_B`. The isolated A/B kubeconfig directory also supplies `a-helm-denied.kubeconfig` and `b-helm-denied.kubeconfig` for synthetic principals that can list Pods and cannot list Helm storage.

## Management verification

`management/` adds a benign hook, a retained ConfigMap and the isolated
`fixtures.management.kubesift.test` CRD. `customResource: true` creates its namespaced
custom resource after CRD installation. `hookMode: fail` uses an invalid executable;
`hookMode: wait` uses a nonterminating pause container. `hookEvents` chooses the
fixture event, including `pre-delete` for uninstall. These are synthetic test inputs.

The opt-in `TestHelmManagement` creates fresh owned namespaces on **only**
`kind-dev-local-a` and `kind-dev-local-b`, uses secret and configmap storage,
serves private HTTPS/OCI through a temporary loopback TLS server, and removes its
namespaces and labelled CRD. It refuses a pre-existing fixture CRD. Supply an
isolated `KUBESIFT_EXPLORER_CONFIG_DIR` containing `a.kubeconfig` and `b.kubeconfig`,
plus `KUBESIFT_HELM3` pointing to a verified Helm 3 binary used only to seed records.
Set `KUBESIFT_HELM_INTEGRATION=1` and run the plan's exact test command.

For packaged desktop checks, run `python3 tests/fixtures/helm/serve.py --output
<ignored-phase-workspace>/desktop-source`. It emits versions 1.0.0 and 1.1.0, a
short-lived fixture CA and loopback source URLs. Select the emitted CA through
Settings and use the printed synthetic credentials. Stop the server afterwards;
remove only owned namespaces and the labelled fixture CRD after verification.
No Helm CLI or registry process is required by the application.
