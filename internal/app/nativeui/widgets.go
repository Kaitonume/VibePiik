package nativeui

import (
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

type tone int

const (
	toneDefault tone = iota
	tonePrimary
	toneQuiet
	toneDanger
)

type buttonSize int

const (
	sizeDefault buttonSize = iota // 52px
	sizeCompact                   // 44px
	sizeDense                     // 36px
)

// Button is .lr-btn: a pill with the 3D step shadow that lifts on hover and
// sinks on press.
type Button struct {
	click   widget.Clickable
	lift    tween
	spin    float32
	hoverAt time.Time
}

type ButtonStyle struct {
	Icon     string
	Caption  string
	Tone     tone
	Size     buttonSize
	Pressed  bool // .is-on
	Disabled bool
	Busy     bool // spinning loader instead of the icon
	Fill     bool // stretch to the available width
	MinH     float32
	PadX     float32
	TextSize float32
	IconSize float32
	Trailing string // trailing glyph (disclosure chevron)
	TrailRot float32
	Tip      string // tooltip after 500 ms of hover (the caption in ✦ mode)
	Comic    string // comic kind shown in the tooltip (hint-*)
}

func (b *Button) Clicked(gtx layout.Context) bool { return b.click.Clicked(gtx) }

func (b *Button) Layout(gtx layout.Context, u *UI, s ButtonStyle) layout.Dimensions {
	p := &u.P
	height, pad, icon := float32(52), float32(14), float32(18)
	switch s.Size {
	case sizeCompact:
		height, pad = 44, 10
	case sizeDense:
		height, pad, icon = 36, 8, 16
	}
	if s.MinH > 0 {
		height = s.MinH
	}
	if s.PadX > 0 {
		pad = s.PadX
	}
	if s.IconSize > 0 {
		icon = s.IconSize
	}
	textSize := float32(12.5)
	if s.TextSize > 0 {
		textSize = s.TextSize
	}
	hovered := !s.Disabled && b.click.Hovered()
	pressed := !s.Disabled && b.click.Pressed()
	target := float32(0)
	duration := 260 * time.Millisecond
	switch {
	case pressed:
		target, duration = 2, 80*time.Millisecond
	case hovered:
		target = -2
	}
	lift := b.lift.set(gtx, target, duration, spring)

	background, border, ink := p.Paper, p.ControlEdge, p.Ink
	switch s.Tone {
	case tonePrimary:
		background, border, ink = p.Primary, p.Primary, p.PrimaryInk
		if hovered {
			background, border = p.PrimaryHover, p.PrimaryHover
		}
	case toneQuiet:
		background, border, ink = color.NRGBA{}, color.NRGBA{}, p.Muted
		if hovered {
			background, ink = p.ControlHover, p.Ink
		}
	case toneDanger:
		border, ink = p.Danger, p.DangerText
		if hovered {
			background = p.DangerHover
		}
	default:
		if hovered {
			background, border = p.ControlHover, p.Ink
		}
	}
	if s.Pressed {
		background, border, ink = p.ControlSelected, p.Action, p.ControlSelectedInk
	}
	alpha := float32(1)
	if s.Disabled {
		alpha = .4
	}

	// Measure the content.
	gap := px(gtx, 8)
	content := func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{}
		if s.Icon != "" || s.Busy {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				name := s.Icon
				if s.Busy {
					name = "loader"
				}
				if s.Busy {
					b.spin = float32(gtx.Now.UnixMilli()%900) / 900
					gtx.Execute(op.InvalidateCmd{})
					side := px(gtx, icon)
					center := f32.Pt(side/2, side/2)
					defer op.Affine(f32.Affine2D{}.Rotate(center, b.spin*6.2832)).Push(gtx.Ops).Pop()
				}
				return glyph(gtx, name, icon, withAlpha(ink, alpha))
			}))
		}
		if s.Caption != "" && !u.Visual() {
			if len(children) > 0 {
				children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout))
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, s.Caption, textStyle{size: textSize, weight: font.SemiBold, color: withAlpha(ink, alpha), maxLines: 1})
			}))
		}
		if s.Trailing != "" {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				side := px(gtx, 15)
				center := f32.Pt(side/2, side/2)
				defer op.Affine(f32.Affine2D{}.Rotate(center, s.TrailRot)).Push(gtx.Ops).Pop()
				return glyph(gtx, s.Trailing, 15, withAlpha(ink, alpha))
			}))
		}
		_ = gap
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
	}
	macro := op.Record(gtx.Ops)
	measure := gtx
	measure.Constraints.Min = image.Point{}
	cd := content(measure)
	call := macro.Stop()

	h := int(px(gtx, height))
	w := cd.Size.X + 2*int(px(gtx, pad))
	if s.Caption == "" || u.Visual() {
		w = max(w, h)
	}
	if s.Fill {
		w = gtx.Constraints.Max.X
	}
	w = max(w, gtx.Constraints.Min.X)
	size := image.Pt(w, h)
	radius := float32(h) / 2
	if s.Tone == toneQuiet {
		radius = px(gtx, 12)
	}
	return liftClick(gtx, &b.click, !s.Disabled, func(gtx layout.Context) layout.Dimensions {
		shape := boxOf(size, radius)
		if s.Tone != toneQuiet && !pressed {
			controlShadow(gtx.Ops, p, shape.offset(0, lift), 1)
		}
		face := shape.offset(0, lift)
		fillBox(gtx.Ops, face, withAlpha(background, alpha))
		if border.A > 0 {
			strokeBox(gtx.Ops, face, px(gtx, 2), withAlpha(border, alpha))
		}
		if s.Pressed {
			strokeBox(gtx.Ops, face.inset(px(gtx, 2)), px(gtx, 1), p.Action)
		}
		if gtx.Focused(&b.click) {
			// Keyboard focus ring (:focus-visible), outside the button.
			strokeBox(gtx.Ops, face.inset(-px(gtx, 4)), px(gtx, 2.5), withAlpha(p.Action, .85))
		}
		offset := op.Offset(image.Pt((w-cd.Size.X)/2, (h-cd.Size.Y)/2+int(lift))).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
		tip := s.Tip
		if tip == "" && u.Visual() {
			tip = s.Caption
		}
		b.hoverAt = hoverTooltipComic(gtx, u, b.click.Hovered(), b.hoverAt, tip, s.Comic, size)
		return layout.Dimensions{Size: size}
	})
}

