/*
Purpose: Protect preferences persistence and on-screen geometry.
Used by: Go unit checks.
Responsibilities: Exercise strict decoding, rejected writes and display fallback.
Important: Only temporary files and synthetic rectangles are used.
*/
package preferences

import (
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
