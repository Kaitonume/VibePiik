//go:build windows

package appwindow

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/jchv/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

var (
	user32                            = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procIsIconic                      = user32.NewProc("IsIconic")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procPostMessageW                  = user32.NewProc("PostMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procLoadCursorW                   = user32.NewProc("LoadCursorW")
	procSetWindowTextW                = user32.NewProc("SetWindowTextW")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetWindowPlacement            = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement            = user32.NewProc("SetWindowPlacement")
	procMonitorFromRect               = user32.NewProc("MonitorFromRect")
	procGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procExtractIconExW                = windows.NewLazySystemDLL("shell32.dll").NewProc("ExtractIconExW")
	procCreateSolidBrush              = windows.NewLazySystemDLL("gdi32.dll").NewProc("CreateSolidBrush")
	procDwmSetWindowAttribute         = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
)

const (
	wmDestroy       = 0x0002
	wmMove          = 0x0003
	wmSize          = 0x0005
	wmActivate      = 0x0006
	wmClose         = 0x0010
	wmGetMinMaxInfo = 0x0024
	wmDispatch      = 0x8000 + 1 // WM_APP + 1
	sizeMinimized   = 1
	swShowNormal    = 1
	swShowMaximized = 3
	swRestore       = 9
	wsOverlapped    = 0x00CF0000 // WS_OVERLAPPEDWINDOW
	idcArrow        = 32512
	smCxScreen      = 0
	smCyScreen      = 1
	// DWMWA_USE_IMMERSIVE_DARK_MODE: the App pages are dark.
	dwmUseDarkMode = 20
)

// Matches the App's dark page background so nothing flashes white on start.
var background = edge.COREWEBVIEW2_COLOR{A: 255, R: 22, G: 23, B: 28}

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }

type windowPlacement struct {
	Length, Flags, ShowCmd   uint32
	MinPosition, MaxPosition point
	NormalPosition           rect
	Device                   rect
}

type minMaxInfo struct {
	Reserved, MaxSize, MaxPosition, MinTrackSize, MaxTrackSize point
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type window struct {
	manager   *Manager
	mu        sync.Mutex
	hwnd      uintptr
	chromium  *edge.Chromium
	queue     []func()
	quiet     bool // closed by the App, not by the user
	done      chan struct{}
	dpi       uint32
	placement string
}

// The App has at most one window; its procedure reaches it through this variable.
var (
	current     *window
	windowProc  = windows.NewCallback(windowProcedure)
	classRegist sync.Once
)

// Manager owns the VibePiik window. External links open with browser; a window
// closed by the user calls onUserClose (the App quits).
type Manager struct {
	mu          sync.Mutex
	open        *window
	failed      bool
	browser     func(string) error
	onUserClose func()
}

func NewManager(browser func(string) error, onUserClose func()) *Manager {
	return &Manager{browser: browser, onUserClose: onUserClose}
}

// Available reports whether the window can be shown: the WebView2 Runtime is
// installed and the window has not failed before.
func (m *Manager) Available() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	failed := m.failed
	m.mu.Unlock()
	return !failed && runtimeInstalled()
}

func runtimeInstalled() bool {
	version, err := webviewloader.GetInstalledVersion()
	return err == nil && version != ""
}

// Open shows target in the window, creating the window when none is open.
func (m *Manager) Open(target string) error {
	m.mu.Lock()
	if m.failed {
		m.mu.Unlock()
		return ErrUnavailable
	}
	w := m.open
	if w == nil {
		w = &window{manager: m, done: make(chan struct{}), placement: filepath.Join(dataDirectory(), "window.json")}
		m.open = w
		m.mu.Unlock()
		if err := w.start(target); err != nil {
			m.mu.Lock()
			m.open, m.failed = nil, true
			m.mu.Unlock()
			return err
		}
		return nil
	}
	m.mu.Unlock()
	w.dispatch(func() {
		w.chromium.Navigate(target)
		w.show()
	})
	return nil
}

