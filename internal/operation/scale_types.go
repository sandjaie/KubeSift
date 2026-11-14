/*
Purpose: Carry the immutable scaling review and session result.
Used by: The native service and generated desktop bridge.
Responsibilities: Expose only target identity, reviewed counts, impact and outcome.
Important: Execution accepts an opaque token and acknowledgement only.
*/
package operation

import (
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"time"
)

type ScaleRequest struct {
	Resource      kube.ResourceRef `json:"resource"`
	SourceVersion string           `json:"sourceVersion"`
	ReplicasText  string           `json:"replicasText"`
}
type ScaleImpact struct {
	Direction     string `json:"direction"`
	WhenScaled    string `json:"whenScaled"`
	WhenDeleted   string `json:"whenDeleted"`
	PolicyDefault bool   `json:"policyDefault"`
}
type ScalePreview struct {
	Cluster       string           `json:"cluster"`
	Token         string           `json:"token"`
	Resource      kube.ResourceRef `json:"resource"`
	SourceVersion string           `json:"sourceVersion"`
	Before        int32            `json:"before"`
	After         int32            `json:"after"`
	ExpiresAt     time.Time        `json:"expiresAt"`
	Permission    string           `json:"permission"`
	Warnings      []string         `json:"warnings"`
	Impact        ScaleImpact      `json:"impact"`
	NoChange      bool             `json:"noChange"`
}
type ScaleResult struct {
	Cluster     string           `json:"cluster"`
	OperationID string           `json:"operationID"`
	Resource    kube.ResourceRef `json:"resource"`
	Before      int32            `json:"before"`
	Requested   int32            `json:"requested"`
	Reported    *int32           `json:"reported"`
	StartedAt   time.Time        `json:"startedAt"`
	FinishedAt  *time.Time       `json:"finishedAt"`
	State       string           `json:"state"`
	Problem     *model.Problem   `json:"problem"`
}
