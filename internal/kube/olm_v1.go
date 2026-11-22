/*
Purpose: Present catalog and extension state from the served OLM v1 APIs.
Used by: Dedicated inspection and projected generic JSON and YAML.
Responsibilities: Describe source, package selection, install namespace and observed bundle safely.
Important: No catalog content, service account credentials or controller configuration is returned.
*/
package kube

import (
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	"strings"
)

type olmV1View struct {
	Spec struct {
		Namespace      string `json:"namespace,omitempty"`
		ServiceAccount *struct {
			Name string `json:"name"`
		} `json:"serviceAccount,omitempty"`
		AvailabilityMode string `json:"availabilityMode,omitempty"`
		Priority         *int32 `json:"priority,omitempty"`
		Source           struct {
			SourceType string `json:"sourceType"`
			Image      *struct {
				Ref string `json:"ref"`
			} `json:"image,omitempty"`
			Catalog *struct {
				PackageName string   `json:"packageName"`
				Version     string   `json:"version,omitempty"`
				Channels    []string `json:"channels,omitempty"`
			} `json:"catalog,omitempty"`
		} `json:"source"`
	} `json:"spec"`
	Status struct {
		Conditions []RouteCondition `json:"conditions,omitempty"`
		Install    *struct {
			Bundle struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"bundle"`
		} `json:"install,omitempty"`
	} `json:"status"`
}

func projectOLMv1(raw []byte) (olmV1View, error) {
	var v olmV1View
	if json.Unmarshal(raw, &v) != nil || len(v.Status.Conditions) > 500 {
		return olmV1View{}, model.Fail("invalid", "Invalid OLM v1 evidence")
	}
	if img := v.Spec.Source.Image; img != nil {
		if strings.ContainsAny(img.Ref, "?#") || strings.Contains(img.Ref, "://") || strings.Contains(img.Ref, "@") && !strings.Contains(img.Ref, "@sha256:") {
			img.Ref = ""
		}
	}
	return v, nil
}
