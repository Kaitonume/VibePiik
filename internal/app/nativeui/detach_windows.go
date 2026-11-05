//go:build windows

package nativeui

import "unsafe"

// SWP_ASYNCWINDOWPOS
const swpAsync = 0x4000

var procGetWindowRectDet = user32ov.NewProc("GetWindowRect")

// placeBeside moves window to the inner right edge of owner's rectangle, so a
// detached panel opens on the same screen as VibePiik.
func placeBeside(window, owner uintptr) {
	if owner == 0 {
		procSetWindowPos.Call(window, ^uintptr(0), 0, 0, 0, 0, swpNoActivate|swpNoSize|swpNoMove|swpAsync)
		return
	}
	var o, w winRect
	procGetWindowRectDet.Call(owner, uintptr(unsafe.Pointer(&o)))
	procGetWindowRectDet.Call(window, uintptr(unsafe.Pointer(&w)))
	width := w.Right - w.Left
	x, y := o.Right-width-24, o.Top+80
	// Asynchronous: the window's thread may be waiting for this frame.
	procSetWindowPos.Call(window, ^uintptr(0), uintptr(int(x)), uintptr(int(y)), 0, 0, swpNoActivate|swpNoSize|swpAsync) // HWND_TOPMOST
}
