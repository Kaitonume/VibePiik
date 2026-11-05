package nativeui

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// frect is a float rectangle in pixels.
type frect struct{ Min, Max f32.Point }

func (r frect) Dx() float32           { return r.Max.X - r.Min.X }
func (r frect) Dy() float32           { return r.Max.Y - r.Min.Y }
func (r frect) Size() f32.Point       { return r.Max.Sub(r.Min) }
func (r frect) Add(p f32.Point) frect { return frect{Min: r.Min.Add(p), Max: r.Max.Add(p)} }

// box is a rectangle in pixels with a corner radius.
type box struct {
	rect   frect
	radius float32
}

func boxOf(size image.Point, radius float32) box {
	return box{rect: frect{Max: f32.Pt(float32(size.X), float32(size.Y))}, radius: radius}
}

func (b box) inset(d float32) box {
	return box{rect: frect{Min: b.rect.Min.Add(f32.Pt(d, d)), Max: b.rect.Max.Sub(f32.Pt(d, d))},
		radius: max(0, b.radius-d)}
}

func (b box) offset(dx, dy float32) box {
	b.rect = b.rect.Add(f32.Pt(dx, dy))
	return b
}

func (b box) path(ops *op.Ops) clip.PathSpec {
	size := b.rect.Size()
	return pathSpec(ops, roundRectSegments(0, 0, size.X, size.Y, min(b.radius, size.X/2, size.Y/2)), 1, b.rect.Min)
}

func fillBox(ops *op.Ops, b box, c color.NRGBA) {
	if c.A == 0 || b.rect.Dx() <= 0 || b.rect.Dy() <= 0 {
		return
	}
	defer clip.Outline{Path: b.path(ops)}.Op().Push(ops).Pop()
	paint.ColorOp{Color: c}.Add(ops)
	paint.PaintOp{}.Add(ops)
}

// strokeBox draws a border of width inside the box, like CSS border-box.
func strokeBox(ops *op.Ops, b box, width float32, c color.NRGBA) {
	if c.A == 0 || width <= 0 {
		return
	}
	inner := b.inset(width / 2)
	defer clip.Stroke{Path: inner.path(ops), Width: width}.Op().Push(ops).Pop()
	paint.ColorOp{Color: c}.Add(ops)
	paint.PaintOp{}.Add(ops)
}

// shadow approximates a CSS box-shadow (offset y, blur radius) with layered
// translucent boxes. Gio has no blur; at these small radii the steps blend.
func shadow(ops *op.Ops, b box, dy, blur float32, c color.NRGBA) {
	if c.A == 0 {
		return
	}
	if blur <= 0.5 {
		fillBox(ops, b.offset(0, dy), c)
		return
	}
	steps := int(math.Min(10, math.Max(3, float64(blur)/2)))
	for i := 0; i < steps; i++ {
		t := (float32(i) + 0.5) / float32(steps)
		spread := blur * (0.5 - t)
		layer := box{rect: frect{
			Min: b.rect.Min.Sub(f32.Pt(spread, spread)).Add(f32.Pt(0, dy)),
			Max: b.rect.Max.Add(f32.Pt(spread, spread)).Add(f32.Pt(0, dy)),
		}, radius: max(0, b.radius+spread)}
		fillBox(ops, layer, withAlpha(c, 1/float32(steps)))
	}
}

// controlShadow is --control-shadow: a hard 3px step plus a soft 10px blur.
func controlShadow(ops *op.Ops, p *Palette, b box, scale float32) {
	shadow(ops, b, 5*scale, 10*scale, withAlpha(p.Shade, .45))
	fillBox(ops, b.offset(0, 3*scale), p.Shade)
}

// panelShadow is --panel-shadow without its inset rim (drawn by glazeCard).
func panelShadow(ops *op.Ops, p *Palette, b box, scale float32) {
	shadow(ops, b, 16*scale, 36*scale, withAlpha(p.Shade, .55))
	fillBox(ops, b.offset(0, 3*scale), p.Shade)
}

// glaze paints --surface-glaze over a paper box: a 165° gradient.
func glaze(ops *op.Ops, p *Palette, b box) {
	defer clip.Outline{Path: b.path(ops)}.Op().Push(ops).Pop()
	paint.ColorOp{Color: p.Paper}.Add(ops)
	paint.PaintOp{}.Add(ops)
	angle := 165.0 * math.Pi / 180
	dir := f32.Pt(float32(math.Sin(angle)), float32(-math.Cos(angle)))
	center := b.rect.Min.Add(b.rect.Max).Mul(0.5)
	half := (abs32(dir.X)*b.rect.Dx() + abs32(dir.Y)*b.rect.Dy()) / 2
	start, end := center.Sub(dir.Mul(half)), center.Add(dir.Mul(half))
	paint.LinearGradientOp{Stop1: start, Color1: p.GlazeTop, Stop2: start.Add(end.Sub(start).Mul(.65)), Color2: color.NRGBA{}}.Add(ops)
	paint.PaintOp{}.Add(ops)
	paint.LinearGradientOp{Stop1: start.Add(end.Sub(start).Mul(.65)), Color1: color.NRGBA{}, Stop2: end, Color2: p.GlazeBottom}.Add(ops)
	paint.PaintOp{}.Add(ops)
}

// px converts CSS pixels to physical pixels.
func px(gtx layout.Context, v float32) float32 { return float32(gtx.Dp(unit.Dp(v*100))) / 100 }

func fillRect(ops *op.Ops, size image.Point, c color.NRGBA) {
	defer clip.Rect{Max: size}.Push(ops).Pop()
	paint.ColorOp{Color: c}.Add(ops)
	paint.PaintOp{}.Add(ops)
}
