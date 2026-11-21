/*
Purpose: Describe Node reviews and retained scheduling and eviction evidence.
Used by: Node operations and the desktop review and progress surfaces.
Responsibilities: Carry only reviewed identities, scheduling state and safe impact reasons.
Important: Raw Pod manifests and write settings stay native.
*/
package operation

import (
	"github.com/sandjaie/KubeSift/internal/kube"
	"time"
)

type NodeRequest struct {
	Resource      kube.ResourceRef `json:"resource"`
	SourceVersion string           `json:"sourceVersion"`
	Action        string           `json:"action"`
	WaitPreset    int              `json:"waitPreset"`
}
type NodePod struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
	State     string `json:"state"`
	Reason    string `json:"reason"`
	owner     string
}
type NodeProgress struct {
	DetailsExpired bool           `json:"detailsExpired"`
	Action         string         `json:"action"`
	Cordoned       *bool          `json:"cordoned"`
	Rows           []NodePod      `json:"rows,omitempty"`
	Counts         map[string]int `json:"counts"`
	Reason         string         `json:"reason"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}
type NodePreview struct {
	Environment string `json:"environment"`
	Typed       bool   `json:"typed"`
	NodeRequest
	Cluster   string         `json:"cluster"`
	Cordoned  bool           `json:"cordoned"`
	Rows      []NodePod      `json:"rows"`
	Counts    map[string]int `json:"counts"`
	NoChange  bool           `json:"noChange"`
	Token     string         `json:"token"`
	ExpiresAt time.Time      `json:"expiresAt"`
}

func nodeCounts(rows []NodePod) map[string]int {
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.State]++
	}
	return counts
}
