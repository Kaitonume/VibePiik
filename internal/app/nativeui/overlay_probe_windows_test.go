//go:build windows

package nativeui

import (
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// TestOverlayWindowProbe (NATIVEUI_OVERLAY_PROBE=1) shows the chat over other
// apps for two seconds on the screen left of the main one (or the main one)
// and checks the window's styles, place and capture exclusion. The screen is
// never captured.
func TestOverlayWindowProbe(t *testing.T) {
	if os.Getenv("NATIVEUI_OVERLAY_PROBE") != "1" {
		t.Skip("NATIVEUI_OVERLAY_PROBE not set")
	}
	monitors := listMonitors()
	index := 0
	for i, m := range monitors {
		if m.bounds.Min.X < 0 {
			index = i
		}
	}
	t.Logf("screens: %d, using %d (%v)", len(monitors), index, monitors[index].bounds)
	o := &windowsChatOverlay{wake: make(chan struct{}, 1), stop: make(chan struct{})}
	go o.run()
	now := time.Now()
	o.Update(overlayState{Lang: "en", Look: defaultLook(), Desktop: desktopOverlay{On: true, Monitor: index, Corner: "top-right", Seconds: 10},
		Lines: []overlayLine{{ID: "probe", PeerID: "vibepiik-test", Name: "VibePiik", Text: "Overlay probe (closes by itself)", Until: now.Add(10 * time.Second)}}})
	time.Sleep(1500 * time.Millisecond)
	user32 := syscall.NewLazyDLL("user32.dll")
	hwnd, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(overlayClassName)), 0)
	if hwnd == 0 {
		o.Close()
		t.Fatal("overlay window not found")
	}
	visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
	exStyle, _, _ := user32.NewProc("GetWindowLongPtrW").Call(hwnd, ^uintptr(19)) // GWL_EXSTYLE = -20
	var affinity uint32
	user32.NewProc("GetWindowDisplayAffinity").Call(hwnd, uintptr(unsafe.Pointer(&affinity)))
	var rect winRect
	user32.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	foreground, _, _ := user32.NewProc("GetForegroundWindow").Call()
	time.Sleep(500 * time.Millisecond)
	o.Close()
	time.Sleep(500 * time.Millisecond)
	gone, _, _ := user32.NewProc("IsWindow").Call(hwnd)
	t.Logf("visible=%d exStyle=%#x affinity=%#x rect=%v foreground-is-overlay=%v destroyed=%v", visible, exStyle, affinity, rect, foreground == hwnd, gone == 0)
	want := uintptr(wsExLayered | wsExTransparent | wsExToolWindow | wsExNoActivate | wsExTopmost)
	if visible == 0 || exStyle&want != want || affinity != wdaExcludeFromCapure || foreground == hwnd || gone != 0 {
		t.Fatal("overlay window is not as expected")
	}
	b := monitors[index].bounds
	if int(rect.Left) < b.Min.X || int(rect.Right) > b.Max.X || int(rect.Top) < b.Min.Y || int(rect.Bottom) > b.Max.Y {
		t.Fatalf("overlay outside its screen: %v vs %v", rect, b)
	}
}
