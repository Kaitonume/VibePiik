//go:build windows

package nativeui

import (
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// systemDark reports Windows' app theme (Settings > Personalization > Colors).
func systemDark() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("AppsUseLightTheme")
	return err == nil && value == 0
}

var procGetUserDefaultUILanguage = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// systemLanguage is the web client's default: Chinese on a Chinese Windows,
// English otherwise (navigator.language → zh or en).
func systemLanguage() string {
	if procGetUserDefaultUILanguage.Find() != nil {
		return "en"
	}
	language, _, _ := procGetUserDefaultUILanguage.Call()
	switch language & 0x3ff {
	case 0x04: // LANG_CHINESE
		return "zh"
	case 0x10: // LANG_ITALIAN
		return "it"
	}
	return "en" // any other language: English
}

// reducedMotion is Windows' "Animation effects" turned off (SPI_GETCLIENTAREAANIMATION).
var reducedMotion = func() bool {
	var on uint32 = 1
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SystemParametersInfoW")
	if proc.Find() != nil {
		return false
	}
	if r, _, _ := proc.Call(0x1042, 0, uintptr(unsafe.Pointer(&on)), 0); r == 0 {
		return false
	}
	return on == 0
}()

var (
	procGetCursorPosTip   = windows.NewLazySystemDLL("user32.dll").NewProc("GetCursorPos")
	procScreenToClientTip = windows.NewLazySystemDLL("user32.dll").NewProc("ScreenToClient")
)

// cursorInWindow is the mouse position in the window's client area.
func cursorInWindow(hwnd uintptr) (image.Point, bool) {
	var p struct{ X, Y int32 }
	if ok, _, _ := procGetCursorPosTip.Call(uintptr(unsafe.Pointer(&p))); ok == 0 {
		return image.Point{}, false
	}
	if ok, _, _ := procScreenToClientTip.Call(hwnd, uintptr(unsafe.Pointer(&p))); ok == 0 {
		return image.Point{}, false
	}
	return image.Pt(int(p.X), int(p.Y)), true
}

// cursorOnScreen is the mouse position on the desktop.
func cursorOnScreen() (image.Point, bool) {
	var p struct{ X, Y int32 }
	if ok, _, _ := procGetCursorPosTip.Call(uintptr(unsafe.Pointer(&p))); ok == 0 {
		return image.Point{}, false
	}
	return image.Pt(int(p.X), int(p.Y)), true
}
