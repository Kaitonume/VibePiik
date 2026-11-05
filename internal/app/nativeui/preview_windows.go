//go:build windows

package nativeui

import (
	"image"
	"log/slog"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/TNTcraftHIM/Piik/internal/app/nativecapture"
)

// dwmPreview shows the shared window live inside the TV with a DWM
// thumbnail: Windows composites it at the window's own frame rate, with no
// capture, decoding or copying in VibePiik.
type dwmPreview struct {
	fit    string
	dest   uintptr
	thumb  uintptr
	source uintptr
	last   dwmThumbnailProperties
}

var (
	dwmapi                       = syscall.NewLazyDLL("dwmapi.dll")
	procDwmRegisterThumbnail     = dwmapi.NewProc("DwmRegisterThumbnail")
	procDwmUnregisterThumbnail   = dwmapi.NewProc("DwmUnregisterThumbnail")
	procDwmUpdateThumbnailProps  = dwmapi.NewProc("DwmUpdateThumbnailProperties")
	procDwmQueryThumbnailSrcSize = dwmapi.NewProc("DwmQueryThumbnailSourceSize")
	procIsWindow                 = syscall.NewLazyDLL("user32.dll").NewProc("IsWindow")
)

type winRect struct{ Left, Top, Right, Bottom int32 }

type dwmThumbnailProperties struct {
	Flags                uint32
	Destination          winRect
	Source               winRect
	Opacity              uint8
	_                    [3]byte
	Visible              int32
	SourceClientAreaOnly int32
}

const (
	dwmTNPRectDestination = 0x1
	dwmTNPRectSource      = 0x2
	dwmTNPOpacity         = 0x4
	dwmTNPVisible         = 0x8
)

func newLivePreview(hwnd uintptr) livePreview {
	if hwnd == 0 || dwmapi.Load() != nil {
		return nil
	}
	return &dwmPreview{dest: hwnd}
}

func (p *dwmPreview) Show(target nativecapture.CaptureTarget, dest, clip image.Rectangle) bool {
	if target.Kind != "window" {
		return false
	}
	value, err := strconv.ParseUint(target.SourceID, 10, 64)
	if err != nil || value == 0 {
		return false
	}
	source := uintptr(value)
	if alive, _, _ := procIsWindow.Call(source); alive == 0 {
		p.Hide()
		return false
	}
	if p.thumb == 0 || p.source != source {
		p.Hide()
		var thumb uintptr
		if hr, _, _ := procDwmRegisterThumbnail.Call(p.dest, source, uintptr(unsafe.Pointer(&thumb))); hr != 0 {
			return false
		}
		p.thumb, p.source = thumb, source
		slog.Debug("vibepiik-native", "event", "live-preview-registered")
		p.last = dwmThumbnailProperties{}
	}
	var size struct{ CX, CY int32 }
	if hr, _, _ := procDwmQueryThumbnailSrcSize.Call(p.thumb, uintptr(unsafe.Pointer(&size))); hr != 0 || size.CX <= 0 || size.CY <= 0 {
		return false
	}
	// object-fit: contain (default), cover or fill inside the screen.
	scaleX := float64(dest.Dx()) / float64(size.CX)
	scaleY := float64(dest.Dy()) / float64(size.CY)
	switch p.fit {
	case "cover":
		scaleX = max(scaleX, scaleY)
		scaleY = scaleX
	case "fill":
	default:
		scaleX = min(scaleX, scaleY)
		scaleY = scaleX
	}
	w, h := int(float64(size.CX)*scaleX), int(float64(size.CY)*scaleY)
	origin := image.Pt(dest.Min.X+(dest.Dx()-w)/2, dest.Min.Y+(dest.Dy()-h)/2)
	fit := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(w, h))}
	shown := fit.Intersect(clip).Intersect(dest)
	if shown.Empty() {
		return false
	}
	// Crop the source to the visible part of the picture.
	sx := func(x int) int32 { return int32(float64(x-fit.Min.X) / scaleX) }
	sy := func(y int) int32 { return int32(float64(y-fit.Min.Y) / scaleY) }
	props := dwmThumbnailProperties{
		Flags:       dwmTNPRectDestination | dwmTNPRectSource | dwmTNPOpacity | dwmTNPVisible,
		Destination: winRect{int32(shown.Min.X), int32(shown.Min.Y), int32(shown.Max.X), int32(shown.Max.Y)},
		Source:      winRect{sx(shown.Min.X), sy(shown.Min.Y), sx(shown.Max.X), sy(shown.Max.Y)},
		Opacity:     255,
		Visible:     1,
	}
	if props != p.last {
		if hr, _, _ := procDwmUpdateThumbnailProps.Call(p.thumb, uintptr(unsafe.Pointer(&props))); hr != 0 {
			return false
		}
		if p.last.Flags == 0 {
			slog.Debug("vibepiik-native", "event", "live-preview-shown", "dest", dest.String(), "clip", clip.String(), "shown", shown.String(), "source", size.CX, "sourceH", size.CY)
		}
		p.last = props
	}
	return true
}

func (p *dwmPreview) SetFit(mode string) { p.fit = mode }

func (p *dwmPreview) Hide() {
	if p.thumb != 0 {
		procDwmUnregisterThumbnail.Call(p.thumb)
		p.thumb, p.source = 0, 0
		p.last = dwmThumbnailProperties{}
	}
}
