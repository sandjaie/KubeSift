/*
Purpose: Protect preferences persistence and on-screen geometry.
Used by: Go unit checks.
Responsibilities: Exercise strict decoding, rejected writes and display fallback.
Important: Only temporary files and synthetic rectangles are used.
*/
package preferences

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStrictRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	p := Default()
	p.Appearance = "dark"
	p.Window = &Rect{-100, 20, 1100, 700}
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Appearance != "dark" || *got.Window != *p.Window {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	before, _ := os.ReadFile(path)
	p.Appearance = "invalid"
	if Save(path, p) == nil {
		t.Fatal("invalid save accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("rejected save overwrote file")
	}
	for _, s := range []string{`{"version":2}`, `{"version":1,"appearance":"other"}`, `{"version":1,"unknown":true}`, string(before) + ` {}`, string(before[:len(before)-1]) + `,"appearance":null}`, string(before[:len(before)-1]) + `,"aiWidth":999999}`} {
		if err := os.WriteFile(path, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = Load(path); err == nil {
			t.Fatalf("accepted invalid settings %s", s)
		}
	}
}
func TestClampWindow(t *testing.T) {
	for _, area := range []Rect{{0, 24, 1440, 876}, {-800, -20, 800, 500}} {
		got := ClampWindow(Rect{9999, 9999, 1920, 1080}, []Rect{area})
		if got.Width <= 0 || got.Height <= 0 || got.X < area.X || got.Y < area.Y || got.X+got.Width > area.X+area.Width || got.Y+got.Height > area.Y+area.Height {
			t.Fatalf("out of bounds: %+v in %+v", got, area)
		}
	}
}

func TestDetailSplitMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	legacy := `{"version":1,"appearance":"dark","window":null,"navigationWidth":240,"aiWidth":400,"dockHeight":180}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil || p.Version != 2 || p.Appearance != "dark" || p.NavigationWidth != 240 || p.AIWidth != 400 || p.DockHeight != 180 {
		t.Fatalf("legacy preferences not preserved/migrated: %+v %v", p, err)
	}
	before, _ := os.ReadFile(path)
	if string(before) != legacy {
		t.Fatal("load rewrote legacy preferences")
	}
	// Inspect the persisted contract rather than depending on generated field names.
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["detailWidthPercent"] != float64(50) || fields["detailHeightPercent"] != float64(50) {
		t.Fatal("missing default split shares")
	}
	fields["detailWidthPercent"], fields["detailHeightPercent"] = 35, 65
	write := func() {
		t.Helper()
		data, _ := json.Marshal(fields)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	p, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = Save(path, p); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	json.Unmarshal(data, &fields)
	if fields["detailWidthPercent"] != float64(35) || fields["detailHeightPercent"] != float64(65) {
		t.Fatal("split shares were not retained")
	}
	for _, value := range []any{nil, 0, 19, 81, 40.5} {
		fields["detailWidthPercent"] = value
		write()
		if _, err = Load(path); err == nil {
			t.Fatalf("accepted split %v", value)
		}
	}
	delete(fields, "detailWidthPercent")
	write()
	if _, err = Load(path); err == nil {
		t.Fatal("accepted missing v2 split")
	}
}
