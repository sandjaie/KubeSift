/*
Purpose: Carry bounded infrastructure inventory and Event evidence.
Used by: Readers, desktop bindings and the Cluster views.
Responsibilities: Preserve captured identity and optional observation data.
Important: Inventory never returns full resource bodies.
*/
package kube

import "time"

type ClusterResourceRow struct {
	Resource   ResourceRef `json:"resource"`
	CreatedAt  *time.Time  `json:"createdAt"`
	ObjectJSON string      `json:"objectJSON"`
}
type ClusterResourcePage struct {
	Items []ClusterResourceRow `json:"items"`
	Page  PageInfo             `json:"page"`
}

type EventInventoryRow struct {
	Resource  ResourceRef   `json:"resource"`
	Event     ResourceEvent `json:"event"`
	Regarding string        `json:"regarding"`
	Target    *ResourceRef  `json:"target"`
}
type EventInventoryPage struct {
	Items []EventInventoryRow `json:"items"`
	Page  PageInfo            `json:"page"`
}
type EventTargetRequest struct {
	Source        ResourceRef `json:"source"`
	SourceVersion string      `json:"sourceVersion"`
}
type ResolvedEventTarget struct {
	Detail ResourceDetail `json:"detail"`
}
