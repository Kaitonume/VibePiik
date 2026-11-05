package appwindow

import (
	"path/filepath"
	"testing"
)

func TestParseMessage(t *testing.T) {
	for _, test := range []struct {
		raw   string
		ok    bool
		open  string
		title string
	}{
		{raw: "vibepiik:open:https://github.com/TNTcraftHIM/Piik", ok: true, open: "https://github.com/TNTcraftHIM/Piik"},
		{raw: "vibepiik:open:https://x.trycloudflare.com/r/6049#v=abc", ok: true, open: "https://x.trycloudflare.com/r/6049#v=abc"},
		{raw: "vibepiik:open:file:///C:/Windows/notepad.exe"},
		{raw: "vibepiik:open:javascript:alert(1)"},
		{raw: "vibepiik:open:ms-settings:display"},
		{raw: "vibepiik:open:https://"},
		{raw: "vibepiik:title:Piik | 6049 · Sharing", ok: true, title: "VibePiik | 6049 · Sharing"},
		{raw: "vibepiik:title:", ok: true, title: "VibePiik"},
		{raw: "vibepiik:run:calc"},
		{raw: "open:https://example.com"},
	} {
		value, ok := parseMessage(test.raw)
		if ok != test.ok || value.open != test.open || value.title != test.title {
			t.Errorf("%q: got %+v %v", test.raw, value, ok)
		}
	}
}

func TestWindowTitle(t *testing.T) {
	if got := windowTitle("Piik"); got != "VibePiik" {
		t.Fatal(got)
	}
	if got := windowTitle("Other page"); got != "VibePiik | Other page" {
		t.Fatal(got)
	}
}

func TestPlacementRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "VibePiik", "window.json")
	if _, ok := loadPlacement(path); ok {
		t.Fatal("missing file must not restore a placement")
	}
	want := placement{Left: -2400, Top: 100, Right: -1200, Bottom: 980, Maximized: true}
	savePlacement(path, want)
	if got, ok := loadPlacement(path); !ok || got != want {
		t.Fatalf("got %+v %v", got, ok)
	}
	savePlacement(path, placement{Left: 0, Top: 0, Right: 10, Bottom: 10})
	if got, _ := loadPlacement(path); got != want {
		t.Fatal("a degenerate rectangle must not replace the saved placement")
	}
}
