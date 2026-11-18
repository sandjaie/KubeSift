/*
Purpose: Define explicit chart-source and release-management contracts.
Used by: Native Helm management and its desktop adapter.
Responsibilities: Separate public metadata from session-only credentials and chart content.
Important: Execute accepts only a native review token, never replacement inputs.
*/
package helmmanager

import (
	"github.com/sandjaie/KubeSift/internal/helmreader"
	"time"
)

type Source struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	CAFile   string `json:"caFile"`
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`
}
type SourceCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type ChartRequest struct {
	SourceID    string `json:"sourceID"`
	LocalHandle string `json:"localHandle"`
	Chart       string `json:"chart"`
	Version     string `json:"version"`
	Digest      string `json:"digest"`
}
type ChartSnapshot struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Digest    string    `json:"digest"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type ChartEntry struct {
	Name    string   `json:"name" yaml:"name"`
	Version string   `json:"version" yaml:"version"`
	Digest  string   `json:"digest" yaml:"digest"`
	URLs    []string `json:"-" yaml:"urls"`
}
type ChartPage struct {
	Items      []ChartEntry `json:"items"`
	NextCursor string       `json:"nextCursor"`
}

type Target = helmreader.Scope

type Options struct {
	DisableHooks    bool `json:"disableHooks"`
	CreateNamespace bool `json:"createNamespace"`
	KeepHistory     bool `json:"keepHistory"`
	TimeoutSeconds  int  `json:"timeoutSeconds"`
}

type PrepareRequest struct {
	Action         string                `json:"action"`
	Target         Target                `json:"target"`
	ChartID        string                `json:"chartID"`
	ValuesYAML     string                `json:"valuesYAML"`
	RollbackRecord *helmreader.RecordRef `json:"rollbackRecord"`
	Options        Options               `json:"options"`
}

type Review struct {
	Token        string                `json:"token"`
	Target       Target                `json:"target"`
	Action       string                `json:"action"`
	ChartName    string                `json:"chartName"`
	ChartVersion string                `json:"chartVersion"`
	Digest       string                `json:"digest"`
	ExpiresAt    time.Time             `json:"expiresAt"`
	Current      *helmreader.RecordRef `json:"current"`
	Rollback     *helmreader.RecordRef `json:"rollback"`
	Options      Options               `json:"options"`
	Sections     []string              `json:"sections"`
	Warnings     []string              `json:"warnings"`
}

type ReviewSection struct {
	Text       string `json:"text"`
	Offset     int    `json:"offset"`
	NextOffset int    `json:"nextOffset"`
	Done       bool   `json:"done"`
}
