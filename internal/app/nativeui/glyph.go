package nativeui

import (
	"image"
	"image/color"
	"sync"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

var (
	glyphMu    sync.Mutex
	glyphCache = map[string][]vshape{}
)

func glyphShapes(name string) []vshape {
	glyphMu.Lock()
	defer glyphMu.Unlock()
	if shapes, ok := glyphCache[name]; ok {
		return shapes
	}
	source, ok := glyphSources[name]
	if !ok {
		return nil
	}
	shapes, err := parseFragment(source.svg, source.solid)
	if err != nil {
		shapes = nil
	}
	glyphCache[name] = shapes
	return shapes
}

// drawVector paints shapes from a viewBox of size view at origin, scaled to
// side pixels. strokeWidth is in viewBox units.
func drawVector(ops *op.Ops, shapes []vshape, view, side float32, origin f32.Point, strokeWidth float32, c color.NRGBA) {
	scale := side / view
	for _, shape := range shapes {
		shapeColor := withAlpha(c, shape.opacity)
		width := strokeWidth
		if shape.strokeWidth > 0 {
			width = shape.strokeWidth
		}
		if len(shape.segs) > 0 {
			spec := pathSpec(ops, shape.segs, scale, origin)
			var stack clip.Stack
			if shape.fill {
				stack = clip.Outline{Path: spec}.Op().Push(ops)
			} else {
				stack = clip.Stroke{Path: spec, Width: width * scale}.Op().Push(ops)
			}
			paint.ColorOp{Color: shapeColor}.Add(ops)
			paint.PaintOp{}.Add(ops)
			stack.Pop()
		}
		for _, dot := range shape.dots {
			r := width / 2
			spec := pathSpec(ops, ellipseSegments(dot.X, dot.Y, r, r), scale, origin)
			stack := clip.Outline{Path: spec}.Op().Push(ops)
			paint.ColorOp{Color: shapeColor}.Add(ops)
			paint.PaintOp{}.Add(ops)
			stack.Pop()
		}
	}
}

// glyph lays out a Piik icon (24x24, stroke 2) at size CSS pixels.
func glyph(gtx layout.Context, name string, size float32, c color.NRGBA) layout.Dimensions {
	side := px(gtx, size)
	drawVector(gtx.Ops, glyphShapes(name), 24, side, f32.Point{}, 2, c)
	n := int(side + 0.5)
	return layout.Dimensions{Size: image.Pt(n, n)}
}

func init() {
	// The language switch's 中 without loading the Chinese fonts.
	glyphSources["zhong"] = glyphSource{svg: `<rect x="4.5" y="7.5" width="15" height="9" rx="1.5"/><path d="M12 3v18"/>`}
}

var brandShapes = sync.OnceValue(func() []vshape {
	// BrandMark.tsx at rest: frame, antenna, feet, left eye open, right eye winking.
	shapes, _ := parseFragment(`<rect x="4" y="8" width="24" height="18" rx="4"/>`+
		`<path d="m11 4 5 4 5-4"/><path d="M10 29l2-3m10 3-2-3"/>`+
		`<circle cx="11.5" cy="16.5" r="1.25" fill="currentColor" stroke="none"/>`+
		`<path d="M19 17c1.5-1.8 3.2-1.8 4.7 0"/>`, false)
	return shapes
})

// brandMark lays out the Piik mascot (viewBox 32, stroke 2.4) at size pixels.
func brandMark(gtx layout.Context, size float32, c color.NRGBA) layout.Dimensions {
	side := px(gtx, size)
	drawVector(gtx.Ops, brandShapes(), 32, side, f32.Point{}, 2.4, c)
	n := int(side + 0.5)
	return layout.Dimensions{Size: image.Pt(n, n)}
}

var brandOpenEye = sync.OnceValue(func() []vshape {
	shapes, _ := parseFragment(`<circle cx="21.35" cy="16.5" r="1.25" fill="currentColor" stroke="none"/>`, false)
	return shapes
})

// brandMarkOnce is BrandMark motion="once": the strokes draw in (a wipe from
// the feet up, 600 ms), the eyes open, then the right eye winks. since is
// when the play started; zero or animations off draws the mark at rest.
func brandMarkOnce(gtx layout.Context, size float32, c color.NRGBA, since time.Time) layout.Dimensions {
	elapsed := gtx.Now.Sub(since)
	if since.IsZero() || reducedMotion || elapsed >= 1100*time.Millisecond || len(brandShapes()) != 5 {
		return brandMark(gtx, size, c)
	}
	side := px(gtx, size)
	n := int(side + 0.5)
	gtx.Execute(op.InvalidateCmd{})
	draw := min(float32(elapsed)/float32(600*time.Millisecond), 1)
	draw = easeOut(draw)
	shapes := brandShapes()
	if draw < 1 {
		top := int(side * (1 - draw))
		stack := clip.Rect{Min: image.Pt(0, top), Max: image.Pt(n, n)}.Push(gtx.Ops)
		drawVector(gtx.Ops, shapes[:3], 32, side, f32.Point{}, 2, c) // frame, antenna, feet
		stack.Pop()
		return layout.Dimensions{Size: image.Pt(n, n)}
	}
	drawVector(gtx.Ops, shapes[:4], 32, side, f32.Point{}, 2, c)
	if elapsed < 900*time.Millisecond {
		drawVector(gtx.Ops, brandOpenEye(), 32, side, f32.Point{}, 2, c) // eyes open
	} else {
		drawVector(gtx.Ops, shapes[4:], 32, side, f32.Point{}, 2, c) // wink
	}
	return layout.Dimensions{Size: image.Pt(n, n)}
}

// glyphDrawIn is the icons' draw= entrance: the glyph wipes in from the left
// over 420 ms after since (zero or animations off: drawn at once).
func glyphDrawIn(gtx layout.Context, name string, size float32, c color.NRGBA, since time.Time) layout.Dimensions {
	elapsed := gtx.Now.Sub(since)
	if since.IsZero() || reducedMotion || elapsed >= 420*time.Millisecond {
		return glyph(gtx, name, size, c)
	}
	gtx.Execute(op.InvalidateCmd{})
	side := px(gtx, size)
	n := int(side + 0.5)
	reveal := easeOut(float32(elapsed) / float32(420*time.Millisecond))
	defer clip.Rect{Max: image.Pt(int(side*reveal+0.5), n)}.Push(gtx.Ops).Pop()
	return glyph(gtx, name, size, c)
}
