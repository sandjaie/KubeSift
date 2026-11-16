/*
Purpose: Define the captured target, source, preview and result for workload rollback.
Used by: The rollback service and desktop bindings.
Responsibilities: Carry exact identities and ephemeral review documents.
Important: Retained results never contain templates, patches or diffs.
*/
package operation

import (
	"time"

	"github.com/sandjaie/KubeSift/internal/kube"
)

type RollbackRequest struct {
	Resource        kube.ResourceRef `json:"resource"`
	ResourceVersion string           `json:"resourceVersion"`
	Source          kube.ResourceRef `json:"source"`
	SourceVersion   string           `json:"sourceVersion"`
	Revision        string           `json:"revision"`
}

type RollbackPreview struct {
	Token        string          `json:"token"`
	ExpiresAt    time.Time       `json:"expiresAt"`
	Request      RollbackRequest `json:"request"`
	Cluster      string          `json:"cluster"`
	BeforeYAML   string          `json:"beforeYAML"`
	ProposedYAML string          `json:"proposedYAML"`
	ServerYAML   string          `json:"serverYAML"`
	Notices      []string        `json:"notices"`
	Warnings     []string        `json:"warnings"`
}

type RollbackResultData struct {
	SourceName string `json:"sourceName"`
	SourceUID  string `json:"sourceUID"`
	Revision   string `json:"revision"`
}
