package nativeui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

// Small controls of the room panels (styles.css .lr-chip, .lr-tile, toolbar
// buttons, range inputs).

// FlatButton is a toolbar button (.lr-host-share-controls button): no border,
// radius 12, hover tint; Selected paints it like a pressed toggle.
type FlatButton struct {
	click   widget.Clickable
	hoverAt time.Time
	Tip     string
	Comic   string // comic kind of the tooltip
}

func (b *FlatButton) Clicked(gtx layout.Context) bool { return b.click.Clicked(gtx) }

func (b *FlatButton) Layout(gtx layout.Context, u *UI, icon, caption string, ink color.NRGBA, selected bool) layout.Dimensions {
	p := &u.P
	tip := b.Tip
	if u.Visual() && icon != "" {
		// ✦ mode: the glyph carries the meaning, the caption becomes the hint.
		if tip == "" {
			tip = caption
		}
		caption = ""
	}
	return b.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		macro := op.Record(gtx.Ops)
		content := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				c := ink
				if selected {
					c = p.ControlSelectedInk
				}
				return glyph(gtx, icon, 18, c)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if caption == "" {
					return layout.Dimensions{}
				}
				return layout.Spacer{Width: unit.Dp(10)}.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				c := ink
				if selected {
					c = p.ControlSelectedInk
				}
				return u.label(gtx, caption, textStyle{size: 12.5, weight: font.SemiBold, color: c, maxLines: 1})
			}))
		call := macro.Stop()
		pad := int(px(gtx, 12))
		size := image.Pt(content.Size.X+2*pad, int(px(gtx, 44)))
		shape := boxOf(size, px(gtx, 12))
		switch {
		case selected:
			fillBox(gtx.Ops, shape, p.ControlSelected)
		case b.click.Hovered():
			fillBox(gtx.Ops, shape, p.ControlHover)
		}
		o := op.Offset(image.Pt(pad, (size.Y-content.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		o.Pop()
		pointerHand(gtx, size)
		b.hoverAt = hoverTooltipComic(gtx, u, b.click.Hovered(), b.hoverAt, tip, b.Comic, size)
		return layout.Dimensions{Size: size}
	})
}

// Chip is .lr-chip: pill, 2px border, control shadow, 13/600; selected = paper inverse.
type Chip struct {
	click widget.Clickable
	lift  tween
}

func (c *Chip) Clicked(gtx layout.Context) bool { return c.click.Clicked(gtx) }

func (c *Chip) Layout(gtx layout.Context, u *UI, icon, caption, small string, selected, disabled bool) layout.Dimensions {
	p := &u.P
	hovered := !disabled && c.click.Hovered()
	target := float32(0)
	if hovered && !selected {
		target = -2
	}
	if !disabled && c.click.Pressed() {
		target = 2
	}
	lift := c.lift.set(gtx, target, 260*time.Millisecond, spring)
	return liftClick(gtx, &c.click, !disabled, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		ink := p.Ink
		if selected {
			ink = p.ControlSelectedInk
		}
		macro := op.Record(gtx.Ops)
		content := layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if icon == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Right: unit.Dp(6), Top: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return glyph(gtx, icon, 18, ink)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, caption, textStyle{size: 13, weight: font.SemiBold, color: ink, maxLines: 1})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if small == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Left: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, small, textStyle{size: 10, mono: true, color: withAlpha(ink, .8)})
				})
			}))
		call := macro.Stop()
		pad := int(px(gtx, 14))
		size := image.Pt(max(content.Size.X+2*pad, int(px(gtx, 51))), int(px(gtx, 40)))
		shape := boxOf(size, float32(size.Y)/2).offset(0, lift)
		alpha := float32(1)
		if disabled {
			alpha = .5
		}
		if !selected && !disabled {
			controlShadow(gtx.Ops, p, shape, 1)
		}
		switch {
		case selected:
			fillBox(gtx.Ops, shape, withAlpha(p.ControlSelected, alpha))
			strokeBox(gtx.Ops, shape, px(gtx, 2), withAlpha(p.ControlSelected, alpha))
		default:
			bg := p.Paper
			if hovered {
				bg = p.ControlHover
			}
			fillBox(gtx.Ops, shape, withAlpha(bg, alpha))
			strokeBox(gtx.Ops, shape, px(gtx, 2), withAlpha(p.ControlEdge, alpha))
		}
		o := op.Offset(image.Pt((size.X-content.Size.X)/2, (size.Y-content.Size.Y)/2+int(lift))).Push(gtx.Ops)
		call.Add(gtx.Ops)
		o.Pop()
		if !disabled {
			pointerHand(gtx, size)
		}
		return layout.Dimensions{Size: size}
	})
}

// Tile is .lr-tile: quality preset card (glyph, title, mono detail).
type Tile struct {
	click widget.Clickable
	lift  tween
}

func (t *Tile) Clicked(gtx layout.Context) bool { return t.click.Clicked(gtx) }

