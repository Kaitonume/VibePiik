package app

import (
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type fakeWindow struct {
	mu        sync.Mutex
	available bool
	opened    []string
	closed    int
}

func (w *fakeWindow) Available() bool { return w.available }
func (w *fakeWindow) Open(target string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.opened = append(w.opened, target)
	return nil
}
func (w *fakeWindow) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed++
}
func (w *fakeWindow) state() ([]string, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.opened...), w.closed
}

func waitOpening(t *testing.T, console *console) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		console.mu.Lock()
		opening := console.opening
		console.mu.Unlock()
		if !opening {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("page handoff did not finish")
}

func TestConsoleOpensPagesInTheVibePiikWindow(t *testing.T) {
	window := &fakeWindow{available: true}
	console := newConsole(func() {}, true, consoleWindow{opener: window, prefer: true})
	browsed := make(chan string, 4)
	console.openURL = func(target string) error { browsed <- target; return nil }

	console.openBrowser("http://127.0.0.1:1/client")
	waitOpening(t, console)
	if opened, _ := window.state(); len(opened) != 1 || len(browsed) != 0 {
		t.Fatalf("automatic page: window %v, browser %d", opened, len(browsed))
	}

	// [o] closes the window and moves the page to the browser; the App keeps running.
	console.openInBrowser("http://127.0.0.1:8787/")
	waitOpening(t, console)
	if _, closed := window.state(); closed != 1 || len(browsed) != 1 {
		t.Fatalf("[o]: window closed %d times, browser %d", closed, len(browsed))
	}
	// Later automatic pages follow that choice.
	console.openBrowser("http://127.0.0.1:8787/")
	waitOpening(t, console)
	if opened, _ := window.state(); len(opened) != 1 || len(browsed) != 2 {
		t.Fatalf("after [o]: window %v, browser %d", opened, len(browsed))
	}

	// [w] brings the page back to the window.
	console.openInWindow("http://127.0.0.1:8787/")
	waitOpening(t, console)
	console.openBrowser("http://127.0.0.1:8787/")
	waitOpening(t, console)
	if opened, _ := window.state(); len(opened) != 3 || len(browsed) != 2 {
		t.Fatalf("after [w]: window %v, browser %d", opened, len(browsed))
	}
}

func TestConsoleWithoutWindowKeepsTheBrowser(t *testing.T) {
	window := &fakeWindow{available: false}
	console := newConsole(func() {}, true, consoleWindow{opener: window, prefer: true})
	browsed := make(chan string, 2)
	console.openURL = func(target string) error { browsed <- target; return nil }
	console.openBrowser("http://127.0.0.1:1/client")
	waitOpening(t, console)
	if opened, _ := window.state(); len(opened) != 0 || len(browsed) != 1 || console.plain.windowKey {
		t.Fatal("an unavailable window must leave the browser and hide [w]")
	}
}

func TestConsoleWindowKey(t *testing.T) {
	var opened string
	model := consoleModel{width: 80, language: "en", windowKey: true,
		view:       consoleView{state: "ready", entry: "http://127.0.0.1:8787/"},
		openWindow: func(target string) { opened = target }}
	content := model.content(true)
	for _, want := range []string{"VibePiik App", "[w]", "Open in window", "[o]", "Open in browser"} {
		if !strings.Contains(content, want) {
			t.Fatalf("console lacks %q:\n%s", want, content)
		}
	}
	_, command := model.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	if command == nil {
		t.Fatal("[w] did not open the window")
	}
	command()
	if opened != "http://127.0.0.1:8787/" {
		t.Fatalf("[w] opened %q", opened)
	}
	model.windowKey = false
	if strings.Contains(model.content(true), "[w]") {
		t.Fatal("[w] shown without a window")
	}
}
