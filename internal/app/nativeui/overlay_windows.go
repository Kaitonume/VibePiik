//go:build windows

package nativeui

import (
	"image"
	"log/slog"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// Chat windows drawn above other windows. Each is a plain top-level layered
// window of this process: transparent, click-through (WS_EX_TRANSPARENT),
// never activated, hidden from the taskbar and excluded from screen capture.
// Nothing is injected into, hooked in or read from other processes, so games
// and their anti-cheat see an ordinary window of another app on top, like a
// notification. Exclusive full screen hides it: borderless full screen shows it.

var (
	user32ov                     = syscall.NewLazyDLL("user32.dll")
	gdi32ov                      = syscall.NewLazyDLL("gdi32.dll")
	procRegisterClassExW         = user32ov.NewProc("RegisterClassExW")
	procCreateWindowExW          = user32ov.NewProc("CreateWindowExW")
	procDefWindowProcW           = user32ov.NewProc("DefWindowProcW")
	procDestroyWindow            = user32ov.NewProc("DestroyWindow")
	procPeekMessageW             = user32ov.NewProc("PeekMessageW")
	procTranslateMessage         = user32ov.NewProc("TranslateMessage")
	procDispatchMessageW         = user32ov.NewProc("DispatchMessageW")
	procShowWindow               = user32ov.NewProc("ShowWindow")
	procUpdateLayeredWindow      = user32ov.NewProc("UpdateLayeredWindow")
	procSetWindowDisplayAffinity = user32ov.NewProc("SetWindowDisplayAffinity")
	procClientToScreen           = user32ov.NewProc("ClientToScreen")
	procIsIconic                 = user32ov.NewProc("IsIconic")
	procIsWindowVisibleOv        = user32ov.NewProc("IsWindowVisible")
	procGetDpiForWindow          = user32ov.NewProc("GetDpiForWindow")
	procSetWindowPos             = user32ov.NewProc("SetWindowPos")
	procEnumDisplayMonitors      = user32ov.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW          = user32ov.NewProc("GetMonitorInfoW")
	procGetDCov                  = user32ov.NewProc("GetDC")
	procReleaseDCov              = user32ov.NewProc("ReleaseDC")
	procCreateCompatibleDC       = gdi32ov.NewProc("CreateCompatibleDC")
	procCreateDIBSection         = gdi32ov.NewProc("CreateDIBSection")
	procSelectObject             = gdi32ov.NewProc("SelectObject")
	procDeleteObject             = gdi32ov.NewProc("DeleteObject")
	procDeleteDC                 = gdi32ov.NewProc("DeleteDC")
)

const (
	wsPopup              = 0x80000000
	wsExLayered          = 0x00080000
	wsExTransparent      = 0x00000020
	wsExToolWindow       = 0x00000080
	wsExNoActivate       = 0x08000000
	wsExTopmost          = 0x00000008
	swHide               = 0
	swShowNoActivate     = 4
	ulwAlpha             = 2
	wdaExcludeFromCapure = 0x11
	pmRemove             = 1
	swpNoActivate        = 0x0010
	swpNoMove            = 0x0002
	swpNoSize            = 0x0001
	monitorInfoPrimary   = 1
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	Private uint32
}

type winPoint struct{ X, Y int32 }
type winSize struct{ CX, CY int32 }

type monitorInfoEx struct {
	Size    uint32
	Monitor winRect
	Work    winRect
	Flags   uint32
	Device  [32]uint16
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type blendFunction struct {
	Op, Flags, Alpha, Format byte
}

var (
	overlayClassOnce sync.Once
	overlayClassName = syscall.StringToUTF16Ptr("VibePiikChatOverlay")
)

func registerOverlayClass() {
	overlayClassOnce.Do(func() {
		class := wndClassEx{WndProc: syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
			const wmNCHitTest = 0x0084
			if msg == wmNCHitTest {
				return ^uintptr(0) // HTTRANSPARENT: never takes the mouse
			}
			result, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
			return result
		}), ClassName: overlayClassName}
		class.Size = uint32(unsafe.Sizeof(class))
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	})
}