// Close closes the window, if any, without quitting the App.
func (m *Manager) Close() {
	m.mu.Lock()
	w := m.open
	m.mu.Unlock()
	if w == nil {
		return
	}
	w.mu.Lock()
	w.quiet = true
	hwnd := w.hwnd
	w.mu.Unlock()
	_, _, _ = procPostMessageW.Call(hwnd, wmClose, 0, 0)
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
	}
}

func (w *window) start(target string) error {
	if !runtimeInstalled() {
		return ErrUnavailable
	}
	ready := make(chan error, 1)
	go func() {
		// The window, its WebView2 controller and their messages share this thread.
		runtime.LockOSThread()
		_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
		if err := w.create(target); err != nil {
			ready <- err
			return
		}
		ready <- nil
		var message msg
		for {
			result, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
			if int32(result) <= 0 {
				break
			}
			_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
			_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
		}
		m := w.manager
		m.mu.Lock()
		if m.open == w {
			m.open = nil
		}
		m.mu.Unlock()
		w.mu.Lock()
		quiet := w.quiet
		w.mu.Unlock()
		close(w.done)
		if !quiet && m.onUserClose != nil {
			m.onUserClose()
		}
	}()
	return <-ready
}

func (w *window) create(target string) error {
	// Sharp text on scaled displays; WebView2 follows the window's DPI.
	_, _, _ = procSetProcessDpiAwarenessContext.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2 (-4)
	w.dpi = 96
	if procGetDpiForSystem.Find() == nil {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi != 0 {
			w.dpi = uint32(dpi)
		}
	}
	instance, err := moduleHandle()
	if err != nil {
		return err
	}
	className, _ := windows.UTF16PtrFromString("VibePiikWindow")
	var registerErr error
	classRegist.Do(func() {
		var large, small windows.Handle
		if executable, err := os.Executable(); err == nil {
			path, _ := windows.UTF16PtrFromString(executable)
			_, _, _ = procExtractIconExW.Call(uintptr(unsafe.Pointer(path)), 0,
				uintptr(unsafe.Pointer(&large)), uintptr(unsafe.Pointer(&small)), 1)
		}
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
		brush, _, _ := procCreateSolidBrush.Call(uintptr(background.R) | uintptr(background.G)<<8 | uintptr(background.B)<<16)
		class := wndClassEx{WndProc: windowProc, Instance: instance, Icon: large, IconSm: small,
			Cursor: windows.Handle(cursor), Background: windows.Handle(brush), ClassName: className}
		class.Size = uint32(unsafe.Sizeof(class))
		if atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
			registerErr = callErr
		}
	})
	if registerErr != nil {
		return registerErr
	}

	width, height := w.scale(defaultWidth), w.scale(defaultHeight)
	screenWidth, _, _ := procGetSystemMetrics.Call(smCxScreen)
	screenHeight, _, _ := procGetSystemMetrics.Call(smCyScreen)
	x, y := (int32(screenWidth)-width)/2, (int32(screenHeight)-height)/2
	title, _ := windows.UTF16PtrFromString(defaultTitle)
	current = w
	hwnd, _, callErr := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		wsOverlapped, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, uintptr(instance), 0)
	if hwnd == 0 {
		return callErr
	}
	w.mu.Lock()
	w.hwnd = hwnd
	w.mu.Unlock()
	if procGetDpiForWindow.Find() == nil {
		if dpi, _, _ := procGetDpiForWindow.Call(hwnd); dpi != 0 {
			w.dpi = uint32(dpi)
		}
	}
	if procDwmSetWindowAttribute.Find() == nil {
		dark := int32(1)
		_, _, _ = procDwmSetWindowAttribute.Call(hwnd, dwmUseDarkMode, uintptr(unsafe.Pointer(&dark)), 4)
	}
	w.restorePlacement()

	chromium := edge.NewChromium()
	chromium.DataPath = filepath.Join(dataDirectory(), "WebView2")
	chromium.MessageCallback = w.receive
	if !chromium.Embed(hwnd) {
		_, _, _ = procDestroyWindow.Call(hwnd)
		return errors.New("WebView2 could not be embedded")
	}
	w.chromium = chromium
	if controller := chromium.GetController(); controller != nil {
		if second := controller.GetICoreWebView2Controller2(); second != nil {
			_ = second.PutDefaultBackgroundColor(background)
		}
	}
	if settings, err := chromium.GetSettings(); err == nil {
		_ = settings.PutAreDevToolsEnabled(false)
	}
	chromium.Resize()
	chromium.Init(initScript)
	chromium.Navigate(target)
	chromium.Focus()
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	return nil
}

