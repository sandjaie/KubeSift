/*
Purpose: Describe bounded, projected Kubernetes discovery data.
Used by: Native catalog and installed API browser.
Responsibilities: Expose canonical API identities and opaque browse handles.
Important: Discovery hints do not establish read permission.
*/
package kube

import (
	"github.com/sandjaie/KubeSift/internal/model"
	"time"
)

type APIGroup struct {
	Group     string   `json:"group"`
	Versions  []string `json:"versions"`
	Preferred string   `json:"preferred"`
}
type APIGroupCatalog struct {
	Groups      []APIGroup     `json:"groups"`
	CoreProblem *model.Problem `json:"coreProblem"`
	APIsProblem *model.Problem `json:"apisProblem"`
	FetchedAt   time.Time      `json:"fetchedAt"`
	Epoch       string         `json:"epoch"`
}
type APIResourceDescriptor struct {
	Group      string `json:"group"`
	Version    string `json:"version"`
	Resource   string `json:"resource"`
	Kind       string `json:"kind"`
	Namespaced bool   `json:"namespaced"`
	List       bool   `json:"list"`
	Get        bool   `json:"get"`
	Mode       string `json:"mode"`
	Reason     string `json:"reason"`
	Handle     string `json:"handle"`
}

func (d APIResourceDescriptor) api() apiResource {
	return apiResource{d.Group, d.Version, d.Resource, d.Kind, d.Namespaced}
}

type APIResourceCatalog struct {
	Resources []APIResourceDescriptor `json:"resources"`
	FetchedAt time.Time               `json:"fetchedAt"`
	Epoch     string                  `json:"epoch"`
}
type APIGroupRequest struct {
	Connection model.ConnectionRef `json:"connection"`
	Refresh    bool                `json:"refresh"`
}
type APIResourceRequest struct {
	Connection model.ConnectionRef `json:"connection"`
	Group      string              `json:"group"`
	Version    string              `json:"version"`
	Refresh    bool                `json:"refresh"`
}
