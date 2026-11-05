package nativeui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/TNTcraftHIM/Piik/internal/server/protocol"
)

// Panels of the host room: sharing settings, access row, connection details
// and topology (ROOM-SPEC.md, HostPage).

type resolutionChoice struct {
	label string
	w, h  int
}

var resolutionChoices = []resolutionChoice{
	{"480p", 854, 480}, {"720p", 1280, 720}, {"1080p", 1920, 1080}, {"1440p", 2560, 1440}, {"2160p", 3840, 2160},
}

var presetOrder = []struct {
	key, title, icon string
}{
	{"720p30", "host.quality.light", "frames"},
	{"1080p30", "host.quality.hd", "frames"},
	{"1080p60", "host.quality.highFps", "frames"},
}

var audioChoices = []struct {
	key  string
	kbps int
}{
	{"host.advanced.audio.saver", 64}, {"host.advanced.audio.music", 128}, {"host.advanced.audio.veryHigh", 192},
}

// sharingPanel holds the widgets of .lr-sharing-panel.
type sharingPanel struct {
	tabs       Segmented
	sound      bool
	tiles      [3]Tile
	fine       Summary
	conn       Summary
	resChips   [5]Chip
	prefChips  [3]Chip
	codecChips [3]Chip
	audioChips [3]Chip
	fps        Slider
	bitrate    Slider
	topology   Switch
	privacy    Switch
	mic        Switch
	volume     Slider
	muteShare  Button
	audioOn    Switch

	resW, resH, fpsBox, rateBox, audioBox NumberBox
	resInvalid                            bool
	fpsCommon, rateCommon                 Button
	micPicker                             devicePicker
	micVolume                             Slider
	autoPause                             Switch
	codecInUse                            string
}

