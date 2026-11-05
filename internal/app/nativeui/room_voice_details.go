package nativeui

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
)

// voiceDetails lists the room-voice connections (voice.details.*), read from
// the voice endpoint every 2 s while Connection details is open.
func (r *Room) voiceDetails(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	if !snap.Voice.Enabled || !snap.Voice.Joined {
		return layout.Dimensions{}
	}
	r.mu.Lock()
	links, at := r.voiceLinkList, r.voiceLinksAt
	if gtx.Now.Sub(at) > 2*time.Second && !r.voiceLinksBusy {
		r.voiceLinksBusy = true
		go func() {
			list := r.host.VoiceLinks()
			r.mu.Lock()
			r.voiceLinkList, r.voiceLinksAt, r.voiceLinksBusy = list, time.Now(), false
			r.mu.Unlock()
			r.invalidate()
		}()
	}
	r.mu.Unlock()
	p := &u.P
	names := map[string]string{snap.HostID: displayName(u, snap.Name)}
	for _, viewer := range snap.Viewers {
		names[viewer.ID] = viewer.Name
	}
	rows := []layout.FlexChild{
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return heading(gtx, u, "headphones", u.T("voice.details.title"))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
	}
	if len(links) == 0 {
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, u.T("voice.details.joined"), textStyle{size: 11.5, color: p.Muted})
		}))
	}
	for _, link := range links {
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			name := names[link.Peer]
			if name == "" {
				name = u.T("voice.participant")
			}
			metrics := u.T("stats.unknown")
			if link.HasStats {
				metrics = fmt.Sprintf("RTT %d ms · %s %.1f%% · %s %.1f ms", int(link.RTTms), u.T("stats.loss"), link.LossPercent,
					u.T("stats.jitter"), link.JitterMs)
			}
			ink := p.Live
			if link.State != "connected" {
				ink = p.Warn
			}
			return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return miniPerson(gtx, 20, participantColor(link.Peer)) }),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(140)
						return u.label(gtx, name, textStyle{size: 12, weight: font.Bold, color: p.Ink, maxLines: 1})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(110)
						return u.label(gtx, link.State, textStyle{size: 11.5, mono: true, color: ink, maxLines: 1})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, metrics, textStyle{size: 11.5, mono: true, color: p.Ink, maxLines: 1})
					}))
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

// voiceMeter is the member level meter: three bars that light up with the
// voice level (RMS, 0–1).
func voiceMeter(gtx layout.Context, level float32, on, off color.NRGBA) layout.Dimensions {
	w, gap := px(gtx, 3), px(gtx, 2)
	heights := []float32{6, 10, 14}
	thresholds := []float32{0.02, 0.06, 0.14}
	full := px(gtx, 14)
	for i, h := range heights {
		c := off
		if level >= thresholds[i] {
			c = on
		}
		bar := boxOf(image.Pt(int(w), int(px(gtx, h))), w/2).offset(float32(i)*(w+gap), full-px(gtx, h))
		fillBox(gtx.Ops, bar, c)
	}
	return dimsOf(int(3*w+2*gap), int(full))
}