// RadioCard is .lr-tile.lr-client-mode: a large selectable card.
type RadioCard struct {
	click   widget.Clickable
	lift    tween
	Comic   string // tooltip comic (the web mode cards' hint-client-*)
	Tip     string
	hoverAt time.Time
}

func (c *RadioCard) Clicked(gtx layout.Context) bool { return c.click.Clicked(gtx) }

func (c *RadioCard) Layout(gtx layout.Context, u *UI, icon, title, hint string, selected bool) layout.Dimensions {
	p := &u.P
	hovered, pressed := c.click.Hovered(), c.click.Pressed()
	target, duration := float32(0), 260*time.Millisecond
	if pressed {
		target, duration = 2, 80*time.Millisecond
	} else if hovered {
		target = -2
	}
	lift := c.lift.set(gtx, target, duration, spring)
	background, border, ink := p.Paper, p.ControlEdge, p.Ink
	if hovered {
		background, border = p.ControlHover, p.Ink
	}
	if selected {
		background, border, ink = p.ControlSelected, p.Action, p.ControlSelectedInk
	}
	width := gtx.Constraints.Max.X
	contentGtx := gtx
	contentGtx.Constraints = layout.Constraints{Max: image.Pt(width-int(px(gtx, 36)), gtx.Constraints.Max.Y)}
	macro := op.Record(gtx.Ops)
	cd := layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(contentGtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, icon, 27, ink) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.Visual() {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, title, textStyle{size: 14, weight: font.Bold, color: ink, align: text.Middle})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, hint, textStyle{size: 11, color: withAlpha(ink, .7), align: text.Middle})
					}))
			})
		}))
	call := macro.Stop()
	height := max(int(px(gtx, 128)), cd.Size.Y+int(px(gtx, 36)))
	size := image.Pt(width, height)
	return liftClick(gtx, &c.click, true, func(gtx layout.Context) layout.Dimensions {
		shape := boxOf(size, px(gtx, 18))
		if !pressed {
			controlShadow(gtx.Ops, p, shape.offset(0, lift), 1)
		}
		face := shape.offset(0, lift)
		fillBox(gtx.Ops, face, background)
		strokeBox(gtx.Ops, face, px(gtx, 2), border)
		if selected {
			strokeBox(gtx.Ops, face.inset(px(gtx, 2)), px(gtx, 1), p.Action)
		}
		offset := op.Offset(image.Pt((width-cd.Size.X)/2, (height-cd.Size.Y)/2+int(lift))).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
		if gtx.Focused(&c.click) {
			strokeBox(gtx.Ops, face.inset(-px(gtx, 4)), px(gtx, 2.5), withAlpha(p.Action, .85))
		}
		c.hoverAt = hoverTooltipComic(gtx, u, hovered, c.hoverAt, c.Tip, c.Comic, size)
		return layout.Dimensions{Size: size}
	})
}

