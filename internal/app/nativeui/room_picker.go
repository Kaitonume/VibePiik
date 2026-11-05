package nativeui

import (
	"bytes"
	"encoding/base64"
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
	"golang.org/x/image/bmp"

	"github.com/TNTcraftHIM/Piik/internal/app/nativecapture"
)

// pickerSource is one capturable window or screen with its preview.
type pickerSource struct {
	Target  nativecapture.CaptureTarget
	Preview image.Image
	card    widget.Clickable
	lift    tween
	op      paint.ImageOp
	opSet   bool
}

// decodePreview turns a source-preview payload (base64 BMP) into an image.
func decodePreview(data string) image.Image {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil
	}
	picture, err := bmp.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	return picture
}

// SourcePicker is .lr-source-picker: it covers the TV screen while choosing.
type SourcePicker struct {
	tabs    Segmented
	refresh Button
	close   Button
	border  Switch
	sound   Switch
	list    widget.List
	Tab     string // "window" or "display"
}

type pickerAction struct {
	Close, Refresh bool
	Chosen         *pickerSource
}

func (s *SourcePicker) Layout(gtx layout.Context, u *UI, sources []*pickerSource, loading bool, status string, showBorder, sound *bool, exclusion layout.Widget) (layout.Dimensions, pickerAction) {
	var action pickerAction
	if s.Tab == "" {
		s.Tab = "window"
	}
	if s.close.Clicked(gtx) {
		action.Close = true
	}
	if s.refresh.Clicked(gtx) {
		action.Refresh = true
	}
	for _, source := range sources {
		if source.card.Clicked(gtx) {
			action.Chosen = source
		}
	}
	p := &u.P
	fillRect(gtx.Ops, gtx.Constraints.Max, p.Wall)
	s.list.Axis = layout.Vertical
	dims := layout.UniformInset(unit.Dp(18)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(720))
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.head(gtx, u, loading) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					options := []string{u.T("host.sourcePicker.tab.window"), u.T("host.sourcePicker.tab.display")}
					keys := []string{"window", "display"}
					selected := 0
					if s.Tab == "display" {
						selected = 1
					}
					d, chosen := s.tabs.LayoutWide(gtx, u, []string{"window", "display"}, options, selected)
					s.Tab = keys[chosen]
					return d
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Spacing: layout.SpaceStart, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							s.border.Tip = u.T("host.sourcePicker.showCaptureBorderHint")
							return s.border.LayoutLabeled(gtx, u, "window", u.T("host.sourcePicker.showCaptureBorder"), showBorder)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							key := "host.sourcePicker.windowAudio"
							if s.Tab == "display" {
								key = "host.sourcePicker.systemAudio"
							}
							// CaptureSourcePicker: what a click does, then the audio hint.
							action := u.T("host.noAudio")
							if sound != nil {
								action = u.T(map[bool]string{true: "host.sourcePicker.audioOff", false: "host.sourcePicker.audioOn"}[*sound])
							}
							s.sound.Tip = action + "\n" + u.T("host.sourcePicker.audioHint")
							return s.sound.LayoutLabeled(gtx, u, "speaker", u.T(key), sound)
						}))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if exclusion == nil || s.Tab != "display" || sound == nil || !*sound {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, exclusion)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					var shown []*pickerSource
					for _, source := range sources {
						if source.Target.Kind == s.Tab {
							shown = append(shown, source)
						}
					}
					if len(shown) == 0 {
						text := status
						if text == "" && loading {
							text = u.T("common.loading")
						}
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if loading {
								return u.label(gtx, text, textStyle{size: 11.5, color: p.Muted})
							}
							return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return comic(gtx, u, "hint-no-sources", 280) }),
								layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if u.Visual() {
										return layout.Dimensions{}
									}
									return u.label(gtx, text, textStyle{size: 11.5, color: p.Muted})
								}))
						})
					}
					return s.grid(gtx, u, shown)
				}),
			)
		})
	})
	return dims, action
}