// layeredWindow is one overlay surface; used only on its overlay thread.
type layeredWindow struct {
	hwnd     uintptr
	shown    bool
	gpu      *headless.Window
	gpuSize  image.Point
	dc       uintptr
	bitmap   uintptr
	bits     unsafe.Pointer
	bmSize   image.Point
	img      *image.RGBA
	hiddenAt time.Time
}

func newLayeredWindow(owner uintptr, topmost bool) *layeredWindow {
	registerOverlayClass()
	style := uintptr(wsExLayered | wsExTransparent | wsExToolWindow | wsExNoActivate)
	if topmost {
		style |= wsExTopmost
	}
	hwnd, _, _ := procCreateWindowExW.Call(style, uintptr(unsafe.Pointer(overlayClassName)), 0, wsPopup,
		0, 0, 1, 1, owner, 0, 0, 0)
	if hwnd == 0 {
		return nil
	}
	// Keep the chat out of every capture, our own shares included.
	procSetWindowDisplayAffinity.Call(hwnd, wdaExcludeFromCapure)
	return &layeredWindow{hwnd: hwnd}
}

func (w *layeredWindow) hide() {
	if w.shown {
		procShowWindow.Call(w.hwnd, swHide)
		w.shown = false
		w.hiddenAt = time.Now()
	}
	// Nothing on screen for a while: give back the GPU surface and bitmap.
	if w.gpu != nil && !w.hiddenAt.IsZero() && time.Since(w.hiddenAt) > 10*time.Second {
		w.gpu.Release()
		w.gpu, w.gpuSize, w.img = nil, image.Point{}, nil
		w.freeBitmap()
		w.bmSize = image.Point{}
	}
}

func (w *layeredWindow) destroy() {
	w.hide()
	if w.gpu != nil {
		w.gpu.Release()
	}
	w.freeBitmap()
	procDestroyWindow.Call(w.hwnd)
}

func (w *layeredWindow) freeBitmap() {
	if w.dc != 0 {
		procDeleteDC.Call(w.dc)
		w.dc = 0
	}
	if w.bitmap != 0 {
		procDeleteObject.Call(w.bitmap)
		w.bitmap = 0
	}
}