func (r *Room) sharingSettings(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	p := &u.P
	s := &r.panel
	settings := snap.Settings
	// While a slider is dragged its value waits for the release before it
	// goes to the App, but the numbers follow it live.
	r.mu.Lock()
	if r.pendingSettings != nil {
		settings = *r.pendingSettings
	}
	r.mu.Unlock()
	quality := settings.Quality
	busy := snap.Phase == phaseStarting || snap.Switching
	apply := func() {
		if !busy {
			r.host.SetSettings(settings)
		}
	}
	live := snap.Phase == phaseLive || snap.Phase == phasePaused
	s.codecInUse = snap.CodecInUse
	maxAudio := settings.limits().MaxAudioBitrate

	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(820))
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return card(gtx, u, 24, layout.UniformInset(unit.Dp(24)), func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(236))
					selected := 0
					if s.sound {
						selected = 1
					}
					d, chosen := s.tabs.LayoutWide(gtx, u, []string{"tv", "speaker"},
						[]string{u.T("host.settings.picture"), u.T("host.settings.sound")}, selected)
					s.sound = chosen == 1
					return d
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(22)}.Layout),
		}
		if !s.sound {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gap := int(px(gtx, 12))
					tileW := (gtx.Constraints.Max.X - 2*gap) / 3
					height := 0
					for i, preset := range presetOrder {
						value := qualityPresets[preset.key]
						if s.tiles[i].Clicked(gtx) {
							value.MaxAudioBitrate = quality.MaxAudioBitrate
							value.DegradationPreference = quality.DegradationPreference
							settings.Quality = value
							apply()
						}
						o := op.Offset(image.Pt(i*(tileW+gap), 0)).Push(gtx.Ops)
						tg := gtx
						tg.Constraints = layout.Exact(image.Pt(tileW, int(px(gtx, 106))))
						selected := quality.Resolution == value.Resolution && quality.MaxFramerate == value.MaxFramerate && quality.MaxBitrate == value.MaxBitrate
						lines := 3
						if value.Resolution.Height <= 720 {
							lines = 2
						}
						d := s.tiles[i].Layout(tg, u, lines, value.MaxFramerate > 30, u.T(preset.title),
							fmt.Sprintf("%dp %d fps", value.Resolution.Height, int(value.MaxFramerate)), selected)
						o.Pop()
						height = d.Size.Y
					}
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					summary := fmt.Sprintf("%d × %d   %d fps   %s Mbps", quality.Resolution.Width, quality.Resolution.Height,
						int(quality.MaxFramerate), trimFloat(float64(quality.MaxBitrate)/1e6))
					return s.fine.Layout(gtx, u, "sliders", u.T("host.settings.pictureDetails"), summary)
				}),
			)
			if s.fine.Open {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return r.fineTune(gtx, u, &settings, apply, live)
					})
				}))
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							on := r.autoPause
							d := switchRow(gtx, u, &s.autoPause, u.T("host.previewAutoPause"), &on, false)
							if on != r.autoPause {
								r.autoPause = on
								go updatePreferences(func(p *preferences) { p.PreviewNoAutoPause = !on })
							}
							return d
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if u.Visual() {
								return layout.Dimensions{}
							}
							return u.label(gtx, u.T("host.previewAutoPauseHint"), textStyle{size: 11, color: p.Muted, lineH: 1.4})
						}))
				})
			}))
		} else {
			if live && snap.ShareVolumeOK {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if s.muteShare.Clicked(gtx) {
						r.host.SetShareVolume(snap.ShareVolume, !snap.ShareMuted)
					}
					percent := int(math.Round(snap.ShareVolume * 100))
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									icon := "speaker"
									if snap.ShareMuted {
										icon = "speakerOff"
									}
									return s.muteShare.Layout(gtx, u, ButtonStyle{Icon: icon, Size: sizeDense, Pressed: snap.ShareMuted})
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return u.label(gtx, u.T("host.shareVolume"), textStyle{size: 12.5, weight: font.SemiBold, color: p.Muted, maxLines: 2})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return valueBox(gtx, u, itoa(percent), "%") }))
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							d, value, moved := s.volume.Layout(gtx, u, float32(snap.ShareVolume*100), 0, 200, 5)
							if moved && int(value) != percent {
								r.host.SetShareVolume(float64(value)/100, false)
							}
							return d
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout))
				}))
			}
			if live {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					key := "host.sourcePicker.windowAudio"
					if snap.Target.Kind == "display" {
						key = "host.sourcePicker.systemAudio"
					}
					on := snap.Audio
					d := switchRow(gtx, u, &s.audioOn, u.T(key), &on, snap.Switching)
					if on != snap.Audio {
						r.host.SetSourceAudio(on)
					}
					return layout.Inset{Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return d })
				}))
			}
			if live && snap.CanExclude && snap.Target.Kind == "display" && snap.Audio {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					r.mu.Lock()
					apps, busy, failed := r.apps, r.appsBusy, r.appsFailed
					r.mu.Unlock()
					r.liveExclude.VoiceOn = snap.Voice.Joined
					d, result := r.liveExclude.Layout(gtx, u, apps, snap.Exclude, busy, failed, snap.Switching || snap.Phase == phasePaused)
					if result.Opened || result.Refresh {
						go r.loadApps()
					}
					if result.Changed {
						r.host.SetExcludeAudio(result.Target)
					}
					return layout.Inset{Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return d })
				}))
			}
			if settings.MicAvail && live {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					mic := settings.Microphone
					d := switchRow(gtx, u, &s.mic, u.T("host.commentary.include"), &mic, false)
					if mic != settings.Microphone {
						settings.Microphone = mic
						apply()
					}
					return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return d })
				}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if u.Visual() {
							return layout.Dimensions{}
						}
						text, color := u.T("host.commentary.description"), p.Muted
						if settings.Microphone && snap.Voice.Microphone {
							text, color = u.T("host.commentary.duplicate"), p.DangerText
						}
						return u.label(gtx, text, textStyle{size: 11.5, color: color, lineH: 1.4})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout))
				if settings.Microphone {
					if r.needDevices(&r.micList) {
						r.loadDevices(&r.micList, r.commentaryMicrophones)
					}
					children = append(children,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							r.mu.Lock()
							list := r.micList
							r.mu.Unlock()
							d, changed, chosen, refresh := s.micPicker.Layout(gtx, u, list, settings.MicDevice)
							if changed {
								r.host.SetCommentary(chosen, settings.MicVolume)
							}
							if refresh {
								r.loadDevices(&r.micList, r.commentaryMicrophones)
							}
							return d
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return volumeRow(gtx, u, &s.micVolume, "microphone", u.T("host.microphone.volume"), settings.MicVolume, -1,
								func(volume float64) { r.host.SetCommentary(settings.MicDevice, volume) })
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout))
				}
			}
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return heading(gtx, u, "speaker", u.T("host.advanced.audio"))
						}),
						layout.Flexed(1, layout.Spacer{}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							d, typed, ok := s.audioBox.Layout(gtx, u, itoa(int(quality.MaxAudioBitrate)/1000), "kbps", 110, false)
							if ok {
								value, valid := parseNumber(typed)
								if !valid || value != math.Trunc(value) || value < minAudioBitrate/1000 || int(value)*1000 > maxAudio {
									s.audioBox.Invalid = true
								} else if int(value)*1000 != int(quality.MaxAudioBitrate) {
									s.audioBox.Invalid = false
									settings.Quality.MaxAudioBitrate = protocol.Int(int(value) * 1000)
									apply()
								}
							}
							return d
						}))
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					var row []layout.FlexChild
					for i, choice := range audioChoices {
						allowed := choice.kbps*1000 <= maxAudio
						if s.audioChips[i].Clicked(gtx) && allowed {
							s.audioBox.Invalid = false
							settings.Quality.MaxAudioBitrate = protocol.Int(choice.kbps * 1000)
							apply()
						}
						row = append(row, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.audioChips[i].Layout(gtx, u, "", u.T(choice.key), itoa(choice.kbps),
								int(quality.MaxAudioBitrate) == choice.kbps*1000, !allowed)
						}), layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
					}
					return layout.Flex{}.Layout(gtx, row...)
				}),
			)
		}
		// Connection settings.
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, int(px(gtx, 1)))
				fillRect(gtx.Ops, size, p.Wall2)
				return layout.Dimensions{Size: size}
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.conn.Layout(gtx, u, "branch", u.T("host.settings.connection"), "")
			}),
		)
		if s.conn.Open {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					half := (gtx.Constraints.Max.X - int(px(gtx, 16))) / 2
					return layout.Flex{}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X, gtx.Constraints.Max.X = half, half
							topology := settings.Topology
							d := switchRow(gtx, u, &s.topology, u.T("host.advanced.route.topo"), &topology, false)
							if topology != settings.Topology {
								settings.Topology = topology
								apply()
							}
							d.Size.X = half
							if u.Visual() {
								return d
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return d }),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.label(gtx, u.T("host.advanced.route.topoHint"), textStyle{size: 11, color: u.P.Muted, lineH: 1.4})
								}))
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							on := true
							d := switchRow(gtx, u, &s.privacy, u.T("host.advanced.route.peerOnly"), &on, true)
							if u.Visual() {
								return d
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return d }),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.label(gtx, u.T("native.privacyHint"), textStyle{size: 11, color: u.P.Muted, lineH: 1.4})
								}))
						}))
				})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}

