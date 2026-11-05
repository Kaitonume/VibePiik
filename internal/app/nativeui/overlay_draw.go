package nativeui

import (
	"hash/fnv"
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
)

// On-screen chat (RoomChatOverlay) and the chat over other apps.

const (
	overlayDuration = 6 * time.Second // CHAT_OVERLAY_DURATION_MS
	overlayLanes    = 3
)

// overlayLook is overlayAppearance (CHAT_OVERLAY_LIMITS).
type overlayLook struct {
	Scale, Opacity, Speed float64
	ColorBySender         bool
}

func defaultLook() overlayLook { return overlayLook{Scale: 1, Opacity: 1, Speed: 1} }

func (l overlayLook) clamped() overlayLook {
	clamp := func(v, lo, hi, fallback float64) float64 {
		if v == 0 {
			return fallback
		}
		return min(max(v, lo), hi)
	}
	l.Scale = clamp(l.Scale, .8, 1.3, 1)
	l.Opacity = clamp(l.Opacity, .4, 1, 1)
	l.Speed = clamp(l.Speed, .5, 1.5, 1)
	return l
}

// overlayFlight is one message crossing the picture.
type overlayFlight struct {
	ID, PeerID, Text string
	Lane             int
	Start            time.Time
	Duration         time.Duration
}

func (f overlayFlight) done(now time.Time) bool { return now.Sub(f.Start) >= f.Duration }

// flightLane is the fixed lane of a message: identityHash(peer:id) % 3.
func flightLane(peerID, id string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(peerID + ":" + id))
	return int(h.Sum32() % overlayLanes)
}

// addFlight adds a message; the newest on its lane wins (no queue).
func addFlight(flights []overlayFlight, message chatMessage, look overlayLook, now time.Time) []overlayFlight {
	lane := flightLane(message.PeerID, message.ID)
	kept := flights[:0]
	for _, f := range flights {
		if f.Lane != lane && !f.done(now) {
			kept = append(kept, f)
		}
	}
	duration := time.Duration(float64(overlayDuration) / look.clamped().Speed)
	return append(kept, overlayFlight{ID: message.ID, PeerID: message.PeerID, Text: message.Text, Lane: lane, Start: now, Duration: duration})
}

var overlayPill = color.NRGBA{R: 15, G: 25, B: 35, A: 173} // rgb(15 25 35 / .68)

