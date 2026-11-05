package nativeui

import (
	"image"
	"image/color"
	"math"
	"sync"
	"time"
	"unicode/utf16"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

// Scene pieces of the host room (ROOM-SPEC.md): TV, couch, pawns, LCD code.

var (
	couchColor     = rgb(0xe9ad63)
	couchDarkColor = rgb(0xcd8845)
	tvFrameColor   = rgb(0x1d2633)
	tvHighlight    = rgb(0x3e4b5b)
	tvInk          = rgb(0xdfe8f2)
	youColor       = rgb(0x2fa66a)
	personInk      = rgb(0x263b43)
	hostColor      = color.NRGBA{R: 164, G: 116, B: 213, A: 255}
	welcomeStar    = rgb(0xd7982b)
)

// participantColor is participant-color.ts: identity alone owns the colour.
func participantColor(identity string) color.NRGBA {
	seed := uint32(2166136261)
	for _, r := range identity {
		unit := uint32(r)
		if r > 0xffff {
			high, _ := utf16.EncodeRune(r)
			unit = uint32(high)
		}
		seed = (seed ^ unit) * 16777619
	}
	seed = (seed ^ (seed >> 16)) * 0x85ebca6b
	seed = (seed ^ (seed >> 13)) * 0xc2b2ae35
	seed = seed ^ (seed >> 16)
	hue := math.Mod(140+float64(seed&0xffff)/0xffff*250, 360)
	lightness := 0.59 + float64((seed>>16)&255)/255*0.18
	chroma := 0.11 + float64(seed>>24)/255*0.08
	round4 := func(v float64) float64 { return math.Round(v*10000) / 10000 }
	return oklchToRGB(round4(lightness), round4(chroma), round4(hue))
}

func oklchToRGB(lightness, chroma, hue float64) color.NRGBA {
	angle := hue * math.Pi / 180
	cos, sin := math.Cos(angle), math.Sin(angle)
	linear := func(c float64) [3]float64 {
		a, b := c*cos, c*sin
		l := math.Pow(lightness+.3963377773761749*a+.2158037573099136*b, 3)
		m := math.Pow(lightness-.1055613458156586*a-.0638541728258133*b, 3)
		s := math.Pow(lightness-.0894841775298119*a-1.2914855480194092*b, 3)
		return [3]float64{
			4.076741636075958*l - 3.3077115392580616*m + .2309699031821043*s,
			-1.2684379732850317*l + 2.6097573492876887*m - .34131937600265727*s,
			-.004196076138675564*l - .7034186179359363*m + 1.7076146940746117*s,
		}
	}
	inGamut := func(c [3]float64) bool {
		for _, v := range c {
			if v < 0 || v > 1 {
				return false
			}
		}
		return true
	}
	rgbValue := linear(chroma)
	if !inGamut(rgbValue) {
		low, high := 0.0, chroma
		for step := 0; step < 12; step++ {
			middle := (low + high) / 2
			if candidate := linear(middle); inGamut(candidate) {
				low, rgbValue = middle, candidate
			} else {
				high = middle
			}
		}
	}
	var out [3]uint8
	for i, channel := range rgbValue {
		var v float64
		if channel <= .0031308 {
			v = 12.92 * channel
		} else {
			v = 1.055*math.Pow(channel, 1/2.4) - .055
		}
		out[i] = uint8(math.Max(0, math.Min(255, math.Round(255*v))))
	}
	return color.NRGBA{R: out[0], G: out[1], B: out[2], A: 255}
}

// fillShapes paints parsed SVG shapes in one colour at scale/origin.
func fillShapes(ops *op.Ops, shapes []vshape, scale float32, origin f32.Point, c color.NRGBA) {
	for _, shape := range shapes {
		spec := pathSpec(ops, shape.segs, scale, origin)
		stack := clip.Outline{Path: spec}.Op().Push(ops)
		paint.ColorOp{Color: c}.Add(ops)
		paint.PaintOp{}.Add(ops)
		stack.Pop()
	}
}

func strokeShapes(ops *op.Ops, shapes []vshape, scale float32, origin f32.Point, width float32, c color.NRGBA) {
	for _, shape := range shapes {
		spec := pathSpec(ops, shape.segs, scale, origin)
		stack := clip.Stroke{Path: spec, Width: width * scale}.Op().Push(ops)
		paint.ColorOp{Color: c}.Add(ops)
		paint.PaintOp{}.Add(ops)
		stack.Pop()
	}
}

func mustShapes(fragment string) []vshape {
	shapes, err := parseFragment(fragment, true)
	if err != nil {
		panic(err)
	}
	return shapes
}

type couchArt struct {
	legs, back, seat, front, seams, arms []vshape
}

var couchShapes = sync.OnceValue(func() couchArt {
	return couchArt{
		legs:  mustShapes(`<path d="M38 98v7m204 -7v7"/>`),
		back:  mustShapes(`<rect x="18" y="32" width="244" height="65" rx="24"/>`),
		seat:  mustShapes(`<rect x="28" y="24" width="224" height="60" rx="23"/>`),
		front: mustShapes(`<rect x="28" y="76" width="224" height="24" rx="10"/>`),
		seams: mustShapes(`<path d="M93.33 79v16m93.33 -16v16"/>`),
		arms:  mustShapes(`<rect x="14" y="59" width="25" height="39" rx="12"/><rect x="241" y="59" width="25" height="39" rx="12"/>`),
	}
})

// drawCouch paints the 280×108 benches at origin, scale px per unit.
func drawCouch(ops *op.Ops, origin f32.Point, scale float32) {
	art := couchShapes()
	roundStroke(ops, art.legs, scale, origin, 6, tvFrameColor)
	fillShapes(ops, art.back, scale, origin, couchDarkColor)
	fillShapes(ops, art.seat, scale, origin, couchColor)
	fillShapes(ops, art.front, scale, origin, couchColor)
	roundStroke(ops, art.seams, scale, origin, 2, couchDarkColor)
	fillShapes(ops, art.arms, scale, origin, couchDarkColor)
}

// roundStroke strokes line segments with round caps (stroke + end dots).
func roundStroke(ops *op.Ops, shapes []vshape, scale float32, origin f32.Point, width float32, c color.NRGBA) {
	strokeShapes(ops, shapes, scale, origin, width, c)
	for _, shape := range shapes {
		for _, s := range shape.segs {
			for _, p := range []f32.Point{s.pts[0]} {
				if s.op != 'L' && s.op != 'M' {
					continue
				}
				dot := pathSpec(ops, ellipseSegments(p.X, p.Y, width/2, width/2), scale, origin)
				stack := clip.Outline{Path: dot}.Op().Push(ops)
				paint.ColorOp{Color: c}.Add(ops)
				paint.PaintOp{}.Add(ops)
				stack.Pop()
			}
		}
	}
}

type personArt struct {
	body, eyes, crown, crownLine []vshape
}

var personShapes = sync.OnceValue(func() personArt {
	return personArt{
		body:      mustShapes(`<circle cx="24" cy="14" r="8"/><path d="M10 47c0-13 5-20 14-20s14 7 14 20q0 3-3 3H13q-3 0-3-3Z"/>`),
		eyes:      mustShapes(`<ellipse cx="21.2" cy="14.1" rx="1.1" ry="1.5"/><ellipse cx="26.8" cy="14.1" rx="1.1" ry="1.5"/>`),
		crown:     mustShapes(`<path d="m1.2 6.3-1.2-3.7q-.2-.7.5-.4L3 3.6l2.4-2.5q.6-.7 1.2 0L9 3.6l2.5-1.4q.7-.3.5.4l-1.2 3.7q-.2.9-1.2.9H2.4q-1 0-1.2-.9Z"/>`),
		crownLine: mustShapes(`<path d="M2.1 6.5h7.8"/>`),
	}
})

type pawn struct {
	name    string
	color   color.NRGBA
	host    bool
	you     bool
	ready   bool // viewer has the picture: green status dot
	stutter bool // recent playback stutter: amber dot (state.peer.stuttering)

	id       string
	reaction string            // reaction rising above the pawn
	progress float32           // 0..1 of the reaction's lifetime
	click    *widget.Clickable // opens the person menu
	mic      int               // room voice: 0 none, 1 microphone on, 2 speaking
	remote   bool              // controls this computer (remote control)
	gifts    []floating        // reactions others sent this person
	tip      string            // hint: name, voice state (overflow text + personHint)
	tipComic string
	tipAt    *time.Time
}

// layoutPawn is .lr-pawn: person 40×48 (viewBox 48×56), "you" marker and name tag.
func layoutPawn(gtx layout.Context, u *UI, p pawn) layout.Dimensions {
	width := int(px(gtx, 48))
	scale := px(gtx, 40) / 48
	personH := int(px(gtx, 48))
	art := personShapes()
	origin := f32.Pt((float32(width)-px(gtx, 40))/2, 0)
	if p.you {
		// 13×10 triangle, 13px above the pawn.
		w, h := px(gtx, 13), px(gtx, 10)
		x := float32(width)/2 - w/2
		y := -px(gtx, 13)
		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(f32.Pt(x, y))
		path.LineTo(f32.Pt(x+w, y))
		path.LineTo(f32.Pt(x+w/2, y+h))
		path.Close()
		paint.FillShape(gtx.Ops, youColor, clip.Outline{Path: path.End()}.Op())
	}
	fillShapes(gtx.Ops, art.body, scale, origin, p.color)
	if p.ready {
		// .lr-person-status dot at the head's upper right.
		dot := box{rect: frect{Min: f32.Pt(origin.X+33*scale, 0), Max: f32.Pt(origin.X+41*scale, 8*scale)}, radius: 4 * scale}
		dotColor := youColor
		if p.stutter {
			dotColor = rgb(0xd98e04)
		}
		fillBox(gtx.Ops, dot, dotColor)
		strokeBox(gtx.Ops, dot, 1.5*scale, personInk)
	}
	fillShapes(gtx.Ops, art.eyes, scale, origin, personInk)
	if p.host {
		crownOrigin := origin.Add(f32.Pt(18*scale, 0))
		fillShapes(gtx.Ops, art.crown, scale, crownOrigin, rgb(0xf7d861))
		strokeShapes(gtx.Ops, art.crown, scale, crownOrigin, 1, rgb(0x705025))
		strokeShapes(gtx.Ops, art.crownLine, scale, crownOrigin, 1, rgb(0xb58a2f))
	}
	// Name tag: 11px/850 on paper, radius 7, padding 1 6, margin-top -4.
	macro := op.Record(gtx.Ops)
	tagGtx := gtx
	// The tag never spills into the next seat (Couch: names ellipsize in their cell).
	tagGtx.Constraints = layout.Constraints{Max: image.Pt(min(int(px(gtx, 120)), max(gtx.Constraints.Max.X-int(px(gtx, 12)), int(px(gtx, 36)))), int(px(gtx, 40)))}
	label := u.label(tagGtx, p.name, textStyle{size: 11, weight: font.ExtraBold, color: u.P.Ink, maxLines: 1})
	call := macro.Stop()
	tagW := label.Size.X + int(px(gtx, 12))
	tagH := label.Size.Y + int(px(gtx, 2))
	tagX := (width - tagW) / 2
	tagY := personH - int(px(gtx, 4))
	tag := box{rect: frect{Min: f32.Pt(float32(tagX), float32(tagY)), Max: f32.Pt(float32(tagX+tagW), float32(tagY+tagH))}, radius: px(gtx, 7)}
	fillBox(gtx.Ops, tag, u.P.Paper)
	offset := op.Offset(image.Pt(tagX+int(px(gtx, 6)), tagY+int(px(gtx, 1)))).Push(gtx.Ops)
	call.Add(gtx.Ops)
	offset.Pop()
	// Badges left of the head: voice microphone, remote control.
	badge := func(y int, icon string, ink color.NRGBA) {
		side := int(px(gtx, 16))
		b := boxOf(image.Pt(side, side), float32(side)/2)
		o := op.Offset(image.Pt(-int(px(gtx, 4)), y)).Push(gtx.Ops)
		fillBox(gtx.Ops, b, u.P.Paper)
		strokeBox(gtx.Ops, b, px(gtx, 1), u.P.ControlEdge)
		g := op.Offset(image.Pt(int(px(gtx, 2)), int(px(gtx, 2)))).Push(gtx.Ops)
		glyph(gtx, icon, 12, ink)
		g.Pop()
		o.Pop()
	}
	if p.mic > 0 {
		ink := u.P.Ink
		if p.mic == 2 {
			ink = u.P.Live
		}
		badge(0, "microphone", ink)
	}
	if p.remote {
		badge(int(px(gtx, 18)), "mouse", u.P.Action)
	}
	return layout.Dimensions{Size: image.Pt(max(width, tagW), tagY+tagH)}
}

// couchSeat is the grid cell of the web couch (Couch.tsx): 72 px per seat,
// up to 8 a row, rows added as people arrive.
const couchSeat, couchRowH = 72, 108

// couchGrid is Couch's capacity/rows/columns/width for count people.
func couchGrid(containerDp float32, count int) (rows, columns int, widthDp float32) {
	capacity := max(2, min(8, int((containerDp-64)/couchSeat)))
	rows = max(1, (count+capacity-1)/capacity)
	columns = max(1, (count+rows-1)/rows)
	widthDp = min(containerDp, max(280, float32(columns*couchSeat+64)))
	return
}

// drawBench is one couch row (.lr-couch-benches svg) of width w (dp).
func drawBench(gtx layout.Context, w float32, origin f32.Point) {
	s := px(gtx, 1)
	at := func(x, y, bw, bh, r float32) box {
		return boxOf(image.Pt(int(bw*s), int(bh*s)), r*s).offset(origin.X+x*s, origin.Y+y*s)
	}
	leg := func(x float32) {
		fillBox(gtx.Ops, at(x-3, 95, 6, 13, 3), tvFrameColor)
	}
	leg(38)
	leg(w - 38)
	fillBox(gtx.Ops, at(18, 32, w-36, 65, 24), couchDarkColor)
	fillBox(gtx.Ops, at(28, 24, w-56, 60, 23), couchColor)
	fillBox(gtx.Ops, at(28, 76, w-56, 24, 10), couchColor)
	for _, x := range []float32{w / 3, 2 * w / 3} {
		fillBox(gtx.Ops, at(x-1, 79, 2, 16, 1), couchDarkColor)
	}
	fillBox(gtx.Ops, at(14, 59, 25, 39, 12), couchDarkColor)
	fillBox(gtx.Ops, at(w-39, 59, 25, 39, 12), couchDarkColor)
}

// layoutCouch is .lr-couch: benches with the pawns seated, then the count pill.
func layoutCouch(gtx layout.Context, u *UI, pawns []pawn, throws []throwFlight, footer layout.Widget) layout.Dimensions {
	width := gtx.Constraints.Max.X
	rows, columns, benchDp := couchGrid(float32(width)/px(gtx, 1), len(pawns))
	benchW := int(px(gtx, benchDp))
	x := (width - benchW) / 2
	rowH := px(gtx, couchRowH)
	for row := 0; row < rows; row++ {
		drawBench(gtx, benchDp, f32.Pt(float32(x), float32(row)*rowH))
	}
	// Seats: columns over the cushion; short rows centred (Couch seatStyle).
	cellW := (float32(benchW) - px(gtx, 64)) / float32(columns)
	seatAt := func(i int) (float32, float32) {
		row := i / columns
		inRow := min(columns, len(pawns)-row*columns)
		left := float32(x) + px(gtx, 32) + float32(columns-inRow)*cellW/2
		return left + float32(i%columns)*cellW + cellW/2, float32(row) * rowH
	}
	for i, p := range pawns {
		cx, top := seatAt(i)
		o := op.Offset(image.Pt(int(cx-px(gtx, 24)), int(top+px(gtx, 24)))).Push(gtx.Ops)
		seat := gtx
		seat.Constraints.Max.X = int(cellW)
		pd := layoutPawn(seat, u, p)
		if p.click != nil {
			p.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				pointerHand(gtx, pd.Size)
				return pd
			})
		}
		if p.tipAt != nil {
			hovered := p.click != nil && p.click.Hovered()
			*p.tipAt = hoverTooltipComic(gtx, u, hovered, *p.tipAt, p.tip, p.tipComic, pd.Size)
		}
		if p.reaction != "" {
			drawFloatingReaction(gtx, p.reaction, p.progress, int(px(gtx, 24)), -int(px(gtx, 14)))
		}
		// Gifts rise beside the person's own expression.
		for g, gift := range p.gifts {
			drawFloatingReaction(gtx, gift.id, gift.progress, int(px(gtx, 24)+px(gtx, 22)*float32(g+1)), -int(px(gtx, 6)))
		}
		o.Pop()
	}
	// Throws arc from the sender's seat to the target's (RoomReaction flight).
	for _, flight := range throws {
		if flight.from >= len(pawns) || flight.to >= len(pawns) {
			continue
		}
		fx, fy := seatAt(flight.from)
		tx, ty := seatAt(flight.to)
		cx := fx + (tx-fx)*flight.t
		arc := px(gtx, 34) * 4 * flight.t * (1 - flight.t)
		cy := fy + (ty-fy)*flight.t + px(gtx, 38) - arc
		side := px(gtx, 26)
		o := op.Offset(image.Pt(int(cx-side/2), int(cy-side/2))).Push(gtx.Ops)
		spin := flight.t * 6.28
		r := op.Affine(rotateAbout(side/2, side/2, spin)).Push(gtx.Ops)
		reactionIcon(gtx, flight.reaction, 26)
		r.Pop()
		o.Pop()
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(16 * time.Millisecond)})
	}
	// Footer: count pill 4px under the benches.
	footerY := int(float32(rows)*rowH) + int(px(gtx, 4))
	macro := op.Record(gtx.Ops)
	pill := footer(gtx)
	call := macro.Stop()
	o := op.Offset(image.Pt((width-pill.Size.X)/2, footerY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	return layout.Dimensions{Size: image.Pt(width, footerY+max(pill.Size.Y, int(px(gtx, 28))))}
}
func countPill(gtx layout.Context, u *UI, count int) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	macro := op.Record(gtx.Ops)
	content := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "users", 14, u.P.Ink) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, itoa(count), textStyle{size: 11, weight: font.Bold, mono: true, color: u.P.Ink})
		}))
	call := macro.Stop()
	padX, padY := int(px(gtx, 9)), int(px(gtx, 5))
	size := image.Pt(content.Size.X+2*padX, content.Size.Y+2*padY)
	shape := boxOf(size, float32(size.Y)/2)
	fillBox(gtx.Ops, shape, u.P.Paper)
	strokeBox(gtx.Ops, shape, px(gtx, 1), u.P.Wall2)
	o := op.Offset(image.Pt(padX, padY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	return layout.Dimensions{Size: size}
}

// layoutTV is .lr-tv: frame (padding 12 12 0), 16:9 screen, chin 26 with the
// status indicator. screen draws inside the screen rectangle.
func layoutTV(gtx layout.Context, u *UI, maxWidth int, screen layout.Widget, chin layout.Widget) layout.Dimensions {
	width := min(gtx.Constraints.Max.X, maxWidth)
	pad := int(px(gtx, 15)) // 3 border + 12 padding
	screenW := width - 2*pad
	screenH := screenW * 9 / 16
	chinH := int(px(gtx, 26))
	height := pad + screenH + chinH + int(px(gtx, 3))
	size := image.Pt(width, height)
	frame := box{rect: frect{Max: f32.Pt(float32(width), float32(height))}, radius: px(gtx, 26)}
	// shadow 0 2 0 shade(.38) + 0 18 35 shade(.22)
	shade := u.P.Shade
	shadow(gtx.Ops, frame, px(gtx, 18), px(gtx, 35), withAlpha(shade, .22))
	shadow(gtx.Ops, frame, px(gtx, 2), 0, withAlpha(shade, .38))
	fillTVFrame(gtx.Ops, frame)
	strokeBox(gtx.Ops, frame, px(gtx, 3), tvFrameColor)
	// inset highlight line
	hl := frame.inset(px(gtx, 3))
	hl.rect.Max.Y = hl.rect.Min.Y + px(gtx, 1)
	hl.radius = 0
	hl.rect.Min.X += px(gtx, 20)
	hl.rect.Max.X -= px(gtx, 20)
	fillBox(gtx.Ops, hl, rgba(255, 255, 255, .16))

	screenBox := box{rect: frect{Min: f32.Pt(float32(pad), float32(pad)), Max: f32.Pt(float32(pad+screenW), float32(pad+screenH))}, radius: px(gtx, 14)}
	screenColor := rgb(0x17191c)
	if u.P.Dark {
		screenColor = rgb(0x141517)
	}
	fillScreen(gtx.Ops, screenBox, px(gtx, 14), px(gtx, 8), screenColor)
	o := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
	sgtx := gtx
	sgtx.Constraints = layout.Exact(image.Pt(screenW, screenH))
	clipStack := clip.Rect{Max: image.Pt(screenW, screenH)}.Push(gtx.Ops)
	screen(sgtx)
	clipStack.Pop()
	o.Pop()
	o = op.Offset(image.Pt(pad, pad+screenH)).Push(gtx.Ops)
	cgtx := gtx
	cgtx.Constraints = layout.Exact(image.Pt(screenW, chinH))
	chin(cgtx)
	o.Pop()
	return layout.Dimensions{Size: size}
}

// fillTVFrame paints linear-gradient(155deg, #3e4b5b, #1d2633 75%).
func fillTVFrame(ops *op.Ops, b box) {
	defer clip.Outline{Path: frameSpec(ops, b, b.radius, b.radius*20/26)}.Op().Push(ops).Pop()
	angle := 155.0 * math.Pi / 180
	dir := f32.Pt(float32(math.Sin(angle)), float32(-math.Cos(angle)))
	center := b.rect.Min.Add(b.rect.Max).Mul(0.5)
	half := (abs32(dir.X)*b.rect.Dx() + abs32(dir.Y)*b.rect.Dy()) / 2
	start := center.Sub(dir.Mul(half))
	end := center.Add(dir.Mul(half * 0.5)) // 75% stop: start + 1.5*half
	paint.LinearGradientOp{Stop1: start, Color1: tvHighlight, Stop2: end, Color2: tvFrameColor}.Add(ops)
	paint.PaintOp{}.Add(ops)
}

// frameSpec is a rounded rectangle with top radius top and bottom radius bottom.
func frameSpec(ops *op.Ops, b box, top, bottom float32) clip.PathSpec {
	r := b.rect
	var path clip.Path
	path.Begin(ops)
	const k = 0.5523
	path.MoveTo(f32.Pt(r.Min.X+top, r.Min.Y))
	path.LineTo(f32.Pt(r.Max.X-top, r.Min.Y))
	path.CubeTo(f32.Pt(r.Max.X-top+top*k, r.Min.Y), f32.Pt(r.Max.X, r.Min.Y+top-top*k), f32.Pt(r.Max.X, r.Min.Y+top))
	path.LineTo(f32.Pt(r.Max.X, r.Max.Y-bottom))
	path.CubeTo(f32.Pt(r.Max.X, r.Max.Y-bottom+bottom*k), f32.Pt(r.Max.X-bottom+bottom*k, r.Max.Y), f32.Pt(r.Max.X-bottom, r.Max.Y))
	path.LineTo(f32.Pt(r.Min.X+bottom, r.Max.Y))
	path.CubeTo(f32.Pt(r.Min.X+bottom-bottom*k, r.Max.Y), f32.Pt(r.Min.X, r.Max.Y-bottom+bottom*k), f32.Pt(r.Min.X, r.Max.Y-bottom))
	path.LineTo(f32.Pt(r.Min.X, r.Min.Y+top))
	path.CubeTo(f32.Pt(r.Min.X, r.Min.Y+top-top*k), f32.Pt(r.Min.X+top-top*k, r.Min.Y), f32.Pt(r.Min.X+top, r.Min.Y))
	path.Close()
	return path.End()
}

func fillScreen(ops *op.Ops, b box, top, bottom float32, c color.NRGBA) {
	defer clip.Outline{Path: frameSpec(ops, b, top, bottom)}.Op().Push(ops).Pop()
	paint.ColorOp{Color: c}.Add(ops)
	paint.PaintOp{}.Add(ops)
}

// RoundAction is .lr-tv-big: an 84×84 round button with a 34px glyph and a
// caption under it.
type RoundAction struct {
	click  widget.Clickable
	lift   tween
	icon   string    // last icon shown
	shown  time.Time // when it appeared (draw-in start)
	seenAt time.Time // last frame drawn
}

func (r *RoundAction) Clicked(gtx layout.Context) bool { return r.click.Clicked(gtx) }

func (r *RoundAction) Layout(gtx layout.Context, u *UI, icon, caption string, primary, disabled bool) layout.Dimensions {
	side := int(px(gtx, 84))
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			hovered := !disabled && r.click.Hovered()
			target := float32(0)
			if hovered {
				target = 1
			}
			grow := r.lift.set(gtx, target, 260e6, spring)
			return liftClick(gtx, &r.click, !disabled, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(side, side)
				inflate := grow * px(gtx, 3)
				shape := box{rect: frect{Min: f32.Pt(-inflate, -inflate), Max: f32.Pt(float32(side)+inflate, float32(side)+inflate)}, radius: float32(side)/2 + inflate}
				alpha := float32(1)
				if disabled {
					alpha = .45
				}
				if primary {
					fillBox(gtx.Ops, shape, withAlpha(u.P.Primary, alpha))
					strokeBox(gtx.Ops, shape, px(gtx, 3), withAlpha(u.P.Primary, alpha))
				} else {
					fillBox(gtx.Ops, shape, rgba(255, 255, 255, .07*alpha))
					strokeBox(gtx.Ops, shape, px(gtx, 3), withAlpha(tvInk, alpha))
				}
				g := int(px(gtx, 34))
				o := op.Offset(image.Pt((side-g)/2, (side-g)/2)).Push(gtx.Ops)
				// A new icon, or one shown again after a while, draws itself in.
				if icon != r.icon || gtx.Now.Sub(r.seenAt) > time.Second {
					r.icon, r.shown = icon, gtx.Now
				}
				r.seenAt = gtx.Now
				glyphDrawIn(gtx, icon, 34, withAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 255}, alpha), r.shown)
				o.Pop()
				if !disabled {
					pointerHand(gtx, size)
				}
				return layout.Dimensions{Size: size}
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return tvMessage(gtx, u, caption, 14.5)
		}))
}