func (r *Room) fineTune(gtx layout.Context, u *UI, settings *shareSettings, apply func(), live bool) layout.Dimensions {
	s := &r.panel
	q := settings.Quality
	limits := settings.limits()
	half := (gtx.Constraints.Max.X - int(px(gtx, 24))) / 2
	setResolution := func(width, height int) {
		settings.Quality.Resolution = protocol.QualityResolution{Width: protocol.Int(width), Height: protocol.Int(height)}
		if ceiling := framerateCeiling(width, height, limits); ceiling > 0 && int(settings.Quality.MaxFramerate) > ceiling {
			settings.Quality.MaxFramerate = protocol.Num(ceiling)
		}
		apply()
	}
	resolutionText := []string{"width", itoa(limits.MaxWidth), "height", itoa(limits.MaxHeight), "pixels", groupNumber(limits.MaxPixels)}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return heading(gtx, u, "expand", u.T("host.advanced.resolution"))
				}),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.resolutionBoxes(gtx, u, q, limits, setResolution)
				}))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !s.resInvalid {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T("host.video.resolutionInvalid", resolutionText...), textStyle{size: 11.5, color: u.P.DangerText})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var row []layout.FlexChild
			for i, choice := range resolutionChoices {
				allowed := framerateCeiling(choice.w, choice.h, limits) >= minVideoFramerate
				if s.resChips[i].Clicked(gtx) && allowed {
					s.resInvalid = false
					setResolution(choice.w, choice.h)
				}
				selected := int(q.Resolution.Width) == choice.w && int(q.Resolution.Height) == choice.h
				row = append(row, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = int(px(gtx, 98))
					return s.resChips[i].Layout(gtx, u, "", choice.label, "", selected, !allowed)
				}), layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
			}
			return layout.Flex{}.Layout(gtx, row...)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			fpsMax := framerateCeiling(int(q.Resolution.Width), int(q.Resolution.Height), limits)
			bitrateMax := float64(limits.MaxBitrate) / 1e6
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = half, half
					return r.rateControl(gtx, u, rateSpec{icon: "frames", caption: u.T("host.advanced.framerate"), unit: "fps",
						box: &s.fpsBox, slider: &s.fps, common: &s.fpsCommon,
						value: float64(q.MaxFramerate), min: minVideoFramerate, max: float64(fpsMax), whole: true,
						sliderMin: 15, sliderMax: 60, step: 5,
						set: func(v float64, rest bool) {
							settings.Quality.MaxFramerate = protocol.Num(v)
							if rest {
								apply()
							} else {
								r.queueSettings(*settings)
							}
						}})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = half, half
					return r.rateControl(gtx, u, rateSpec{icon: "gauge", caption: u.T("host.advanced.bitrate"), unit: "Mbps",
						box: &s.rateBox, slider: &s.bitrate, common: &s.rateCommon,
						value: float64(q.MaxBitrate) / 1e6, min: float64(minVideoBitrate) / 1e6, max: bitrateMax,
						sliderMin: 2, sliderMax: 12, step: 0.5,
						set: func(v float64, rest bool) {
							settings.Quality.MaxBitrate = protocol.Int(math.Round(v * 1e6))
							if rest {
								apply()
							} else {
								r.queueSettings(*settings)
							}
						}})
				}))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return heading(gtx, u, "mountain", u.T("host.advanced.preference"))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			prefs := []struct{ value, key, icon string }{
				{"maintain-resolution", "host.advanced.preference.resolution", "mountain"},
				{"balanced", "host.advanced.preference.balanced", "balance"},
				{"maintain-framerate", "host.advanced.preference.framerate", "frames"},
			}
			var row []layout.FlexChild
			for i, pref := range prefs {
				if s.prefChips[i].Clicked(gtx) {
					settings.Quality.DegradationPreference = pref.value
					apply()
				}
				row = append(row, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.prefChips[i].Layout(gtx, u, pref.icon, u.T(pref.key), "", q.DegradationPreference == pref.value, false)
				}), layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
			}
			return layout.Flex{}.Layout(gtx, row...)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return heading(gtx, u, "puzzle", u.T("host.advanced.codec"))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			codecs := []struct{ value, label string }{{"vp8", "VP8"}, {"auto", "Auto"}, {"h264", "H264"}}
			var row []layout.FlexChild
			for i, codec := range codecs {
				if !live && s.codecChips[i].Clicked(gtx) {
					settings.Codec = codec.value
					apply()
				}
				label := codec.label
				if codec.value == "auto" && settings.Codec == "auto" && live && s.codecInUse != "" && s.codecInUse != "auto" {
					label = "Auto (" + strings.ToUpper(s.codecInUse) + ")" // what Auto resolved to
				}
				row = append(row, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.codecChips[i].Layout(gtx, u, "", label, u.T("host.advanced.codec."+codec.value+"Hint"), settings.Codec == codec.value, live && settings.Codec != codec.value)
				}), layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
			}
			return layout.Flex{}.Layout(gtx, row...)
		}),
	)
}

// resolutionBoxes is the typed width × height pair (ResolutionControl).
func (r *Room) resolutionBoxes(gtx layout.Context, u *UI, q protocol.QualitySettings, limits videoLimits, set func(w, h int)) layout.Dimensions {
	s := &r.panel
	commit := func(width, height string) {
		w, okW := parseNumber(width)
		h, okH := parseNumber(height)
		if !okW || !okH || w != math.Trunc(w) || h != math.Trunc(h) || framerateCeiling(int(w), int(h), limits) < minVideoFramerate {
			s.resInvalid = true
			return
		}
		s.resInvalid = false
		if int(w) != int(q.Resolution.Width) || int(h) != int(q.Resolution.Height) {
			set(int(w), int(h))
		}
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			s.resW.Invalid = s.resInvalid
			d, text, ok := s.resW.Layout(gtx, u, itoa(int(q.Resolution.Width)), "", 70, false)
			if ok {
				commit(text, s.resH.editor.Text())
			}
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, "×", textStyle{size: 13, mono: true, color: u.P.Muted})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			s.resH.Invalid = s.resInvalid
			d, text, ok := s.resH.Layout(gtx, u, itoa(int(q.Resolution.Height)), "px", 92, false)
			if ok {
				commit(s.resW.editor.Text(), text)
			}
			return d
		}))
}