func (s *SourcePicker) head(gtx layout.Context, u *UI, loading bool) layout.Dimensions {
	p := &u.P
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			side := int(px(gtx, 42))
			b := boxOf(image.Pt(side, side), float32(side)/2)
			shadow(gtx.Ops, b, px(gtx, 2), 0, withAlpha(p.Shade, p.ShadeAlpha))
			fillBox(gtx.Ops, b, p.ControlSelected)
			strokeBox(gtx.Ops, b, px(gtx, 2), p.Action)
			g := int(px(gtx, 20))
			o := op.Offset(image.Pt((side-g)/2, (side-g)/2)).Push(gtx.Ops)
			glyph(gtx, "window", 20, p.ControlSelectedInk)
			o.Pop()
			return layout.Dimensions{Size: image.Pt(side, side)}
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, u.T("host.sourcePicker.title"), textStyle{size: 15, weight: font.Bold, color: p.Ink, maxLines: 1})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.refresh.Layout(gtx, u, ButtonStyle{Comic: "hint-refresh-sources", Icon: "refresh", Size: sizeDense, Busy: loading, Tip: u.T("host.sourcePicker.refresh")})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.close.Layout(gtx, u, ButtonStyle{Comic: "hint-close", Icon: "x", Size: sizeDense, Tone: toneQuiet})
		}))
}

// grid is .lr-source-picker-list: auto-fit columns of at least 240px, gap 8.
func (s *SourcePicker) grid(gtx layout.Context, u *UI, sources []*pickerSource) layout.Dimensions {
	gap := int(px(gtx, 8))
	columns := max(1, (gtx.Constraints.Max.X+gap)/(int(px(gtx, 240))+gap))
	cellW := (gtx.Constraints.Max.X - (columns-1)*gap) / columns
	rows := (len(sources) + columns - 1) / columns
	return s.list.Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			height := 0
			for c := 0; c < columns; c++ {
				index := row*columns + c
				if index >= len(sources) {
					break
				}
				o := op.Offset(image.Pt(c*(cellW+gap), 0)).Push(gtx.Ops)
				cgtx := gtx
				cgtx.Constraints = layout.Constraints{Min: image.Pt(cellW, 0), Max: image.Pt(cellW, gtx.Constraints.Max.Y)}
				d := sourceCard(cgtx, u, sources[index])
				o.Pop()
				height = max(height, d.Size.Y)
			}
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
		})
	})
}

// sourceCard is .lr-source-option: title (2 lines) over a 144px preview.
func sourceCard(gtx layout.Context, u *UI, source *pickerSource) layout.Dimensions {
	p := &u.P
	hovered := source.card.Hovered()
	pressed := source.card.Pressed()
	target := float32(0)
	switch {
	case pressed:
		target = 2
	case hovered:
		target = -2
	}
	lift := source.lift.set(gtx, target, 260*time.Millisecond, spring)
	width := gtx.Constraints.Min.X
	return liftClick(gtx, &source.card, true, func(gtx layout.Context) layout.Dimensions {
		pad := int(px(gtx, 8))
		macro := op.Record(gtx.Ops)
		cgtx := gtx
		cgtx.Constraints = layout.Constraints{Min: image.Pt(width-2*pad, 0), Max: image.Pt(width-2*pad, gtx.Constraints.Max.Y)}
		content := layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = int(px(gtx, 36))
				d := u.label(gtx, source.Target.Title, textStyle{size: 13, weight: font.Bold, color: p.Ink, maxLines: 2, lineH: 1.35})
				d.Size.Y = max(d.Size.Y, gtx.Constraints.Min.Y)
				return d
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Min.X, int(px(gtx, 144)))
				frame := boxOf(size, px(gtx, 8))
				fillBox(gtx.Ops, frame, p.Wall2)
				if source.Preview != nil {
					if !source.opSet {
						source.op = paint.NewImageOp(source.Preview)
						source.op.Filter = paint.FilterLinear
						source.opSet = true
					}
					drawContain(gtx.Ops, source.op, size)
				} else {
					g := int(px(gtx, 28))
					icon := "window"
					if source.Target.Kind == "display" {
						icon = "display"
					}
					o := op.Offset(image.Pt((size.X-g)/2, (size.Y-g)/2)).Push(gtx.Ops)
					glyph(gtx, icon, 28, withAlpha(p.Ink, .5))
					o.Pop()
				}
				return layout.Dimensions{Size: size}
			}))
		call := macro.Stop()
		size := image.Pt(width, content.Size.Y+2*pad)
		shape := boxOf(size, px(gtx, 18)).offset(0, lift)
		if !pressed {
			controlShadow(gtx.Ops, p, shape, 1)
		}
		bg, border := p.Paper, p.ControlEdge
		if hovered {
			bg, border = p.ControlHover, p.Action
		}
		fillBox(gtx.Ops, shape, bg)
		if !hovered {
			glaze(gtx.Ops, p, shape)
		}
		strokeBox(gtx.Ops, shape, px(gtx, 2), border)
		if hovered {
			strokeBox(gtx.Ops, shape.inset(px(gtx, 2)), px(gtx, 1), p.Action)
		}
		o := op.Offset(image.Pt(pad, pad+int(lift))).Push(gtx.Ops)
		call.Add(gtx.Ops)
		o.Pop()
		pointerHand(gtx, size)
		return layout.Dimensions{Size: size}
	})
}

