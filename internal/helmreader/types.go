/*
Purpose: Describe the read-only Helm inspection requests and responses.
Used by: The native reader and desktop bridge.
Responsibilities: Bind release evidence to a namespace, storage driver and exact recorded revision.
Important: Stored values and manifests belong only to their requested section.
*/
package helmreader

import (
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
)

type Scope struct {
	Connection  model.ConnectionRef `json:"connection"`
	Namespace   string              `json:"namespace"`
	Driver      string              `json:"driver"`
	ReleaseName string              `json:"releaseName"`
}

type RecordRef struct {
	Scope
	Revision        string `json:"revision"`
	RecordName      string `json:"recordName"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resourceVersion"`
}

type ReleaseSummary struct {
	Record       RecordRef      `json:"record"`
	Status       string         `json:"status"`
	ChartName    string         `json:"chartName"`
	ChartVersion string         `json:"chartVersion"`
	AppVersion   string         `json:"appVersion"`
	UpdatedAt    *time.Time     `json:"updatedAt"`
	Problem      *model.Problem `json:"problem"`
}

type ListRequest struct {
	Scope      Scope  `json:"scope"`
	SnapshotID string `json:"snapshotID"`
	Page       int    `json:"page"`
}

type ReleasePage struct {
	SnapshotID string           `json:"snapshotID"`
	Items      []ReleaseSummary `json:"items"`
	Page       int              `json:"page"`
	Total      int              `json:"total"`
	FetchedAt  time.Time        `json:"fetchedAt"`
}

type HistoryRequest struct {
	Record RecordRef `json:"record"`
	Page   int       `json:"page"`
}

type HistoryPage struct {
	Items     []ReleaseSummary `json:"items"`
	Page      int              `json:"page"`
	Total     int              `json:"total"`
	FetchedAt time.Time        `json:"fetchedAt"`
}

type DetailRequest struct {
	Record         RecordRef `json:"record"`
	Section        string    `json:"section"`
	ComputedValues bool      `json:"computedValues"`
}

type ReleaseSection struct {
	Record    RecordRef          `json:"record"`
	Section   string             `json:"section"`
	Text      string             `json:"text"`
	FetchedAt time.Time          `json:"fetchedAt"`
	Overview  *ReleaseOverview   `json:"overview"`
	Resources []ManifestResource `json:"resources"`
	Truncated bool               `json:"truncated"`
	Problem   *model.Problem     `json:"problem"`
}

type ReleaseOverview struct {
	ReleaseSummary
	Description   string     `json:"description"`
	FirstDeployed *time.Time `json:"firstDeployed"`
	DeletedAt     *time.Time `json:"deletedAt"`
}

type ManifestResource struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Resolvable bool   `json:"resolvable"`
	Issue      string `json:"issue"`
}