type rateSpec struct {
	icon, caption, unit        string
	box                        *NumberBox
	slider                     *Slider
	common                     *Button
	value, min, max            float64
	whole                      bool
	sliderMin, sliderMax, step float64
	set                        func(value float64, rest bool)
}

// rateControl is NumberControl: heading + typed value, then the slider over
// the common range, or "Custom" with a way back to it.
func (r *Room) rateControl(gtx layout.Context, u *UI, spec rateSpec) layout.Dimensions {
	top, common, custom := sliderRange(spec.value, spec.sliderMin, spec.sliderMax, spec.step, spec.max)
	hasCommon := top >= spec.sliderMin
	disabled := spec.max < spec.min
	if spec.common.Clicked(gtx) && hasCommon {
		spec.set(common, true)
	}
	format := func(v float64) string {
		if spec.whole {
			return itoa(int(math.Round(v)))
		}
		return trimFloat(v)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return heading(gtx, u, spec.icon, spec.caption) }),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					d, typed, ok := spec.box.Layout(gtx, u, format(spec.value), spec.unit, 110, disabled)
					if ok {
						value, valid := parseNumber(typed)
						if spec.whole {
							valid = valid && value == math.Trunc(value)
						} else {
							value = math.Round(value*10) / 10
						}
						if !valid || value < spec.min || value > spec.max {
							spec.box.Invalid = true
						} else if value != spec.value {
							spec.box.Invalid = false
							spec.set(value, true)
						}
					}
					return d
				}))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if custom {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "pencil", 16, u.P.Muted) }),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, u.T("host.video.custom"), textStyle{size: 12, weight: font.SemiBold, color: u.P.Muted})
					}),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !hasCommon {
							return layout.Dimensions{}
						}
						return spec.common.Layout(gtx, u, ButtonStyle{Comic: "hint-quality", Icon: "sliders", Caption: u.T("host.video.returnCommon"), Size: sizeDense,
							Tip: u.T("host.video.returnCommonHint", "value", format(common), "unit", spec.unit)})
					}))
			}
			d, next, moved := spec.slider.Layout(gtx, u, float32(spec.value), float32(spec.sliderMin), float32(top), float32(spec.step))
			if moved && float64(next) != spec.value {
				spec.set(float64(next), false)
			}
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if custom {
				return layout.Dimensions{}
			}
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, format(spec.sliderMin), textStyle{size: 10, mono: true, color: u.P.Muted})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return u.label(gtx, format(top)+"  "+spec.unit, textStyle{size: 10, mono: true, color: u.P.Muted, align: text.End})
				}))
		}))
}

// queueSettings applies slider changes once the drag rests (the App
// re-encodes on every update-share).
func (r *Room) queueSettings(settings shareSettings) {
	r.mu.Lock()
	r.pendingSettings = &settings
	r.mu.Unlock()
}

func (r *Room) flushSettings(gtx layout.Context) {
	if r.panel.fps.float.Dragging() || r.panel.bitrate.float.Dragging() {
		return
	}
	r.mu.Lock()
	pending := r.pendingSettings
	r.pendingSettings = nil
	r.mu.Unlock()
	if pending != nil {
		r.host.SetSettings(*pending)
	}
}

