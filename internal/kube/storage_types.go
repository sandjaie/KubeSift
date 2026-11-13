/*
Purpose: Define storage inventory and reference evidence for the bridge.
Used by: Kubernetes storage readers, the desktop adapter and explorer.
Responsibilities: Separate bounded inventory and resolved identities from captured detail.
Important: Reference evidence does not establish storage availability.
*/
package kube

import "time"

type StorageRow struct {
	Resource   ResourceRef `json:"resource"`
	CreatedAt  *time.Time  `json:"createdAt"`
	ObjectJSON string      `json:"objectJSON"`
}

type StoragePage struct {
	Items []StorageRow `json:"items"`
	Page  PageInfo     `json:"page"`
}

type StorageReferenceRequest struct {
	Source        ResourceRef `json:"source"`
	SourceVersion string      `json:"sourceVersion"`
	Target        ResourceRef `json:"target"`
}

type ResolvedStorageReference struct {
	Detail           ResourceDetail `json:"detail"`
	Resource         ResourceRef    `json:"resource"`
	SourceVersion    string         `json:"sourceVersion"`
	TargetVersion    string         `json:"targetVersion"`
	IdentityEvidence string         `json:"identityEvidence"`
	BindingEvidence  string         `json:"bindingEvidence"`
}
