/*
Purpose: Inspect build source, strategy, output and observed lifecycle safely.
Used by: OpenShift Build and BuildConfig reads and reviewed Build actions.
Responsibilities: Project only descriptive fields and strip URL credentials.
Important: Inline sources, environment values, webhooks and secret references stay native.
*/
package kube

import (
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	"net/url"
	"strings"
)

type BuildReference struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
}
type BuildEvidence struct {
	SourceType  string          `json:"sourceType"`
	Repository  string          `json:"repository,omitempty"`
	Revision    string          `json:"revision,omitempty"`
	Strategy    string          `json:"strategy"`
	RunPolicy   string          `json:"runPolicy,omitempty"`
	Input       *BuildReference `json:"input,omitempty"`
	Output      *BuildReference `json:"output,omitempty"`
	Config      *BuildReference `json:"config,omitempty"`
	Phase       string          `json:"phase,omitempty"`
	Reason      string          `json:"reason,omitempty"`
	Started     string          `json:"started,omitempty"`
	Finished    string          `json:"finished,omitempty"`
	Cancelled   bool            `json:"cancelled,omitempty"`
	LastVersion *int64          `json:"lastVersion,omitempty"`
}

func projectBuild(raw []byte) (BuildEvidence, error) {
	var wire struct {
		Spec struct {
			RunPolicy string
			Source    struct {
				Type string
				Git  *struct{ URI, Ref string }
			}
			Strategy struct {
				Type                                           string
				SourceStrategy, DockerStrategy, CustomStrategy *struct{ From *BuildReference }
			}
			Output   struct{ To *BuildReference }
			Revision *struct{ Git *struct{ Commit string } }
		}
		Status struct {
			Phase, Reason, StartTimestamp, CompletionTimestamp string
			Config                                             *BuildReference
			Cancelled                                          bool
			LastVersion                                        *int64
		}
	}
	if json.Unmarshal(raw, &wire) != nil {
		return BuildEvidence{}, model.Fail("invalid", "Invalid Build evidence")
	}
	sp, st := wire.Spec, wire.Status
	out := BuildEvidence{SourceType: sp.Source.Type, Strategy: sp.Strategy.Type, RunPolicy: sp.RunPolicy, Output: sp.Output.To, Config: st.Config, Phase: st.Phase, Reason: st.Reason, Started: st.StartTimestamp, Finished: st.CompletionTimestamp, Cancelled: st.Cancelled, LastVersion: st.LastVersion}
	if git := sp.Source.Git; git != nil {
		out.Revision = git.Ref
		if u, err := url.Parse(git.URI); err == nil && u.Hostname() != "" && (u.Scheme == "https" || u.Scheme == "http" || u.Scheme == "ssh" || u.Scheme == "git") {
			u.User = nil
			u.RawQuery = ""
			u.ForceQuery = false
			u.Fragment = ""
			u.RawFragment = ""
			out.Repository = u.String()
		}
	}
	if sp.Revision != nil && sp.Revision.Git != nil {
		out.Revision = sp.Revision.Git.Commit
	}
	for _, strategy := range []*struct{ From *BuildReference }{sp.Strategy.SourceStrategy, sp.Strategy.DockerStrategy, sp.Strategy.CustomStrategy} {
		if strategy != nil && strategy.From != nil {
			out.Input = strategy.From
			break
		}
	}
	for _, r := range []*BuildReference{out.Input, out.Output, out.Config} {
		if r != nil && (len(r.Name) > 512 || strings.ContainsAny(r.Name, "?#") || strings.Contains(r.Name, "://")) {
			return BuildEvidence{}, model.Fail("invalid", "Invalid Build reference")
		}
	}
	return out, nil
}