// accessRow is the deck's invite row: copy, rotate, revoke, link, skip
// passwords, open/private.
func (r *Room) accessRow(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	p := &u.P
	link := snap.Invite
	if !r.includeCredential {
		link = snap.Canonical
	}
	if r.copyBtn.Clicked(gtx) && link != "" {
		gtx.Execute(clipboardWrite(link))
		r.copied = gtx.Now
	}
	if r.rotateBtn.Clicked(gtx) {
		r.host.RotateInvite()
	}
	if r.revokeBtn.Clicked(gtx) {
		r.host.RevokeInvite()
	}
	if r.passwordSet.Clicked(gtx) && r.passwordInput.Editor.Text() != "" {
		r.host.SetViewerPassword(r.passwordInput.Editor.Text())
		r.passwordInput.Editor.SetText("")
	}
	if r.passwordRemove.Clicked(gtx) {
		r.host.SetViewerPassword("")
	}
	main := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				caption := u.T("common.copy")
				if !r.copied.IsZero() && gtx.Now.Sub(r.copied) < 2e9 {
					caption = u.T("common.copied")
				}
				return r.copyBtn.Layout(gtx, u, ButtonStyle{Comic: "hint-copy-invite", Icon: "link", Caption: caption, Size: sizeCompact, Disabled: link == ""})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.rotateBtn.Layout(gtx, u, ButtonStyle{Comic: "hint-rotate-invite", Icon: "refresh", Caption: u.T("host.invite.rotateShort"), Size: sizeCompact})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.revokeBtn.Layout(gtx, u, ButtonStyle{Comic: "hint-revoke-invite", Icon: "linkOff", Caption: u.T("host.invite.revokeShort"), Size: sizeCompact, Tone: toneDanger,
					Disabled: snap.Invite == snap.Canonical})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(min(gtx.Constraints.Max.X, int(px(gtx, 302))), int(px(gtx, 33)))
				shape := boxOf(size, px(gtx, 8))
				fillBox(gtx.Ops, shape, p.Wall2)
				o := op.Offset(image.Pt(int(px(gtx, 10)), int(px(gtx, 8)))).Push(gtx.Ops)
				lg := gtx
				lg.Constraints = layout.Constraints{Max: image.Pt(size.X-int(px(gtx, 20)), size.Y)}
				shown := link
				if shown == "" {
					switch {
					case snap.Policy == "private" && snap.PasswordOn:
						shown = u.T("host.invite.emptyPassword")
					case snap.Policy == "private":
						shown = u.T("host.invite.emptyPrivate")
					default:
						shown = u.T("host.invite.emptyOpen")
					}
				}
				if link != "" {
					// host.invite.address: selectable, like the page's read-only field.
					r.inviteSel.MaxLines = 1
					if r.inviteSel.Text() != shown {
						r.inviteSel.SetText(shown)
					}
					ink := op.Record(lg.Ops)
					paint.ColorOp{Color: p.Ink}.Add(lg.Ops)
					inkCall := ink.Stop()
					sel := op.Record(lg.Ops)
					paint.ColorOp{Color: withAlpha(p.Action, .35)}.Add(lg.Ops)
					selCall := sel.Stop()
					r.inviteSel.Layout(lg, u.Shaper, mono, unit.Sp(11), inkCall, selCall)
				} else {
					u.label(lg, shown, textStyle{size: 11, mono: true, color: p.Ink, maxLines: 1})
				}
				o.Pop()
				return layout.Dimensions{Size: size}
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "key", 16, p.Ink) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return switchRow(gtx, u, &r.credential, u.T("host.invite.includeCredential"), &r.includeCredential, false)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				selected := 0
				if snap.Policy == protocol.CodeEntryPrivate {
					selected = 1
				}
				gtx.Constraints.Max.X = int(px(gtx, 230))
				d, chosen := r.policyToggle.LayoutWide(gtx, u, []string{"globe", "lock"},
					[]string{u.T("host.policy.open"), u.T("host.policy.private")}, selected)
				if chosen != selected {
					r.host.SetPolicy([]string{protocol.CodeEntryOpen, protocol.CodeEntryPrivate}[chosen])
					if chosen == 1 && !snap.PasswordOn {
						gtx.Execute(key.FocusCmd{Tag: &r.passwordInput.Editor}) // ready to type a password
					}
				}
				return d
			}))
	}
	return deckRow(gtx, u, false, func(gtx layout.Context) layout.Dimensions {
		if snap.Policy != protocol.CodeEntryPrivate {
			return main(gtx)
		}
		// Private rooms: a room password lets viewers in with the code too.
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(main),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// Invite-only without the credential in the link: nobody could enter.
				if snap.PasswordOn || r.includeCredential || u.Visual() {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "alert", 14, p.Warn) }),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, u.T("host.invite.credentialRequired"), textStyle{size: 11.5, color: p.Ink})
						}))
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						key := "host.password.unset"
						if snap.PasswordOn {
							key = "host.password.set"
						}
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "key", 16, p.Ink) }),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, u.T(key), textStyle{size: 12.5, weight: font.SemiBold, color: p.Ink})
							}))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return r.passwordInput.Layout(gtx, u, "lock", u.T("host.password.placeholder"), !r.showPassword)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if r.passwordEye.Clicked(gtx) {
							r.showPassword = !r.showPassword
						}
						icon, tip, kind := "eye", u.T("host.password.show"), "hint-password-show"
						if r.showPassword {
							icon, tip, kind = "eyeOff", u.T("host.password.hide"), "hint-password-hide"
						}
						return r.passwordEye.Layout(gtx, u, ButtonStyle{Icon: icon, Size: sizeDense, Tone: toneQuiet, Tip: tip, Comic: kind})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						key := "host.password.setAction"
						if snap.PasswordOn {
							key = "host.password.changeAction"
						}
						return r.passwordSet.Layout(gtx, u, ButtonStyle{Comic: "hint-password", Icon: "check", Caption: u.T(key), Size: sizeCompact,
							Disabled: r.passwordInput.Editor.Text() == ""})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !snap.PasswordOn {
							return layout.Dimensions{}
						}
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return r.passwordRemove.Layout(gtx, u, ButtonStyle{Comic: "hint-password-remove", Icon: "x", Caption: u.T("host.password.remove"), Size: sizeCompact, Tone: toneDanger})
						})
					}))
			}))
	})
}

// deckRow is .lr-deck .lr-row: paper card, 1px edge, radius 24, padding 15 18;
// sub rows (details, topology) sit on wall-2 without an edge.
func deckRow(gtx layout.Context, u *UI, sub bool, body layout.Widget) layout.Dimensions {
	p := &u.P
	macro := op.Record(gtx.Ops)
	inner := gtx
	inner.Constraints.Min = image.Point{}
	inner.Constraints.Max.X -= int(px(gtx, 36))
	content := body(inner)
	call := macro.Stop()
	padX, padY := int(px(gtx, 18)), int(px(gtx, 15))
	size := image.Pt(gtx.Constraints.Max.X, content.Size.Y+2*padY)
	shape := boxOf(size, px(gtx, 24))
	if sub {
		fillBox(gtx.Ops, shape, p.Wall2)
	} else {
		fillBox(gtx.Ops, shape, p.Paper)
		strokeBox(gtx.Ops, shape, px(gtx, 1), p.ControlEdge)
	}
	o := op.Offset(image.Pt(padX, padY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	return layout.Dimensions{Size: size}
}

// detailsRows are "Connection details": local capture facts and the viewer table.
func (r *Room) detailsRows(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	p := &u.P
	codec := strings.ToUpper(snap.CodecInUse)
	if codec == "" {
		codec = u.T("stats.unknown")
	}
	audio := u.T("stats.unknown")
	if snap.SourceAudio != nil {
		audio = map[bool]string{true: u.T("stats.audioPresent"), false: u.T("stats.audioAbsent")}[*snap.SourceAudio]
	}
	resolution := u.T("stats.unknown")
	fps := u.T("stats.unknown")
	if (snap.Phase == phaseLive || snap.Phase == phasePaused) && time.Since(snap.Capture.At) < 6*time.Second {
		resolution = fmt.Sprintf("%d×%d", snap.Capture.Width, snap.Capture.Height)
		fps = fmt.Sprintf("%.0f fps · %d kbps", snap.Capture.FPS, int(snap.Capture.Kbps))
	} else if snap.Phase == phaseLive || snap.Phase == phasePaused {
		q := snap.Settings.Quality
		resolution = fmt.Sprintf("≤ %d×%d", q.Resolution.Width, q.Resolution.Height)
		fps = fmt.Sprintf("≤ %d fps", int(q.MaxFramerate))
	}
	stat := func(icon, value, label string) layout.FlexChild {
		return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Start}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, icon, 15, p.Muted) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, value, textStyle{size: 12, mono: true, weight: font.Bold, color: p.Ink, maxLines: 1})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, label, textStyle{size: 10, color: p.Muted, maxLines: 1})
						}))
				}))
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return deckRow(gtx, u, true, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Flex{Alignment: layout.Start}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "display", 15, p.Ink) }),
							layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, "↑ "+u.T("stats.localCapture"), textStyle{size: 11, weight: font.Bold, color: p.Ink})
							}))
					}),
					stat("expand", resolution, u.T("stats.resolution")),
					stat("frames", fps, u.T("stats.fps")),
					stat("puzzle", codec, u.T("stats.codec")),
					stat("speaker", audio, u.T("stats.group.sourceAudio")))
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return r.viewerTable(gtx, u, snap) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return r.voiceDetails(gtx, u, snap) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			for _, viewer := range snap.Viewers {
				if viewer.ID == r.selectedViewer {
					return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return r.viewerCard(gtx, u, snap, viewer)
					})
				}
			}
			return layout.Dimensions{}
		}),
	)
}

