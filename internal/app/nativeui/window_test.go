package nativeui

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeWeb struct {
	mu     sync.Mutex
	opened []string
	closed int
}

func (w *fakeWeb) Available() bool { return true }
func (w *fakeWeb) Open(target string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.opened = append(w.opened, target)
	return nil
}
func (w *fakeWeb) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed++
}

func TestLauncherPagesAreNative(t *testing.T) {
	for target, launcher := range map[string]bool{
		"http://127.0.0.1:51234/client":                               true,
		"http://127.0.0.1:51234/client#piik-client=1":                 true,
		"http://127.0.0.1:8787/?piik-client-port=39721#piik-client=1": false,
		"https://dec-load.trycloudflare.com/r/6049#v=abc":             false,
	} {
		if got := isLauncherPage(target); got != launcher {
			t.Errorf("%s: launcher page = %v", target, got)
		}
	}
}

func TestRoomPagesOpenInTheWebWindow(t *testing.T) {
	web := &fakeWeb{}
	manager := NewManager(web, func() { t.Error("opening a room must not quit the App") })
	if err := manager.Open("http://127.0.0.1:8787/?piik-client-port=39721#piik-client=1"); err != nil {
		t.Fatal(err)
	}
	if len(web.opened) != 1 {
		t.Fatalf("web window opened %v", web.opened)
	}
}

func TestOpenPiikHandsTheRoomToTheManager(t *testing.T) {
	backend := &fakeLauncher{state: LauncherState{Modes: []string{"link"}}}
	l := newLauncher(backend, func() {})
	targets := make(chan string, 1)
	l.onTarget = func(target string) { targets <- target }
	deadline := time.Now().Add(2 * time.Second)
	for !l.loaded && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	l.launch(&UI{Lang: "en"})
	select {
	case target := <-targets:
		if target != "http://127.0.0.1:8787/" {
			t.Fatalf("target %q", target)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Open Piik did not hand over the room")
	}
	_ = context.Background()
}
