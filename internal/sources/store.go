/*
Purpose: Persist source references and user labels separately from appearance.
Used by: The source registry.
Responsibilities: Reject malformed metadata and atomically save bounded private settings.
Important: This file never stores credentials or writes kubeconfig.
*/
package sources

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

type SourceConfig struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}
type Label struct {
	SourceID string `json:"sourceId"`
	Context  string `json:"context"`
	Value    string `json:"value"`
	Identity string `json:"identity"`
}
type settings struct {
	Version int            `json:"version"`
	Sources []SourceConfig `json:"sources"`
	Labels  []Label        `json:"labels"`
}

func defaults() settings {
	return settings{1, []SourceConfig{{ID: "default", Enabled: true}}, []Label{}}
}
func validate(p settings) error {
	if p.Version != 1 || p.Sources == nil || p.Labels == nil || len(p.Sources) > 32 {
		return errors.New("invalid source settings version or capacity")
	}
	ids, paths := map[string]bool{}, map[string]bool{}
	for _, s := range p.Sources {
		if s.ID == "" || ids[s.ID] || (s.ID == "default" && s.Path != "") || (s.ID != "default" && (!filepath.IsAbs(s.Path) || paths[s.Path])) {
			return errors.New("invalid source identity or path")
		}
		ids[s.ID] = true
		paths[s.Path] = true
	}
	if !ids["default"] {
		return errors.New("default source missing")
	}
	labels := map[string]bool{}
	for _, l := range p.Labels {
		key := l.SourceID + "\x00" + l.Context
		if !ids[l.SourceID] || l.Context == "" || utf8.RuneCountInString(l.Value) > 30 || labels[key] || len(l.Identity) != 64 {
			return errors.New("invalid label")
		}
		labels[key] = true
	}
	return nil
}

// Scan each JSON container before typed decoding to reject duplicate keys and nulls.
func scan(d *json.Decoder) error {
	token, e := d.Token()
	if e != nil {
		return e
	}
	if token == nil {
		return errors.New("null setting")
	}
	if delim, ok := token.(json.Delim); ok {
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return errors.New("duplicate setting")
				}
				seen[key] = true
				if e = scan(d); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = scan(d); e != nil {
					return e
				}
			}
		default:
			return errors.New("invalid JSON container")
		}
		_, e = d.Token()
		return e
	}
	return nil
}
func load(path string) (settings, error) {
	f, e := os.Open(path)
	if errors.Is(e, os.ErrNotExist) {
		return defaults(), nil
	}
	if e != nil {
		return settings{}, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, 65537))
	if e != nil {
		return settings{}, e
	}
	if len(data) > 65536 {
		return settings{}, errors.New("source settings exceed 64 KiB")
	}
	if e = scan(json.NewDecoder(bytes.NewReader(data))); e != nil {
		return settings{}, e
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var p settings
	if e = d.Decode(&p); e != nil {
		return p, e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return p, errors.New("trailing settings")
	}
	return p, validate(p)
}
func save(path string, p settings) error {
	if e := validate(p); e != nil {
		return e
	}
	data, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	if len(data) > 65536 {
		return errors.New("source settings exceed 64 KiB")
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".sources-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, e = f.Write(data); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
