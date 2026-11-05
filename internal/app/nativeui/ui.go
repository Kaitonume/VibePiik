package nativeui

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

// UI holds what every widget needs: palette, text shaper, language.
type UI struct {
	P      Palette
	Shaper *text.Shaper
	Lang   string // "en", "zh", "it" or "vis"
	// Spoken is the language kept under ✦ mode for tooltips and names
	// (the web keeps lang beside vis).
	Spoken string
}

// textLang is the language texts are written in.
func (u *UI) textLang() string {
	if u.Lang == "vis" {
		if u.Spoken == "zh" || u.Spoken == "it" {
			return u.Spoken
		}
		return "en"
	}
	return u.Lang
}

// T returns the web client's text for key in the current language.
func (u *UI) T(key string, vars ...string) string {
	catalog, native := copy_en, nativeCopy_en
	switch u.textLang() {
	case "zh":
		catalog, native = copy_zh, nativeCopy_zh
	case "it":
		if italian, ok := extraCatalogs["it"]; ok {
			catalog = italian
		}
		native = nativeCopy_it
	}
	value, ok := native[key]
	if !ok {
		value, ok = catalog[key]
	}
	if !ok {
		value, ok = nativeCopy_en[key]
	}
	if !ok {
		value = copy_en[key]
	}
	for i := 0; i+1 < len(vars); i += 2 {
		value = strings.ReplaceAll(value, "{"+vars[i]+"}", vars[i+1])
	}
	return value
}

func (u *UI) Visual() bool { return u.Lang == "vis" }

type textStyle struct {
	size     float32
	weight   font.Weight
	mono     bool
	color    color.NRGBA
	align    text.Alignment
	maxLines int
	lineH    float32 // line-height multiplier, 0 = font default
}

func (u *UI) label(gtx layout.Context, s string, style textStyle) layout.Dimensions {
	face := sans
	if style.mono {
		face = mono
	}
	face.Weight = style.weight
	material := op.Record(gtx.Ops)
	paint.ColorOp{Color: style.color}.Add(gtx.Ops)
	call := material.Stop()
	label := widget.Label{Alignment: style.align, MaxLines: style.maxLines}
	if style.lineH > 0 {
		label.LineHeightScale = style.lineH
	}
	return label.Layout(gtx, u.Shaper, face, unit.Sp(style.size), s, call)
}

// spring is the web --spring easing, cubic-bezier(.3, 1.45, .45, 1): it overshoots.
func spring(t float32) float32 { return cubicBezier(t, .3, 1.45, .45, 1) }

func easeOut(t float32) float32 { return cubicBezier(t, .25, .1, .25, 1) }

func cubicBezier(t, x1, y1, x2, y2 float32) float32 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	// Solve x(s) = t by Newton iterations, then return y(s).
	s := t
	for i := 0; i < 8; i++ {
		x := bezier(s, x1, x2) - t
		dx := bezierSlope(s, x1, x2)
		if math.Abs(float64(dx)) < 1e-6 {
			break
		}
		s -= x / dx
	}
	return bezier(s, y1, y2)
}

func bezier(s, p1, p2 float32) float32 {
	return 3*(1-s)*(1-s)*s*p1 + 3*(1-s)*s*s*p2 + s*s*s
}

func bezierSlope(s, p1, p2 float32) float32 {
	return 3*(1-s)*(1-s)*p1 + 6*(1-s)*s*(p2-p1) + 3*s*s*(1-p2)
}

// tween animates a value toward a target over a duration with an easing.
type tween struct {
	from, to, value float32
	start           time.Time
	duration        time.Duration
	ease            func(float32) float32
}

// to sets a new target; the animation starts from the current value.
func (t *tween) set(gtx layout.Context, target float32, duration time.Duration, ease func(float32) float32) float32 {
	if reducedMotion {
		// Windows "Animation effects" off: settle at once (prefers-reduced-motion).
		t.value, t.from, t.to, t.start = target, target, target, gtx.Now
		return target
	}
	if target != t.to || t.start.IsZero() {
		if t.start.IsZero() {
			t.value = target
		}
		t.from, t.to, t.start, t.duration, t.ease = t.value, target, gtx.Now, duration, ease
	}
	elapsed := gtx.Now.Sub(t.start)
	if elapsed >= t.duration || t.duration <= 0 {
		t.value = t.to
		return t.value
	}
	progress := t.ease(float32(elapsed) / float32(t.duration))
	t.value = t.from + (t.to-t.from)*progress
	gtx.Execute(op.InvalidateCmd{})
	return t.value
}

func dimsOf(w, h int) layout.Dimensions { return layout.Dimensions{Size: image.Pt(w, h)} }
