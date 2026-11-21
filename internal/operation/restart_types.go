/*
Purpose: Define safe restart requests and review evidence.
Used by: Native restart and generated desktop bindings.
Responsibilities: Expose only identity, rollout observations and the single reviewed annotation.
Important: Full manifests, patch bytes and spec hashes stay native.
*/
package operation

import (
	"time"

	"github.com/sandjaie/KubeSift/internal/kube"
)

type RestartRequest struct {
	Resource      kube.ResourceRef `json:"resource"`
	SourceVersion string           `json:"sourceVersion"`
}

type RestartPreview struct {
	Token             string           `json:"token"`
	Cluster           string           `json:"cluster"`
	Resource          kube.ResourceRef `json:"resource"`
	SourceVersion     string           `json:"sourceVersion"`
	ExpiresAt         time.Time        `json:"expiresAt"`
	Permission        string           `json:"permission"`
	Warnings          []string         `json:"warnings"`
	RestartAt         string           `json:"restartAt"`
	PreviousValue     string           `json:"previousValue"`
	PreviousSet       bool             `json:"previousSet"`
	PreviousTruncated bool             `json:"previousTruncated"`
	Impact            RestartImpact    `json:"impact"`
}

type RestartImpact struct {
	Strategy              string `json:"strategy"`
	StrategyDefault       bool   `json:"strategyDefault"`
	Desired               *int32 `json:"desired"`
	DesiredDefault        bool   `json:"desiredDefault"`
	Current               *int32 `json:"current"`
	Ready                 *int32 `json:"ready"`
	MaxSurge              string `json:"maxSurge"`
	MaxSurgeDefault       bool   `json:"maxSurgeDefault"`
	MaxUnavailable        string `json:"maxUnavailable"`
	MaxUnavailableDefault bool   `json:"maxUnavailableDefault"`
	Partition             *int32 `json:"partition"`
	PartitionDefault      bool   `json:"partitionDefault"`
	Recreate              bool   `json:"recreate"`
	ZeroPods              bool   `json:"zeroPods"`
}
