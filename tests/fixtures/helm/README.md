# Helm inspection fixture

Use one disposable namespace per approved context and pass explicit context and namespace flags to Helm. The chart uses only ConfigMaps, with one inert hook definition and synthetic values and notes. Create two revisions of release `phase1e1` for each storage driver. Set `identity` to its driver and `marker` to `one` for the first install and `two` for the upgrade. Use `--no-hooks` for both revisions. The historical and live values intentionally differ after upgrade.

The integration test reads fixture namespaces from `KUBESIFT_HELM_NAMESPACE_A` and `KUBESIFT_HELM_NAMESPACE_B`. The isolated A/B kubeconfig directory also supplies `a-helm-denied.kubeconfig` and `b-helm-denied.kubeconfig` for synthetic principals that can list Pods and cannot list Helm storage.