// tvMessage is .lr-tv-msg: 600 #dfe8f2 with a soft dark shadow.
func tvMessage(gtx layout.Context, u *UI, s string, size float32) layout.Dimensions {
	style := textStyle{size: size, weight: font.SemiBold, color: rgba(0, 0, 0, .35), align: text.Middle}
	macro := op.Record(gtx.Ops)
	o := op.Offset(image.Pt(0, int(px(gtx, 1)))).Push(gtx.Ops)
	u.label(gtx, s, style)
	o.Pop()
	call := macro.Stop()
	call.Add(gtx.Ops)
	style.color = tvInk
	return u.label(gtx, s, style)
}

// LCD is the room code display (.lr-lcd): empty dashed slot or green digits.
type LCD struct {
	click widget.Clickable
}

func (l *LCD) Clicked(gtx layout.Context) bool { return l.click.Clicked(gtx) }

func (l *LCD) Layout(gtx layout.Context, u *UI, code string, busy bool) layout.Dimensions {
	return l.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := image.Pt(int(px(gtx, 173)), int(px(gtx, 58)))
		shape := boxOf(size, px(gtx, 12))
		digits := []rune("----")
		digitColor := u.P.Muted
		action := "plus"
		if code != "" {
			digits = []rune(code)
			bg := rgb(0x1d2b22)
			if u.P.Dark {
				bg = rgb(0x0d1611)
			}
			fillBox(gtx.Ops, shape, bg)
			strokeBox(gtx.Ops, shape, px(gtx, 3), u.P.Ink)
			digitColor = rgb(0x9de8bf)
			action = "copy"
		} else {
			fillBox(gtx.Ops, shape, u.P.Wall2)
			dashedBox(gtx.Ops, shape.inset(px(gtx, 1.5)), px(gtx, 3), px(gtx, 9), px(gtx, 6), u.P.ControlEdge)
		}
		x := int(px(gtx, 15))
		for _, d := range digits {
			cell := int(px(gtx, 25))
			o := op.Offset(image.Pt(x, int(px(gtx, 11)))).Push(gtx.Ops)
			cgtx := gtx
			cgtx.Constraints = layout.Exact(image.Pt(cell, int(px(gtx, 36))))
			layout.Center.Layout(cgtx, func(gtx layout.Context) layout.Dimensions {
				if d == '-' {
					w, h := int(px(gtx, 14)), int(px(gtx, 3))
					r := image.Rect(0, 0, w, h)
					defer clip.Rect(r).Push(gtx.Ops).Pop()
					paint.ColorOp{Color: digitColor}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)
					return layout.Dimensions{Size: image.Pt(w, h)}
				}
				return u.label(gtx, string(d), textStyle{size: 24, weight: font.Bold, mono: true, color: digitColor})
			})
			o.Pop()
			x += cell
		}
		if busy {
			action = "loader"
		}
		g := int(px(gtx, 16))
		o := op.Offset(image.Pt(int(px(gtx, 125))+(int(px(gtx, 34))-g)/2, (size.Y-g)/2)).Push(gtx.Ops)
		glyph(gtx, action, 16, digitColor)
		o.Pop()
		pointerHand(gtx, size)
		return layout.Dimensions{Size: size}
	})
}

