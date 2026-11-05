//go:build windows

package nativeui

import (
	"errors"
	"image"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Sharp screen preview: the App's screen preview is a 320 px thumbnail every
// few seconds. With "sharp preview" on, the native UI copies the shared
// monitor itself (GDI, scaled with HALFTONE) several times a second.

var (
	gdi32sp                  = windows.NewLazySystemDLL("gdi32.dll")
	user32sp                 = windows.NewLazySystemDLL("user32.dll")
	procGetDCsp              = user32sp.NewProc("GetDC")
	procReleaseDCsp          = user32sp.NewProc("ReleaseDC")
	procGetMonitorInfoWsp    = user32sp.NewProc("GetMonitorInfoW")
	procCreateCompatibleDCSP = gdi32sp.NewProc("CreateCompatibleDC")
	procCreateDIBSectionSP   = gdi32sp.NewProc("CreateDIBSection")
	procSelectObjectSP       = gdi32sp.NewProc("SelectObject")
	procDeleteObjectSP       = gdi32sp.NewProc("DeleteObject")
	procDeleteDCSP           = gdi32sp.NewProc("DeleteDC")
	procStretchBltSP         = gdi32sp.NewProc("StretchBlt")
	procSetStretchBltModeSP  = gdi32sp.NewProc("SetStretchBltMode")
	procSetBrushOrgExSP      = gdi32sp.NewProc("SetBrushOrgEx")
	procGdiFlushSP           = gdi32sp.NewProc("GdiFlush")
)

type spRect struct{ Left, Top, Right, Bottom int32 }

// captureMonitor copies the monitor named by the App's display source id
// (its HMONITOR) scaled to at most maxWidth pixels wide.
func captureMonitor(sourceID string, maxWidth int) (*image.RGBA, error) {
	handle, err := strconv.ParseUint(sourceID, 10, 64)
	if err != nil || handle == 0 {
		return nil, errors.New("not a monitor")
	}
	info := struct {
		Size          uint32
		Monitor, Work spRect
		Flags         uint32
	}{Size: 40}
	if ok, _, _ := procGetMonitorInfoWsp.Call(uintptr(handle), uintptr(unsafe.Pointer(&info))); ok == 0 {
		return nil, errors.New("monitor not found")
	}
	srcW, srcH := int(info.Monitor.Right-info.Monitor.Left), int(info.Monitor.Bottom-info.Monitor.Top)
	if srcW <= 0 || srcH <= 0 {
		return nil, errors.New("empty monitor")
	}
	w, h := srcW, srcH
	if w > maxWidth {
		w, h = maxWidth, srcH*maxWidth/srcW
	}
	screen, _, _ := procGetDCsp.Call(0)
	if screen == 0 {
		return nil, errors.New("no screen DC")
	}
	defer procReleaseDCsp.Call(0, screen)
	memory, _, _ := procCreateCompatibleDCSP.Call(screen)
	if memory == 0 {
		return nil, errors.New("no memory DC")
	}
	defer procDeleteDCSP.Call(memory)
	header := struct {
		Size                   uint32
		Width, Height          int32
		Planes, BitCount       uint16
		Compression, SizeImage uint32
		XPels, YPels           int32
		ClrUsed, ClrImportant  uint32
	}{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	bitmap, _, _ := procCreateDIBSectionSP.Call(memory, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		return nil, errors.New("no bitmap")
	}
	defer procDeleteObjectSP.Call(bitmap)
	old, _, _ := procSelectObjectSP.Call(memory, bitmap)
	defer procSelectObjectSP.Call(memory, old)
	const halftone, srccopy = 4, 0x00CC0020
	procSetStretchBltModeSP.Call(memory, halftone)
	procSetBrushOrgExSP.Call(memory, 0, 0, 0)
	if ok, _, _ := procStretchBltSP.Call(memory, 0, 0, uintptr(w), uintptr(h),
		screen, uintptr(info.Monitor.Left), uintptr(info.Monitor.Top), uintptr(srcW), uintptr(srcH), srccopy); ok == 0 {
		return nil, errors.New("copy failed")
	}
	procGdiFlushSP.Call()
	src := unsafe.Slice((*byte)(bits), w*h*4)
	picture := image.NewRGBA(image.Rect(0, 0, w, h))
	dst := picture.Pix
	for i := 0; i < len(src); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i+2], src[i+1], src[i], 255
	}
	return picture, nil
}
