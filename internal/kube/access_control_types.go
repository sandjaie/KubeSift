/*
Purpose: Define bounded access control evidence for the desktop bridge.
Used by: Kubernetes readers, reference navigation and explorer views.
Responsibilities: Separate inventory summaries from captured detail.
Important: These records describe configuration, not effective permissions.
*/
package kube

import "time"

type AccessControlRow struct {
	Resource   ResourceRef `json:"resource"`
	CreatedAt  *time.Time  `json:"createdAt"`
	ObjectJSON string      `json:"objectJSON"`
}
type AccessControlPage struct {
	Items []AccessControlRow `json:"items"`
	Page  PageInfo           `json:"page"`
}
type AccessControlReferenceRequest struct {
	Source        ResourceRef `json:"source"`
	Target        ResourceRef `json:"target"`
	SourceVersion string      `json:"sourceVersion"`
}
type ResolvedAccessControlReference struct {
	Detail           ResourceDetail `json:"detail"`
	Resource         ResourceRef    `json:"resource"`
	SourceVersion    string         `json:"sourceVersion"`
	TargetVersion    string         `json:"targetVersion"`
	IdentityEvidence string         `json:"identityEvidence"`
}