func (t *Tile) Layout(gtx layout.Context, u *UI, lines int, fast bool, title, detail string, selected bool) layout.Dimensions {
	p := &u.P
	hovered := t.click.Hovered()
	target := float32(0)
	if hovered && !selected {
		target = -2
	}
	if t.click.Pressed() {
		target = 2
	}
	lift := t.lift.set(gtx, target, 260*time.Millisecond, spring)
	width := gtx.Constraints.Max.X
	return liftClick(gtx, &t.click, true, func(gtx layout.Context) layout.Dimensions {
		ink, sub := p.Ink, p.Muted
		if selected {
			ink, sub = p.ControlSelectedInk, p.ControlSelectedInk
		}
		size := image.Pt(width, int(px(gtx, 106)))
		shape := boxOf(size, px(gtx, 16)).offset(0, lift)
		if !selected {
			controlShadow(gtx.Ops, p, shape, 1)
			bg := p.Paper
			if hovered {
				bg = p.ControlHover
			}
			fillBox(gtx.Ops, shape, bg)
			strokeBox(gtx.Ops, shape, px(gtx, 2), p.ControlEdge)
		} else {
			fillBox(gtx.Ops, shape, p.ControlSelected)
		}
		o := op.Offset(image.Pt(0, int(px(gtx, 14))+int(lift))).Push(gtx.Ops)
		cgtx := gtx
		cgtx.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, size.Y)}
		layout.N.Layout(cgtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{}
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return qualityGlyph(gtx, lines, fast, ink) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, title, textStyle{size: 14, weight: font.SemiBold, color: ink, maxLines: 1})
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, detail, textStyle{size: 11, mono: true, color: sub, maxLines: 1})
				}))
		})
		o.Pop()
		pointerHand(gtx, size)
		return layout.Dimensions{Size: size}
	})
}

// Slider is a range input: widget.Float over [Min, Max] snapped to Step.
type Slider struct {
	float widget.Float
	last  float32
}

// Layout draws the range and returns the (snapped) value, changed when dragged.
func (s *Slider) Layout(gtx layout.Context, u *UI, value, lo, hi, step float32) (layout.Dimensions, float32, bool) {
	p := &u.P
	span := hi - lo
	if span <= 0 {
		span = 1
	}
	if !s.float.Dragging() {
		s.float.Value = (value - lo) / span
	}
	width := gtx.Constraints.Max.X
	height := int(px(gtx, 28))
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	changed := s.float.Update(gtx)
	knob := px(gtx, 18)
	margin := int(knob / 2)
	inner := gtx
	inner.Constraints = layout.Exact(image.Pt(width-2*margin, height))
	o := op.Offset(image.Pt(margin, 0)).Push(gtx.Ops)
	s.float.Layout(inner, layout.Horizontal, unit.Dp(knob/2))
	o.Pop()
	v := lo + s.float.Value*span
	if step > 0 {
		v = float32(math.Round(float64((v-lo)/step)))*step + lo
	}
	v = min(max(v, lo), hi)
	pos := float32(margin) + (v-lo)/span*float32(width-2*margin)
	trackH := px(gtx, 6)
	y := float32(height)/2 - trackH/2
	track := box{rect: frect{Min: f32.Pt(float32(margin), y), Max: f32.Pt(float32(width-margin), y+trackH)}, radius: trackH / 2}
	fillBox(gtx.Ops, track, p.Wall2)
	filled := track
	filled.rect.Max.X = pos
	fillBox(gtx.Ops, filled, p.Ink)
	k := box{rect: frect{Min: f32.Pt(pos-knob/2, float32(height)/2-knob/2), Max: f32.Pt(pos+knob/2, float32(height)/2+knob/2)}, radius: knob / 2}
	fillBox(gtx.Ops, k, p.Ink)
	strokeBox(gtx.Ops, k, px(gtx, 2), p.Paper)
	defer clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops).Pop()
	pointer.CursorPointer.Add(gtx.Ops)
	moved := changed || (s.float.Dragging() && v != s.last)
	s.last = v
	return layout.Dimensions{Size: image.Pt(width, height)}, v, moved
}

// valueBox is .lr-quality-number: wall background, 1px edge, mono value + unit.
func valueBox(gtx layout.Context, u *UI, value, unitText string) layout.Dimensions {
	p := &u.P
	gtx.Constraints.Min = image.Point{}
	macro := op.Record(gtx.Ops)
	content := layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, value, textStyle{size: 15, mono: true, weight: font.Bold, color: p.Ink})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, unitText, textStyle{size: 11, mono: true, color: p.Muted})
		}))
	call := macro.Stop()
	pad := int(px(gtx, 12))
	size := image.Pt(max(content.Size.X+2*pad, int(px(gtx, 110))), int(px(gtx, 38)))
	shape := boxOf(size, px(gtx, 12))
	fillBox(gtx.Ops, shape, p.Wall)
	strokeBox(gtx.Ops, shape, px(gtx, 1), p.ControlEdge)
	o := op.Offset(image.Pt(size.X-pad-content.Size.X, (size.Y-content.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	return layout.Dimensions{Size: size}
}

// heading is .lr-door-glyph: glyph + 13/600 muted caption.
func heading(gtx layout.Context, u *UI, icon, caption string) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, icon, 18, u.P.Muted) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, caption, textStyle{size: 13, weight: font.SemiBold, color: u.P.Muted, maxLines: 1})
		}))
}

