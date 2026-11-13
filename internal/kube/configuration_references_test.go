/*
Purpose: Protect the Pod configuration field allowlist.
Used by: Configuration reference tests.
Responsibilities: Exercise every supported location and exclude plugin references.
Important: Only synthetic resource names are used.
*/
package kube

import (
	"encoding/json"
	"testing"

	core "k8s.io/api/core/v1"
)

func TestPodConfigurationFields(t *testing.T) {
	var pod core.Pod
	err := json.Unmarshal([]byte(`{"spec":{
 "containers":[{"env":[{"valueFrom":{"secretKeyRef":{"name":"env-secret","key":"key"}}}],"envFrom":[{"configMapRef":{"name":"env-map"}}]}],
 "initContainers":[{"envFrom":[{"secretRef":{"name":"init"}}]}],
 "ephemeralContainers":[{"env":[{"valueFrom":{"configMapKeyRef":{"name":"ephemeral","key":"key"}}}]}],
 "volumes":[{"secret":{"secretName":"volume-secret"}},{"configMap":{"name":"volume-map"}},
 {"projected":{"sources":[{"secret":{"name":"projected-secret"}},{"configMap":{"name":"projected-map"}}]}},
 {"csi":{"nodePublishSecretRef":{"name":"denied"}}}],
 "imagePullSecrets":[{"name":"pull"}]}}`), &pod)
	if err != nil {
		t.Fatal(err)
	}
	for kind, names := range map[string][]string{"Secret": {"env-secret", "init", "volume-secret", "projected-secret", "pull"}, "ConfigMap": {"env-map", "ephemeral", "volume-map", "projected-map"}} {
		for _, name := range names {
			if !hasConfigurationReference(&pod, kind, name) {
				t.Errorf("missing %s %s", kind, name)
			}
		}
	}
	if hasConfigurationReference(&pod, "Secret", "denied") || hasConfigurationReference(&pod, "Service", "env-secret") {
		t.Fatal("unsupported reference allowed")
	}
}