func (w *window) scale(logical int32) int32 { return logical * int32(w.dpi) / 96 }

func (w *window) restorePlacement() {
	saved, ok := loadPlacement(w.placement)
	show := uintptr(swShowNormal)
	if ok {
		bounds := rect{saved.Left, saved.Top, saved.Right, saved.Bottom}
		// Only on a monitor that is still connected (MONITOR_DEFAULTTONULL).
		if monitor, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&bounds)), 0); monitor != 0 {
			value := windowPlacement{ShowCmd: swShowNormal, NormalPosition: bounds}
			value.Length = uint32(unsafe.Sizeof(value))
			_, _, _ = procSetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&value)))
			if saved.Maximized {
				show = swShowMaximized
			}
		}
	}
	_, _, _ = procShowWindow.Call(w.hwnd, show)
}

func (w *window) savePlacement() {
	var value windowPlacement
	value.Length = uint32(unsafe.Sizeof(value))
	if ok, _, _ := procGetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&value))); ok == 0 {
		return
	}
	bounds := value.NormalPosition
	savePlacement(w.placement, placement{Left: bounds.Left, Top: bounds.Top, Right: bounds.Right,
		Bottom: bounds.Bottom, Maximized: value.ShowCmd == swShowMaximized})
}

func (w *window) show() {
	if iconic, _, _ := procIsIconic.Call(w.hwnd); iconic != 0 {
		_, _, _ = procShowWindow.Call(w.hwnd, swRestore)
	}
	_, _, _ = procSetForegroundWindow.Call(w.hwnd)
}

func (w *window) dispatch(task func()) {
	w.mu.Lock()
	w.queue = append(w.queue, task)
	hwnd := w.hwnd
	w.mu.Unlock()
	_, _, _ = procPostMessageW.Call(hwnd, wmDispatch, 0, 0)
}

// receive runs on the window thread for messages posted by initScript.
func (w *window) receive(raw string) {
	value, ok := parseMessage(raw)
	if !ok {
		return
	}
	if value.open != "" {
		if browser := w.manager.browser; browser != nil {
			go func() { _ = browser(value.open) }()
		}
		return
	}
	title, _ := windows.UTF16PtrFromString(value.title)
	_, _, _ = procSetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(title)))
}

func windowProcedure(hwnd, message, wParam, lParam uintptr) uintptr {
	w := current
	if w != nil && w.hwnd == hwnd {
		chromium := w.chromium
		live := chromium != nil && chromium.GetController() != nil
		switch message {
		case wmSize:
			if live {
				if wParam == sizeMinimized {
					// Hidden pages pause the local preview: nothing to draw while minimized.
					_ = chromium.Hide()
				} else {
					_ = chromium.Show()
					chromium.Resize()
				}
			}
			return 0
		case wmMove:
			if live {
				_ = chromium.NotifyParentWindowPositionChanged()
			}
			return 0
		case wmActivate:
			if live && wParam&0xffff != 0 {
				chromium.Focus()
			}
			return 0
		case wmGetMinMaxInfo:
			// lParam is a MINMAXINFO pointer owned by the system for this call.
			info := *(**minMaxInfo)(unsafe.Pointer(&lParam))
			info.MinTrackSize = point{w.scale(minimumWidth), w.scale(minimumHeight)}
			return 0
		case wmDispatch:
			w.mu.Lock()
			queue := w.queue
			w.queue = nil
			w.mu.Unlock()
			for _, task := range queue {
				task()
			}
			return 0
		case wmClose:
			w.savePlacement()
			_, _, _ = procDestroyWindow.Call(hwnd)
			return 0
		case wmDestroy:
			_, _, _ = procPostQuitMessage.Call(0)
			return 0
		}
	}
	result, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return result
}

