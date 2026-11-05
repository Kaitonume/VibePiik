package nativeui

import (
	"context"
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/TNTcraftHIM/Piik/internal/app/nativecapture"
)

// exclusionPicker is AudioExclusionSelect: a <details> listing the
// applications behind open windows; one of them can be left out of the
// sound of a screen share.
type exclusionPicker struct {
	disc    Disclosure
	none    Chip
	chips   []Chip
	refresh Button
	VoiceOn bool // in room voice: VibePiik leaves out its own sound instead
}

type exclusionResult struct {
	Changed bool
	Target  *nativecapture.CaptureTarget
	Refresh bool
	Opened  bool
}

func (e *exclusionPicker) Layout(gtx layout.Context, u *UI, apps []nativecapture.CaptureTarget, current *nativecapture.CaptureTarget,
	busy, failed, disabled bool) (layout.Dimensions, exclusionResult) {
	var result exclusionResult
	wasOpen := e.disc.Open
	if e.none.Clicked(gtx) && !disabled && current != nil {
		result.Changed = true
	}
	for len(e.chips) < len(apps) {
		e.chips = append(e.chips, Chip{})
	}
	for i := range apps {
		if e.chips[i].Clicked(gtx) && !disabled {
			chosen := apps[i]
			result.Changed, result.Target = true, &chosen
		}
	}
	if e.refresh.Clicked(gtx) && !busy {
		result.Refresh = true
	}
	currentKey := ""
	if current != nil {
		currentKey = audioAppKey(*current)
	}
	missing := current != nil
	for _, app := range apps {
		if audioAppKey(app) == currentKey {
			missing = false
		}
	}
	caption := u.T("host.sourcePicker.excludeAudio")
	if current != nil && !u.Visual() {
		caption += ": " + current.Title
	}
	dims := e.disc.Layout(gtx, u, "speakerOff", caption, "", func(gtx layout.Context) layout.Dimensions {
		hint := "host.sourcePicker.excludeAudioChoose"
		switch {
		case failed:
			hint = "host.sourcePicker.excludeListFailed"
		case current != nil:
			hint = "host.sourcePicker.excludeAudioLifetime"
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var items []layout.Widget
				items = append(items, func(gtx layout.Context) layout.Dimensions {
					return e.none.Layout(gtx, u, "", u.T("host.sourcePicker.excludeNone"), "", current == nil, disabled)
				})
				if missing {
					title := current.Title + " (" + u.T("host.sourcePicker.excludeMissing") + ")"
					items = append(items, func(gtx layout.Context) layout.Dimensions {
						var idle Chip
						return idle.Layout(gtx, u, "", title, "", true, true)
					})
				}
				for i, app := range apps {
					items = append(items, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(260))
						return e.chips[i].Layout(gtx, u, "", app.Title, "", audioAppKey(app) == currentKey, disabled)
					})
				}
				items = append(items, func(gtx layout.Context) layout.Dimensions {
					return e.refresh.Layout(gtx, u, ButtonStyle{Icon: "refresh", Size: sizeDense, Disabled: busy || disabled,
						Tip: u.T("host.sourcePicker.refreshApplications")})
				})
				return flowRow(gtx, 8, items)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if u.Visual() {
					return layout.Dimensions{}
				}
				color := u.P.Muted
				if failed {
					color = u.P.DangerText
				}
				return u.label(gtx, u.T(hint), textStyle{size: 11.5, color: color, maxLines: 2})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !e.VoiceOn || u.Visual() {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, u.T("native.excludeVoice"), textStyle{size: 11.5, color: u.P.Warn, maxLines: 3})
				})
			}))
	})
	if e.disc.Open && !wasOpen {
		result.Opened = true
	}
	if result.Changed {
		e.disc.Open = false // a choice closes the list
	}
	return dims, result
}

// countBadge is .lr-remote-count: a small red pill with a number at the
// top-right corner of a button of the given size.
func countBadge(gtx layout.Context, u *UI, size image.Point, count int) {
	macro := op.Record(gtx.Ops)
	text := u.label(gtx, itoa(count), textStyle{size: 10.5, weight: font.Bold, color: u.P.ActionInk})
	call := macro.Stop()
	h := int(px(gtx, 18))
	w := max(h, text.Size.X+int(px(gtx, 10)))
	o := op.Offset(image.Pt(size.X-w+int(px(gtx, 4)), -int(px(gtx, 5)))).Push(gtx.Ops)
	shape := boxOf(image.Pt(w, h), float32(h)/2)
	fillBox(gtx.Ops, shape, u.P.Danger)
	strokeBox(gtx.Ops, shape, px(gtx, 1.5), u.P.Paper)
	t := op.Offset(image.Pt((w-text.Size.X)/2, (h-text.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	t.Pop()
	o.Pop()
}

// flowRow lays items left to right and wraps them (flex-wrap: wrap).
func flowRow(gtx layout.Context, gapDp float32, items []layout.Widget) layout.Dimensions {
	gap := int(px(gtx, gapDp))
	width := gtx.Constraints.Max.X
	x, y, rowH, used := 0, 0, 0, 0
	for _, item := range items {
		macro := op.Record(gtx.Ops)
		cg := gtx
		cg.Constraints = layout.Constraints{Max: image.Pt(width, gtx.Constraints.Max.Y)}
		d := item(cg)
		call := macro.Stop()
		if x > 0 && x+d.Size.X > width {
			x, y, rowH = 0, y+rowH+gap, 0
		}
		o := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		o.Pop()
		x += d.Size.X + gap
		rowH = max(rowH, d.Size.Y)
		used = max(used, x-gap)
	}
	return layout.Dimensions{Size: image.Pt(max(used, gtx.Constraints.Min.X), y+rowH)}
}

// loadApps lists applications for the live exclusion choice (LiveAudioExclusion).
func (r *Room) loadApps() {
	r.mu.Lock()
	if r.appsBusy {
		r.mu.Unlock()
		return
	}
	r.appsBusy = true
	r.mu.Unlock()
	r.invalidate()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targets, err := r.host.Sources(ctx)
	r.mu.Lock()
	r.appsBusy, r.appsFailed = false, err != nil
	if err == nil {
		r.apps = audioApps(targets)
	}
	r.mu.Unlock()
	r.invalidate()
}
