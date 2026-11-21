/*
Purpose: Define generic resource reads separately from fixed-kind targeting.
Used by: Installed API inventory and detail views.
Responsibilities: Carry metadata and exact, bounded object captures.
Important: Only opaque descriptor handles can address a generic read.
*/
package kube

import (
	"github.com/sandjaie/KubeSift/internal/model"
	"time"
)

type GenericResourceRef struct {
	Connection model.ConnectionRef `json:"connection"`
	Group      string              `json:"group"`
	Version    string              `json:"version"`
	Resource   string              `json:"resource"`
	Kind       string              `json:"kind"`
	Namespace  string              `json:"namespace"`
	Name       string              `json:"name"`
	UID        string              `json:"uid"`
}
type GenericPageRequest struct {
	Connection model.ConnectionRef `json:"connection"`
	Handle     string              `json:"handle"`
	Namespace  string              `json:"namespace"`
	Cursor     string              `json:"cursor"`
}
type GenericDetailRequest struct {
	Connection model.ConnectionRef `json:"connection"`
	Handle     string              `json:"handle"`
	Namespace  string              `json:"namespace"`
	Name       string              `json:"name"`
	UID        string              `json:"uid"`
}
type GenericResourceRow struct {
	Resource  GenericResourceRef `json:"resource"`
	CreatedAt *time.Time         `json:"createdAt"`
}
type GenericResourcePage struct {
	Items []GenericResourceRow `json:"items"`
	Page  PageInfo             `json:"page"`
}
type GenericResourceDetail struct {
	Resource        GenericResourceRef `json:"resource"`
	ResourceVersion string             `json:"resourceVersion"`
	FetchedAt       time.Time          `json:"fetchedAt"`
	ObjectJSON      string             `json:"objectJSON"`
	YAML            string             `json:"yaml"`
}
