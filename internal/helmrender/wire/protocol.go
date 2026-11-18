/*
Purpose: Define the bounded renderer's private in-memory protocol.
Used by: The native host and its bundled WebAssembly worker.
Responsibilities: Preserve chart dependencies, typed capabilities and lookup requests.
Important: Payloads never enter files, logs or application result history.
*/
package wire

import (
	"helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
)

const OutputLimit = 8 << 20
const WireLimit = 64 << 20

type Chart struct {
	Data         *chartv2.Chart
	Dependencies []Chart
}

func Capture(ch *chartv2.Chart) Chart {
	data := *ch
	data.Values = Pack(ch.Values).(map[string]any)
	out := Chart{Data: &data}
	for _, dependency := range ch.Dependencies() {
		out.Dependencies = append(out.Dependencies, Capture(dependency))
	}
	return out
}

func (ch Chart) Restore() *chartv2.Chart {
	ch.Data.Values = Unpack(ch.Data.Values).(map[string]any)
	for _, dependency := range ch.Dependencies {
		ch.Data.AddDependency(dependency.Restore())
	}
	return ch.Data
}

type Request struct {
	Chart        Chart
	Values       common.Values
	Release      map[string]any
	Capabilities *common.Capabilities
}

type Message struct {
	Lookup []string          `json:"lookup,omitempty"`
	Files  map[string]string `json:"files,omitempty"`
	Error  string            `json:"error,omitempty"`
}

type LookupResponse struct {
	Object map[string]any
	Failed bool
}
