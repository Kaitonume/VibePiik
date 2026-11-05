//go:build windows

package nativeui

import (
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/TNTcraftHIM/Piik/internal/app/nativecapture"
)

// TestPipProbe (NATIVEUI_PIP_PROBE=<window title>) opens picture in picture of
// that window for two seconds and checks the window, without capturing the screen.
func TestPipProbe(t *testing.T) {
	title := os.Getenv("NATIVEUI_PIP_PROBE")
	if title == "" {
		t.Skip("NATIVEUI_PIP_PROBE not set")
	}
	user32 := syscall.NewLazyDLL("user32.dll")
	source, _, _ := user32.NewProc("FindWindowW").Call(0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))))
	if source == 0 {
		t.Fatal("source window not found")
	}
	// Keep the probe on the screen left of the main one (the tester's screen).
	for i, m := range listMonitors() {
		if m.bounds.Min.X < 0 {
			pipScreenForTests = i
		}
	}
	closeFn, done, ok := openPictureInPicture(nativecapture.CaptureTarget{Kind: "window", SourceID: itoa(int(source)), Title: title}, "VibePiik probe")
	if !ok {
		t.Fatal("picture in picture did not open")
	}
	time.Sleep(1500 * time.Millisecond)
	pip, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(pipClassName)), 0)
	var rect winRect
	user32.NewProc("GetWindowRect").Call(pip, uintptr(unsafe.Pointer(&rect)))
	ex, _, _ := user32.NewProc("GetWindowLongPtrW").Call(pip, ^uintptr(19))
	var affinity uint32
	user32.NewProc("GetWindowDisplayAffinity").Call(pip, uintptr(unsafe.Pointer(&affinity)))
	closeFn()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("did not close")
	}
	t.Logf("rect=%v exStyle=%#x affinity=%#x", rect, ex, affinity)
	if pip == 0 || ex&wsExTopmost == 0 || affinity != wdaExcludeFromCapure {
		t.Fatal("picture in picture window is not as expected")
	}
}
