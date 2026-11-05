package nativeui

import (
	"image"
	"image/color"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// Local preview controls on the TV chin (HostPage playback controls): the
// preview can be paused on this PC (sharing continues), fit Fit/Fill/Stretch,
// and a no-audio mark when the share carries no sound.

var fitModes = []string{"contain", "cover", "fill"}

// chinIcon is a small icon button with a (comic) tooltip.
type chinIcon struct {
	click   widget.Clickable
	hoverAt time.Time
}

func (c *chinIcon) Layout(gtx layout.Context, u *UI, icon string, ink color.NRGBA, tip, comicKind string) layout.Dimensions {
	return c.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		side := gtx.Dp(24)
		size := image.Pt(side, side)
		if c.click.Hovered() {
			fillBox(gtx.Ops, boxOf(size, float32(side)/2), withAlpha(tvInk, .14))
		}
		g := gtx.Dp(16)
		o := op.Offset(image.Pt((side-g)/2, (side-g)/2)).Push(gtx.Ops)
		glyph(gtx, icon, 16, ink)
		o.Pop()
		pointerHand(gtx, size)
		c.hoverAt = hoverTooltipComic(gtx, u, c.click.Hovered(), c.hoverAt, tip, comicKind, size)
		return layout.Dimensions{Size: size}
	})
}

type previewControls struct {
	fit, play chinIcon
	noAudio   chinIcon
	theater   chinIcon
	full      chinIcon
	pip       chinIcon
	sharp     chinIcon
	fps       [2]widget.Clickable // 30 / 60 above the sharp button
	fpsUntil  time.Time
}

// fitIcon names the glyph and copy key of a fit mode.
func fitIcon(mode string) (icon, key, comicKind string) {
	switch mode {
	case "cover":
		return "fitCover", "playback.fit.cover", "hint-fit-cover"
	case "fill":
		return "fitStretch", "playback.fit.fill", "hint-fit-fill"
	}
	return "fitContain", "playback.fit.contain", "hint-fit-contain"
}

func nextFit(mode string) string {
	for i, m := range fitModes {
		if m == mode {
			return fitModes[(i+1)%len(fitModes)]
		}
	}
	return fitModes[1]
}

// chinTools draws the preview controls at the chin's right while sharing.
func (r *Room) chinTools(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	if snap.Phase != phaseLive && snap.Phase != phasePaused {
		return layout.Dimensions{}
	}
	c := &r.previewCtl
	if c.fit.click.Clicked(gtx) {
		r.setFit(nextFit(r.fit))
	}
	if c.play.click.Clicked(gtx) {
		r.togglePreview()
	}
	if c.theater.click.Clicked(gtx) {
		r.theater = !r.theater
	}
	if c.full.click.Clicked(gtx) {
		r.setFullscreen(true)
	}
	if c.pip.click.Clicked(gtx) {
		r.togglePip(snap)
	}
	if c.sharp.click.Clicked(gtx) {
		setSharpPreview(!sharpPreview())
	}
	ink := withAlpha(tvInk, .85)
	children := []layout.FlexChild{}
	if snap.SourceAudio != nil && !*snap.SourceAudio || !snap.Audio {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			key := "host.noAudio"
			return c.noAudio.Layout(gtx, u, "speakerOff", withAlpha(tvInk, .6), u.T(key), "hint-no-audio")
		}), layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
	}
	if snap.Target.Kind == "display" {
		// Sharp or light screen preview (windows are always live and sharp).
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			ink, tip := withAlpha(tvInk, .5), u.T("native.previewSharpOff")
			if sharpPreview() {
				ink, tip = withAlpha(u.P.Live, .95), u.T("native.previewSharpOn")
			}
			d := c.sharp.Layout(gtx, u, "zap", ink, tip, "")
			r.previewRate(gtx, u, d.Size)
			return d
		}), layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
	}
	icon, key, kind := fitIcon(r.fit)
	_, nextKey, _ := fitIcon(nextFit(r.fit))
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.fit.Layout(gtx, u, icon, ink, u.T("playback.fitCycle", "current", u.T(key), "next", u.T(nextKey)), kind)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if r.previewOff {
				return c.play.Layout(gtx, u, "play", ink, u.T("playback.play"), "hint-local-play")
			}
			return c.play.Layout(gtx, u, "pause", ink, u.T("playback.pause"), "hint-local-pause")
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if r.theater {
				return c.theater.Layout(gtx, u, "theaterExit", ink, u.T("viewer.theater.exit"), "hint-theater-exit")
			}
			return c.theater.Layout(gtx, u, "theater", ink, u.T("viewer.theater"), "hint-theater")
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.full.Layout(gtx, u, "expand", ink, u.T("playback.fullscreen"), "hint-fullscreen")
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			switch {
			case snap.Target.Kind != "window":
				return c.pip.Layout(gtx, u, "pip", withAlpha(tvInk, .4), u.T("native.pipScreen"), "hint-pip-unavailable")
			case r.pipClose != nil:
				return c.pip.Layout(gtx, u, "pipExit", ink, u.T("playback.pip"), "hint-pip-exit")
			}
			return c.pip.Layout(gtx, u, "pip", ink, u.T("playback.pip"), "hint-pip")
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout))
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (r *Room) setFit(mode string) {
	r.fit = mode
	if r.preview != nil {
		r.preview.SetFit(mode)
	}
	go updatePreferences(func(p *preferences) { p.PreviewFit = mode })
}

