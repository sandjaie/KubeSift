/*
Purpose: Define the reviewed deletion contract.
Used by: Native deletion and the desktop bindings.
Responsibilities: Carry captured identity, safe impact evidence and readback separately.
Important: Full objects and serialized request options stay native.
*/
package operation

import (
	"time"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

type DeleteRequest struct {
	Resource      kube.ResourceRef `json:"resource"`
	SourceVersion string           `json:"sourceVersion"`
}

type DeletePreview struct {
	Token         string           `json:"token"`
	Cluster       string           `json:"cluster"`
	Resource      kube.ResourceRef `json:"resource"`
	SourceVersion string           `json:"sourceVersion"`
	ExpiresAt     time.Time        `json:"expiresAt"`
	Permission    string           `json:"permission"`
	Warnings      []string         `json:"warnings"`
	Impact        DeleteImpact     `json:"impact"`
}

type DeleteImpact struct {
	Propagation       string       `json:"propagation"`
	GracePolicy       string       `json:"gracePolicy"`
	PodGrace          *int64       `json:"podGrace"`
	PodGraceDefault   bool         `json:"podGraceDefault"`
	Owner             *DeleteOwner `json:"owner"`
	OwnershipUnknown  bool         `json:"ownershipUnknown"`
	Finalizers        []string     `json:"finalizers"`
	FinalizerCount    int          `json:"finalizerCount"`
	FinalizersPartial bool         `json:"finalizersPartial"`
	WhenDeleted       string       `json:"whenDeleted"`
	RetentionDefault  bool         `json:"retentionDefault"`
	Desired           *int32       `json:"desired"`
	Current           *int32       `json:"current"`
	Ready             *int32       `json:"ready"`
	Active            *int32       `json:"active"`
}

type DeleteOwner struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	UID        string `json:"uid"`
}

type DeleteResultData struct {
	Propagation string `json:"propagation"`
	GracePolicy string `json:"gracePolicy"`
}

type DeleteObservation struct {
	OperationID string           `json:"operationID"`
	Resource    kube.ResourceRef `json:"resource"`
	CheckedAt   time.Time        `json:"checkedAt"`
	State       string           `json:"state"`
	Problem     *model.Problem   `json:"problem"`
}
