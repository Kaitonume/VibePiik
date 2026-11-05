package nativeui

import (
	"image"
	"math"
	"os"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
)

// chatOverlayState is the Room's side of the on-screen chat and the chat
// over other apps: settings, what was already seen, what is on screen.
type chatOverlayState struct {
	window   chatOverlay
	on       bool // on-screen chat over the TV picture (overlayEnabled)
	look     overlayLook
	desk     desktopOverlay
	flights  []overlayFlight
	lines    []overlayLine
	seen     map[string]bool
	started  bool
	settings bool // Interactions shows the chat settings page

	toggle, open, test              Button
	scale, opacity, speed, duration Slider
	colors, deskOn, showSw          Switch
	list                            widget.List
	screens                         []Chip
	corners                         [4]Chip
	screenList                      []screenChoice
	screensAt                       time.Time
	testAt                          time.Time
}

// testChatEvery (VIBEPIIK_TEST_CHAT=1, automated tests only) injects a sample
// on-screen message every few seconds.
var testChatEvery = func() time.Duration {
	if os.Getenv("VIBEPIIK_TEST_CHAT") == "1" {
		return 3 * time.Second
	}
	return 0
}()

// invalidateAt asks for a frame at t.
func (r *Room) invalidateAt(t time.Time) {
	time.AfterFunc(time.Until(t)+10*time.Millisecond, r.invalidate)
}

func loadOverlaySettings() chatOverlayState {
	prefs := loadPreferences()
	look := overlayLook{Scale: prefs.ChatScale, Opacity: prefs.ChatOpacity, Speed: prefs.ChatSpeed, ColorBySender: prefs.ChatColors}.clamped()
	desk := desktopOverlay{On: prefs.DeskOverlay, Monitor: prefs.DeskMonitor, Corner: prefs.DeskCorner, Seconds: prefs.DeskSeconds}
	if desk.Corner == "" {
		desk.Corner = "top-right"
	}
	if desk.Seconds == 0 {
		desk.Seconds = 15
	}
	return chatOverlayState{on: prefs.ChatOverlay, look: look, desk: desk}
}

func (c *chatOverlayState) save() {
	look, desk, on := c.look, c.desk, c.on
	go updatePreferences(func(p *preferences) {
		p.ChatOverlay, p.ChatScale, p.ChatOpacity, p.ChatSpeed, p.ChatColors = on, look.Scale, look.Opacity, look.Speed, look.ColorBySender
		p.DeskOverlay, p.DeskMonitor, p.DeskCorner, p.DeskSeconds = desk.On, desk.Monitor, desk.Corner, desk.Seconds
	})
}

// feedOverlay turns new chat messages and reactions into flights over the
// picture and lines over other apps, then hands the overlay its state.
func (r *Room) feedOverlay(gtx layout.Context, u *UI, snap roomSnapshot, preview, clip image.Rectangle) {
	c := &r.chat
	now := gtx.Now
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	names := map[string]string{snap.HostID: displayName(u, snap.Name)}
	for _, viewer := range snap.Viewers {
		names[viewer.ID] = viewer.Name
	}
	for _, message := range snap.Chat {
		if c.seen[message.ID] {
			continue
		}
		c.seen[message.ID] = true
		if !c.started {
			continue // backlog when the room opened: not "new"
		}
		if c.on {
			c.flights = addFlight(c.flights, message, c.look, now)
		}
		if c.desk.On {
			name := message.Name
			if name == "" {
				name = names[message.PeerID]
			}
			c.lines = append(c.lines, overlayLine{ID: message.ID, PeerID: message.PeerID, Name: name, Text: message.Text,
				Until: now.Add(time.Duration(c.desk.Seconds) * time.Second)})
		}
	}
	for _, reaction := range snap.Reactions {
		key := "reaction:" + reaction.ID
		if c.seen[key] {
			continue
		}
		c.seen[key] = true
		if c.started && c.desk.On {
			c.lines = append(c.lines, overlayLine{ID: key, PeerID: reaction.From, Name: names[reaction.From],
				Reaction: reaction.Reaction, TargetName: names[reaction.Target],
				Until: now.Add(time.Duration(c.desk.Seconds) * time.Second)})
		}
	}
	c.started = true
	if testChatEvery > 0 && now.Sub(c.testAt) > testChatEvery && (snap.Phase == phaseLive || snap.Phase == phasePaused) {
		// Automated tests only: a sample message instead of a real viewer.
		c.testAt, c.on = now, true
		id := "test-" + itoa(int(now.UnixMilli()%1e9))
		c.flights = addFlight(c.flights, chatMessage{ID: id, PeerID: id, Text: "Test overlay message"}, c.look, now)
		r.invalidateAt(now.Add(testChatEvery))
	}
	c.flights = activeFlights(c.flights, now)
	c.lines = activeLines(c.lines, now)
	if len(c.lines) > 20 {
		c.lines = c.lines[len(c.lines)-20:]
	}
	if c.window == nil {
		return
	}
	state := overlayState{Lang: u.Lang, Spoken: u.textLang(), Look: c.look, Flights: c.flights, Desktop: c.desk, Lines: c.lines}
	if c.on && !clip.Empty() {
		state.Preview, state.Clip = preview, clip
	}
	c.window.Update(state)
}

