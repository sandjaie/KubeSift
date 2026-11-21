/*
Purpose: Describe ephemeral editor drafts and server reviews.
Used by: Native editing and desktop bindings.
Responsibilities: Carry the frozen target and editable YAML.
Important: Edit documents are never part of retained operation results.
*/
package operation

import (
	"time"

	"github.com/sandjaie/KubeSift/internal/kube"
)

type EditDraft struct {
	ID              string          `json:"id"`
	Target          kube.EditTarget `json:"target"`
	ResourceVersion string          `json:"resourceVersion"`
	Cluster         string          `json:"cluster"`
	YAML            string          `json:"yaml"`
}

type EditRequest struct {
	DraftID string `json:"draftID"`
	YAML    string `json:"yaml"`
}

type EditPreview struct {
	Token        string          `json:"token"`
	ExpiresAt    time.Time       `json:"expiresAt"`
	Target       kube.EditTarget `json:"target"`
	BeforeYAML   string          `json:"beforeYAML"`
	ProposedYAML string          `json:"proposedYAML"`
	ServerYAML   string          `json:"serverYAML"`
	Warnings     []string        `json:"warnings"`
}