// Input is .lr-input: 48px, leading glyph, 16px text.
type Input struct {
	Editor widget.Editor
}

func (in *Input) Layout(gtx layout.Context, u *UI, icon, placeholder string, password bool) layout.Dimensions {
	p := &u.P
	in.Editor.SingleLine = true
	if password {
		in.Editor.Mask = '•'
	} else {
		in.Editor.Mask = 0
	}
	focused := gtx.Focused(&in.Editor)
	size := image.Pt(gtx.Constraints.Max.X, int(px(gtx, 48)))
	shape := boxOf(size, px(gtx, 18))
	fillBox(gtx.Ops, shape, p.Paper)
	border := p.ControlEdge
	if focused {
		border = p.Action
		outer := box{rect: frect{Min: f32.Pt(-px(gtx, 5), -px(gtx, 5)), Max: f32.Pt(float32(size.X)+px(gtx, 5), float32(size.Y)+px(gtx, 5))}, radius: shape.radius + px(gtx, 5)}
		strokeBox(gtx.Ops, outer, px(gtx, 3), p.Action)
	}
	strokeBox(gtx.Ops, shape, px(gtx, 2), border)
	padding := int(px(gtx, 12))
	iconSide := int(px(gtx, 18))
	offset := op.Offset(image.Pt(padding, (size.Y-iconSide)/2)).Push(gtx.Ops)
	glyph(gtx, icon, 18, p.Ink)
	offset.Pop()
	textX := padding + iconSide + int(px(gtx, 8))
	editorGtx := gtx
	editorGtx.Constraints = layout.Exact(image.Pt(size.X-textX-padding, size.Y))
	textOffset := op.Offset(image.Pt(textX, 0)).Push(gtx.Ops)
	defer textOffset.Pop()
	layout.Center.Layout(editorGtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		ink := op.Record(gtx.Ops)
		paint.ColorOp{Color: p.Ink}.Add(gtx.Ops)
		inkCall := ink.Stop()
		selection := op.Record(gtx.Ops)
		paint.ColorOp{Color: withAlpha(p.Action, .3)}.Add(gtx.Ops)
		selectionCall := selection.Stop()
		if in.Editor.Len() == 0 && placeholder != "" {
			macro := op.Record(gtx.Ops)
			u.label(gtx, placeholder, textStyle{size: 16, color: withAlpha(p.Muted, .85), maxLines: 1})
			macro.Stop().Add(gtx.Ops)
		}
		return in.Editor.Layout(gtx, u.Shaper, sans, unit.Sp(16), inkCall, selectionCall)
	})
	return layout.Dimensions{Size: size}
}