// present renders draw at size and shows it with its top-left at screen pos.
func (w *layeredWindow) present(pos, size image.Point, dpi float32, draw func(gtx layout.Context)) bool {
	if size.X <= 0 || size.Y <= 0 {
		w.hide()
		return false
	}
	if w.gpu == nil || w.gpuSize != size {
		if w.gpu != nil {
			w.gpu.Release()
		}
		gpu, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			slog.Debug("vibepiik-native", "event", "chat-overlay-gpu-failed", "error", err)
			w.gpu = nil
			return false
		}
		w.gpu, w.gpuSize = gpu, size
		w.img = image.NewRGBA(image.Rectangle{Max: size})
	}
	if w.bmSize != size {
		w.freeBitmap()
		screen, _, _ := procGetDCov.Call(0)
		w.dc, _, _ = procCreateCompatibleDC.Call(screen)
		header := bitmapInfoHeader{Width: int32(size.X), Height: -int32(size.Y), Planes: 1, BitCount: 32}
		header.Size = uint32(unsafe.Sizeof(header))
		var bits unsafe.Pointer
		w.bitmap, _, _ = procCreateDIBSection.Call(screen, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
		procReleaseDCov.Call(0, screen)
		if w.bitmap == 0 || w.dc == 0 {
			w.freeBitmap()
			return false
		}
		procSelectObject.Call(w.dc, w.bitmap)
		w.bits, w.bmSize = bits, size
	}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: dpi, PxPerSp: dpi},
		Constraints: layout.Exact(size), Source: input.Source{}}
	draw(gtx)
	if err := w.gpu.Frame(&ops); err != nil {
		return false
	}
	if err := w.gpu.Screenshot(w.img); err != nil {
		return false
	}
	// RGBA (premultiplied) → BGRA for the DIB.
	dst := unsafe.Slice((*byte)(w.bits), size.X*size.Y*4)
	src := w.img.Pix
	for i := 0; i+3 < len(src) && i+3 < len(dst); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i+2], src[i+1], src[i], src[i+3]
	}
	point := winPoint{int32(pos.X), int32(pos.Y)}
	extent := winSize{int32(size.X), int32(size.Y)}
	origin := winPoint{}
	blend := blendFunction{Alpha: 255, Format: 1} // AC_SRC_OVER, AC_SRC_ALPHA
	procUpdateLayeredWindow.Call(w.hwnd, 0, uintptr(unsafe.Pointer(&point)), uintptr(unsafe.Pointer(&extent)), w.dc,
		uintptr(unsafe.Pointer(&origin)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	if !w.shown {
		procShowWindow.Call(w.hwnd, swShowNoActivate)
		w.shown = true
	}
	return true
}

// windowsChatOverlay runs both overlay windows on one locked OS thread.
type windowsChatOverlay struct {
	owner uintptr
	mu    sync.Mutex
	state overlayState
	wake  chan struct{}
	stop  chan struct{}
	lang  string
}

// newChatOverlay starts the overlay thread for the window owner.
func newChatOverlay(owner uintptr) chatOverlay {
	if owner == 0 {
		return nil
	}
	o := &windowsChatOverlay{owner: owner, wake: make(chan struct{}, 1), stop: make(chan struct{})}
	go o.run()
	return o
}

func (o *windowsChatOverlay) Update(state overlayState) {
	o.mu.Lock()
	o.state = state
	o.mu.Unlock()
	select {
	case o.wake <- struct{}{}:
	default:
	}
}

func (o *windowsChatOverlay) Close() {
	select {
	case <-o.stop:
	default:
		close(o.stop)
	}
}

func (o *windowsChatOverlay) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var preview, desktop *layeredWindow
	defer func() {
		if preview != nil {
			preview.destroy()
		}
		if desktop != nil {
			desktop.destroy()
		}
	}()
	var shaper *UI
	lastDesktop := ""
	for {
		var msg winMsg
		for {
			if ok, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmRemove); ok == 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
		o.mu.Lock()
		state := o.state
		o.mu.Unlock()
		now := time.Now()
		if shaper == nil || shaper.Lang != state.Lang {
			shaper = &UI{P: DarkPalette(), Shaper: newShaper(false), Lang: state.Lang, Spoken: state.Spoken}
		}
		wait := 500 * time.Millisecond
		// Over the TV preview: follows the VibePiik window.
		active := activeFlights(state.Flights, now)
		ownerShown, _, _ := procIsWindowVisibleOv.Call(o.owner)
		iconic, _, _ := procIsIconic.Call(o.owner)
		if len(active) > 0 && !state.Preview.Empty() && ownerShown != 0 && iconic == 0 {
			if preview == nil {
				preview = newLayeredWindow(o.owner, false)
			}
			if preview != nil {
				origin := winPoint{int32(state.Preview.Min.X), int32(state.Preview.Min.Y)}
				procClientToScreen.Call(o.owner, uintptr(unsafe.Pointer(&origin)))
				dpi := windowDPI(o.owner)
				clipOffset := state.Clip.Min.Sub(state.Preview.Min)
				size := state.Clip.Size()
				pos := image.Pt(int(origin.X)+clipOffset.X, int(origin.Y)+clipOffset.Y)
				full := state.Preview.Size()
				preview.present(pos, size, dpi, func(gtx layout.Context) {
					defer op.Offset(image.Pt(-clipOffset.X, -clipOffset.Y)).Push(gtx.Ops).Pop()
					gtx.Constraints = layout.Exact(full)
					drawFlights(gtx, shaper, active, state.Look, now)
				})
				wait = 16 * time.Millisecond
			}
		} else if preview != nil {
			preview.hide()
		}
		// Over every app: redrawn only when its lines change.
		lines := activeLines(state.Lines, now)
		if state.Desktop.On && len(lines) > 0 {
			if desktop == nil {
				desktop = newLayeredWindow(0, true)
			}
			if desktop != nil {
				area, ok := monitorArea(state.Desktop.Monitor)
				if ok {
					key := desktopKey(lines, state)
					if key != lastDesktop || !desktop.shown {
						dpi := float32(area.dpi) / 96
						width := int(float32(420) * dpi * float32(state.Look.clamped().Scale))
						height := min(area.work.Dy()*6/10, int(520*dpi))
						margin := int(24 * dpi)
						pos := image.Pt(area.work.Min.X+margin, area.work.Min.Y+margin)
						if state.Desktop.Corner == "top-right" || state.Desktop.Corner == "bottom-right" {
							pos.X = area.work.Max.X - margin - width
						}
						if state.Desktop.Corner == "bottom-left" || state.Desktop.Corner == "bottom-right" {
							pos.Y = area.work.Max.Y - margin - height
						}
						if desktop.present(pos, image.Pt(width, height), dpi, func(gtx layout.Context) {
							drawChatBox(gtx, shaper, lines, state.Look, state.Desktop.Corner)
						}) {
							lastDesktop = key
							// Stay above windows that became topmost after us.
							procSetWindowPos.Call(desktop.hwnd, ^uintptr(0), 0, 0, 0, 0, swpNoActivate|swpNoMove|swpNoSize)
						}
					}
					if next := nextExpiry(lines, now); next > 0 && next < wait {
						wait = next
					}
				}
			}
		} else if desktop != nil {
			desktop.hide()
			lastDesktop = ""
		}
		select {
		case <-o.stop:
			return
		case <-o.wake:
		case <-time.After(wait):
		}
	}
}

