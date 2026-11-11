/*
Purpose: Keep independently configured kubeconfig sources and their latest snapshots.
Used by: The explorer coordinator and source settings.
Responsibilities: Load safe catalogs, enforce snapshot identities and persist source labels.
Important: Local catalog loading never resolves helpers or contacts Kubernetes.
*/
package sources

import (
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/model"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

type CatalogEntry struct {
	SourceID         string `json:"sourceId"`
	SnapshotID       string `json:"snapshotId"`
	Context          string `json:"context"`
	Namespace        string `json:"namespace"`
	Endpoint         string `json:"endpoint"`
	SourceDisplay    string `json:"sourceDisplay"`
	Environment      string `json:"environment"`
	LabelNeedsReview bool   `json:"labelNeedsReview"`
}
type SourceResult struct {
	SourceID   string         `json:"sourceId"`
	SnapshotID string         `json:"snapshotId"`
	Path       string         `json:"path"`
	Enabled    bool           `json:"enabled"`
	Contexts   []CatalogEntry `json:"contexts"`
	Problem    *model.Problem `json:"problem"`
}
type record struct {
	snapshot *config.Snapshot
	result   SourceResult
}
type Registry struct {
	mu       sync.Mutex
	path     string
	settings settings
	records  map[string]record
}

func NewRegistry(path string) (*Registry, error) {
	p, e := load(path)
	if e != nil {
		return nil, model.Fail("invalid_source", "Source settings are invalid; restore or reset sources.json explicitly in Settings")
	}
	r := &Registry{path: path, settings: p, records: map[string]record{}}
	for _, s := range p.Sources {
		r.reload(s)
	}
	return r, nil
}
func (r *Registry) List() []SourceResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []SourceResult{}
	for _, s := range r.settings.Sources {
		v := r.records[s.ID].result
		v.Contexts = append([]CatalogEntry{}, v.Contexts...)
		out = append(out, v)
	}
	return out
}
func (r *Registry) reload(s SourceConfig) SourceResult {
	out := SourceResult{SourceID: s.ID, SnapshotID: model.ID(), Path: s.Path, Enabled: s.Enabled, Contexts: []CatalogEntry{}}
	rec := record{result: out}
	defer func() { r.records[s.ID] = rec }()
	if !s.Enabled {
		return out
	}
	paths := []string{}
	if s.ID != "default" {
		paths = []string{s.Path}
		if _, e := os.Stat(s.Path); e != nil {
			out.Problem = &model.Problem{Code: "invalid_source", Message: "Source file is missing or unreadable"}
			rec.result = out
			return out
		}
	}
	snap, e := config.Load(paths)
	if e != nil {
		out.Problem = &model.Problem{Code: "invalid_source", Message: "Cannot parse kubeconfig source"}
		rec.result = out
		return out
	}
	out.Path = snap.SourceDisplay()
	contexts := snap.Contexts()
	if len(contexts) > 2000 {
		out.Problem = &model.Problem{Code: "capacity", Message: "Source exceeds 2,000 contexts"}
		rec.result = out
		return out
	}
	for _, c := range contexts {
		entry := CatalogEntry{SourceID: s.ID, SnapshotID: out.SnapshotID, Context: c.Name, Namespace: c.Namespace, Endpoint: c.Endpoint, SourceDisplay: out.Path}
		for _, l := range r.settings.Labels {
			if l.SourceID == s.ID && l.Context == c.Name {
				entry.LabelNeedsReview = l.Identity != snap.LabelIdentity(c.Name)
				if !entry.LabelNeedsReview {
					entry.Environment = l.Value
				}
			}
		}
		out.Contexts = append(out.Contexts, entry)
	}
	rec = record{snap, out}
	return out
}
func (r *Registry) Reload(id string) (SourceResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.settings.Sources {
		if s.ID == id {
			return r.reload(s), nil
		}
	}
	return SourceResult{}, model.Fail("invalid_source", "Unknown source")
}
func (r *Registry) Resolve(id, snapshot, name string) (config.Resolved, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[id]
	if !ok || rec.snapshot == nil || snapshot == "" || rec.result.SnapshotID != snapshot {
		return config.Resolved{}, model.Fail("stale_target", "Reload source and select its current context")
	}
	v, e := rec.snapshot.Resolve(name)
	return v.WithSource(id), e
}
func (r *Registry) Add(path string) (SourceResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !filepath.IsAbs(path) {
		return SourceResult{}, model.Fail("invalid_source", "Enter an absolute kubeconfig path")
	}
	path = filepath.Clean(path)
	if p, e := filepath.EvalSymlinks(path); e == nil {
		path = p
	}
	for _, s := range r.settings.Sources {
		if s.Path == path {
			return SourceResult{}, model.Fail("invalid_source", "This source path is already configured")
		}
	}
	if len(r.settings.Sources) >= 32 {
		return SourceResult{}, model.Fail("capacity", "At most 32 source groups can be configured")
	}
	s := SourceConfig{model.ID(), path, true}
	p := r.copySettings()
	p.Sources = append(p.Sources, s)
	if e := r.persist(p); e != nil {
		return SourceResult{}, e
	}
	return r.reload(s), nil
}
func (r *Registry) copySettings() settings {
	p := r.settings
	p.Sources = append([]SourceConfig{}, p.Sources...)
	p.Labels = append([]Label{}, p.Labels...)
	return p
}
func (r *Registry) persist(p settings) error {
	if e := save(r.path, p); e != nil {
		return model.Fail("invalid_source", "Could not save source settings; previous configuration retained")
	}
	r.settings = p
	return nil
}
func (r *Registry) SetEnabled(id string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.copySettings()
	for i := range p.Sources {
		if p.Sources[i].ID == id {
			p.Sources[i].Enabled = enabled
			if e := r.persist(p); e != nil {
				return e
			}
			r.reload(p.Sources[i])
			return nil
		}
	}
	return model.Fail("invalid_source", "Unknown source")
}
func (r *Registry) Remove(id string) error {
	if id == "default" {
		return r.SetEnabled(id, false)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.copySettings()
	found := false
	for i, s := range p.Sources {
		if s.ID == id {
			p.Sources = append(p.Sources[:i], p.Sources[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return model.Fail("invalid_source", "Unknown source")
	}
	labels := []Label{}
	for _, l := range p.Labels {
		if l.SourceID != id {
			labels = append(labels, l)
		}
	}
	p.Labels = labels
	if e := r.persist(p); e != nil {
		return e
	}
	delete(r.records, id)
	return nil
}
func (r *Registry) SetLabel(id, name, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > 30 {
		return model.Fail("invalid_source", "Labels are limited to 30 characters")
	}
	rec, ok := r.records[id]
	if !ok || rec.snapshot == nil || rec.snapshot.LabelIdentity(name) == "" {
		return model.Fail("stale_target", "Reload this source before assigning a label")
	}
	p := r.copySettings()
	labels := []Label{}
	for _, l := range p.Labels {
		if l.SourceID != id || l.Context != name {
			labels = append(labels, l)
		}
	}
	if value != "" {
		labels = append(labels, Label{id, name, value, rec.snapshot.LabelIdentity(name)})
	}
	p.Labels = labels
	sort.Slice(p.Labels, func(i, j int) bool {
		return p.Labels[i].SourceID+p.Labels[i].Context < p.Labels[j].SourceID+p.Labels[j].Context
	})
	if e := r.persist(p); e != nil {
		return e
	}
	for i := range rec.result.Contexts {
		if rec.result.Contexts[i].Context == name {
			rec.result.Contexts[i].Environment = value
			rec.result.Contexts[i].LabelNeedsReview = false
		}
	}
	r.records[id] = rec
	return nil
}