// Disclosure is a <details> whose summary is a small .lr-btn with a chevron.
type Disclosure struct {
	Button
	Open   bool
	rotate tween
}

func (d *Disclosure) Layout(gtx layout.Context, u *UI, icon, caption, marker string, body layout.Widget) layout.Dimensions {
	if d.Button.Clicked(gtx) {
		d.Open = !d.Open
	}
	angle := float32(0)
	if d.Open {
		angle = 3.14159
	}
	rotation := d.rotate.set(gtx, angle, 180*time.Millisecond, spring)
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			label := caption
			if marker != "" && !u.Visual() {
				label = caption + "   ✓ " + marker
			}
			return d.Button.Layout(gtx, u, ButtonStyle{Icon: icon, Caption: label, MinH: 40, PadX: 12, IconSize: 17,
				Trailing: "chevron", TrailRot: rotation})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !d.Open {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, body)
		}))
}

// card is a glaze surface with a 1px line border and the control shadow.
func card(gtx layout.Context, u *UI, radius float32, inset layout.Inset, body layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	inner := gtx
	inner.Constraints.Min = image.Point{X: gtx.Constraints.Max.X}
	dims := inset.Layout(inner, body)
	call := macro.Stop()
	size := image.Pt(gtx.Constraints.Max.X, dims.Size.Y)
	shape := boxOf(size, px(gtx, radius))
	controlShadow(gtx.Ops, &u.P, shape, 1)
	glaze(gtx.Ops, &u.P, shape)
	strokeBox(gtx.Ops, shape, px(gtx, 1), u.P.Line)
	call.Add(gtx.Ops)
	return layout.Dimensions{Size: size}
}

// Segmented is .lr-lang: options in a pill track with a sliding selection.
type Segmented struct {
	clicks []widget.Clickable
	slide  tween
}

func (s *Segmented) Layout(gtx layout.Context, u *UI, options []string, selected int) (layout.Dimensions, int) {
	p := &u.P
	if len(s.clicks) != len(options) {
		s.clicks = make([]widget.Clickable, len(options))
	}
	chosen := s.arrows(gtx, selected)
	for i := range s.clicks {
		if s.clicks[i].Clicked(gtx) {
			chosen = i
		}
	}
	item := int(px(gtx, 32))
	padding := int(px(gtx, 4)) + int(px(gtx, 2))
	widths := make([]int, len(options))
	total := 0
	for i, option := range options {
		macro := op.Record(gtx.Ops)
		measure := gtx
		measure.Constraints.Min = image.Point{}
		d := layout.Dimensions{Size: image.Pt(int(px(gtx, 14)), 0)}
		if !strings.HasPrefix(option, "glyph:") {
			d = u.label(measure, option, textStyle{size: 13, weight: font.Bold, maxLines: 1})
		}
		macro.Stop()
		widths[i] = max(item, d.Size.X+int(px(gtx, 10)))
		total += widths[i]
	}
	size := image.Pt(total+2*padding, item+2*padding)
	track := boxOf(size, float32(size.Y)/2)
	fillBox(gtx.Ops, track, p.Paper)
	strokeBox(gtx.Ops, track, px(gtx, 2), p.ControlEdge)
	x := padding
	positions := make([]int, len(options))
	for i := range options {
		positions[i] = x
		x += widths[i]
	}
	position := s.slide.set(gtx, float32(positions[selected]), 260*time.Millisecond, spring)
	pill := box{rect: frect{Min: f32.Pt(position, float32(padding)), Max: f32.Pt(position+float32(widths[selected]), float32(padding+item))}, radius: float32(item) / 2}
	fillBox(gtx.Ops, pill, p.ControlSelected)
	strokeBox(gtx.Ops, pill, px(gtx, 2), p.Action)
	for i, option := range options {
		offset := op.Offset(image.Pt(positions[i], padding)).Push(gtx.Ops)
		cell := gtx
		cell.Constraints = layout.Exact(image.Pt(widths[i], item))
		s.clicks[i].Layout(cell, func(gtx layout.Context) layout.Dimensions {
			pointer.CursorPointer.Add(gtx.Ops)
			ink := p.SegmentText
			if i == selected {
				ink = p.ControlSelectedInk
			}
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if name, ok := strings.CutPrefix(option, "glyph:"); ok {
					return glyph(gtx, name, 14, ink)
				}
				return u.label(gtx, option, textStyle{size: 13, weight: font.Bold, color: ink, maxLines: 1})
			})
		})
		offset.Pop()
	}
	return layout.Dimensions{Size: size}, chosen
}

