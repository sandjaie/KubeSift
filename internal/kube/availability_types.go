/*
Purpose: Define bounded availability and relationship evidence.
Used by: Kubernetes readers and the desktop bridge.
Responsibilities: Carry exact numbers, identity and source versions to the explorer.
Important: Selector association is not controller ownership.
*/
package kube

import (
	"github.com/sandjaie/KubeSift/internal/model"
	"time"
)

type AvailabilitySummary struct {
	Observation string `json:"observation"`
	Revision    string `json:"revision"`
}
type AvailabilityRow struct {
	Resource   ResourceRef         `json:"resource"`
	CreatedAt  *time.Time          `json:"createdAt"`
	ObjectJSON string              `json:"objectJSON"`
	Summary    AvailabilitySummary `json:"summary"`
}
type AvailabilityPage struct {
	Items []AvailabilityRow `json:"items"`
	Page  PageInfo          `json:"page"`
}
type SelectedPodPage struct {
	SourceVersion string         `json:"sourceVersion"`
	SelectorJSON  string         `json:"selectorJSON"`
	Items         []WorkloadRow  `json:"items"`
	Page          PageInfo       `json:"page"`
	Complete      bool           `json:"complete"`
	Problem       *model.Problem `json:"problem"`
}
type ControllerRevisionPage struct {
	SourceVersion   string            `json:"sourceVersion"`
	CurrentRevision string            `json:"currentRevision"`
	UpdateRevision  string            `json:"updateRevision"`
	Items           []AvailabilityRow `json:"items"`
	Page            PageInfo          `json:"page"`
	Complete        bool              `json:"complete"`
	Problem         *model.Problem    `json:"problem"`
}
type AvailabilityReferenceRequest struct {
	Source        ResourceRef `json:"source"`
	SourceVersion string      `json:"sourceVersion"`
	Target        ResourceRef `json:"target"`
}
type ResolvedAvailabilityReference struct {
	Detail           ResourceDetail `json:"detail"`
	Resource         ResourceRef    `json:"resource"`
	SourceVersion    string         `json:"sourceVersion"`
	TargetVersion    string         `json:"targetVersion"`
	IdentityEvidence string         `json:"identityEvidence"`
	Association      string         `json:"association"`
}
