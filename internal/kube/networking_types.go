/*
Purpose: Define typed networking inventory and relationship evidence for the bridge.
Used by: The networking reader, desktop adapter and workspace.
Responsibilities: Keep projections separate from complete captured resource details.
Important: A label association does not establish ownership or connectivity.
*/
package kube

import (
	"github.com/sandjaie/KubeSift/internal/model"
	"time"
)

type NetworkRow struct {
	Resource   ResourceRef `json:"resource"`
	CreatedAt  *time.Time  `json:"createdAt"`
	ObjectJSON string      `json:"objectJSON"`
}
type NetworkPage struct {
	Items []NetworkRow `json:"items"`
	Page  PageInfo     `json:"page"`
}

type ServiceSliceRow struct {
	Slice       NetworkRow `json:"slice"`
	Association string     `json:"association"`
}
type ServiceSlicePage struct {
	SourceVersion string            `json:"sourceVersion"`
	Items         []ServiceSliceRow `json:"items"`
	Page          PageInfo          `json:"page"`
	Complete      bool              `json:"complete"`
	Problem       *model.Problem    `json:"problem"`
}

type NetworkReferenceRequest struct {
	Source        ResourceRef `json:"source"`
	SourceVersion string      `json:"sourceVersion"`
	Target        ResourceRef `json:"target"`
}
type ResolvedNetworkReference struct {
	Association      string      `json:"association"`
	Resource         ResourceRef `json:"resource"`
	IdentityEvidence string      `json:"identityEvidence"`
}
