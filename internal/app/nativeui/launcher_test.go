package nativeui

import (
	"context"
	"image"
	"testing"
	"time"

	"gioui.org/layout"
)

type fakeLauncher struct{ state LauncherState }

func (f *fakeLauncher) State(context.Context) (LauncherState, error) { return f.state, nil }
func (f *fakeLauncher) Launch(context.Context, string, string, string, string, bool) (string, error) {
	return "http://127.0.0.1:8787/", nil
}
func (f *fakeLauncher) SendTV(context.Context, string) ([]string, int, error) {
	return []string{"TCL 43T7B"}, 200, nil
}
func (f *fakeLauncher) EnableDebug(context.Context) error            { return nil }
func (f *fakeLauncher) SetSite(context.Context, string, bool) error  { return nil }
func (f *fakeLauncher) SetStartupSite(context.Context, string) error { return nil }

func readyLauncher(t *testing.T) *Launcher {
	t.Helper()
	backend := &fakeLauncher{state: LauncherState{Version: "v2.5.3", Modes: []string{"local", "link"}, DefaultMode: "link",
		PiikTvLanguage: "it", Debug: new(bool)}}
	l := newLauncher(backend, func() {})
	l.welcome = 0
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		loaded := l.loaded
		l.mu.Unlock()
		if loaded {
			return l
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("launcher state not loaded")
	return nil
}

func TestRenderLauncher(t *testing.T) {
	shaper := newShaper(false)
	for _, palette := range []Palette{LightPalette(), DarkPalette()} {
		theme := "light"
		if palette.Dark {
			theme = "dark"
		}
		l := readyLauncher(t)
		u := &UI{P: palette, Shaper: shaper, Lang: "en"}
		renderPNG(t, "launcher-"+theme, image.Pt(1200, 880), func(gtx layout.Context) {
			gtx.Now = time.Now().Add(time.Hour)
			l.Layout(gtx, u)
		})
	}
}
