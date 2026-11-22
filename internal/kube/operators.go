/*
Purpose: Project OLM Classic and v1 evidence without exposing controller configuration.
Used by: Operator inspections and lifecycle reviews.
Responsibilities: Separate catalog, install and effective target scopes and capture safe plan identities.
Important: Step manifests, deployment templates and credentials never leave the native reader.
*/
package kube

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/model"
)

type OperatorAPI struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}
type InstallMode struct {
	Type      string `json:"type"`
	Supported bool   `json:"supported"`
}
type OperatorChannel struct {
	Name           string `json:"name"`
	CurrentCSV     string `json:"currentCSV"`
	CurrentCSVDesc struct {
		InstallModes []InstallMode `json:"installModes"`
	} `json:"currentCSVDesc"`
}
type OperatorStep struct {
	Namespace      string `json:"namespace,omitempty"`
	ManifestDigest string `json:"manifestDigest,omitempty"`
	Resolving      string `json:"resolving"`
	Status         string `json:"status"`
	Resource       struct {
		Group            string `json:"group"`
		Version          string `json:"version"`
		Kind             string `json:"kind"`
		Name             string `json:"name"`
		CatalogSource    string `json:"sourceName"`
		CatalogNamespace string `json:"sourceNamespace"`
	} `json:"resource"`
}

// Only the manifest identity and fingerprint are retained.
func (s *OperatorStep) UnmarshalJSON(raw []byte) error {
	type plain OperatorStep
	var value plain
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	var wire struct{ Resource struct{ Manifest *string } }
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	if manifest := wire.Resource.Manifest; manifest != nil {
		value.ManifestDigest = ""
		value.Namespace = ""
		var obj struct {
			APIVersion, Kind string
			Metadata         struct{ Name, Namespace string }
		}
		if json.Unmarshal([]byte(*manifest), &obj) == nil && obj.Metadata.Name == value.Resource.Name && obj.Kind == value.Resource.Kind && obj.APIVersion == (apiResource{group: value.Resource.Group, version: value.Resource.Version}).apiVersion() {
			value.Namespace = obj.Metadata.Namespace
			value.ManifestDigest = fmt.Sprintf("%x", sha256.Sum256([]byte(*manifest)))
		}
	}
	*s = OperatorStep(value)
	return nil
}

type OperatorEvidence struct {
	ResolutionIncomplete            bool              `json:"resolutionIncomplete,omitempty"`
	Catalog                         string            `json:"catalog,omitempty"`
	CatalogNamespace                string            `json:"catalogNamespace,omitempty"`
	Package                         string            `json:"package,omitempty"`
	Channel                         string            `json:"channel,omitempty"`
	Channels                        []OperatorChannel `json:"channels,omitempty"`
	CurrentCSV                      string            `json:"currentCSV,omitempty"`
	InstalledCSV                    string            `json:"installedCSV,omitempty"`
	Approval                        string            `json:"approval,omitempty"`
	Approved                        bool              `json:"approved,omitempty"`
	Plan                            *BuildReference   `json:"plan,omitempty"`
	CSVs                            []string          `json:"csvs,omitempty"`
	Steps                           []OperatorStep    `json:"steps,omitempty"`
	Targets                         *[]string         `json:"targets,omitempty"`
	Owned, Required                 []OperatorAPI
	OwnedServices, RequiredServices []OperatorAPI
	Copied                          bool             `json:"copied,omitempty"`
	Phase                           string           `json:"phase,omitempty"`
	Reason                          string           `json:"reason,omitempty"`
	Conditions                      []RouteCondition `json:"conditions,omitempty"`
}

func projectOperator(kind string, raw []byte) (OperatorEvidence, error) {
	var w struct {
		Metadata struct {
			Labels    map[string]string
			Namespace string
		}
		Spec struct {
			Name, Source, SourceNamespace, Channel, InstallPlanApproval, Approval, SourceType string
			Approved                                                                          bool
			ClusterServiceVersionNames                                                        []string
			CustomResourceDefinitions, APIServiceDefinitions                                  struct{ Owned, Required []OperatorAPI }
		}
		Status struct {
			BundleLookups                                                                                               []json.RawMessage
			CatalogSource, CatalogSourceNamespace, PackageName, DefaultChannel, CurrentCSV, InstalledCSV, Phase, Reason string
			Channels                                                                                                    []OperatorChannel
			InstallPlanRef                                                                                              *BuildReference
			Plan                                                                                                        []OperatorStep
			Namespaces                                                                                                  *[]string
			Conditions                                                                                                  []RouteCondition
			ConnectionState                                                                                             struct{ LastObservedState string }
		}
	}
	if json.Unmarshal(raw, &w) != nil {
		return OperatorEvidence{}, model.Fail("invalid", "Invalid Operator evidence")
	}
	s, t := w.Spec, w.Status
	out := OperatorEvidence{Phase: t.Phase, Reason: t.Reason, Conditions: t.Conditions}
	switch kind {
	case "CatalogSource":
		out.Phase = t.ConnectionState.LastObservedState
	case "PackageManifest":
		out.Catalog = t.CatalogSource
		out.CatalogNamespace = t.CatalogSourceNamespace
		out.Package = t.PackageName
		out.Channel = t.DefaultChannel
		out.Channels = t.Channels
	case "OperatorGroup":
		out.Targets = t.Namespaces
	case "Subscription":
		out.Catalog = s.Source
		out.CatalogNamespace = s.SourceNamespace
		out.Package = s.Name
		out.Channel = s.Channel
		out.Approval = s.InstallPlanApproval
		out.Plan = t.InstallPlanRef
		out.InstalledCSV = t.InstalledCSV
		out.CurrentCSV = t.CurrentCSV
	case "InstallPlan":
		out.ResolutionIncomplete = len(t.BundleLookups) > 0
		for _, condition := range t.Conditions {
			if condition.Type == "Resolved" && condition.Status != "True" {
				out.ResolutionIncomplete = true
			}
		}
		out.Approval = s.Approval
		out.Approved = s.Approved
		out.CSVs = s.ClusterServiceVersionNames
		out.Steps = t.Plan
		var doc struct {
			Status struct {
				Plan []struct{ Resource struct{ Manifest string } }
			}
		}
		if json.Unmarshal(raw, &doc) != nil {
			return OperatorEvidence{}, model.Fail("invalid", "Invalid InstallPlan")
		}
		for i, step := range doc.Status.Plan {
			if step.Resource.Manifest == "" {
				out.Steps[i].ManifestDigest = ""
				out.Steps[i].Namespace = ""
			}
		}
	case "ClusterServiceVersion":
		_, copied := w.Metadata.Labels["olm.copiedFrom"]
		out.Copied = copied || t.Reason == "Copied"
		out.Owned = s.CustomResourceDefinitions.Owned
		out.Required = s.CustomResourceDefinitions.Required
		out.OwnedServices = s.APIServiceDefinitions.Owned
		out.RequiredServices = s.APIServiceDefinitions.Required
	}
	if len(out.Steps) > 500 || len(out.CSVs) > 500 || len(out.Channels) > 500 || len(out.Conditions) > 500 {
		return OperatorEvidence{}, model.Fail("capacity", "Operator evidence exceeds the display limit")
	}
	return out, nil
}
