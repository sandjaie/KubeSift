/*
Purpose: Describe one page of retained rollout revisions.
Used by: Native history, the desktop bridge and the Revisions tab.
Responsibilities: Bind source rows to the captured parent and continuation.
Important: Historical templates stay native and never enter history rows.
*/
package kube

import (
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
)

type RolloutHistoryRequest struct {
	Resource ResourceRef `json:"resource"`
	Cursor   string      `json:"cursor"`
}

type RolloutRevision struct {
	Resource        ResourceRef `json:"resource"`
	ResourceVersion string      `json:"resourceVersion"`
	Revision        string      `json:"revision"`
	CreatedAt       *time.Time  `json:"createdAt"`
	Images          []string    `json:"images"`
	ChangeCause     string      `json:"changeCause"`
}

type RolloutHistoryPage struct {
	Items           []RolloutRevision `json:"items"`
	SourceVersion   string            `json:"sourceVersion"`
	CurrentRevision string            `json:"currentRevision"`
	UpdateRevision  string            `json:"updateRevision"`
	Page            PageInfo          `json:"page"`
	Complete        bool              `json:"complete"`
	Problem         *model.Problem    `json:"problem"`
}
