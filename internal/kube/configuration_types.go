/*
Purpose: Define bounded namespace policy and configuration bridge contracts.
Used by: Kubernetes readers, desktop bindings and workspace.
Responsibilities: Keep inventory projections separate from captured details.
Important: Secret projections contain identity only.
*/
package kube

import "time"

type ConfigurationRow struct {
	Resource   ResourceRef `json:"resource"`
	CreatedAt  *time.Time  `json:"createdAt"`
	ObjectJSON string      `json:"objectJSON"`
}

type ConfigurationPage struct {
	Items []ConfigurationRow `json:"items"`
	Page  PageInfo           `json:"page"`
}

type ConfigurationReferenceRequest struct {
	Source        ResourceRef `json:"source"`
	SourceVersion string      `json:"sourceVersion"`
	Target        ResourceRef `json:"target"`
}

type ResolvedConfigurationReference struct {
	Resource         ResourceRef `json:"resource"`
	IdentityEvidence string      `json:"identityEvidence"`
}
