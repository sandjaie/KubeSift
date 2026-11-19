/*
Purpose: Describe current CPU and memory evidence.
Used by: The metrics reader and desktop presentation.
Responsibilities: Keep missing quantities distinct from zero and bind samples to captured resources.
Important: Amounts are decimal strings, CPU in millicores and memory in bytes.
*/
package kube

import (
	"github.com/sandjaie/KubeSift/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

type MetricsRequest struct {
	Connection       model.ConnectionRef `json:"connection"`
	Targets          []ResourceRef       `json:"targets"`
	Detail           *ResourceRef        `json:"detail"`
	RefreshDiscovery bool                `json:"refreshDiscovery"`
}
type MetricValue struct {
	Amount             *string `json:"amount"`
	Request            *string `json:"request"`
	Limit              *string `json:"limit"`
	Allocatable        *string `json:"allocatable"`
	RequestPercent     *string `json:"requestPercent"`
	LimitPercent       *string `json:"limitPercent"`
	AllocatablePercent *string `json:"allocatablePercent"`
}
type ContainerUsage struct {
	Name     string      `json:"name"`
	Category string      `json:"category"`
	CPU      MetricValue `json:"cpu"`
	Memory   MetricValue `json:"memory"`
}
type ResourceUsage struct {
	Resource          ResourceRef      `json:"resource"`
	APIVersion        string           `json:"apiVersion"`
	State             string           `json:"state"`
	SampledAt         *time.Time       `json:"sampledAt"`
	WindowSeconds     string           `json:"windowSeconds"`
	FetchedAt         time.Time        `json:"fetchedAt"`
	ConfiguredAt      *time.Time       `json:"configuredAt"`
	CPU               MetricValue      `json:"cpu"`
	Memory            MetricValue      `json:"memory"`
	CPUComplete       bool             `json:"cpuComplete"`
	MemoryComplete    bool             `json:"memoryComplete"`
	MissingContainers []string         `json:"missingContainers"`
	Containers        []ContainerUsage `json:"containers"`
	PodLevelBudget    bool             `json:"podLevelBudget"`
	Problem           *model.Problem   `json:"problem"`
}
type MetricsBatch struct {
	Items     []ResourceUsage `json:"items"`
	FetchedAt time.Time       `json:"fetchedAt"`
	Problem   *model.Problem  `json:"problem"`
}
type containerMetric struct {
	Name  string            `json:"name"`
	Usage map[string]string `json:"usage"`
}
type metricsWire struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   metav1.ObjectMeta `json:"metadata"`
	Timestamp  metav1.Time       `json:"timestamp"`
	Window     string            `json:"window"`
	Containers []containerMetric `json:"containers"`
	Usage      map[string]string `json:"usage"`
}
