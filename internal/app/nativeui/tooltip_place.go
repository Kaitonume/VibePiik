package nativeui

import (
	"image"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
)

// Tooltips stay inside the window: Gio cannot tell a widget where it sits in
// the window, so the hovered control reports the pointer's position inside
// it, Windows tells where the pointer is in the window, and the difference is
// the control's place.

// frameWindow is the window being drawn (frames are drawn one at a time).
var frameWindow struct {
	hwnd uintptr
	size image.Point
}

func beginFrame(hwnd uintptr, size image.Point) {
	frameWindow.hwnd, frameWindow.size = hwnd, size
}

// tipTracker receives the pointer position inside the hovered control.
var tipTracker struct {
	local f32.Point
	ok    bool
}

// trackTip registers the hovered control's area for pointer positions.
func trackTip(gtx layout.Context, size image.Point) {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &tipTracker, Kinds: pointer.Enter | pointer.Move})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			tipTracker.local, tipTracker.ok = e.Position, true
		}
	}
	// Pass-through: the control under it keeps its clicks and hover.
	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &tipTracker)
}

// controlOrigin is the hovered control's top-left corner in the window.
func controlOrigin() (image.Point, bool) {
	if !tipTracker.ok || frameWindow.hwnd == 0 {
		return image.Point{}, false
	}
	cursor, ok := cursorInWindow(frameWindow.hwnd)
	if !ok {
		return image.Point{}, false
	}
	return cursor.Sub(image.Pt(int(tipTracker.local.X), int(tipTracker.local.Y))), true
}

// placeTip moves a tooltip box (default place at, relative to a control of
// size) so it stays in the window: below the control, or above it when
// there is no room below, and never past the sides.
func placeTip(at, box, size image.Point, gap int) image.Point {
	origin, ok := controlOrigin()
	window := frameWindow.size
	if !ok || window.X == 0 {
		return at
	}
	margin := 4
	if origin.Y+at.Y+box.Y > window.Y-margin {
		above := -box.Y - gap
		below := window.Y - (origin.Y + size.Y + gap)
		if origin.Y+above >= margin || origin.Y > below {
			at.Y = above
		}
	}
	at.Y = max(margin-origin.Y, min(at.Y, window.Y-margin-box.Y-origin.Y))
	at.X = max(margin-origin.X, min(at.X, window.X-margin-box.X-origin.X))
	return at
}
