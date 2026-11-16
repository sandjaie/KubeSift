/*
Purpose: Define the desktop contract for a named create draft and preview.
Used by: The native operation and generated Wails bindings.
Responsibilities: Carry frozen scope and bounded proposed and admitted documents.
Important: Only a token is needed to confirm the prepared POST.
*/
package operation

import (
	"time"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

type CreateDraft struct {
	ID         string              `json:"id"`
	Connection model.ConnectionRef `json:"connection"`
	Cluster    string              `json:"cluster"`
	Namespace  string              `json:"namespace"`
}

type CreateRequest struct {
	DraftID   string `json:"draftID"`
	YAML      string `json:"yaml"`
	Namespace string `json:"namespace"`
	Resource  string `json:"resource"`
}

type CreatePreview struct {
	Token        string               `json:"token"`
	ExpiresAt    time.Time            `json:"expiresAt"`
	Target       kube.ManifestAddress `json:"target"`
	ProposedYAML string               `json:"proposedYAML"`
	ServerYAML   string               `json:"serverYAML"`
	Warnings     []string             `json:"warnings"`
}