func (r *Room) viewerTable(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	p := &u.P
	width := gtx.Constraints.Max.X
	cols := []float32{0.3, 0.12, 0.12, 0.12, 0.12, 0.12, 0.1}
	headers := []string{strings.ToUpper(u.T("common.viewers")), strings.ToUpper(u.T("host.settings.picture")), u.T("stats.resolution"),
		u.T("stats.fps"), u.T("stats.bitrate"), u.T("stats.loss"), u.T("stats.rtt")}
	row := func(gtx layout.Context, cells []layout.Widget) layout.Dimensions {
		inner := width - int(px(gtx, 24))
		height := 0
		x := int(px(gtx, 12))
		macro := op.Record(gtx.Ops)
		for i, cell := range cells {
			w := int(float32(inner) * cols[i])
			o := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
			cg := gtx
			cg.Constraints = layout.Constraints{Max: image.Pt(w, gtx.Constraints.Max.Y)}
			d := cell(cg)
			o.Pop()
			height = max(height, d.Size.Y)
			x += w
		}
		call := macro.Stop()
		call.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(width, height)}
	}
	textCell := func(s string, style textStyle) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions { return u.label(gtx, s, style) }
	}
	macro := op.Record(gtx.Ops)
	content := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var cells []layout.Widget
			for _, h := range headers {
				cells = append(cells, textCell(h, textStyle{size: 9.5, weight: font.Bold, color: p.Muted, maxLines: 1}))
			}
			return row(gtx, cells)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(snap.Viewers) == 0 {
				return layout.Inset{Left: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, u.T("host.viewers.empty"), textStyle{size: 12, color: p.Muted})
				})
			}
			var rows []layout.FlexChild
			for _, viewer := range snap.Viewers {
				stats, ok := snap.Stats[viewer.ID]
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(width, int(px(gtx, 44)))
					click := r.viewerClick(viewer.ID)
					if click.Clicked(gtx) {
						if r.selectedViewer == viewer.ID {
							r.selectedViewer = ""
						} else {
							r.selectedViewer = viewer.ID
						}
					}
					background := withAlpha(p.Wall2, .7)
					switch {
					case r.selectedViewer == viewer.ID:
						background = withAlpha(p.Action, .22)
					case click.Hovered():
						background = p.ControlHover
					}
					fillRect(gtx.Ops, size, background)
					click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						pointerHand(gtx, size)
						return layout.Dimensions{Size: size}
					})
					o := op.Offset(image.Pt(0, int(px(gtx, 8)))).Push(gtx.Ops)
					stateKey, _ := viewerState(viewer, stats, ok)
					status := u.T(stateKey)
					value := func(s string) layout.Widget {
						return textCell(s, textStyle{size: 11.5, mono: true, weight: font.Bold, color: p.Ink, maxLines: 1})
					}
					unknown := u.T("stats.unknown")
					res, fps, kbps, loss, rtt := unknown, unknown, unknown, unknown, unknown
					if ok {
						if stats.Width > 0 {
							res = fmt.Sprintf("%dx%d", stats.Width, stats.Height)
						}
						fps = fmt.Sprintf("%.1f fps", stats.FPS)
						kbps = fmt.Sprintf("%d kbps", int(stats.Kbps))
						loss = fmt.Sprintf("%.1f%%", stats.LossPercent)
						if stats.HasRTT {
							rtt = fmt.Sprintf("%d ms", int(stats.RTTms))
						}
					}
					path := "P2P"

					row(gtx, []layout.Widget{
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return miniPerson(gtx, 22, participantColor(viewer.ID))
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(textCell(viewer.Name, textStyle{size: 11.5, weight: font.Bold, mono: true, color: p.Ink, maxLines: 1})),
										layout.Rigid(textCell(status, textStyle{size: 9.5, color: p.Muted, maxLines: 1})))
								}))
						},
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "branch", 13, p.Live) }),
								layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout),
								layout.Rigid(textCell(path, textStyle{size: 11, mono: true, weight: font.Bold, color: p.Live})))
						},
						value(res), value(fps), value(kbps), value(loss), value(rtt),
					})
					o.Pop()
					return layout.Dimensions{Size: size}
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
	)
	call := macro.Stop()
	size := image.Pt(width, content.Size.Y)
	shape := boxOf(size, px(gtx, 16))
	fillBox(gtx.Ops, shape, p.Wall2)
	stack := clip.UniformRRect(image.Rectangle{Max: size}, int(px(gtx, 16))).Push(gtx.Ops)
	call.Add(gtx.Ops)
	stack.Pop()
	strokeBox(gtx.Ops, shape, px(gtx, 2), p.Ink)
	return layout.Dimensions{Size: size}
}