// arrows moves the selection with the arrow keys while an option has the
// keyboard focus (radio-group behaviour, KB-4).
func (s *Segmented) arrows(gtx layout.Context, selected int) int {
	chosen := selected
	n := len(s.clicks)
	for i := range s.clicks {
		for {
			ev, ok := gtx.Event(key.Filter{Focus: &s.clicks[i], Name: key.NameLeftArrow},
				key.Filter{Focus: &s.clicks[i], Name: key.NameRightArrow},
				key.Filter{Focus: &s.clicks[i], Name: key.NameUpArrow},
				key.Filter{Focus: &s.clicks[i], Name: key.NameDownArrow})
			if !ok {
				break
			}
			e, ok := ev.(key.Event)
			if !ok || e.State != key.Press || n == 0 {
				continue
			}
			step := 1
			if e.Name == key.NameLeftArrow || e.Name == key.NameUpArrow {
				step = n - 1
			}
			chosen = (i + step) % n
			gtx.Execute(key.FocusCmd{Tag: &s.clicks[chosen]})
		}
	}
	return chosen
}

// LayoutWide is a full-width .lr-toggle (source picker tabs): equal cells of
// at least 44px with a glyph and a 12px/700 caption.
func (s *Segmented) LayoutWide(gtx layout.Context, u *UI, icons, options []string, selected int) (layout.Dimensions, int) {
	p := &u.P
	if len(s.clicks) != len(options) {
		s.clicks = make([]widget.Clickable, len(options))
	}
	chosen := s.arrows(gtx, selected)
	for i := range s.clicks {
		if s.clicks[i].Clicked(gtx) {
			chosen = i
		}
	}
	item := int(px(gtx, 44))
	padding := int(px(gtx, 6))
	width := gtx.Constraints.Max.X
	cellW := (width - 2*padding) / max(1, len(options))
	size := image.Pt(width, item+2*padding)
	track := boxOf(size, float32(size.Y)/2)
	fillBox(gtx.Ops, track, p.Paper)
	strokeBox(gtx.Ops, track, px(gtx, 2), p.ControlEdge)
	position := s.slide.set(gtx, float32(padding+chosen*cellW), 260*time.Millisecond, spring)
	pill := box{rect: frect{Min: f32.Pt(position, float32(padding)), Max: f32.Pt(position+float32(cellW), float32(padding+item))}, radius: float32(item) / 2}
	fillBox(gtx.Ops, pill, p.ControlSelected)
	for i, option := range options {
		offset := op.Offset(image.Pt(padding+i*cellW, padding)).Push(gtx.Ops)
		cell := gtx
		cell.Constraints = layout.Exact(image.Pt(cellW, item))
		s.clicks[i].Layout(cell, func(gtx layout.Context) layout.Dimensions {
			pointer.CursorPointer.Add(gtx.Ops)
			ink := p.SegmentText
			if i == chosen {
				ink = p.ControlSelectedInk
			} else if s.clicks[i].Hovered() {
				fillBox(gtx.Ops, boxOf(gtx.Constraints.Min, float32(item)/2), p.ControlHover)
			}
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if i >= len(icons) || icons[i] == "" {
							return layout.Dimensions{}
						}
						return glyph(gtx, icons[i], 16, ink)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(7)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if u.Visual() && i < len(icons) && icons[i] != "" {
							return layout.Dimensions{} // ✦: the glyph carries the meaning
						}
						return u.label(gtx, option, textStyle{size: 12, weight: font.Bold, color: ink, maxLines: 1})
					}))
			})
		})
		offset.Pop()
	}
	return layout.Dimensions{Size: size}, chosen
}

