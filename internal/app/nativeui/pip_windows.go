//go:build windows

package nativeui

import (
	"image"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"github.com/TNTcraftHIM/Piik/internal/app/nativecapture"
)

// Picture in picture (playback.pip): a small always-on-top window with the
// live DWM thumbnail of the shared window. It moves by dragging anywhere,
// resizes from its edges, closes on a double click, and stays out of every
// capture. Nothing is captured or encoded for it: Windows draws it.

var (
	procGetClientRect       = user32ov.NewProc("GetClientRect")
	procGetForegroundWindow = user32ov.NewProc("GetForegroundWindow")
	procGetWindowRectPip    = user32ov.NewProc("GetWindowRect")
	procPostMessageW        = user32ov.NewProc("PostMessageW")
	procGetMessageW         = user32ov.NewProc("GetMessageW")
	pipClassOnce            sync.Once
	pipClassName            = syscall.StringToUTF16Ptr("VibePiikPictureInPicture")
	pipWindows              sync.Map // hwnd → *pipWindow
)

const (
	wsThickFrame   = 0x00040000
	wsVisible      = 0x10000000
	wmClose        = 0x0010
	wmDestroy      = 0x0002
	wmSize         = 0x0005
	wmNCHitTest    = 0x0084
	wmNCLButtonDbl = 0x00A3
	htCaption      = 2
	htClient       = 1
)

// pipScreenForTests pins the window to a screen (tests only; -1 = normal).
var pipScreenForTests = -1

type pipWindow struct {
	hwnd   uintptr
	thumb  uintptr
	source uintptr
	closed chan struct{}
}

func registerPipClass() {
	pipClassOnce.Do(func() {
		class := wndClassEx{WndProc: syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
			switch msg {
			case wmNCHitTest:
				hit, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
				if hit == htClient {
					return htCaption // drag from anywhere
				}
				return hit
			case wmNCLButtonDbl:
				procPostMessageW.Call(hwnd, wmClose, 0, 0)
				return 0
			case wmSize:
				if value, ok := pipWindows.Load(hwnd); ok {
					value.(*pipWindow).fit()
				}
			case wmDestroy:
				if value, ok := pipWindows.Load(hwnd); ok {
					w := value.(*pipWindow)
					if w.thumb != 0 {
						procDwmUnregisterThumbnail.Call(w.thumb)
					}
					pipWindows.Delete(hwnd)
					close(w.closed)
				}
			}
			result, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
			return result
		}), ClassName: pipClassName, Background: 1 + 1} // COLOR_BACKGROUND+1 → dark
		class.Size = uint32(unsafe.Sizeof(class))
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	})
}

// fit sizes the thumbnail to the client area, keeping the source's shape.
func (w *pipWindow) fit() {
	var client winRect
	procGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	var size struct{ CX, CY int32 }
	if hr, _, _ := procDwmQueryThumbnailSrcSize.Call(w.thumb, uintptr(unsafe.Pointer(&size))); hr != 0 || size.CX <= 0 || size.CY <= 0 {
		return
	}
	cw, ch := float64(client.Right), float64(client.Bottom)
	scale := min(cw/float64(size.CX), ch/float64(size.CY))
	dw, dh := int32(float64(size.CX)*scale), int32(float64(size.CY)*scale)
	x, y := (int32(cw)-dw)/2, (int32(ch)-dh)/2
	props := dwmThumbnailProperties{Flags: dwmTNPRectDestination | dwmTNPOpacity | dwmTNPVisible,
		Destination: winRect{x, y, x + dw, y + dh}, Opacity: 255, Visible: 1}
	procDwmUpdateThumbnailProps.Call(w.thumb, uintptr(unsafe.Pointer(&props)))
}

// openPictureInPicture shows target live in a small topmost window; it
// reports false when the source cannot be shown (screens, closed windows).
func openPictureInPicture(target nativecapture.CaptureTarget, title string) (close func(), done <-chan struct{}, ok bool) {
	if target.Kind != "window" || dwmapi.Load() != nil {
		return nil, nil, false
	}
	value, err := strconv.ParseUint(target.SourceID, 10, 64)
	if err != nil || value == 0 {
		return nil, nil, false
	}
	w := &pipWindow{source: uintptr(value), closed: make(chan struct{})}
	ready := make(chan bool, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		registerPipClass()
		// On the screen of the window that asked for it (VibePiik, in the foreground).
		area, _ := monitorArea(0)
		if pipScreenForTests >= 0 {
			area, _ = monitorArea(pipScreenForTests)
		} else if fg, _, _ := procGetForegroundWindow.Call(); fg != 0 {
			var rect winRect
			procGetWindowRectPip.Call(fg, uintptr(unsafe.Pointer(&rect)))
			centre := image.Pt(int(rect.Left+rect.Right)/2, int(rect.Top+rect.Bottom)/2)
			for _, m := range listMonitors() {
				if centre.In(m.bounds) {
					area = m
				}
			}
		}
		width, height := 480, 270
		x, y := area.work.Max.X-width-24, area.work.Max.Y-height-24
		hwnd, _, _ := procCreateWindowExW.Call(wsExTopmost|wsExToolWindow, uintptr(unsafe.Pointer(pipClassName)),
			uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))), wsPopup|wsThickFrame|wsVisible,
			uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, 0, 0)
		if hwnd == 0 {
			ready <- false
			return
		}
		procSetWindowDisplayAffinity.Call(hwnd, wdaExcludeFromCapure)
		var thumb uintptr
		if hr, _, _ := procDwmRegisterThumbnail.Call(hwnd, w.source, uintptr(unsafe.Pointer(&thumb))); hr != 0 {
			procDestroyWindow.Call(hwnd)
			ready <- false
			return
		}
		w.hwnd, w.thumb = hwnd, thumb
		pipWindows.Store(hwnd, w)
		w.fit()
		ready <- true
		var msg winMsg
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
			select {
			case <-w.closed:
				return
			default:
			}
		}
	}()
	if !<-ready {
		return nil, nil, false
	}
	return func() { procPostMessageW.Call(w.hwnd, wmClose, 0, 0) }, w.closed, true
}