// Summary is a <details><summary> row: glyph + caption, optional right text, chevron.
type Summary struct {
	click  widget.Clickable
	Open   bool
	rotate tween
}

func (s *Summary) Layout(gtx layout.Context, u *UI, icon, caption, right string) layout.Dimensions {
	if s.click.Clicked(gtx) {
		s.Open = !s.Open
	}
	p := &u.P
	angle := float32(0)
	if s.Open {
		angle = math.Pi
	}
	rotation := s.rotate.set(gtx, angle, 180*time.Millisecond, easeOut)
	return s.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := gtx.Constraints.Max.X
		height := int(px(gtx, 39))
		size := image.Pt(width, height)
		if s.click.Hovered() {
			fillBox(gtx.Ops, boxOf(size, px(gtx, 12)), p.ControlHover)
		}
		o := op.Offset(image.Pt(int(px(gtx, 8)), 0)).Push(gtx.Ops)
		rgtx := gtx
		rgtx.Constraints = layout.Constraints{Min: image.Pt(width-int(px(gtx, 16)), 0), Max: image.Pt(width-int(px(gtx, 16)), height)}
		rowMacro := op.Record(gtx.Ops)
		rowDims := layout.Flex{Alignment: layout.Middle}.Layout(rgtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, icon, 17, p.Muted) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, caption, textStyle{size: 12.5, weight: font.SemiBold, color: p.Muted, maxLines: 1})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return u.label(gtx, right, textStyle{size: 11, mono: true, color: p.Muted, align: text.End, maxLines: 1})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				side := px(gtx, 17)
				defer op.Affine(rotateAbout(side/2, side/2, rotation)).Push(gtx.Ops).Pop()
				return glyph(gtx, "chevron", 17, p.Muted)
			}))
		rowCall := rowMacro.Stop()
		centre := op.Offset(image.Pt(0, (height-rowDims.Size.Y)/2)).Push(gtx.Ops)
		rowCall.Add(gtx.Ops)
		centre.Pop()
		o.Pop()
		pointerHand(gtx, size)
		return layout.Dimensions{Size: size}
	})
}

// switchRow is .lr-switch-item: switch, then a label (optional lock glyph).
func switchRow(gtx layout.Context, u *UI, s *Switch, caption string, value *bool, disabled bool) layout.Dimensions {
	p := &u.P
	if !disabled && s.click.Clicked(gtx) && value != nil {
		*value = !*value
	}
	on := value != nil && *value
	return s.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		d := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if disabled {
					macro := op.Record(gtx.Ops)
					d := s.track(gtx, u, on)
					call := macro.Stop()
					fillBox(gtx.Ops, boxOf(d.Size, float32(d.Size.Y)/2), withAlpha(p.ControlEdge, .9))
					_ = call
					k := px(gtx, 24)
					x := float32(d.Size.X) - k - px(gtx, 5)
					kb := box{rect: frect{Min: f32.Pt(x, (float32(d.Size.Y)-k)/2), Max: f32.Pt(x+k, (float32(d.Size.Y)+k)/2)}, radius: k / 2}
					fillBox(gtx.Ops, kb, p.Paper)
					return d
				}
				return s.track(gtx, u, on)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(9)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !disabled {
					return layout.Dimensions{}
				}
				return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return glyph(gtx, "lock", 12, p.Ink)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, caption, textStyle{size: 12.5, weight: font.SemiBold, color: p.Ink, maxLines: 1})
			}))
		if !disabled {
			pointerHand(gtx, d.Size)
		}
		return d
	})
}

var qualityGlyphCache = map[[2]int][]vshape{}

// qualityGlyph is QualityTileGlyph: a screen with 2 or 3 scanlines, plus a
// motion arrow above 30 fps (viewBox 28×38).
func qualityGlyph(gtx layout.Context, lines int, fast bool, c color.NRGBA) layout.Dimensions {
	key := [2]int{lines, 0}
	if fast {
		key[1] = 1
	}
	shapes, ok := qualityGlyphCache[key]
	if !ok {
		fragment := `<rect x="2" y="3" width="24" height="26" rx="4" stroke-width="2.2"/>`
		for i := 0; i < lines; i++ {
			fragment += fmt.Sprintf(`<path d="M6 %dh16" stroke-width="1.6"/>`, 10+i*5)
		}
		if fast {
			fragment += `<path d="M7 33h4m3 0h7m-3-2 3 2-3 2" stroke-width="1.6"/>`
		}
		shapes, _ = parseFragment(fragment, false)
		qualityGlyphCache[key] = shapes
	}
	side := px(gtx, 38)
	drawVector(gtx.Ops, shapes, 38, side, f32.Pt(px(gtx, 5)-px(gtx, 5), 0), 2, c)
	return layout.Dimensions{Size: image.Pt(int(px(gtx, 28)), int(px(gtx, 36)))}
}
