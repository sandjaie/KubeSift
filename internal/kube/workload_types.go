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

type DetailRequest struct {
	Resource ResourceRef `json:"resource"`
}
type ResourceDetail struct {
	Resource        ResourceRef `json:"resource"`
	ResourceVersion string      `json:"resourceVersion"`
	FetchedAt       time.Time   `json:"fetchedAt"`
	ObjectJSON      string      `json:"objectJSON"`
}
type YAMLDocument struct {
	Resource             ResourceRef `json:"resource"`
	ResourceVersion      string      `json:"resourceVersion"`
	FetchedAt            time.Time   `json:"fetchedAt"`
	WithoutManagedFields string      `json:"withoutManagedFields"`
	WithManagedFields    string      `json:"withManagedFields"`
}
type RelatedRequest struct {
	Resource ResourceRef `json:"resource"`
	Cursor   string      `json:"cursor"`
}
type RelatedPod struct {
	Pod        WorkloadRow `json:"pod"`
	ReplicaSet ResourceRef `json:"replicaSet"`
}
type RelatedPage struct {
	Items    []RelatedPod   `json:"items"`
	Page     PageInfo       `json:"page"`
	Complete bool           `json:"complete"`
	Problem  *model.Problem `json:"problem"`
}

type EventRequest struct {
	Resource ResourceRef `json:"resource"`
	Cursor   string      `json:"cursor"`
}
type ResourceEvent struct {
	UID      string     `json:"uid"`
	Type     string     `json:"type"`
	Reason   string     `json:"reason"`
	Message  string     `json:"message"`
	Reporter string     `json:"reporter"`
	Count    *int32     `json:"count"`
	FirstAt  *time.Time `json:"firstAt"`
	LastAt   *time.Time `json:"lastAt"`
}
type EventPage struct {
	Items []ResourceEvent `json:"items"`
	Page  PageInfo        `json:"page"`
}