// tvPicture is the TV screen in window pixels and its visible part (under the
// header, left of floating panels); ok is false when it is not on screen.
func (r *Room) tvPicture(gtx layout.Context, snap roomSnapshot, headerH int) (dest, clip image.Rectangle, ok bool) {
	r.mu.Lock()
	picking := r.picking
	r.mu.Unlock()
	if picking || r.list.Position.First != 0 || (snap.Phase != phaseLive && snap.Phase != phasePaused) {
		return image.Rectangle{}, image.Rectangle{}, false
	}
	window := gtx.Constraints.Max
	area := window.X - gtx.Dp(80)
	content := min(area, r.contentCap(gtx))
	x0 := gtx.Dp(40) + (area-content)/2
	tvW := min(content, r.tvCap(gtx))
	tvX := x0 + (content-tvW)/2
	tvY := headerH + gtx.Dp(8) - r.list.Position.Offset
	pad := int(px(gtx, 15))
	screenW := tvW - 2*pad
	screenH := screenW * 9 / 16
	dest = image.Rect(tvX+pad, tvY+pad, tvX+pad+screenW, tvY+pad+screenH)
	clip = image.Rect(0, headerH, window.X, window.Y)
	for _, panel := range []image.Rectangle{r.interact.panelRect, r.interact.remoteRect, r.interact.helpRect, r.person.rect} {
		if !panel.Empty() && panel.Min.Y < dest.Max.Y && panel.Max.Y > dest.Min.Y && panel.Min.X < clip.Max.X {
			clip.Max.X = max(clip.Min.X, panel.Min.X)
		}
	}
	return dest, clip, true
}