// miniPerson draws the person glyph (body + eyes) side pixels tall.
func miniPerson(gtx layout.Context, side float32, c color.NRGBA) layout.Dimensions {
	art := personShapes()
	scale := px(gtx, side) / 56
	w := int(48 * scale)
	fillShapes(gtx.Ops, art.body, scale, f32.Point{}, c)
	fillShapes(gtx.Ops, art.eyes, scale, f32.Point{}, personInk)
	return layout.Dimensions{Size: image.Pt(w, int(px(gtx, side)))}
}

// topologyRow is "Connection topology": the host and viewers as a tree, an
// edge per media route (green when the viewer has the picture).
func (r *Room) topologyRow(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	p := &u.P
	if len(snap.Viewers) == 0 {
		return deckRow(gtx, u, true, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, u.T("host.topology.idle"), textStyle{size: 12, color: p.Muted})
		})
	}
	return deckRow(gtx, u, true, func(gtx layout.Context) layout.Dimensions {
		width := gtx.Constraints.Max.X
		// Depth of every viewer from its upstream chain.
		parent := map[string]string{}
		for _, viewer := range snap.Viewers {
			up := viewer.Upstream
			if up == snap.HostID {
				up = ""
			}
			parent[viewer.ID] = up
		}
		depth := map[string]int{}
		var depthOf func(id string, guard int) int
		depthOf = func(id string, guard int) int {
			if d, ok := depth[id]; ok {
				return d
			}
			up := parent[id]
			d := 1
			if up != "" && guard < 8 {
				if _, known := parent[up]; known {
					d = depthOf(up, guard+1) + 1
				}
			}
			depth[id] = d
			return d
		}
		levels := map[int][]roomPeer{}
		maxDepth := 0
		for _, viewer := range snap.Viewers {
			d := depthOf(viewer.ID, 0)
			levels[d] = append(levels[d], viewer)
			maxDepth = max(maxDepth, d)
		}
		// Taller with crowded levels: about 48 px per person in the biggest one.
		crowd := 0
		for _, list := range levels {
			crowd = max(crowd, len(list))
		}
		height := int(px(gtx, max(160, float32(crowd*48+32))))
		columns := maxDepth + 1
		colW := float32(width) / float32(max(columns, 2))
		type node struct {
			x, y float32
		}
		pos := map[string]node{}
		hostNode := node{x: colW / 2, y: float32(height) / 2}
		if columns == 1 {
			hostNode.x = float32(width) / 2
		}
		for d := 1; d <= maxDepth; d++ {
			list := levels[d]
			sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
			for i, viewer := range list {
				pos[viewer.ID] = node{x: colW*float32(d) + colW/2, y: float32(height) * float32(i+1) / float32(len(list)+1)}
			}
		}
		// Edges.
		for _, viewer := range snap.Viewers {
			from := hostNode
			if up := parent[viewer.ID]; up != "" {
				if n, ok := pos[up]; ok {
					from = n
				}
			}
			to := pos[viewer.ID]
			c := p.Live
			if !viewer.MediaReady {
				c = withAlpha(p.Muted, .6)
			}
			var path clip.Path
			path.Begin(gtx.Ops)
			start := f32.Pt(from.x+px(gtx, 22), from.y)
			end := f32.Pt(to.x-px(gtx, 22), to.y)
			path.MoveTo(start)
			mid := f32.Pt((start.X+end.X)/2, start.Y)
			path.QuadTo(mid, end)
			paint.FillShape(gtx.Ops, c, clip.Stroke{Path: path.End(), Width: px(gtx, 2.5)}.Op())
			// Route label on the edge (RouteTree): direct P2P, or waiting.
			label := "P2P"
			if !viewer.MediaReady {
				label = u.T("host.topology.pending")
			}
			lm := op.Record(gtx.Ops)
			ld := u.label(gtx, label, textStyle{size: 9.5, mono: true, weight: font.Bold, color: c, maxLines: 1})
			lc := lm.Stop()
			at := image.Pt(int((start.X+end.X)/2)-ld.Size.X/2, int((start.Y+end.Y)/2)-ld.Size.Y-int(px(gtx, 2)))
			lo := op.Offset(at).Push(gtx.Ops)
			fillBox(gtx.Ops, boxOf(ld.Size.Add(image.Pt(int(px(gtx, 6)), 0)), px(gtx, 4)).offset(-px(gtx, 3), 0), withAlpha(p.Wall2, .9))
			lc.Add(gtx.Ops)
			lo.Pop()
		}
		// Voice routes (DK-36): thin dashed lines under the people, between
		// the voice tree's members; a small microphone marks each one.
		place := func(id string) (node, bool) {
			if id == snap.HostID || id == "" {
				return hostNode, true
			}
			n, ok := pos[id]
			return n, ok
		}
		for _, edge := range snap.Voice.Edges {
			a, okA := place(edge.Parent)
			b, okB := place(edge.Child)
			if !okA || !okB || a == b {
				continue
			}
			c := withAlpha(p.Action, .75)
			if edge.Pending {
				c = withAlpha(p.Muted, .5)
			}
			start, end := f32.Pt(a.x, a.y+px(gtx, 14)), f32.Pt(b.x, b.y+px(gtx, 14))
			length := float32(math.Hypot(float64(end.X-start.X), float64(end.Y-start.Y)))
			dash, gap := px(gtx, 5), px(gtx, 4)
			for at := float32(0); at < length; at += dash + gap {
				t0, t1 := at/length, min(at+dash, length)/length
				var seg clip.Path
				seg.Begin(gtx.Ops)
				seg.MoveTo(f32.Pt(start.X+(end.X-start.X)*t0, start.Y+(end.Y-start.Y)*t0))
				seg.LineTo(f32.Pt(start.X+(end.X-start.X)*t1, start.Y+(end.Y-start.Y)*t1))
				paint.FillShape(gtx.Ops, c, clip.Stroke{Path: seg.End(), Width: px(gtx, 1.5)}.Op())
			}
			mo := op.Offset(image.Pt(int((start.X+end.X)/2-px(gtx, 6)), int((start.Y+end.Y)/2-px(gtx, 6)))).Push(gtx.Ops)
			fillBox(gtx.Ops, boxOf(image.Pt(int(px(gtx, 12)), int(px(gtx, 12))), px(gtx, 6)), p.Wall2)
			glyph(gtx, "microphone", 12, c)
			mo.Pop()
		}
		drawNode := func(n node, name string, c color.NRGBA, host bool) {
			side := px(gtx, 34)
			o := op.Offset(image.Pt(int(n.x-side*48/56/2), int(n.y-side/2-px(gtx, 8)))).Push(gtx.Ops)
			art := personShapes()
			scale := side / 56
			fillShapes(gtx.Ops, art.body, scale, f32.Point{}, c)
			fillShapes(gtx.Ops, art.eyes, scale, f32.Point{}, personInk)
			if host {
				crown := f32.Pt(18*scale, 0)
				fillShapes(gtx.Ops, art.crown, scale, crown, rgb(0xf7d861))
			}
			o.Pop()
			lg := gtx
			lg.Constraints = layout.Constraints{Max: image.Pt(int(colW), int(px(gtx, 20)))}
			macro := op.Record(gtx.Ops)
			d := u.label(lg, name, textStyle{size: 11, weight: font.ExtraBold, color: p.Ink, maxLines: 1})
			call := macro.Stop()
			o = op.Offset(image.Pt(int(n.x)-d.Size.X/2, int(n.y+side/2-px(gtx, 4)))).Push(gtx.Ops)
			call.Add(gtx.Ops)
			o.Pop()
		}
		hostColor := hostColor
		if snap.HostID != "" {
			hostColor = participantColor(snap.HostID)
		}
		drawNode(hostNode, displayName(u, snap.Name), hostColor, true)
		for _, viewer := range snap.Viewers {
			n := pos[viewer.ID]
			drawNode(n, viewer.Name, participantColor(viewer.ID), false)
			// Selecting a viewer here opens their card in Connection details.
			click := r.viewerClick("topology:" + viewer.ID)
			if click.Clicked(gtx) {
				r.detailsOpen, r.selectedViewer = true, viewer.ID
			}
			side := int(px(gtx, 44))
			o := op.Offset(image.Pt(int(n.x)-side/2, int(n.y)-side/2-int(px(gtx, 8)))).Push(gtx.Ops)
			click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				pointerHand(gtx, image.Pt(side, side))
				return layout.Dimensions{Size: image.Pt(side, side)}
			})
			o.Pop()
		}
		return layout.Dimensions{Size: image.Pt(width, height)}
	})
}

