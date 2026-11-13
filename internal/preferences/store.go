/*
Purpose: Store non-sensitive appearance and window preferences.
Used by: Desktop PreferencesService.
Responsibilities: Validate a strict versioned file, save atomically and clamp geometry.
Important: No Wails, credentials, resource identities or session state belong here.
*/
package preferences

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}
type Preferences struct {
	Version             int    `json:"version"`
	Appearance          string `json:"appearance"`
	Window              *Rect  `json:"window"`
	NavigationWidth     int    `json:"navigationWidth"`
	AIWidth             int    `json:"aiWidth"`
	DockHeight          int    `json:"dockHeight"`
	DetailWidthPercent  int    `json:"detailWidthPercent"`
	DetailHeightPercent int    `json:"detailHeightPercent"`
}

func Default() Preferences {
	return Preferences{
		Version: 2, Appearance: "system", NavigationWidth: 220, AIWidth: 360, DockHeight: 220,
		DetailWidthPercent: 50, DetailHeightPercent: 50,
	}
}
func Validate(p Preferences) error {
	if p.Version != 2 {
		return errors.New("unsupported preferences version")
	}
	if p.Appearance != "system" && p.Appearance != "light" && p.Appearance != "dark" {
		return errors.New("invalid appearance")
	}
	if p.NavigationWidth < 160 || p.NavigationWidth > 320 || p.AIWidth < 300 || p.AIWidth > 560 || p.DockHeight < 120 || p.DockHeight > 480 {
		return errors.New("invalid pane dimensions")
	}
	if p.DetailWidthPercent < 20 || p.DetailWidthPercent > 80 || p.DetailHeightPercent < 20 || p.DetailHeightPercent > 80 {
		return errors.New("invalid detail split dimensions")
	}
	if r := p.Window; r != nil && (r.Width < 1 || r.Width > 32768 || r.Height < 1 || r.Height > 32768 || r.X < -100000 || r.X > 100000 || r.Y < -100000 || r.Y > 100000) {
		return errors.New("invalid window bounds")
	}
	return nil
}
func Load(path string) (Preferences, error) {
	fallback := Default()
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return fallback, err
	}
	if len(data) > 65536 {
		return fallback, errors.New("preferences exceed 64 KiB")
	}
	// Reject duplicate and null scalar fields before typed decoding.
	tokens := json.NewDecoder(bytes.NewReader(data))
	if err := validateObject(tokens); err != nil {
		return fallback, err
	}
	// Decode into zero values so missing or null required fields fail validation.
	var p Preferences
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&p); err != nil {
		return fallback, fmt.Errorf("invalid preferences: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return fallback, errors.New("trailing preferences data")
	}
	if p.Version == 1 {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return fallback, err
		}
		if _, ok := fields["detailWidthPercent"]; ok {
			return fallback, errors.New("unexpected version-1 detail split")
		}
		if _, ok := fields["detailHeightPercent"]; ok {
			return fallback, errors.New("unexpected version-1 detail split")
		}
		p.Version = 2
		p.DetailWidthPercent, p.DetailHeightPercent = 50, 50
	}
	if err = Validate(p); err != nil {
		return fallback, err
	}
	return p, nil
}
func Save(path string, p Preferences) error {
	if err := Validate(p); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".preferences-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// WindowWorkArea chooses the saved display by overlap, falling back to primary.
// Returning the complete work area keeps launch independent of the saved window size.
func WindowWorkArea(saved Rect, areas []Rect) Rect {
	valid := make([]Rect, 0, len(areas))
	for _, a := range areas {
		if a.Width > 0 && a.Height > 0 {
			valid = append(valid, a)
		}
	}
	if len(valid) == 0 {
		valid = []Rect{{0, 0, 1440, 900}}
	}
	best := valid[0]
	overlap := 0
	for _, a := range valid {
		n := max(0, min(saved.X+saved.Width, a.X+a.Width)-max(saved.X, a.X)) * max(0, min(saved.Y+saved.Height, a.Y+a.Height)-max(saved.Y, a.Y))
		if n > overlap {
			best = a
			overlap = n
		}
	}
	return best
}

// ClampWindow keeps normal window bounds inside the selected display.
func ClampWindow(saved Rect, areas []Rect) Rect {
	best := WindowWorkArea(saved, areas)
	overlap := max(0, min(saved.X+saved.Width, best.X+best.Width)-max(saved.X, best.X)) * max(0, min(saved.Y+saved.Height, best.Y+best.Height)-max(saved.Y, best.Y))
	saved.Width = min(max(saved.Width, 1100), best.Width)
	saved.Height = min(max(saved.Height, 700), best.Height)
	if overlap == 0 {
		saved.X = best.X + (best.Width-saved.Width)/2
		saved.Y = best.Y + (best.Height-saved.Height)/2
	}
	saved.X = max(best.X, min(saved.X, best.X+best.Width-saved.Width))
	saved.Y = max(best.Y, min(saved.Y, best.Y+best.Height-saved.Height))
	return saved
}

func validateObject(d *json.Decoder) error {
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return errors.New("preferences must be an object")
	}
	seen := map[string]bool{}
	for d.More() {
		tok, err = d.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok || seen[key] {
			return errors.New("duplicate preferences field")
		}
		seen[key] = true
		var raw json.RawMessage
		if err = d.Decode(&raw); err != nil {
			return err
		}
		if bytes.Equal(raw, []byte("null")) && key != "window" {
			return errors.New("null preferences field")
		}
		if key == "window" && !bytes.Equal(raw, []byte("null")) {
			if err = validateObject(json.NewDecoder(bytes.NewReader(raw))); err != nil {
				return err
			}
		}
	}
	_, err = d.Token()
	return err
}