// chatSettings is the Interactions panel's settings page
// (RoomChatOverlaySettings) plus the chat over other apps.
func (r *Room) chatSettings(gtx layout.Context, u *UI) layout.Dimensions {
	c := &r.chat
	p := &u.P
	changed := false
	if c.test.Clicked(gtx) {
		c.lines = append(c.lines, overlayLine{ID: "test-" + itoa(int(gtx.Now.UnixMilli()%1e9)), PeerID: "vibepiik-test",
			Name: "VibePiik", Text: u.T("native.overlay.testMessage"), Until: gtx.Now.Add(time.Duration(c.desk.Seconds) * time.Second)})
		if c.on {
			c.flights = addFlight(c.flights, chatMessage{ID: "test-" + itoa(int(gtx.Now.UnixMilli()%1e9)), PeerID: "vibepiik-test",
				Text: u.T("native.overlay.testMessage")}, c.look, gtx.Now)
		}
	}
	if gtx.Now.Sub(c.screensAt) > 3*time.Second {
		c.screenList, c.screensAt = screenChoices(), gtx.Now
	}
	percentRow := func(slider *Slider, icon, key string, value *float64, lo, hi float64) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			percent := int(math.Round(*value * 100))
			return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return heading(gtx, u, icon, u.T(key)) }),
							layout.Flexed(1, layout.Spacer{}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, itoa(percent)+"%", textStyle{size: 12, mono: true, color: p.Muted})
							}))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						d, next, moved := slider.Layout(gtx, u, float32(percent), float32(lo*100), float32(hi*100), 10)
						if moved && int(next) != percent {
							*value = float64(next) / 100
							changed = true
						}
						return d
					}))
			})
		})
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			on := c.on
			d := switchRow(gtx, u, &c.showSw, u.T("interaction.overlay.show"), &on, false)
			if on != c.on {
				c.on, changed = on, true
				c.flights = nil
			}
			return d
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		percentRow(&c.scale, "textSize", "interaction.overlay.scale", &c.look.Scale, .8, 1.3),
		percentRow(&c.opacity, "opacity", "interaction.overlay.opacity", &c.look.Opacity, .4, 1),
		percentRow(&c.speed, "gauge", "interaction.overlay.speed", &c.look.Speed, .5, 1.5),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			colors := c.look.ColorBySender
			d := switchRow(gtx, u, &c.colors, u.T("interaction.overlay.colorBySender"), &colors, false)
			if colors != c.look.ColorBySender {
				c.look.ColorBySender, changed = colors, true
			}
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.Visual() {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T("interaction.settingsHint")+" "+u.T("interaction.overlay.speedHint"),
					textStyle{size: 11, color: p.Muted, lineH: 1.4})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))
			fillRect(gtx.Ops, size, p.Wall2)
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			on := c.desk.On
			d := switchRow(gtx, u, &c.deskOn, u.T("native.overlay.desktop"), &on, false)
			if on != c.desk.On {
				c.desk.On, changed = on, true
				if !on {
					c.lines = nil
				}
			}
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.Visual() {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T("native.overlay.desktopHint"), textStyle{size: 11, color: p.Muted, lineH: 1.4})
			})
		}),
	}
	if c.desk.On {
		for len(c.screens) < len(c.screenList) {
			c.screens = append(c.screens, Chip{})
		}
		for i := range c.screenList {
			if c.screens[i].Clicked(gtx) && c.desk.Monitor != i {
				c.desk.Monitor, changed = i, true
			}
		}
		for i, corner := range desktopCorners {
			if c.corners[i].Clicked(gtx) && c.desk.Corner != corner {
				c.desk.Corner, changed = corner, true
			}
		}
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return heading(gtx, u, "display", u.T("native.overlay.screen"))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var items []layout.Widget
				for i, screen := range c.screenList {
					label := u.T("native.overlay.screenN", "n", itoa(i+1))
					small := itoa(screen.Width) + "×" + itoa(screen.Height)
					if screen.Primary {
						small += " · " + u.T("native.overlay.primary")
					}
					items = append(items, func(gtx layout.Context) layout.Dimensions {
						return c.screens[i].Layout(gtx, u, "display", label, small, c.desk.Monitor == i, false)
					})
				}
				return flowRow(gtx, 8, items)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return heading(gtx, u, "grip", u.T("native.overlay.corner"))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var items []layout.Widget
				for i, corner := range desktopCorners {
					items = append(items, func(gtx layout.Context) layout.Dimensions {
						return c.corners[i].Layout(gtx, u, "", u.T("native.overlay."+corner), "", c.desk.Corner == corner, false)
					})
				}
				return flowRow(gtx, 8, items)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return heading(gtx, u, "clock", u.T("native.overlay.seconds"))
							}),
							layout.Flexed(1, layout.Spacer{}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, itoa(c.desk.Seconds)+" s", textStyle{size: 12, mono: true, color: p.Muted})
							}))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						d, next, moved := c.duration.Layout(gtx, u, float32(c.desk.Seconds), 5, 60, 5)
						if moved && int(next) != c.desk.Seconds {
							c.desk.Seconds, changed = int(next), true
						}
						return d
					}))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// Room between the test button and the chat box under the settings.
				return layout.Inset{Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return c.test.Layout(gtx, u, ButtonStyle{Icon: "chat", Caption: u.T("native.overlay.test"), Size: sizeCompact})
				})
			}))
	} else {
		children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !c.on {
					return layout.Dimensions{}
				}
				// Room between the test button and the chat box under the settings.
				return layout.Inset{Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return c.test.Layout(gtx, u, ButtonStyle{Icon: "chat", Caption: u.T("native.overlay.test"), Size: sizeCompact})
				})
			}))
	}
	c.list.Axis = layout.Vertical
	dims := c.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Top: unit.Dp(12), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
	if changed {
		c.look = c.look.clamped()
		c.save()
	}
	return dims
}

var _ = font.Bold
