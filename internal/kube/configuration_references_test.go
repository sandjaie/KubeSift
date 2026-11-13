/*
Purpose: Protect the Pod configuration field allowlist.
Used by: Configuration reference tests.
Responsibilities: Exercise every supported location and exclude plugin references.
Important: Only synthetic resource names are used.
*/
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
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

func TestConfigurationResolverValidatesBeforeTargetIO(t *testing.T) {
	var targetCalls atomic.Int32
	s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/test/pods/pod":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"pod","namespace":"test","uid":"pod-uid","resourceVersion":"7"},"spec":{"imagePullSecrets":[{"name":"example"}]}}`)
		case "/api/v1/namespaces/test/secrets/example":
			targetCalls.Add(1)
			if !strings.Contains(r.Header.Get("Accept"), "as=PartialObjectMetadata;") {
				t.Error("target must use partial metadata")
			}
			fmt.Fprint(w, `{"apiVersion":"meta.k8s.io/v1","kind":"PartialObjectMetadata","metadata":{"name":"example","namespace":"test","uid":"current","resourceVersion":"2"}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	src := ResourceRef{Connection: c, APIVersion: "v1", Kind: "Pod", Namespace: "test", Name: "pod", UID: "pod-uid"}
	dst := ResourceRef{Connection: c, APIVersion: "v1", Kind: "Secret", Namespace: "test", Name: "example"}
	good := ConfigurationReferenceRequest{Source: src, SourceVersion: "7", Target: dst}
	for _, mutate := range []func(*ConfigurationReferenceRequest){
		func(r *ConfigurationReferenceRequest) { r.SourceVersion = "6" },
		func(r *ConfigurationReferenceRequest) { r.Source.UID = "old" },
		func(r *ConfigurationReferenceRequest) { r.Target.Name = "absent" },
		func(r *ConfigurationReferenceRequest) { r.Target.Namespace = "other" },
		func(r *ConfigurationReferenceRequest) { r.Target.Connection.Generation++ },
		func(r *ConfigurationReferenceRequest) { r.Target.APIVersion = "v2" },
	} {
		req := good
		mutate(&req)
		if _, err := s.ResolveConfigurationReference(context.Background(), req); err == nil {
			t.Fatal("invalid reference accepted")
		}
	}
	if targetCalls.Load() != 0 {
		t.Fatal("invalid reference reached target")
	}
	reused := good
	reused.Target.UID = "old"
	if _, err := s.ResolveConfigurationReference(context.Background(), reused); err == nil || !strings.HasPrefix(err.Error(), "resource_replaced:") {
		t.Fatal(err)
	}
	result, err := s.ResolveConfigurationReference(context.Background(), good)
	if err != nil || result.Resource.UID != "current" || result.IdentityEvidence != "name-resolved" || targetCalls.Load() != 2 {
		t.Fatal(result, err)
	}
}