func windowDPI(hwnd uintptr) float32 {
	if procGetDpiForWindow.Find() != nil {
		return 1
	}
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	if dpi == 0 {
		return 1
	}
	return float32(dpi) / 96
}

type monitorArea2 struct {
	work    image.Rectangle
	bounds  image.Rectangle
	primary bool
	dpi     int
}

var (
	monitorsMu  sync.Mutex
	monitorList []monitorArea2
	monitorCb   = syscall.NewCallback(func(monitor, dc, rect, data uintptr) uintptr {
		info := monitorInfoEx{}
		info.Size = uint32(unsafe.Sizeof(info))
		procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
		toRect := func(r winRect) image.Rectangle {
			return image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom))
		}
		dpi := 96
		if shcore := syscall.NewLazyDLL("shcore.dll"); shcore.Load() == nil {
			var x, y uint32
			if hr, _, _ := shcore.NewProc("GetDpiForMonitor").Call(monitor, 0, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y))); hr == 0 && x > 0 {
				dpi = int(x)
			}
		}
		monitorList = append(monitorList, monitorArea2{work: toRect(info.Work), bounds: toRect(info.Monitor),
			primary: info.Flags&monitorInfoPrimary != 0, dpi: dpi})
		return 1
	})
)

// listMonitors returns the screens, primary first, then left to right.
func listMonitors() []monitorArea2 {
	monitorsMu.Lock()
	defer monitorsMu.Unlock()
	monitorList = nil
	procEnumDisplayMonitors.Call(0, 0, monitorCb, 0)
	list := append([]monitorArea2(nil), monitorList...)
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && monitorBefore(list[j], list[j-1]); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
	return list
}

func monitorBefore(a, b monitorArea2) bool {
	if a.primary != b.primary {
		return a.primary
	}
	return a.bounds.Min.X < b.bounds.Min.X
}

func monitorArea(index int) (monitorArea2, bool) {
	list := listMonitors()
	if len(list) == 0 {
		return monitorArea2{}, false
	}
	if index < 0 || index >= len(list) {
		index = 0
	}
	return list[index], true
}

// screenChoices describes the screens for the settings ("Screen 2 · 2560 × 1440").
func screenChoices() []screenChoice {
	var out []screenChoice
	for _, m := range listMonitors() {
		out = append(out, screenChoice{Width: m.bounds.Dx(), Height: m.bounds.Dy(), Primary: m.primary})
	}
	return out
}
