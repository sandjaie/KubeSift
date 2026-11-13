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
type JobSummary struct {
	Status      string `json:"status"`
	Active      *int32 `json:"active"`
	Succeeded   *int32 `json:"succeeded"`
	Failed      *int32 `json:"failed"`
	Completions *int32 `json:"completions"`
}
type CronJobSummary struct {
	Schedule      string     `json:"schedule"`
	TimeZone      *string    `json:"timeZone"`
	Suspended     *bool      `json:"suspended"`
	Active        int        `json:"active"`
	LastScheduled *time.Time `json:"lastScheduled"`
}

type WorkloadRow struct {
	Total     *int32          `json:"total"`
	Job       *JobSummary     `json:"job"`
	CronJob   *CronJobSummary `json:"cronJob"`
	Resource  ResourceRef     `json:"resource"`
	Phase     string          `json:"phase"`
	Reason    string          `json:"reason"`
	Node      string          `json:"node"`
	Ready     *int32          `json:"ready"`
	Desired   *int32          `json:"desired"`
	Current   *int32          `json:"current"`
	Updated   *int32          `json:"updated"`
	Available *int32          `json:"available"`
	Restarts  *int32          `json:"restarts"`
	CreatedAt *time.Time      `json:"createdAt"`
}
type WorkloadPage struct {
	Items []WorkloadRow `json:"items"`
	Page  PageInfo      `json:"page"`
}

type DetailRequest struct {
	IncludeConfigMapData bool        `json:"includeConfigMapData,omitempty"`
	Resource             ResourceRef `json:"resource"`
}
type ResourceDetail struct {
	Job             *JobSummary `json:"job"`
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
	Controller ResourceRef `json:"controller"`
}
type RelatedPage struct {
	Items    []RelatedPod   `json:"items"`
	Page     PageInfo       `json:"page"`
	Complete bool           `json:"complete"`
	Problem  *model.Problem `json:"problem"`
}

type RelatedReplicaSetPage struct {
	Items    []WorkloadRow  `json:"items"`
	Page     PageInfo       `json:"page"`
	Complete bool           `json:"complete"`
	Problem  *model.Problem `json:"problem"`
}

type RelatedJobPage struct {
	Items    []WorkloadRow  `json:"items"`
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

type LogRequest struct {
	Resource  ResourceRef `json:"resource"`
	Container string      `json:"container"`
	Previous  bool        `json:"previous"`
	Follow    bool        `json:"follow"`
	TailLines int64       `json:"tailLines"`
}
