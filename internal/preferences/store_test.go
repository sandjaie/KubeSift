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

func TestLaunchWorkArea(t *testing.T) {
	primary := Rect{0, 25, 1512, 900}
	secondary := Rect{-1920, 40, 1920, 1040}
	for _, tc := range []struct {
		name  string
		saved Rect
		areas []Rect
		want  Rect
	}{
		{"small saved window fills primary", Rect{0, 25, 1440, 800}, []Rect{primary}, primary},
		{"remember secondary display", Rect{-1700, 60, 1200, 700}, []Rect{primary, secondary}, secondary},
		{"removed display falls back to primary", Rect{9000, 9000, 1200, 700}, []Rect{primary}, primary},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := WindowWorkArea(tc.saved, tc.areas); got != tc.want {
				t.Fatalf("launch bounds = %+v, want entire work area %+v", got, tc.want)
			}
		})
	}
}

func TestLaunchWindowBounds(t *testing.T) {
	laptop := Rect{0, 38, 1512, 906}
	external := Rect{-2560, 24, 2560, 1380}
	portrait := Rect{1512, -600, 1440, 2200}
	for _, tc := range []struct {
		name    string
		saved   Rect
		screens []Rect
		want    Rect
	}{
		{"compact laptop fills its work area", Rect{}, []Rect{laptop}, laptop},
		{"wide display opens centered", Rect{-2300, 150, 1400, 800}, []Rect{laptop, external}, Rect{-2080, 204, 1600, 1020}},
		{"tall display is also capped", Rect{1700, -200, 1200, 800}, []Rect{laptop, portrait}, Rect{1512, -10, 1440, 1020}},
		{"unplugged display returns to laptop", Rect{-2300, 150, 1400, 800}, []Rect{laptop}, laptop},
		{"smaller available area always wins", Rect{}, []Rect{{50, 40, 930, 550}}, Rect{50, 40, 930, 550}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := LaunchWindowBounds(tc.saved, tc.screens); got != tc.want {
				t.Fatalf("launch bounds = %+v, want %+v", got, tc.want)
			}
		})
	}
}