func moduleHandle() (windows.Handle, error) {
	var handle windows.Handle
	err := windows.GetModuleHandleEx(0, nil, &handle)
	return handle, err
}

// RestoreWindowPlacement shows another VibePiik window (the native UI) where
// the last VibePiik window was, so both kinds open on the same monitor.
// Some windows (Gio) ignore SetWindowPlacement; SetWindowPos moves them all.
// A window saved at the old default height grows to minHeightDp (within the monitor's
// work area, moving up if needed), so a new taller default reaches windows
// placed by older builds.
func RestoreWindowPlacement(hwnd uintptr, minHeightDp int) {
	saved, ok := loadPlacement(filepath.Join(dataDirectory(), "window.json"))
	if !ok {
		return
	}
	bounds := rect{saved.Left, saved.Top, saved.Right, saved.Bottom}
	monitor, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&bounds)), 0)
	if monitor == 0 {
		return
	}
	if minHeightDp > 0 && !saved.Maximized {
		dpi := int32(96)
		if procGetDpiForWindow.Find() == nil {
			if value, _, _ := procGetDpiForWindow.Call(hwnd); value != 0 {
				dpi = int32(value)
			}
		}
		info := struct {
			Size          uint32
			Monitor, Work rect
			Flags         uint32
		}{Size: 40}
		want := int32(minHeightDp) * dpi / 96
		if ret, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ret != 0 {
			want = min(want, info.Work.Bottom-info.Work.Top)
			// Only the old default height (880) grows: a size the user chose stays.
			old := 880 * dpi / 96
			if height := bounds.Bottom - bounds.Top; height < want && height >= old-4 && height <= old+4 {
				bounds.Bottom = bounds.Top + want
				if bounds.Bottom > info.Work.Bottom {
					bounds.Top, bounds.Bottom = info.Work.Bottom-want, info.Work.Bottom
				}
			}
		}
	}
	// SWP_ASYNCWINDOWPOS: the window thread may be waiting on its caller.
	const swpNoZOrderNoActivate = 0x0004 | 0x0010 | 0x4000
	_, _, _ = procSetWindowPos.Call(hwnd, 0, uintptr(int(bounds.Left)), uintptr(int(bounds.Top)),
		uintptr(int(bounds.Right-bounds.Left)), uintptr(int(bounds.Bottom-bounds.Top)), swpNoZOrderNoActivate)
	if saved.Maximized {
		_, _, _ = procShowWindow.Call(hwnd, swShowMaximized)
	}
}

// SaveWindowPlacement remembers where hwnd is for the next VibePiik window.
func SaveWindowPlacement(hwnd uintptr) {
	w := &window{hwnd: hwnd, placement: filepath.Join(dataDirectory(), "window.json")}
	w.savePlacement()
}

// SetDarkTitleBar matches the window frame to a dark or light page.
func SetDarkTitleBar(hwnd uintptr, dark bool) {
	if procDwmSetWindowAttribute.Find() != nil {
		return
	}
	value := int32(0)
	if dark {
		value = 1
	}
	_, _, _ = procDwmSetWindowAttribute.Call(hwnd, dwmUseDarkMode, uintptr(unsafe.Pointer(&value)), 4)
}
