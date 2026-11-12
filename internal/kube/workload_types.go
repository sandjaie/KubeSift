/*
Purpose: Define immutable workload investigation requests and bounded read results.
Used by: Kubernetes readers, desktop bindings and workload presentation.
Responsibilities: Carry explicit connection generations, resource UIDs and page evidence.
Important: Optional counts distinguish unavailable status from zero; payloads remain memory-only.
*/
package kube

import (
	"github.com/sandjaie/KubeSift/internal/model"
	"time"
)

type ResourceRef struct {
	Connection model.ConnectionRef `json:"connection"`
	APIVersion string              `json:"apiVersion"`
	Kind       string              `json:"kind"`
	Namespace  string              `json:"namespace"`
	Name       string              `json:"name"`
	UID        string              `json:"uid"`
}
type PageRequest struct {
	Connection model.ConnectionRef `json:"connection"`
	Kind       string              `json:"kind"`
	Namespace  string              `json:"namespace"`
	Cursor     string              `json:"cursor"`
}
type PageInfo struct {
	NextCursor      string    `json:"nextCursor"`
	ResourceVersion string    `json:"resourceVersion"`
	FetchedAt       time.Time `json:"fetchedAt"`
}
type WorkloadRow struct {
	Resource  ResourceRef `json:"resource"`
	Phase     string      `json:"phase"`
	Reason    string      `json:"reason"`
	Node      string      `json:"node"`
	Ready     *int32      `json:"ready"`
	Desired   *int32      `json:"desired"`
	Updated   *int32      `json:"updated"`
	Available *int32      `json:"available"`
	Restarts  *int32      `json:"restarts"`
	CreatedAt *time.Time  `json:"createdAt"`
}
type WorkloadPage struct {
	Items []WorkloadRow `json:"items"`
	Page  PageInfo      `json:"page"`
}
