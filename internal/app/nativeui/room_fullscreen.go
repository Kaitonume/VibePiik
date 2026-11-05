package nativeui

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// Theater mode (viewer.theater): the TV takes the room's full width.
// Fullscreen (playback.fullscreen): the window covers the screen and shows
// the picture alone; Esc, F11 or a double click leave it.

func (r *Room) contentCap(gtx layout.Context) int {
	if r.theater {
		return gtx.Constraints.Max.X
	}
	return gtx.Dp(1100)
}

func (r *Room) tvCap(gtx layout.Context) int {
	if r.theater {
		return 1 << 20
	}
	return int(px(gtx, 960))
}

func (r *Room) setFullscreen(on bool) {
	if r.fullscreen == on {
		return
	}
	r.fullscreen = on
	if r.onFullscreen != nil {
		r.onFullscreen(on)
	}
	r.invalidate()
}

// fullscreenLayout draws the shared picture over the whole window.
func (r *Room) fullscreenLayout(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, rgb(0x000000), clip.Rect{Max: size}.Op())
	for {
		e, ok := r.fullClick.Update(gtx.Source)
		if !ok {
			break
		}
		if e.Kind == gesture.KindClick && e.NumClicks == 2 {
			r.setFullscreen(false)
		}
	}
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	r.fullClick.Add(gtx.Ops)
	pointer.CursorDefault.Add(gtx.Ops)
	area.Pop()
	dest := image.Rectangle{Max: size}
	window := snap.Phase == phaseLive && snap.Target.Kind == "window" && !r.previewPaused()
	if r.preview != nil {
		r.preview.SetFit(r.fit)
		if !window || !r.preview.Show(snap.Target, dest, dest) {
			r.preview.Hide()
			window = false
		}
	}
	if !window {
		gtx.Constraints = layout.Exact(size)
		r.screen(gtx, u, snap)
	}
	r.overlays(gtx, u, snap)
	r.feedOverlay(gtx, u, snap, dest, dest)
	return layout.Dimensions{Size: size}
}
