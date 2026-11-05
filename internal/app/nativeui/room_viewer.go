package nativeui

import (
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

// evidenceDetail is the rest of viewer-quality-evidence metrics: what the
// per-viewer card shows under "Detailed metrics".
type evidenceDetail struct {
	JitterMs              *float64 `json:"jitterMs"`
	FramesDecodedDelta    *float64 `json:"framesDecodedDelta"`
	FramesDroppedDelta    *float64 `json:"framesDroppedDelta"`
	DecodeMsPerFrame      *float64 `json:"decodeMsPerFrame"`
	FreezeCountDelta      *float64 `json:"freezeCountDelta"`
	FreezeDurationMsDelta *float64 `json:"freezeDurationMsDelta"`
	Codec                 *string  `json:"codec"`
	AudioBitrateKbps      *float64 `json:"audioBitrateKbps"`
	AudioLossPercent      *float64 `json:"audioPacketLossPercent"`
	AudioJitterMs         *float64 `json:"audioJitterMs"`
	PlayoutDeltaMs        *float64 `json:"audioVideoPlayoutDeltaMs"`
	VideoBufferMs         *float64 `json:"videoJitterBufferDelayMs"`
	AudioBufferMs         *float64 `json:"audioJitterBufferDelayMs"`
	AudioConcealedPercent *float64 `json:"audioConcealedSamplesPercent"`
	AudioCodec            *string  `json:"audioCodec"`
}

// statLine is one detailed metric: group and label are copy keys.
type statLine struct {
	Group, Label, Value string
	Warn                bool
}

func (d evidenceDetail) lines() []statLine {
	var out []statLine
	add := func(group, label string, value *float64, format string) {
		if value != nil {
			out = append(out, statLine{Group: group, Label: label, Value: fmt.Sprintf(format, *value)})
		}
	}
	add("stats.group.network", "stats.jitter", d.JitterMs, "%.1f ms")
	if d.Codec != nil {
		codec := strings.TrimPrefix(*d.Codec, "video/")
		warn := !strings.EqualFold(codec, "H264") && !strings.EqualFold(codec, "VP8")
		out = append(out, statLine{Group: "stats.group.playback", Label: "stats.codec", Value: codec, Warn: warn})
	}
	add("stats.group.playback", "stats.decodeMs", d.DecodeMsPerFrame, "%.1f ms")
	add("stats.group.playback", "stats.dropped", d.FramesDroppedDelta, "%.0f")
	add("stats.group.playback", "stats.freezeCount", d.FreezeCountDelta, "%.0f")
	add("stats.group.playback", "stats.freezeDuration", d.FreezeDurationMsDelta, "%.0f ms")
	add("stats.group.playback", "stats.videoBuffer", d.VideoBufferMs, "%.0f ms")
	add("stats.group.playback", "stats.playoutDelta", d.PlayoutDeltaMs, "%.0f ms")
	add("stats.group.sourceAudio", "stats.audio", d.AudioBitrateKbps, "%.0f kbps")
	add("stats.group.sourceAudio", "stats.audioLoss", d.AudioLossPercent, "%.1f%%")
	add("stats.group.sourceAudio", "stats.audioJitter", d.AudioJitterMs, "%.1f ms")
	add("stats.group.sourceAudio", "stats.audioBuffer", d.AudioBufferMs, "%.0f ms")
	add("stats.group.sourceAudio", "stats.audioConcealedRate", d.AudioConcealedPercent, "%.1f%%")
	if d.AudioCodec != nil && !strings.EqualFold(strings.TrimPrefix(*d.AudioCodec, "audio/"), "opus") {
		out = append(out, statLine{Group: "stats.group.sourceAudio", Label: "stats.warn.audioCodec",
			Value: strings.TrimPrefix(*d.AudioCodec, "audio/"), Warn: true})
	}
	return out
}

// viewerState is the status label of a viewer row (state.peer.*).
func viewerState(viewer roomPeer, stats viewerStats, hasStats bool) (key string, tone string) {
	switch {
	case !viewer.MediaReady && viewer.Upstream == "":
		return "state.peer.connecting", "busy"
	case !viewer.MediaReady:
		return "state.peer.routing", "busy"
	case hasStats && stats.Stutter && time.Since(stats.At) < 10*time.Second:
		return "state.peer.stuttering", "warn"
	}
	return "state.peer.connected", "good"
}

// viewerCard is PawnDetail: the selected viewer's primary metrics and,
// on demand, the detailed ones grouped like the web page.
type viewerCard struct {
	close Button
	more  Summary
	hints [5]metricHint
}

// metricHint is the hover hint of one metric (stats.*Hint with its comic).
type metricHint struct {
	click   widget.Clickable
	hoverAt time.Time
}

func (r *Room) viewerCard(gtx layout.Context, u *UI, snap roomSnapshot, viewer roomPeer) layout.Dimensions {
	p := &u.P
	c := &r.card
	if c.close.Clicked(gtx) {
		r.selectedViewer = ""
	}
	stats, ok := snap.Stats[viewer.ID]
	unknown := u.T("stats.unknown")
	res, fps, kbps, loss, rtt := unknown, unknown, unknown, unknown, unknown
	if ok {
		if stats.Width > 0 {
			res = fmt.Sprintf("%d×%d", stats.Width, stats.Height)
		}
		fps = fmt.Sprintf("%.1f fps", stats.FPS)
		kbps = fmt.Sprintf("%d kbps", int(stats.Kbps))
		loss = fmt.Sprintf("%.1f%%", stats.LossPercent)
		if stats.HasRTT {
			rtt = fmt.Sprintf("%d ms", int(stats.RTTms))
		}
	}
	metricIndex := 0
	metric := func(icon, label, value string) layout.FlexChild {
		hint := &c.hints[metricIndex%len(c.hints)]
		key := []string{"resolution", "fps", "bitrate", "loss", "rtt"}[metricIndex%5]
		metricIndex++
		return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return hint.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				d := r.metricCell(gtx, u, icon, label, value)
				tip := u.T("stats." + key + "Hint")
				if tip == "" {
					tip = label
				}
				hint.hoverAt = hoverTooltipComic(gtx, u, hint.click.Hovered(), hint.hoverAt, tip, "hint-metric-"+key, d.Size)
				return d
			})
		})
	}
	stateKey, _ := viewerState(viewer, stats, ok)
	return deckRow(gtx, u, true, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return miniPerson(gtx, 26, participantColor(viewer.ID)) }),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, viewer.Name, textStyle{size: 13, weight: font.Bold, color: p.Ink, maxLines: 1})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, u.T("stats.sharedMedia")+" · "+u.T(stateKey)+" · P2P", textStyle{size: 10.5, color: p.Muted, maxLines: 1})
							}))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.close.Layout(gtx, u, ButtonStyle{Icon: "x", Size: sizeDense, Tone: toneQuiet, Tip: u.T("common.close")})
					}))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					metric("expand", u.T("stats.resolution"), res),
					metric("frames", u.T("stats.fps"), fps),
					metric("gauge", u.T("stats.bitrate"), kbps),
					metric("packetLoss", u.T("stats.loss"), loss),
					metric("clock", u.T("stats.rtt"), rtt))
			}),
		}
		if ok && len(stats.Detail) > 0 {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					caption := u.T("stats.more")
					if c.more.Open {
						caption = u.T("stats.less")
					}
					return c.more.Layout(gtx, u, "sliders", caption, "")
				}))
			if c.more.Open {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return detailGroups(gtx, u, stats.Detail)
				}))
			}
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

