/*
Purpose: Describe captured Pod and Service targets for interactive sessions.
Used by: The native session adapters and generated desktop bridge.
Responsibilities: Carry explicit identities, selected ports and safe lifecycle evidence.
Important: Session metadata never holds terminal commands or network payload.
*/
package kube

import "github.com/sandjaie/KubeSift/internal/model"

type PodSessionTarget struct {
	Resource  ResourceRef `json:"resource"`
	Container string      `json:"container"`
}

type TerminalRequest struct {
	Target  PodSessionTarget `json:"target"`
	Command []string         `json:"command"`
}

type ForwardRequest struct {
	Target     PodSessionTarget         `json:"target"`
	LocalPort  uint16                   `json:"localPort"`
	RemotePort uint16                   `json:"remotePort"`
	Service    *ServiceForwardSelection `json:"service"`
}

type ServiceForwardSelection struct {
	Resource        ResourceRef `json:"resource"`
	ResourceVersion string      `json:"resourceVersion"`
	PortName        string      `json:"portName"`
	Port            int32       `json:"port"`
}

type ForwardCandidateRequest struct {
	Service        ResourceRef `json:"service"`
	ServiceVersion string      `json:"serviceVersion"`
	PortName       string      `json:"portName"`
	Port           int32       `json:"port"`
	Cursor         string      `json:"cursor"`
}

type ForwardCandidate struct {
	Pod        ResourceRef `json:"pod"`
	PodVersion string      `json:"podVersion"`
	RemotePort uint16      `json:"remotePort"`
}

type ForwardCandidatePage struct {
	Items    []ForwardCandidate `json:"items"`
	Page     PageInfo           `json:"page"`
	Complete bool               `json:"complete"`
}

type InteractiveEntry struct {
	Session    model.SessionRef         `json:"session"`
	Kind       string                   `json:"kind"`
	Target     PodSessionTarget         `json:"target"`
	Service    *ServiceForwardSelection `json:"service"`
	LocalPort  uint16                   `json:"localPort"`
	RemotePort uint16                   `json:"remotePort"`
	State      string                   `json:"state"`
	Problem    *model.Problem           `json:"problem"`
}

type interactiveState struct {
	State     string         `json:"state"`
	LocalPort uint16         `json:"localPort,omitempty"`
	Problem   *model.Problem `json:"problem,omitempty"`
}
