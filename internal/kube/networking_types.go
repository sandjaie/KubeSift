/*
Purpose: Define typed networking inventory and relationship evidence for the bridge.
Used by: The networking reader, desktop adapter and workspace.
Responsibilities: Keep projections separate from complete captured resource details.
Important: A label association does not establish ownership or connectivity.
*/
package kube

import "time"

type NetworkRow struct {
	Resource   ResourceRef `json:"resource"`
	CreatedAt  *time.Time  `json:"createdAt"`
	ObjectJSON string      `json:"objectJSON"`
}
type NetworkPage struct {
	Items []NetworkRow `json:"items"`
	Page  PageInfo     `json:"page"`
}