// dashedBox strokes a rounded rectangle outline with dashes (border: dashed).
func dashedBox(ops *op.Ops, b box, width, dash, gap float32, c color.NRGBA) {
	r := b.rect
	rad := min(b.radius, r.Dx()/2, r.Dy()/2)
	// Straight edges dashed, corners solid arcs.
	var path clip.Path
	path.Begin(ops)
	line := func(from, to f32.Point) {
		d := to.Sub(from)
		length := float32(math.Hypot(float64(d.X), float64(d.Y)))
		if length <= 0 {
			return
		}
		unitV := d.Mul(1 / length)
		for s := float32(0); s < length; s += dash + gap {
			e := min(s+dash, length)
			path.MoveTo(from.Add(unitV.Mul(s)))
			path.LineTo(from.Add(unitV.Mul(e)))
		}
	}
	line(f32.Pt(r.Min.X+rad, r.Min.Y), f32.Pt(r.Max.X-rad, r.Min.Y))
	line(f32.Pt(r.Max.X, r.Min.Y+rad), f32.Pt(r.Max.X, r.Max.Y-rad))
	line(f32.Pt(r.Max.X-rad, r.Max.Y), f32.Pt(r.Min.X+rad, r.Max.Y))
	line(f32.Pt(r.Min.X, r.Max.Y-rad), f32.Pt(r.Min.X, r.Min.Y+rad))
	const k = 0.5523
	corner := func(start, c1, c2, end f32.Point) {
		path.MoveTo(start)
		path.CubeTo(c1, c2, end)
	}
	corner(f32.Pt(r.Max.X-rad, r.Min.Y), f32.Pt(r.Max.X-rad+rad*k, r.Min.Y), f32.Pt(r.Max.X, r.Min.Y+rad-rad*k), f32.Pt(r.Max.X, r.Min.Y+rad))
	corner(f32.Pt(r.Max.X, r.Max.Y-rad), f32.Pt(r.Max.X, r.Max.Y-rad+rad*k), f32.Pt(r.Max.X-rad+rad*k, r.Max.Y), f32.Pt(r.Max.X-rad, r.Max.Y))
	corner(f32.Pt(r.Min.X+rad, r.Max.Y), f32.Pt(r.Min.X+rad-rad*k, r.Max.Y), f32.Pt(r.Min.X, r.Max.Y-rad+rad*k), f32.Pt(r.Min.X, r.Max.Y-rad))
	corner(f32.Pt(r.Min.X, r.Min.Y+rad), f32.Pt(r.Min.X, r.Min.Y+rad-rad*k), f32.Pt(r.Min.X+rad-rad*k, r.Min.Y), f32.Pt(r.Min.X+rad, r.Min.Y))
	spec := path.End()
	defer clip.Stroke{Path: spec, Width: width}.Op().Push(ops).Pop()
	paint.ColorOp{Color: c}.Add(ops)
	paint.PaintOp{}.Add(ops)
}

