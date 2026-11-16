/*
Purpose: Derive a workload's historical Pod template and conditional patch.
Used by: Native rollback preparation.
Responsibilities: Decode supported history formats and replace only spec.template.
Important: ControllerRevision data never runs as a patch against a live object.
*/
package operation

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
)

const podTemplatePath = "/spec/template"

func invalidRollbackTemplate() error {
	return model.Fail("invalid_history", "This revision does not contain a supported Pod template")
}

func rollbackObject(raw json.RawMessage) map[string]any {
	var obj map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&obj) != nil {
		return nil
	}
	return obj
}

func rollbackField(obj map[string]any, name string) map[string]any {
	field, _ := obj[name].(map[string]any)
	return field
}

func hasPatchDirective(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "$patch" || key == "$retainKeys" || strings.HasPrefix(key, "$setElementOrder/") ||
				strings.HasPrefix(key, "$deleteFromPrimitiveList/") || hasPatchDirective(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if hasPatchDirective(child) {
				return true
			}
		}
	}
	return false
}

func rollbackTemplate(parent kube.ResourceRef, raw json.RawMessage) (json.RawMessage, error) {
	obj := rollbackObject(raw)
	var template map[string]any
	switch parent.Kind {
	case "Deployment":
		template = rollbackField(rollbackField(obj, "spec"), "template")
		labels := rollbackField(rollbackField(template, "metadata"), "labels")
		if _, generated := labels[apps.DefaultDeploymentUniqueLabelKey]; generated {
			delete(labels, apps.DefaultDeploymentUniqueLabelKey)
			if len(labels) == 0 {
				delete(rollbackField(template, "metadata"), "labels")
			}
		}
	case "StatefulSet", "DaemonSet":
		data := rollbackField(obj, "data")
		spec := rollbackField(data, "spec")
		template = rollbackField(spec, "template")
		if len(data) != 1 || len(spec) != 1 || template["$patch"] != "replace" {
			return nil, invalidRollbackTemplate()
		}
		delete(template, "$patch")
		if hasPatchDirective(template) {
			return nil, invalidRollbackTemplate()
		}
	default:
		return nil, invalidRollbackTemplate()
	}
	normalized, err := json.Marshal(template)
	var typed core.PodTemplateSpec
	if err != nil || json.Unmarshal(normalized, &typed) != nil || len(typed.Spec.Containers) == 0 {
		return nil, invalidRollbackTemplate()
	}
	return normalized, nil
}

func rollbackPatch(parent, historical json.RawMessage) ([]byte, error) {
	var live scalePayload
	current := rollbackField(rollbackField(rollbackObject(parent), "spec"), "template")
	if err := json.Unmarshal(parent, &live); err != nil || live.Metadata.UID == "" || live.Metadata.Version == "" || current == nil {
		return nil, invalidRollbackResponse()
	}
	if reflect.DeepEqual(current, rollbackObject(historical)) {
		return nil, model.Fail("no_changes", "The selected revision already matches the current Pod template")
	}
	patch, err := json.Marshal([]patchOperation{
		{Op: "test", Path: "/metadata/uid", Value: live.Metadata.UID},
		{Op: "test", Path: "/metadata/resourceVersion", Value: live.Metadata.Version},
		{Op: "replace", Path: podTemplatePath, Value: historical},
	})
	if err != nil || len(patch) > editLimit {
		return nil, model.Fail("capacity", "Rollback patch exceeds the 2 MiB limit")
	}
	return patch, nil
}

func invalidRollbackResponse() error {
	return model.Fail("invalid_response", "Kubernetes returned malformed rollback evidence")
}
