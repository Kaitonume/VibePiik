package nativeui

import (
	"image"
	"math"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
)

// Noise gate for the room voice microphone: below the threshold the
// microphone sends silence. The threshold is a position on the level meter
// (0–1, the meter's sqrt scale), so the slider and the bar line up; 0 = off.

const gateHold = 300 * time.Millisecond // stays open after speech ends

// gateThreshold turns a meter position into an RMS level (inverse of the
// meter's fill = sqrt(level)*1.6).
func gateThreshold(position float64) float32 {
	if position <= 0 {
		return 0
	}
	return float32(math.Pow(position/1.6, 2))
}

// voiceGate is the gate's state on the capture goroutine.
type voiceGate struct {
	until time.Time
	gain  float32 // current gain, ramped to avoid clicks
}

// apply opens or closes the gate for this frame (level measured before) and
// returns whether the frame carries voice.
func (g *voiceGate) apply(frame []int16, level float32, position float64, now time.Time) bool {
	if position <= 0 {
		g.gain = 1
		return true
	}
	if level >= gateThreshold(position) {
		g.until = now.Add(gateHold)
	}
	target := float32(0)
	if now.Before(g.until) {
		target = 1
	}
	if g.gain == target && target == 1 {
		return true
	}
	// Ramp across the frame (20 ms) from the current gain to the target.
	start := g.gain
	for i := range frame {
		gain := start + (target-start)*float32(i+1)/float32(len(frame))
		frame[i] = int16(float32(frame[i]) * gain)
	}
	g.gain = target
	return target == 1
}

// SetGate sets the noise gate (meter position 0–1, 0 = off) and remembers it.
func (v *voiceEndpoint) SetGate(position float64) {
	position = min(max(position, 0), 1)
	v.mu.Lock()
	v.gate = position
	v.mu.Unlock()
	go updatePreferences(func(p *preferences) { p.VoiceGate = position })
	v.host.changed()
}

func (h *hostSession) VoiceGate(position float64) { h.voice.SetGate(position) }

func init() {
	nativeCopy_en["native.voice.gate"] = "Noise gate"
	nativeCopy_en["native.voice.gateOff"] = "Off"
	nativeCopy_en["native.voice.gateHint"] = "Your microphone is sent only while it is louder than this line. Talk and drag it just above the bar's level when you are silent. 0 = off."
	nativeCopy_en["native.voice.processingHint"] = "Windows voice processing: noise suppression, echo cancellation and automatic volume together. Windows sets its strength; it cannot be adjusted. For finer control, turn it off and use the noise gate."
	nativeCopy_zh["native.voice.gate"] = "噪声门"
	nativeCopy_zh["native.voice.gateOff"] = "关"
	nativeCopy_zh["native.voice.gateHint"] = "只有麦克风音量高于这条线时才会发送。安静时把它拖到电平条稍上方。0 = 关闭。"
	nativeCopy_zh["native.voice.processingHint"] = "Windows 语音处理：同时进行降噪、回声消除和自动音量。强度由 Windows 决定，无法调节。想更精细地控制，请关闭它并使用噪声门。"
	nativeCopy_it["native.voice.gate"] = "Soglia microfono (noise gate)"
	nativeCopy_it["native.voice.gateOff"] = "Spento"
	nativeCopy_it["native.voice.gateHint"] = "Il microfono viene inviato solo quando è più forte di questa linea. Stando in silenzio, trascinala appena sopra il livello della barra. 0 = spento."
	nativeCopy_it["native.voice.processingHint"] = "Elaborazione voce di Windows: riduzione del rumore, cancellazione dell'eco e volume automatico insieme. La forza la decide Windows e non si può regolare. Per un controllo più fine spegnila e usa la soglia del microfono."
}

// gateRow is the noise gate setting: value, slider and the microphone level
// bar with the threshold line on it (green while the gate is open).
func gateRow(gtx layout.Context, u *UI, slider *Slider, position float64, open bool, level float32, set func(float64)) layout.Dimensions {
	p := &u.P
	percent := int(math.Round(position * 100))
	value, unitText := itoa(percent), "%"
	if percent == 0 {
		value, unitText = u.T("native.voice.gateOff"), ""
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return heading(gtx, u, "microphoneOff", u.T("native.voice.gate"))
				}),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return valueBox(gtx, u, value, unitText) }))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d, moved, changed := slider.Layout(gtx, u, float32(percent), 0, 100, 1)
			if changed && int(moved) != percent {
				set(float64(moved) / 100)
			}
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if level < 0 {
				return layout.Dimensions{}
			}
			width := gtx.Constraints.Max.X - gtx.Dp(18)
			size := image.Pt(width, gtx.Dp(6))
			track := boxOf(size, px(gtx, 3))
			defer op.Offset(image.Pt(gtx.Dp(9), gtx.Dp(2))).Push(gtx.Ops).Pop()
			fillBox(gtx.Ops, track, p.Wall2)
			fill := float32(math.Min(1, math.Sqrt(float64(level))*1.6))
			if fill > 0.01 {
				bar := track
				bar.rect.Max.X = bar.rect.Min.X + fill*float32(width)
				c := withAlpha(p.Muted, .7) // cut by the gate
				if open {
					c = p.Live
				}
				fillBox(gtx.Ops, bar, c)
			}
			if position > 0 {
				x := float32(position) * float32(width)
				fillBox(gtx.Ops, boxOf(image.Pt(gtx.Dp(2), gtx.Dp(12)), 1).offset(x-px(gtx, 1), -px(gtx, 3)), p.Warn)
			}
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(80 * time.Millisecond)})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(14))}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.Visual() {
				return layout.Dimensions{}
			}
			return u.label(gtx, u.T("native.voice.gateHint"), textStyle{size: 11, color: p.Muted, lineH: 1.4})
		}))
}

func init() {
	nativeCopy_en["native.excludeVoice"] = "In room voice VibePiik leaves out its own sound (the others' voices) instead, so nobody hears themselves: Windows allows one application only. Your choice returns when you leave the voice."
	nativeCopy_zh["native.excludeVoice"] = "加入房间语音时，VibePiik 会改为排除自己的声音（其他人的语音），避免有人听到自己的回声：Windows 只允许排除一个应用。离开语音后恢复你的选择。"
	nativeCopy_it["native.excludeVoice"] = "Nella voce della stanza VibePiik esclude invece il proprio suono (le voci degli altri), così nessuno sente sé stesso: Windows permette di escluderne una sola. La tua scelta torna quando esci dalla voce."
}