// nameTag is .lr-name-tag: paper pill, 2px ink border, colour dot and name.
func nameTag(gtx layout.Context, u *UI, name string, dot color.NRGBA) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	content := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			s := int(px(gtx, 9))
			b := boxOf(image.Pt(s, s), float32(s)/2)
			fillBox(gtx.Ops, b, dot)
			return layout.Dimensions{Size: image.Pt(s, s)}
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(7)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, name, textStyle{size: 13.5, weight: font.Bold, color: u.P.Ink, maxLines: 1})
		}))
	call := macro.Stop()
	padX := int(px(gtx, 15))
	size := image.Pt(max(content.Size.X+2*padX, int(px(gtx, 132))), int(px(gtx, 40)))
	shape := boxOf(size, float32(size.Y)/2)
	fillBox(gtx.Ops, shape, u.P.Paper)
	strokeBox(gtx.Ops, shape, px(gtx, 2), u.P.Ink)
	o := op.Offset(image.Pt(padX, (size.Y-content.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	return layout.Dimensions{Size: size}
}

// leds is .lr-leds: the server connection pill (3 dots + label).
func leds(gtx layout.Context, u *UI, label string, state string) layout.Dimensions {
	dot := u.P.Dots
	switch state {
	case "live":
		dot = u.P.Live
	case "busy":
		dot = u.P.Busy
	case "warn":
		dot = u.P.Warn
	case "danger":
		dot = u.P.Danger
	}
	macro := op.Record(gtx.Ops)
	content := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			s := int(px(gtx, 6))
			gap := int(px(gtx, 4))
			for i := 0; i < 3; i++ {
				b := boxOf(image.Pt(s, s), float32(s)/2).offset(float32(i*(s+gap)), 0)
				c := dot
				if state == "busy" {
					// marching pulse
					phase := float64(gtx.Now.UnixMilli()%1200)/1200*3 - float64(i)
					if phase < 0 || phase > 1 {
						c = withAlpha(dot, .35)
					}
					gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(100e6)})
				}
				fillBox(gtx.Ops, b, c)
			}
			return layout.Dimensions{Size: image.Pt(3*s+2*gap, s)}
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(11)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, label, textStyle{size: 11.5, weight: font.Bold, color: withAlpha(u.P.Ink, .8), maxLines: 1})
		}))
	call := macro.Stop()
	padX := int(px(gtx, 12))
	size := image.Pt(content.Size.X+2*padX, int(px(gtx, 33)))
	fillBox(gtx.Ops, boxOf(size, float32(size.Y)/2), u.P.Wall2)
	o := op.Offset(image.Pt(padX, (size.Y-content.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	return layout.Dimensions{Size: size}
}

// pointerHand shows the hand cursor over size (inside a Clickable).
func pointerHand(gtx layout.Context, size image.Point) {
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	pointer.CursorPointer.Add(gtx.Ops)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