// nameEditor is the inline display name editor of the deck.
type nameEditor struct {
	editing bool
	failed  bool // host.nameError
	input   widget.Editor
	save    Button
	cancel  Button
}

func (n *nameEditor) Layout(gtx layout.Context, u *UI, current string, save func(string) error) layout.Dimensions {
	p := &u.P
	n.input.SingleLine, n.input.Submit = true, true
	for {
		event, ok := n.input.Update(gtx)
		if !ok {
			break
		}
		if _, submit := event.(widget.SubmitEvent); submit {
			n.failed = save(n.input.Text()) != nil
			n.editing = n.failed
		}
		if _, edit := event.(widget.ChangeEvent); edit {
			n.failed = false
		}
	}
	if n.save.Clicked(gtx) {
		n.failed = save(n.input.Text()) != nil
		n.editing = n.failed
	}
	if n.cancel.Clicked(gtx) {
		n.editing, n.failed = false, false
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(int(px(gtx, 140)), int(px(gtx, 40)))
			shape := boxOf(size, float32(size.Y)/2)
			fillBox(gtx.Ops, shape, p.Paper)
			strokeBox(gtx.Ops, shape, px(gtx, 2), p.Ink)
			strokeBox(gtx.Ops, shape.inset(px(gtx, -3)), px(gtx, 2), withAlpha(p.Ink, .5))
			o := op.Offset(image.Pt(int(px(gtx, 14)), int(px(gtx, 9)))).Push(gtx.Ops)
			eg := gtx
			eg.Constraints = layout.Exact(image.Pt(size.X-int(px(gtx, 28)), int(px(gtx, 22))))
			material := op.Record(gtx.Ops)
			paint.ColorOp{Color: p.Ink}.Add(gtx.Ops)
			ink := material.Stop()
			material = op.Record(gtx.Ops)
			paint.ColorOp{Color: withAlpha(p.Action, .35)}.Add(gtx.Ops)
			selection := material.Stop()
			n.input.Layout(eg, u.Shaper, sans, unit.Sp(14), ink, selection)
			o.Pop()
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			changed := strings.TrimSpace(n.input.Text()) != current && strings.TrimSpace(n.input.Text()) != ""
			return n.save.Layout(gtx, u, ButtonStyle{Icon: "check", Disabled: !changed})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return n.cancel.Layout(gtx, u, ButtonStyle{Icon: "x"})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !n.failed {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(260))
				return u.label(gtx, u.T("host.nameError"), textStyle{size: 11.5, color: p.DangerText, maxLines: 2})
			})
		}))
}