func (r *Room) togglePreview() {
	r.previewOff = !r.previewOff
	if r.previewOff && r.preview != nil {
		r.preview.Hide()
	}
}

// previewPaused reports whether the local preview is not shown: paused by
// the host, or auto-paused while VibePiik is in the background.
func (r *Room) previewPaused() bool {
	return r.previewOff || (r.autoPause && !r.focused)
}

// drawFit paints img into size with object-fit contain, cover or fill.
func drawFit(ops *op.Ops, img paint.ImageOp, size image.Point, mode string) {
	src := img.Size()
	if src.X == 0 || src.Y == 0 {
		return
	}
	sx, sy := float32(size.X)/float32(src.X), float32(size.Y)/float32(src.Y)
	switch mode {
	case "cover":
		sx = max(sx, sy)
		sy = sx
	case "fill":
	default:
		drawContain(ops, img, size)
		return
	}
	w, h := float32(src.X)*sx, float32(src.Y)*sy
	x, y := (float32(size.X)-w)/2, (float32(size.Y)-h)/2
	defer clip.Rect{Max: size}.Push(ops).Pop()
	defer op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(sx, sy)).Offset(f32.Pt(x, y))).Push(ops).Pop()
	img.Add(ops)
	paint.PaintOp{}.Add(ops)
}

// spinner is the turning loader glyph (lr-spin).
func spinner(gtx layout.Context, u *UI, sideDp float32) layout.Dimensions {
	side := px(gtx, sideDp)
	angle := float32(0)
	if !reducedMotion {
		angle = float32(gtx.Now.UnixMilli()%1000) / 1000 * 2 * 3.14159
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(33 * time.Millisecond)})
	}
	defer op.Affine(rotateAbout(side/2, side/2, angle)).Push(gtx.Ops).Pop()
	return glyph(gtx, "loader", sideDp, u.P.Action)
}

// togglePip opens or closes picture in picture for a shared window.
func (r *Room) togglePip(snap roomSnapshot) {
	if r.pipClose != nil {
		r.pipClose()
		return
	}
	closeFn, done, ok := openPictureInPicture(snap.Target, "VibePiik · "+snap.Target.Title)
	if !ok {
		return
	}
	r.pipClose = closeFn
	go func() {
		<-done
		r.mu.Lock()
		r.pipClose = nil
		r.mu.Unlock()
		r.invalidate()
	}()
}

// previewRate is the tiny 30 / 60 choice that appears above the sharp
// preview button while the pointer is on it (or on the choice).
func (r *Room) previewRate(gtx layout.Context, u *UI, button image.Point) {
	c := &r.previewCtl
	rates := [2]int{30, 60}
	for i := range c.fps {
		if c.fps[i].Clicked(gtx) {
			setPreviewFPS(rates[i])
			if !sharpPreview() {
				setSharpPreview(true)
			}
		}
	}
	if c.sharp.click.Hovered() || c.fps[0].Hovered() || c.fps[1].Hovered() {
		c.fpsUntil = gtx.Now.Add(400 * time.Millisecond)
	}
	if !gtx.Now.Before(c.fpsUntil) {
		return
	}
	gtx.Execute(op.InvalidateCmd{At: c.fpsUntil})
	cell := image.Pt(gtx.Dp(30), gtx.Dp(22))
	pad := gtx.Dp(3)
	size := image.Pt(2*cell.X+3*pad, cell.Y+2*pad)
	at := image.Pt((button.X-size.X)/2, -size.Y-gtx.Dp(4))
	o := op.Offset(at).Push(gtx.Ops)
	defer o.Pop()
	shape := boxOf(size, px(gtx, 8))
	fillBox(gtx.Ops, shape, rgba(0x14, 0x18, 0x20, .92))
	strokeBox(gtx.Ops, shape, px(gtx, 1), withAlpha(tvInk, .25))
	for i, rate := range rates {
		co := op.Offset(image.Pt(pad+i*(cell.X+pad), pad)).Push(gtx.Ops)
		c.fps[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			selected := sharpPreview() && previewFPS() == rate
			cellBox := boxOf(cell, px(gtx, 6))
			ink := withAlpha(tvInk, .7)
			switch {
			case selected:
				fillBox(gtx.Ops, cellBox, u.P.Live)
				ink = rgb(0x10141a)
			case c.fps[i].Hovered():
				fillBox(gtx.Ops, cellBox, withAlpha(tvInk, .14))
			}
			lg := gtx
			lg.Constraints = layout.Exact(cell)
			layout.Center.Layout(lg, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, itoa(rate), textStyle{size: 11, mono: true, weight: font.Bold, color: ink})
			})
			pointerHand(gtx, cell)
			return layout.Dimensions{Size: cell}
		})
		co.Pop()
	}
}