// drawContain paints img scaled to fit inside size (object-fit: contain).
func drawContain(ops *op.Ops, img paint.ImageOp, size image.Point) {
	src := img.Size()
	if src.X == 0 || src.Y == 0 {
		return
	}
	scale := min(float32(size.X)/float32(src.X), float32(size.Y)/float32(src.Y))
	w, h := float32(src.X)*scale, float32(src.Y)*scale
	x, y := (float32(size.X)-w)/2, (float32(size.Y)-h)/2
	defer clip.Rect{Min: image.Pt(int(x), int(y)), Max: image.Pt(int(x+w), int(y+h))}.Push(ops).Pop()
	defer op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(scale, scale)).Offset(f32.Pt(x, y))).Push(ops).Pop()
	img.Add(ops)
	paint.PaintOp{}.Add(ops)
}

// Switch is the 58×34 toggle (.lr-switch).
type Switch struct {
	click   widget.Clickable
	knob    tween
	Tip     string // hint shown on hover
	hoverAt time.Time
}

func (s *Switch) LayoutLabeled(gtx layout.Context, u *UI, icon, caption string, value *bool) layout.Dimensions {
	if s.click.Clicked(gtx) && value != nil {
		*value = !*value
	}
	on := value != nil && *value
	p := &u.P
	return s.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		d := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, icon, 18, p.Ink) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(9)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, caption, textStyle{size: 12, weight: font.SemiBold, color: p.Ink, maxLines: 1})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(9)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.track(gtx, u, on) }))
		pointerHand(gtx, d.Size)
		s.hoverAt = hoverTooltip(gtx, u, s.click.Hovered(), s.hoverAt, s.Tip, d.Size)
		return d
	})
}

func (s *Switch) track(gtx layout.Context, u *UI, on bool) layout.Dimensions {
	p := &u.P
	size := image.Pt(int(px(gtx, 58)), int(px(gtx, 34)))
	track := boxOf(size, float32(size.Y)/2)
	bg, knob := p.Wall2, p.Paper
	if on {
		bg, knob = p.Action, p.ActionInk
	}
	fillBox(gtx.Ops, track, bg)
	strokeBox(gtx.Ops, track, px(gtx, 2), p.ControlEdge)
	target := float32(0)
	if on {
		target = 1
	}
	t := s.knob.set(gtx, target, 220*time.Millisecond, spring)
	k := px(gtx, 24)
	x := px(gtx, 5) + t*(float32(size.X)-k-px(gtx, 10))
	knobBox := box{rect: frect{Min: f32.Pt(x, (float32(size.Y)-k)/2), Max: f32.Pt(x+k, (float32(size.Y)+k)/2)}, radius: k / 2}
	fillBox(gtx.Ops, knobBox, knob)
	if !on {
		strokeBox(gtx.Ops, knobBox, px(gtx, 2), p.ControlEdge)
	}
	return layout.Dimensions{Size: size}
}

var _ = color.NRGBA{}