// drawFlights paints the flights over a picture of gtx's size; it reports
// whether any is still moving.
func drawFlights(gtx layout.Context, u *UI, flights []overlayFlight, look overlayLook, now time.Time) bool {
	look = look.clamped()
	size := gtx.Constraints.Max
	moving := false
	defer paint.PushOpacity(gtx.Ops, float32(look.Opacity)).Pop()
	// font-size: clamp(14px, 2cqw, 18px) * scale.
	fontPx := min(max(14, float32(size.X)*0.02/gtx.Metric.PxPerDp), 18) * float32(look.Scale)
	for _, flight := range flights {
		if flight.done(now) {
			continue
		}
		top := gtx.Dp(12) + flight.Lane*gtx.Dp(34)
		if top+gtx.Dp(32) > size.Y {
			continue // the lane does not fit this picture
		}
		moving = true
		ink := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		if look.ColorBySender {
			ink = mix(participantColor(flight.PeerID), ink, .25)
		}
		macro := op.Record(gtx.Ops)
		tg := gtx
		tg.Constraints = layout.Constraints{Max: image.Pt(size.X*8/10-gtx.Dp(20), gtx.Dp(40))}
		text := u.label(tg, flight.Text, textStyle{size: fontPx, weight: font.SemiBold, color: ink, maxLines: 1})
		call := macro.Stop()
		pill := image.Pt(text.Size.X+gtx.Dp(20), max(text.Size.Y+gtx.Dp(4), gtx.Dp(28)))
		progress := float32(now.Sub(flight.Start)) / float32(flight.Duration)
		x := float32(size.X) - progress*float32(size.X+pill.X)
		o := op.Offset(image.Pt(int(x), top)).Push(gtx.Ops)
		fillBox(gtx.Ops, boxOf(pill, float32(pill.Y)/2), overlayPill)
		to := op.Offset(image.Pt(gtx.Dp(10), (pill.Y-text.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		to.Pop()
		o.Pop()
	}
	return moving
}

// overlayLine is one entry of the chat over other apps.
type overlayLine struct {
	ID, PeerID, Name, Text string
	Reaction, TargetName   string
	Until                  time.Time
}

// desktopCorners are the positions of the chat over other apps.
var desktopCorners = []string{"top-left", "top-right", "bottom-left", "bottom-right"}

// drawChatBox paints the latest lines as cards stacked toward corner.
func drawChatBox(gtx layout.Context, u *UI, lines []overlayLine, look overlayLook, corner string) {
	look = look.clamped()
	size := gtx.Constraints.Max
	defer paint.PushOpacity(gtx.Ops, float32(look.Opacity)).Pop()
	bottom := corner == "bottom-left" || corner == "bottom-right"
	right := corner == "top-right" || corner == "bottom-right"
	scale := float32(look.Scale)
	gap := gtx.Dp(6)
	type card struct {
		call op.CallOp
		size image.Point
	}
	var cards []card
	used := 0
	// Newest first when measuring, so the newest always fit.
	for i := len(lines) - 1; i >= 0 && used < size.Y; i-- {
		line := lines[i]
		macro := op.Record(gtx.Ops)
		dims := chatCard(gtx, u, line, look, scale, size.X)
		call := macro.Stop()
		if used+dims.Size.Y > size.Y {
			break
		}
		cards = append(cards, card{call, dims.Size})
		used += dims.Size.Y + gap
	}
	y := 0
	if bottom {
		y = size.Y
	}
	for i := range cards {
		// Chat order either way: bottom corners grow upward from the newest,
		// top corners list from the oldest kept down to the newest.
		c := cards[i]
		if !bottom {
			c = cards[len(cards)-1-i]
		}
		x := 0
		if right {
			x = size.X - c.size.X
		}
		if bottom {
			y -= c.size.Y
			o := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			c.call.Add(gtx.Ops)
			o.Pop()
			y -= gap
		} else {
			o := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			c.call.Add(gtx.Ops)
			o.Pop()
			y += c.size.Y + gap
		}
	}
}

// chatCard is one dark card: sender name in their colour, then the text or reaction.
func chatCard(gtx layout.Context, u *UI, line overlayLine, look overlayLook, scale float32, maxWidth int) layout.Dimensions {
	pad := int(float32(gtx.Dp(10)) * scale)
	inner := maxWidth - 2*pad
	nameColor := mix(participantColor(line.PeerID), color.NRGBA{R: 255, G: 255, B: 255, A: 255}, .35)
	textColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if look.ColorBySender {
		textColor = mix(participantColor(line.PeerID), textColor, .25)
	}
	macro := op.Record(gtx.Ops)
	cg := gtx
	cg.Constraints = layout.Constraints{Max: image.Pt(inner, gtx.Dp(400))}
	content := layout.Flex{Axis: layout.Vertical}.Layout(cg,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			name := line.Name
			if line.TargetName != "" {
				name += " → " + line.TargetName
			}
			return u.label(gtx, name, textStyle{size: 12 * scale, weight: font.Bold, color: nameColor, maxLines: 1})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if line.Reaction != "" {
				side := 26 * scale
				return reactionIcon(gtx, line.Reaction, side)
			}
			return u.label(gtx, line.Text, textStyle{size: 15 * scale, weight: font.SemiBold, color: textColor, maxLines: 3, lineH: 1.3})
		}))
	call := macro.Stop()
	size := image.Pt(content.Size.X+2*pad, content.Size.Y+2*pad*3/4)
	fillBox(gtx.Ops, boxOf(size, float32(gtx.Dp(12))*scale), color.NRGBA{R: 15, G: 25, B: 35, A: 190})
	o := op.Offset(image.Pt(pad, pad*3/8)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	return layout.Dimensions{Size: size}
}