// clipRect keeps children inside size.
func clipRect(gtx layout.Context, size image.Point) clip.Stack {
	return clip.Rect{Max: size}.Push(gtx.Ops)
}

// rotateAbout rotates by angle radians around (x, y).
func rotateAbout(x, y, angle float32) f32.Affine2D {
	return f32.Affine2D{}.Rotate(f32.Pt(x, y), angle)
}

// hoverTooltip shows tip under a control hovered for 500 ms (.lr-comic-tip
// without the comic) and returns the updated hover start.
func hoverTooltip(gtx layout.Context, u *UI, hovered bool, since time.Time, tip string, size image.Point) time.Time {
	return hoverTooltipComic(gtx, u, hovered, since, tip, "", size)
}

// hoverTooltipComic is the comic tooltip (.lr-comic-tip): the comic over the
// text; in ✦ mode the comic alone.
func hoverTooltipComic(gtx layout.Context, u *UI, hovered bool, since time.Time, tip, kind string, size image.Point) time.Time {
	if kind != "" && !hasComic(kind) {
		kind = ""
	}
	if u.Visual() && kind != "" {
		tip = ""
	}
	if !hovered || (tip == "" && kind == "") {
		return time.Time{}
	}
	trackTip(gtx, size) // where the control is, to keep the tooltip in the window
	if since.IsZero() {
		since = gtx.Now
	}
	if gtx.Now.Sub(since) < 500*time.Millisecond {
		gtx.Execute(op.InvalidateCmd{At: since.Add(510 * time.Millisecond)})
		return since
	}
	p := &u.P
	macro := op.Record(gtx.Ops)
	lg := gtx
	lg.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(300), gtx.Dp(320))}
	text := layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(lg,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if kind == "" {
				return layout.Dimensions{}
			}
			// Plays once from the moment the tooltip appears, like the web's replay on hover.
			d := comicPlaying(gtx, kind, 240, u.P.Dark, gtx.Now.Sub(since.Add(500*time.Millisecond)), false)
			if tip != "" {
				d.Size.Y += gtx.Dp(6)
			}
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if tip == "" {
				return layout.Dimensions{}
			}
			return u.label(gtx, tip, textStyle{size: 13, weight: font.SemiBold, color: p.Ink, lineH: 1.5})
		}))
	textCall := macro.Stop()
	padX, padY := gtx.Dp(14), gtx.Dp(9)
	box := image.Pt(text.Size.X+2*padX, text.Size.Y+2*padY)
	at := placeTip(image.Pt((size.X-box.X)/2, size.Y+gtx.Dp(8)), box, size, gtx.Dp(8))
	x, y := at.X, at.Y
	macro = op.Record(gtx.Ops)
	o := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	shape := boxOf(box, px(gtx, 14))
	shadow(gtx.Ops, shape, px(gtx, 6), px(gtx, 14), withAlpha(p.Shade, .25))
	fillBox(gtx.Ops, shape, p.Paper)
	strokeBox(gtx.Ops, shape, px(gtx, 1.5), p.ControlEdge)
	to := op.Offset(image.Pt(padX, padY)).Push(gtx.Ops)
	textCall.Add(gtx.Ops)
	to.Pop()
	o.Pop()
	op.Defer(gtx.Ops, macro.Stop())
	return since
}

// liftClick lays out a control whose picture may leave its box: the hover
// lift, the shadow, the focus ring, the round TV buttons' growth. A Clickable
// clips what it wraps to its size, which cut the top edge of a lifted button;
// here the picture is drawn unclipped and only the hit area is the clickable.
func liftClick(gtx layout.Context, click *widget.Clickable, hand bool, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if hand {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		return dims
	})
	// The picture goes after the hit area: inner controls and the tooltip
	// tracker sit above it (same place on screen, so nothing looks different).
	call.Add(gtx.Ops)
	return dims
}