// detailGroups lists detailed metrics under their group headings, two columns.
func detailGroups(gtx layout.Context, u *UI, lines []statLine) layout.Dimensions {
	p := &u.P
	var groups []string
	byGroup := map[string][]statLine{}
	for _, line := range lines {
		if _, seen := byGroup[line.Group]; !seen {
			groups = append(groups, line.Group)
		}
		byGroup[line.Group] = append(byGroup[line.Group], line)
	}
	var children []layout.FlexChild
	for _, group := range groups {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, strings.ToUpper(u.T(group)), textStyle{size: 9.5, weight: font.Bold, color: p.Muted})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var items []layout.Widget
				colW := (gtx.Constraints.Max.X - int(px(gtx, 16))) / 2
				for _, line := range byGroup[group] {
					items = append(items, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(colW, int(px(gtx, 22))))
						label := u.T(line.Label, "codec", line.Value)
						value, color := line.Value, p.Ink
						if line.Warn {
							color = p.DangerText
						}
						if line.Label == "stats.warn.audioCodec" {
							value = ""
						}
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, label, textStyle{size: 11, color: p.Muted, maxLines: 1})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, value, textStyle{size: 11.5, mono: true, weight: font.Bold, color: color, maxLines: 1})
							}))
					})
				}
				return flowRow(gtx, 16, items)
			}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// viewerClick is the clickable row of one viewer in the overview table.
func (r *Room) viewerClick(id string) *widget.Clickable {
	if r.viewerRows == nil {
		r.viewerRows = map[string]*widget.Clickable{}
	}
	click, ok := r.viewerRows[id]
	if !ok {
		click = &widget.Clickable{}
		r.viewerRows[id] = click
	}
	return click
}

var _ = op.Offset

// metricCell is one primary metric: icon and label over the value.
func (r *Room) metricCell(gtx layout.Context, u *UI, icon, label, value string) layout.Dimensions {
	p := &u.P
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, icon, 13, p.Muted) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, label, textStyle{size: 10, color: p.Muted, maxLines: 1})
				}))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, value, textStyle{size: 12, mono: true, weight: font.Bold, color: p.Ink, maxLines: 1})
		}))
}
