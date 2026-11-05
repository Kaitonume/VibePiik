package nativeui

import (
	"image"
	"math"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/TNTcraftHIM/Piik/internal/app/loopback"
)

// videoLimits mirrors VideoCaptureLimits (media/quality.ts): the local
// ceiling narrowed by what the App advertises.
type videoLimits struct {
	MaxWidth, MaxHeight int
	MaxPixels           int
	MaxMacroblocksPerS  int
	MaxFramerate        int
	MaxBitrate          int // bit/s
	MaxAudioBitrate     int // bit/s
}

const (
	minVideoFramerate = 1
	minVideoBitrate   = 100_000
	maxAudioSend      = 320_000
	minAudioBitrate   = 6_000
)

var localVideoLimits = videoLimits{MaxWidth: 8192, MaxHeight: 8192, MaxPixels: 36_864 * 16 * 16,
	MaxMacroblocksPerS: 2_073_600, MaxFramerate: 240, MaxBitrate: 240_000_000, MaxAudioBitrate: maxAudioSend}

func limitsFrom(caps loopback.NativeMediaCapabilities) videoLimits {
	limits := localVideoLimits
	narrow := func(value *int, advertised int) {
		if advertised > 0 && advertised < *value {
			*value = advertised
		}
	}
	narrow(&limits.MaxWidth, int(caps.MaxVideoResolution.Width))
	narrow(&limits.MaxHeight, int(caps.MaxVideoResolution.Height))
	narrow(&limits.MaxPixels, caps.MaxVideoPixels)
	narrow(&limits.MaxMacroblocksPerS, caps.MaxVideoMacroblocksPerSecond)
	narrow(&limits.MaxFramerate, caps.MaxVideoFramerate)
	narrow(&limits.MaxBitrate, caps.MaxVideoBitrate)
	narrow(&limits.MaxAudioBitrate, caps.MaxAudioBitrate)
	return limits
}

// framerateCeiling is captureFramerateCeiling: 0 when the size is not allowed.
func framerateCeiling(width, height int, limits videoLimits) int {
	if width < 4 || height < 4 || width%2 != 0 || height%2 != 0 || width > limits.MaxWidth || height > limits.MaxHeight ||
		width*height > limits.MaxPixels {
		return 0
	}
	blocks := ((width + 15) / 16) * ((height + 15) / 16)
	if blocks*256 > localVideoLimits.MaxPixels {
		return 0
	}
	return int(math.Floor(math.Min(float64(limits.MaxFramerate), float64(limits.MaxMacroblocksPerS)/float64(blocks))))
}

// sliderRange is the common range of a typed control (NumberControl): the
// slider keeps to its steps, typed values may go anywhere in [min, max].
func sliderRange(value, sliderMin, sliderMax, step, max float64) (top, common float64, custom bool) {
	top = math.Min(sliderMax, math.Floor(max/step)*step)
	if top < sliderMin {
		return top, value, true
	}
	common = math.Min(top, math.Max(sliderMin, math.Round(value/step)*step))
	return top, common, math.Abs(value-common) > 1e-9
}

// NumberBox is the typed half of NumberControl: a .lr-quality-number box the
// host can type into. Enter or leaving the box commits.
type NumberBox struct {
	editor  widget.Editor
	focused bool
	shown   string // the value last shown: a change from elsewhere (slider) replaces the text
	Invalid bool
}

// Layout returns the text to commit, if the host just finished typing.
func (n *NumberBox) Layout(gtx layout.Context, u *UI, value, unitText string, minWidth float32, disabled bool) (layout.Dimensions, string, bool) {
	p := &u.P
	n.editor.SingleLine, n.editor.Submit = true, true
	n.editor.Filter = "0123456789."
	n.editor.MaxLen = 9
	committed, ok := "", false
	for {
		event, more := n.editor.Update(gtx)
		if !more {
			break
		}
		if _, submit := event.(widget.SubmitEvent); submit {
			committed, ok = n.editor.Text(), true
			gtx.Execute(key.FocusCmd{})
		}
	}
	focused := gtx.Focused(&n.editor)
	if n.focused && !focused && !ok {
		committed, ok = n.editor.Text(), true
	}
	n.focused = focused
	switch {
	case !focused && !ok && n.editor.Text() != value:
		n.editor.SetText(value)
	case focused && !ok && value != n.shown:
		// The value changed elsewhere (the slider) while the box has focus:
		// show it, the host is not typing a different one right now.
		n.editor.SetText(value)
		n.editor.SetCaret(n.editor.Len(), n.editor.Len())
	}
	n.shown = value
	if focused {
		n.Invalid = false
	}
	gtx.Constraints.Min = image.Point{}
	unitMacro := op.Record(gtx.Ops)
	unitDims := u.label(gtx, unitText, textStyle{size: 11, mono: true, color: p.Muted})
	unitCall := unitMacro.Stop()
	pad := int(px(gtx, 12))
	fieldW := int(px(gtx, 9.5*float32(max(3, len(value)+1))))
	size := image.Pt(max(fieldW+unitDims.Size.X+int(px(gtx, 6))+2*pad, int(px(gtx, minWidth))), int(px(gtx, 38)))
	shape := boxOf(size, px(gtx, 12))
	fillBox(gtx.Ops, shape, p.Wall)
	edge := p.ControlEdge
	switch {
	case n.Invalid:
		edge = p.Danger
	case focused:
		edge = p.Action
	}
	strokeBox(gtx.Ops, shape, px(gtx, 1), edge)
	if focused {
		strokeBox(gtx.Ops, shape.inset(px(gtx, -3)), px(gtx, 2), withAlpha(p.Action, .5))
	}
	o := op.Offset(image.Pt(size.X-pad-unitDims.Size.X, (size.Y-unitDims.Size.Y)/2)).Push(gtx.Ops)
	unitCall.Add(gtx.Ops)
	o.Pop()
	editW := size.X - 2*pad - unitDims.Size.X - int(px(gtx, 6))
	eo := op.Offset(image.Pt(pad, (size.Y-int(px(gtx, 20)))/2)).Push(gtx.Ops)
	eg := gtx
	eg.Constraints = layout.Exact(image.Pt(editW, int(px(gtx, 20))))
	ink := op.Record(gtx.Ops)
	color := p.Ink
	if disabled {
		color = withAlpha(p.Ink, .45)
	}
	paint.ColorOp{Color: color}.Add(gtx.Ops)
	inkCall := ink.Stop()
	selection := op.Record(gtx.Ops)
	paint.ColorOp{Color: withAlpha(p.Action, .35)}.Add(gtx.Ops)
	selectionCall := selection.Stop()
	face := mono
	face.Weight = font.Bold
	if disabled {
		u.label(eg, value, textStyle{size: 15, mono: true, weight: font.Bold, color: color, align: text.End})
	} else {
		n.editor.Alignment = text.End
		n.editor.Layout(eg, u.Shaper, face, unit.Sp(15), inkCall, selectionCall)
	}
	eo.Pop()
	return layout.Dimensions{Size: size}, committed, ok && !disabled
}

// parseNumber reads a typed value; ok is false for empty or malformed text.
func parseNumber(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// groupNumber formats 9437184 as "9,437,184" (toLocaleString en-US).
func groupNumber(value int) string {
	digits := strconv.Itoa(value)
	var out []byte
	for i := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digits[i])
	}
	return string(out)
}
