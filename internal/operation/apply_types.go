/*
Purpose: Describe an ephemeral apply draft and server review.
Used by: The native apply service and generated desktop bindings.
Responsibilities: Carry the desired YAML, ownership evidence and frozen target.
Important: Retained results never contain draft text, field paths or values.
*/
package operation

import "github.com/sandjaie/KubeSift/internal/kube"

type ApplyDraft struct {
	EditDraft
	BaselineYAML string `json:"baselineYAML"`
}

type ApplyRequest struct {
	DraftID       string `json:"draftID"`
	YAML          string `json:"yaml"`
	TakeOwnership bool   `json:"takeOwnership"`
}

type ApplyReview struct {
	EditPreview
	Conflicts           []ApplyConflict        `json:"conflicts,omitempty"`
	Ownership           []ApplyOwnershipChange `json:"ownership,omitempty"`
	OwnershipIncomplete bool                   `json:"ownershipIncomplete"`
	OwnershipTruncated  bool                   `json:"ownershipTruncated"`
	Force               bool                   `json:"force"`
}

type ApplyConflict struct {
	Manager string `json:"manager"`
	Field   string `json:"field"`
}

type ApplyOwnershipChange struct {
	Field  string `json:"field"`
	Change string `json:"change"`
}

type ApplyResultData struct {
	Target  kube.EditTarget `json:"target"`
	Manager string          `json:"manager"`
	Force   bool            `json:"force"`
}
