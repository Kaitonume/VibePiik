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
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/TNTcraftHIM/Piik/internal/app/buildinfo"
)

// Floating panels of the host room: Interactions (chat and reactions, the
// web's RoomInteractions) and Remote control (HostRemoteControl).

type interactPanel struct {
	open       bool
	tabs       Segmented
	close      Button
	list       widget.List
	input      widget.Editor
	send       Button
	react      Button
	picking    bool
	reactions  [16]widget.Clickable
	lastCount  int
	interact   FlatButton
	panelRect  image.Rectangle
	remoteRect image.Rectangle
	helpRect   image.Rectangle

	voiceTab   bool
	voiceOpen  Button
	voiceMic   Button
	voiceEar   Button
	voiceList  widget.List
	voiceMutes map[string]*widget.Clickable

	voiceSettings Summary
	voiceVolume   Slider
	voiceRates    [3]Chip
	voiceRateBox  NumberBox
	voiceRows     map[string]*widget.Clickable
	voicePerson   map[string]*Slider
	voiceTips     map[string]time.Time // hover start of each member's microphone
	voiceGate     Slider
	voiceOpenRow  string // person whose volume slider is shown
	unseen        int
	latest        Button
	export        Button
	exportText    string
	exportAt      time.Time
	voiceDevice   devicePicker
	voiceMicVol   Slider
	voiceProcess  Switch
}

type remotePanel struct {
	open      bool
	close     Button
	enable    Switch
	together  Switch
	modes     Segmented
	draft     string
	password  Input
	apply     Button
	keyLeft   Button
	keyRight  Button
	decisions map[string]*[2]Button
	toolbar   FlatButton
	seen      map[string]bool
	list      widget.List

	preparing    bool // switched on, waiting for confirmation of the permission
	dirty        bool // draft changed while on
	cancelPrep   Button
	gamepadLink  Button
	enabling     time.Time
	cancelEnable Button
}

// floatingCard draws a floating panel (.lr-anchored-card) with a title bar.
func floatingCard(gtx layout.Context, u *UI, size image.Point, icon, title string, closeBtn *Button, body layout.Widget) layout.Dimensions {
	return floatingCardTag(gtx, u, closeBtn, size, icon, title, closeBtn, body)
}

// floatingCardTag is floatingCard with the tag that owns the card's pointer area.
func floatingCardTag(gtx layout.Context, u *UI, tag event.Tag, size image.Point, icon, title string, closeBtn *Button, body layout.Widget) layout.Dimensions {
	p := &u.P
	shape := boxOf(size, px(gtx, 20))
	panelShadow(gtx.Ops, p, shape, 1)
	fillBox(gtx.Ops, shape, p.Paper)
	glaze(gtx.Ops, p, shape)
	strokeBox(gtx.Ops, shape, px(gtx, 1), p.ControlEdge)
	// Swallow pointer input over the card so the page below does not react.
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	area.Pop()
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Release | pointer.Scroll | pointer.Move,
			ScrollY: pointer.ScrollRange{Min: -1e6, Max: 1e6}}); !ok {
			break
		}
	}
	pad := int(px(gtx, 14))
	headH := int(px(gtx, 50))
	if closeBtn != nil && !drawingDetached {
		movesOf(closeBtn).dragArea(gtx, image.Pt(size.X-gtx.Dp(96), headH))
	}
	o := op.Offset(image.Pt(pad, 0)).Push(gtx.Ops)
	hg := gtx
	hg.Constraints = layout.Constraints{Min: image.Pt(size.X-2*pad, 0), Max: image.Pt(size.X-2*pad, headH)}
	headMacro := op.Record(gtx.Ops)
	headDims := layout.Flex{Alignment: layout.Middle}.Layout(hg,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, icon, 16, p.Muted) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, title, textStyle{size: 13, weight: font.Bold, color: p.Ink, maxLines: 1})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			detach, ok := panelDetachers[closeBtn]
			if !ok || drawingDetached {
				return layout.Dimensions{}
			}
			if detach.button.Clicked(gtx) {
				go detach.open()
			}
			return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return detach.button.Layout(gtx, u, ButtonStyle{Icon: "pip", Size: sizeDense, Tone: toneQuiet,
					Tip: u.T("floating.detach"), Comic: "hint-window-detach"})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return closeBtn.Layout(gtx, u, ButtonStyle{Comic: "hint-close", Icon: "x", Size: sizeDense, Tone: toneQuiet})
		}))
	headCall := headMacro.Stop()
	centre := op.Offset(image.Pt(0, (headH-headDims.Size.Y)/2)).Push(gtx.Ops)
	headCall.Add(gtx.Ops)
	centre.Pop()
	o.Pop()
	line := image.Pt(size.X, int(px(gtx, 1)))
	o = op.Offset(image.Pt(0, headH)).Push(gtx.Ops)
	fillRect(gtx.Ops, line, p.Wall2)
	o.Pop()
	o = op.Offset(image.Pt(pad, headH+pad)).Push(gtx.Ops)
	bg := gtx
	bg.Constraints = layout.Exact(image.Pt(size.X-2*pad, size.Y-headH-2*pad))
	body(bg)
	o.Pop()
	if closeBtn != nil && !drawingDetached {
		movesOf(closeBtn).resizeGrip(gtx, u, size)
	}
	return layout.Dimensions{Size: size}
}

// overlays draws the open floating panels at the window's lower right.
func (r *Room) overlays(gtx layout.Context, u *UI, snap roomSnapshot) {
	window := gtx.Constraints.Max
	margin := gtx.Dp(16)
	right := window.X - margin
	r.interact.panelRect, r.interact.remoteRect, r.interact.helpRect = image.Rectangle{}, image.Rectangle{}, image.Rectangle{}
	r.registerDetachers()
	if r.interact.open && !r.detached.isDetached("interact") {
		size := image.Pt(min(gtx.Dp(420), window.X-2*margin), min(gtx.Dp(560), window.Y-gtx.Dp(90)))
		size = panelSize(size, window, &r.interact.close, gtx.Dp(260), gtx.Dp(240))
		at := placePanel(image.Pt(right-size.X, window.Y-margin-size.Y), size, window, &r.interact.close)
		r.interact.panelRect = image.Rectangle{Min: at, Max: at.Add(size)}
		o := op.Offset(at).Push(gtx.Ops)
		cg := gtx
		cg.Constraints = layout.Exact(size)
		r.interactionsPanel(cg, u, snap)
		o.Pop()
		right = at.X - gtx.Dp(12)
	}
	if r.helpPanel.open {
		size := image.Pt(min(gtx.Dp(400), right-margin), min(gtx.Dp(520), window.Y-gtx.Dp(90)))
		if size.X > gtx.Dp(220) {
			size = panelSize(size, window, &r.helpPanel.close, gtx.Dp(260), gtx.Dp(240))
			at := placePanel(image.Pt(right-size.X, window.Y-margin-size.Y), size, window, &r.helpPanel.close)
			r.interact.helpRect = image.Rectangle{Min: at, Max: at.Add(size)}
			o := op.Offset(at).Push(gtx.Ops)
			cg := gtx
			cg.Constraints = layout.Exact(size)
			r.helpContent(cg, u, snap)
			o.Pop()
			right = at.X - gtx.Dp(12)
		}
	}
	r.person.rect = image.Rectangle{}
	if r.person.target != "" {
		size := image.Pt(min(gtx.Dp(360), right-margin), min(gtx.Dp(360), window.Y-gtx.Dp(90)))
		if size.X > gtx.Dp(220) {
			size = panelSize(size, window, &r.person.close, gtx.Dp(260), gtx.Dp(240))
			at := placePanel(image.Pt(right-size.X, window.Y-margin-size.Y), size, window, &r.person.close)
			r.person.rect = image.Rectangle{Min: at, Max: at.Add(size)}
			o := op.Offset(at).Push(gtx.Ops)
			cg := gtx
			cg.Constraints = layout.Exact(size)
			r.personContent(cg, u, snap)
			o.Pop()
			right = at.X - gtx.Dp(12)
		}
	}
	sharing := snap.Phase == phaseLive || snap.Phase == phasePaused
	if r.remote.open && sharing && snap.Remote.Available && !r.detached.isDetached("remote") {
		size := image.Pt(min(gtx.Dp(340), right-margin), min(gtx.Dp(520), window.Y-gtx.Dp(90)))
		if size.X > gtx.Dp(200) {
			size = panelSize(size, window, &r.remote.close, gtx.Dp(260), gtx.Dp(240))
			at := placePanel(image.Pt(right-size.X, window.Y-margin-size.Y), size, window, &r.remote.close)
			r.interact.remoteRect = image.Rectangle{Min: at, Max: at.Add(size)}
			o := op.Offset(at).Push(gtx.Ops)
			cg := gtx
			cg.Constraints = layout.Exact(size)
			r.remotePanel(cg, u, snap)
			o.Pop()
		}
	}
}

func (r *Room) interactionsPanel(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	s := &r.interact
	p := &u.P
	if s.close.Clicked(gtx) {
		s.open = false
	}
	r.host.MarkChatRead()
	s.list.Axis = layout.Vertical
	// Follow the latest message unless the host scrolled back (interaction.latest).
	s.list.ScrollToEnd = true
	if len(snap.Chat) != s.lastCount {
		if s.list.Position.BeforeEnd && len(snap.Chat) > s.lastCount {
			s.unseen += len(snap.Chat) - s.lastCount
		}
		s.lastCount = len(snap.Chat)
	}
	if !s.list.Position.BeforeEnd {
		s.unseen = 0
	}
	if s.latest.Clicked(gtx) {
		s.list.Position.BeforeEnd, s.unseen = false, 0
	}
	if r.chat.toggle.Clicked(gtx) {
		r.chat.on, r.chat.flights = !r.chat.on, nil
		r.chat.save()
	}
	if r.chat.open.Clicked(gtx) {
		r.chat.settings = !r.chat.settings
	}
	if s.export.Clicked(gtx) && len(snap.Chat) > 0 {
		path, err := saveChat(snap.Chat, displayName(u, snap.Name))
		s.exportText, s.exportAt = u.T("native.chatSaved", "path", path), gtx.Now
		if err != nil {
			s.exportText = u.T("native.chatFailed")
		}
	}
	s.input.SingleLine, s.input.Submit, s.input.MaxLen = true, true, maxChatCodePoints*4
	submit := false
	for {
		event, ok := s.input.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			submit = true
		}
	}
	if s.send.Clicked(gtx) {
		submit = true
	}
	if submit && strings.TrimSpace(s.input.Text()) != "" && r.host.SendChat(s.input.Text()) == nil {
		s.input.SetText("")
		s.list.Position.BeforeEnd, s.unseen = false, 0
	}
	if s.react.Clicked(gtx) {
		s.picking = !s.picking
	}
	for i, id := range pickerReactions {
		if s.reactions[i].Clicked(gtx) {
			r.host.SendReaction(id, "")
			s.picking = false
		}
	}
	return floatingCard(gtx, u, gtx.Constraints.Max, "grip", u.T("interaction.title"), &s.close, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				selected := 0
				if s.voiceTab {
					selected = 1
				}
				d, chosen := s.tabs.LayoutWide(gtx, u, []string{"chat", "microphone"}, []string{u.T("interaction.chat"), u.T("interaction.voice")}, selected)
				s.voiceTab = chosen == 1
				return d
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if s.voiceTab {
					return r.voiceContent(gtx, u, snap)
				}
				if r.chat.settings {
					return r.chatSettings(gtx, u)
				}
				if len(snap.Chat) == 0 {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "chat", 20, p.Muted) }),
							layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, u.T("interaction.empty"), textStyle{size: 13, color: p.Muted})
							}))
					})
				}
				return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Stack{Alignment: layout.S}.Layout(gtx,
						layout.Expanded(func(gtx layout.Context) layout.Dimensions {
							return s.list.Layout(gtx, len(snap.Chat), func(gtx layout.Context, i int) layout.Dimensions {
								continuation := i > 0 && snap.Chat[i-1].PeerID == snap.Chat[i].PeerID
								return r.chatBubble(gtx, u, snap, snap.Chat[i], continuation)
							})
						}),
						layout.Stacked(func(gtx layout.Context) layout.Dimensions {
							if !s.list.Position.BeforeEnd {
								return layout.Dimensions{}
							}
							caption := u.T("interaction.latest")
							if s.unseen > 0 {
								caption = u.T("interaction.unread", "count", itoa(s.unseen))
							}
							return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.latest.Layout(gtx, u, ButtonStyle{Icon: "arrowDown", Caption: caption, Size: sizeDense, Tone: tonePrimary})
							})
						}))
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// RoomReactionActivity: the latest reaction while it plays.
				if s.voiceTab || r.chat.settings || len(snap.Reactions) == 0 {
					return layout.Dimensions{}
				}
				latest := snap.Reactions[len(snap.Reactions)-1]
				if !gtx.Now.Before(latest.Until) {
					return layout.Dimensions{}
				}
				names := map[string]string{snap.HostID: displayName(u, snap.Name)}
				for _, viewer := range snap.Viewers {
					names[viewer.ID] = viewer.Name
				}
				who := names[latest.From]
				if target := names[latest.Target]; target != "" {
					who += " → " + target
				}
				return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return reactionIcon(gtx, latest.Reaction, 20) }),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, who, textStyle{size: 11.5, color: p.Muted, maxLines: 1})
						}))
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !s.picking || s.voiceTab || r.chat.settings {
					return layout.Dimensions{}
				}
				return r.reactionPicker(gtx, u)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.voiceTab {
					return r.voiceControls(gtx, u, snap)
				}
				return r.composer(gtx, u, snap)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.voiceTab {
					text, color := u.T(snap.Voice.Error), p.Danger
					if snap.Voice.Error == "" {
						switch {
						case !snap.Voice.Joined:
							return layout.Dimensions{}
						case snap.Signal == "connecting" || snap.Signal == "reconnecting":
							text, color = u.T("voice.reconnecting"), p.Warn
						case snap.Signal != "connected":
							text, color = u.T("voice.offline"), p.Danger
						case snap.Voice.Unavailable:
							text, color = u.T("voice.unavailable"), p.Muted
						default:
							return layout.Dimensions{}
						}
					}
					return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, text, textStyle{size: 11, color: color, maxLines: 3})
					})
				}
				status := ""
				switch {
				case !snap.InteractionsReady:
					status = u.T("interaction.reconnecting")
				case snap.InteractionError != "":
					status = u.T(snap.InteractionError)
				case s.exportText != "" && gtx.Now.Sub(s.exportAt) < 8*time.Second:
					status = s.exportText
					gtx.Execute(op.InvalidateCmd{At: s.exportAt.Add(8 * time.Second)})
				}
				if status == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, status, textStyle{size: 11, color: p.Muted, maxLines: 2})
				})
			}),
		)
	})
}

func (r *Room) chatBubble(gtx layout.Context, u *UI, snap roomSnapshot, message chatMessage, continuation bool) layout.Dimensions {
	p := &u.P
	width := gtx.Constraints.Max.X
	avatar := gtx.Dp(46)
	maxBubble := width - avatar - gtx.Dp(20)
	top := gtx.Dp(12)
	if continuation {
		top = gtx.Dp(4)
	}
	name := message.Name
	if name == "" {
		name = u.T("interaction.participant")
	}
	c := participantColor(message.PeerID)
	macro := op.Record(gtx.Ops)
	bg := gtx
	bg.Constraints = layout.Constraints{Max: image.Pt(maxBubble-gtx.Dp(26), gtx.Dp(400))}
	textDims := u.label(bg, message.Text, textStyle{size: 14, color: p.Ink, lineH: 1.6})
	textCall := macro.Stop()
	bubble := image.Pt(textDims.Size.X+gtx.Dp(26), textDims.Size.Y+gtx.Dp(18))
	authorH := 0
	var authorCall op.CallOp
	if !continuation {
		macro = op.Record(gtx.Ops)
		d := u.label(bg, name, textStyle{size: 11, color: p.Muted, maxLines: 1})
		authorCall = macro.Stop()
		authorH = d.Size.Y + gtx.Dp(5)
		_ = d
	}
	x := avatar
	if message.Self {
		x = width - avatar - bubble.X
	}
	o := op.Offset(image.Pt(0, top)).Push(gtx.Ops)
	if !continuation {
		ax := gtx.Dp(6)
		if message.Self {
			ax = width - gtx.Dp(34)
		}
		ao := op.Offset(image.Pt(ax, gtx.Dp(2))).Push(gtx.Ops)
		if message.Self {
			miniPerson(gtx, 34, c)
		} else {
			// RoomQuickReactions: the avatar opens the person menu.
			click := r.person.click(&r.person.avatars, message.ID)
			if click.Clicked(gtx) {
				r.person.open(message.PeerID)
			}
			click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				d := miniPerson(gtx, 34, c)
				pointerHand(gtx, d.Size)
				return d
			})
		}
		ao.Pop()
		no := op.Offset(image.Pt(x+gtx.Dp(4), 0)).Push(gtx.Ops)
		authorCall.Add(gtx.Ops)
		no.Pop()
	}
	bo := op.Offset(image.Pt(x, authorH)).Push(gtx.Ops)
	fill := p.Wall
	if message.Self {
		fill = mix(p.Action, p.Paper, .11)
	}
	fillBox(gtx.Ops, boxOf(bubble, px(gtx, 14)), fill)
	to := op.Offset(image.Pt(gtx.Dp(13), gtx.Dp(9))).Push(gtx.Ops)
	textCall.Add(gtx.Ops)
	to.Pop()
	bo.Pop()
	o.Pop()
	return layout.Dimensions{Size: image.Pt(width, top+authorH+bubble.Y)}
}

func (r *Room) composer(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	s := &r.interact
	p := &u.P
	macro := op.Record(gtx.Ops)
	inner := gtx
	inner.Constraints.Min = image.Point{}
	inner.Constraints.Max.X -= gtx.Dp(12)
	content := layout.Flex{Axis: layout.Vertical}.Layout(inner,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(44))
					o := op.Offset(image.Pt(gtx.Dp(10), gtx.Dp(11))).Push(gtx.Ops)
					eg := gtx
					eg.Constraints = layout.Exact(image.Pt(size.X-gtx.Dp(20), gtx.Dp(24)))
					if s.input.Len() == 0 {
						u.label(eg, u.T("interaction.placeholder"), textStyle{size: 15, color: p.Muted, maxLines: 1})
					}
					m := op.Record(gtx.Ops)
					paint.ColorOp{Color: p.Ink}.Add(gtx.Ops)
					ink := m.Stop()
					m = op.Record(gtx.Ops)
					paint.ColorOp{Color: withAlpha(p.Action, .35)}.Add(gtx.Ops)
					sel := m.Stop()
					s.input.Layout(eg, u.Shaper, sans, unit.Sp(15), ink, sel)
					o.Pop()
					return layout.Dimensions{Size: size}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.send.Layout(gtx, u, ButtonStyle{Comic: "hint-chat-send", Icon: "send", Size: sizeCompact, Tone: tonePrimary,
						Disabled: strings.TrimSpace(s.input.Text()) == "" || !snap.InteractionsReady || snap.InteractionPending})
				}))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.react.Layout(gtx, u, ButtonStyle{Comic: "hint-reaction-picker", Icon: "smile", Caption: u.T("interaction.express"), Size: sizeDense,
							Tone: toneQuiet, Pressed: s.picking, Disabled: !snap.InteractionsReady})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						tip := u.T("interaction.overlay.show") + "\n" + u.T("interaction.overlay.showHint")
						if r.chat.on {
							tip = u.T("interaction.overlay.hide") + "\n" + u.T("interaction.overlay.hideHint")
						}
						overlayComic := "hint-chat-overlay-show"
						if r.chat.on {
							overlayComic = "hint-chat-overlay-hide"
						}
						return r.chat.toggle.Layout(gtx, u, ButtonStyle{Comic: overlayComic, Icon: "danmaku", Size: sizeDense, Tone: toneQuiet, Pressed: r.chat.on, Tip: tip})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return r.chat.open.Layout(gtx, u, ButtonStyle{Comic: "hint-chat-settings", Icon: "sliders", Size: sizeDense, Tone: toneQuiet, Pressed: r.chat.settings,
							Tip: u.T("interaction.settings")})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.export.Layout(gtx, u, ButtonStyle{Comic: "hint-chat-export", Icon: "save", Size: sizeDense, Tone: toneQuiet, Disabled: len(snap.Chat) == 0,
							Tip: u.T("interaction.export") + "\n" + u.T("interaction.exportLimit", "count", itoa(chatHistoryLimit))})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						count := len([]rune(s.input.Text()))
						if count == 0 {
							return layout.Dimensions{}
						}
						return u.label(gtx, fmt.Sprintf("%d/%d", count, maxChatCodePoints), textStyle{size: 11, mono: true, color: p.Muted, align: text.End})
					}))
			})
		}))
	call := macro.Stop()
	size := image.Pt(gtx.Constraints.Max.X, content.Size.Y+gtx.Dp(12))
	shape := boxOf(size, px(gtx, 20))
	fillBox(gtx.Ops, shape, p.Wall)
	border := p.ControlEdge
	if gtx.Focused(&s.input) {
		border = p.Action
	}
	strokeBox(gtx.Ops, shape, px(gtx, 1), border)
	o := op.Offset(image.Pt(gtx.Dp(6), gtx.Dp(6))).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	// Clicking anywhere in the composer focuses the input.
	return layout.Dimensions{Size: size}
}

func (r *Room) reactionPicker(gtx layout.Context, u *UI) layout.Dimensions {
	s := &r.interact
	p := &u.P
	cell := gtx.Dp(48)
	columns := max(1, (gtx.Constraints.Max.X-gtx.Dp(16))/cell)
	rows := (len(pickerReactions) + columns - 1) / columns
	size := image.Pt(gtx.Constraints.Max.X, rows*cell+gtx.Dp(16))
	shape := boxOf(size, px(gtx, 18))
	fillBox(gtx.Ops, shape, p.Wall2)
	for i, id := range pickerReactions {
		x := gtx.Dp(8) + (i%columns)*cell
		y := gtx.Dp(8) + (i/columns)*cell
		o := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		cg := gtx
		cg.Constraints = layout.Exact(image.Pt(cell, cell))
		s.reactions[i].Layout(cg, func(gtx layout.Context) layout.Dimensions {
			if s.reactions[i].Hovered() {
				fillBox(gtx.Ops, boxOf(image.Pt(cell, cell), px(gtx, 12)), p.ControlHover)
			}
			io := op.Offset(image.Pt((cell-gtx.Dp(30))/2, (cell-gtx.Dp(30))/2)).Push(gtx.Ops)
			reactionIcon(gtx, id, 30)
			io.Pop()
			pointerHand(gtx, image.Pt(cell, cell))
			return layout.Dimensions{Size: image.Pt(cell, cell)}
		})
		o.Pop()
	}
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: size}
	})
}

// couchFooter is .lr-couch-footer.has-action: viewer count | Interact (+ unread).
func (r *Room) couchFooter(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	p := &u.P
	s := &r.interact
	clicked := s.interact.Clicked(gtx)
	if clicked && r.detached.isDetached("interact") {
		r.detached.raise("interact") // floating.focus
	} else if clicked {
		s.open = !s.open
		if s.open {
			gtx.Execute(key.FocusCmd{Tag: &s.input})
		}
	}
	gtx.Constraints.Min = image.Point{}
	macro := op.Record(gtx.Ops)
	content := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(9), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "users", 14, p.Muted) }),
					layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, itoa(len(snap.Viewers)), textStyle{size: 11, mono: true, weight: font.Bold, color: p.Muted})
					}))
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Dp(1), gtx.Dp(22))
			fillRect(gtx.Ops, size, p.ControlEdge)
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.interact.Layout(gtx, u, "chat", u.T("interaction.open"), p.Ink, s.open)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if snap.Unread == 0 || s.open {
				return layout.Dimensions{}
			}
			macro := op.Record(gtx.Ops)
			d := u.label(gtx, itoa(min(snap.Unread, 99)), textStyle{size: 11, mono: true, weight: font.Bold, color: p.ActionInk})
			call := macro.Stop()
			size := image.Pt(max(d.Size.X+gtx.Dp(12), gtx.Dp(22)), d.Size.Y+gtx.Dp(4))
			o := op.Offset(image.Pt(-gtx.Dp(8), -gtx.Dp(10))).Push(gtx.Ops)
			fillBox(gtx.Ops, boxOf(size, float32(size.Y)/2), p.ActionDeep)
			to := op.Offset(image.Pt((size.X-d.Size.X)/2, gtx.Dp(2))).Push(gtx.Ops)
			call.Add(gtx.Ops)
			to.Pop()
			o.Pop()
			return layout.Dimensions{Size: image.Pt(size.X-gtx.Dp(8), 0)}
		}))
	call := macro.Stop()
	pad := gtx.Dp(3)
	size := image.Pt(content.Size.X+2*pad, content.Size.Y+2*pad)
	shape := boxOf(size, px(gtx, 18))
	fillBox(gtx.Ops, shape, p.Paper)
	strokeBox(gtx.Ops, shape, px(gtx, 1), p.ControlEdge)
	o := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	return layout.Dimensions{Size: size}
}

// reactionFor returns the reaction shown over a pawn (target, else sender)
// and its progress 0..1.
func reactionFor(snap roomSnapshot, peerID string, now time.Time) (string, float32) {
	// The person's own latest expression wins; gifts go beside it (giftsFor).
	for i := len(snap.Reactions) - 1; i >= 0; i-- {
		reaction := snap.Reactions[i]
		if reaction.From == peerID && reaction.Target == "" && reaction.Until.After(now) {
			return reaction.Reaction, min(max(1-float32(reaction.Until.Sub(now))/float32(reactionDuration), 0), 1)
		}
	}
	for i := len(snap.Reactions) - 1; i >= 0; i-- {
		reaction := snap.Reactions[i]
		owner := reaction.Target
		if owner == "" {
			owner = reaction.From
		}
		if owner == peerID && reaction.Until.After(now) {
			progress := 1 - float32(reaction.Until.Sub(now))/float32(reactionDuration)
			if reaction.Target != "" && isThrow(reaction.Reaction) {
				if progress < throwFlightShare {
					continue // still flying (throwsFor)
				}
				progress = (progress - throwFlightShare) / (1 - throwFlightShare)
			}
			return reaction.Reaction, min(max(progress, 0), 1)
		}
	}
	return "", 0
}

// floating is one reaction rising above a pawn.
type floating struct {
	id       string
	progress float32
}

// giftsFor lists the reactions others aimed at peerID (gifts, landed throws)
// besides their own expression: a gift never replaces it (ReactionEffects).
func giftsFor(snap roomSnapshot, peerID string, now time.Time, own string) []floating {
	var gifts []floating
	skipped := own == ""
	for i := len(snap.Reactions) - 1; i >= 0 && len(gifts) < 3; i-- {
		reaction := snap.Reactions[i]
		if reaction.Target != peerID || peerID == "" || !reaction.Until.After(now) {
			continue
		}
		progress := 1 - float32(reaction.Until.Sub(now))/float32(reactionDuration)
		if isThrow(reaction.Reaction) {
			if progress < throwFlightShare {
				continue
			}
			progress = (progress - throwFlightShare) / (1 - throwFlightShare)
		}
		if !skipped && reaction.Reaction == own {
			skipped = true // already shown as the pawn's main reaction
			continue
		}
		gifts = append(gifts, floating{reaction.Reaction, min(max(progress, 0), 1)})
	}
	return gifts
}

// drawFloatingReaction draws a reaction rising above a pawn (x centre, y top).
func drawFloatingReaction(gtx layout.Context, id string, progress float32, x, y int) {
	side := px(gtx, 32)
	rise := progress * px(gtx, 34)
	scale := float32(1)
	if progress < .15 {
		scale = .5 + progress/.15*.5
	}
	alpha := float32(1)
	if progress > .7 {
		alpha = 1 - (progress-.7)/.3
	}
	at := f32.Pt(float32(x)-side*scale/2, float32(y)-side*scale-rise)
	defer op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(scale, scale)).Offset(at)).Push(gtx.Ops).Pop()
	defer paint.PushOpacity(gtx.Ops, alpha).Pop()
	reactionIcon(gtx, id, 32)
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(16 * time.Millisecond)})
}

// mix blends a into b by t (color-mix(in srgb, a t, b)).
func mix(a, b color.NRGBA, t float32) color.NRGBA {
	lerp := func(x, y uint8) uint8 { return uint8(float32(x)*t + float32(y)*(1-t) + .5) }
	return color.NRGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: 255}
}

// remotePanel is HostRemoteControl: the policy and the control requests.
// A remembered "room" or "password" permission is not consent for this
// audience, so switching on asks for confirmation first.
func (r *Room) remotePanel(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	s := &r.remote
	p := &u.P
	remote := snap.Remote
	on := remote.Mode != "off" && remote.Mode != ""
	disabled := snap.Phase == phasePaused || snap.Switching
	if s.close.Clicked(gtx) {
		s.open = false
		s.preparing = false
		s.draft = ""
		s.password.Editor.SetText("")
	}
	if s.draft == "" || (on && !s.dirty) {
		s.draft = remote.Mode
		if !on {
			s.draft = loadPreferences().RemoteMode
		}
		if s.draft == "" || s.draft == "off" {
			s.draft = "ask"
		}
	}
	if s.decisions == nil {
		s.decisions = map[string]*[2]Button{}
	}
	apply := func() {
		password := ""
		if s.draft == "password" {
			password = s.password.Editor.Text()
		}
		r.host.SetRemotePolicy(s.draft, password)
		if !on {
			s.enabling = time.Now() // RC-10: until the App reports the mode
		}
		s.password.Editor.SetText("")
		s.preparing, s.dirty = false, false
	}
	if s.apply.Clicked(gtx) && !disabled && (s.draft != "password" || s.password.Editor.Text() != "") {
		apply()
	}
	if s.cancelPrep.Clicked(gtx) {
		s.preparing, s.dirty, s.draft = false, false, ""
		s.password.Editor.SetText("")
	}
	if s.gamepadLink.Clicked(gtx) && r.onExternal != nil {
		r.onExternal(gamepadGuide(u.textLang()))
	}
	if s.keyLeft.Clicked(gtx) && remote.Shortcut > 1 {
		r.host.SetStopShortcut(remote.Shortcut - 1)
	}
	if s.keyRight.Clicked(gtx) && remote.Shortcut < 11 {
		r.host.SetStopShortcut(max(remote.Shortcut, 0) + 1)
	}
	note := func(key string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.Visual() {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T(key), textStyle{size: 11.5, color: p.Muted, lineH: 1.5})
			})
		})
	}
	return floatingCard(gtx, u, gtx.Constraints.Max, "grip", u.T("remote.title"), &s.close, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				value := on || s.preparing
				d := switchRow(gtx, u, &s.enable, u.T("remote.title"), &value, disabled && !on)
				if value != (on || s.preparing) {
					switch {
					case !value && s.preparing && !on:
						s.preparing, s.dirty = false, false
					case !value:
						r.host.SetRemotePolicy("off", "")
						s.preparing, s.dirty = false, false
					case s.draft == "room" || s.draft == "password":
						s.preparing = true
					default:
						apply()
					}
				}
				return d
			}),
		}
		if on && remote.CoPlay && remote.Simultaneous != nil {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					value := *remote.Simultaneous
					d := switchRow(gtx, u, &s.together, u.T("remote.simultaneous"), &value, disabled)
					if value != *remote.Simultaneous {
						r.host.SetRemoteSimultaneous(value)
					}
					return d
				}),
				note("remote.simultaneousHelp"))
		}
		if on && remote.Gamepad != nil {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "gamepad", 15, p.Muted) }),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if !*remote.Gamepad || u.Visual() {
								return layout.Dimensions{}
							}
							return u.label(gtx, u.T("remote.gamepadHostHelp"), textStyle{size: 11.5, color: p.Muted, maxLines: 2})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.gamepadLink.Layout(gtx, u, ButtonStyle{Comic: "hint-remote-control", Icon: "help", Caption: u.T("remote.gamepadSetup"), Size: sizeDense,
								Tip: u.T("remote.gamepadSetup")})
						}))
				}))
		}
		if len(remote.Requests) > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, u.T("remote.requests"), textStyle{size: 12, weight: font.Bold, color: p.Ink})
				}))
			pending := false
			// Controllers first, pending requests after (like the page).
			ordered := append([]remoteRequest(nil), remote.Requests...)
			sort.SliceStable(ordered, func(i, j int) bool {
				return ordered[i].State != "pending" && ordered[j].State == "pending"
			})
			for _, request := range ordered {
				pending = pending || request.State == "pending"
				children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return r.remoteRequestCard(gtx, u, snap, request) }))
			}
			if pending {
				children = append(children, note("remote.consent"))
			}
		}
		if on || snap.Notice == "remote.decisionFailed" || snap.Notice == "remote.shortcutFailed" {
			s.enabling = time.Time{}
		}
		if s.cancelEnable.Clicked(gtx) {
			s.enabling = time.Time{}
			r.host.SetRemotePolicy("off", "")
		}
		if !s.enabling.IsZero() && time.Since(s.enabling) < 15*time.Second {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return spinner(gtx, u, 18) }),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.cancelEnable.Layout(gtx, u, ButtonStyle{Icon: "x", Caption: u.T("remote.cancel"), Size: sizeCompact, Comic: "hint-remote-stop"})
						}))
				})
			}))
		}
		if on || s.preparing {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, u.T("remote.authorization"), textStyle{size: 12, weight: font.Bold, color: p.Ink})
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					modes := []string{"ask", "room", "password"}
					selected := 0
					for i, mode := range modes {
						if mode == s.draft {
							selected = i
						}
					}
					d, chosen := s.modes.LayoutWide(gtx, u, []string{"check", "users", "key"},
						[]string{u.T("remote.mode.ask"), u.T("remote.mode.room"), u.T("remote.mode.password")}, selected)
					if chosen != selected && !disabled {
						s.draft, s.dirty = modes[chosen], true
						s.password.Editor.SetText("")
					}
					return d
				}),
				note("remote.help."+s.draft))
			if s.draft == "password" {
				children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.password.Layout(gtx, u, "key", u.T("remote.password"), true)
					}))
			}
			if s.draft != remote.Mode || s.draft == "password" {
				action := "remote.apply"
				if !on {
					action = "remote.enable"
					if s.draft == "room" {
						action = "remote.enableRoom"
					}
				}
				children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Spacing: layout.SpaceStart, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !s.preparing {
									return layout.Dimensions{}
								}
								return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.cancelPrep.Layout(gtx, u, ButtonStyle{Icon: "x", Caption: u.T("common.cancel"), Size: sizeCompact})
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.apply.Layout(gtx, u, ButtonStyle{Icon: "check", Caption: u.T(action), Size: sizeCompact, Tone: tonePrimary,
									Tip:      u.T("remote.help." + s.draft),
									Disabled: disabled || (s.draft == "password" && s.password.Editor.Text() == "")})
							}))
					}))
			}
		}
		if snap.Notice == "remote.decisionFailed" || snap.Notice == "remote.shortcutFailed" {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "alert", 14, p.Danger) }),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, u.T(snap.Notice), textStyle{size: 11.5, color: p.DangerText, lineH: 1.5})
					}))
			}))
		}
		children = append(children, note("remote.policyLifetime"))
		if remote.Shortcut > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return heading(gtx, u, "keyboard", u.T("remote.hostStopKey"))
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.keyLeft.Layout(gtx, u, ButtonStyle{Comic: "hint-remote-stop", Icon: "arrowLeft", Size: sizeDense, Disabled: remote.Shortcut <= 1 || disabled})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return valueBoxCentered(gtx, u, fmt.Sprintf("Ctrl + Alt + F%d", remote.Shortcut))
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.keyRight.Layout(gtx, u, ButtonStyle{Comic: "hint-remote-stop", Icon: "arrowRight", Size: sizeDense, Disabled: remote.Shortcut >= 11 || disabled})
						}))
				}),
				note("remote.hostStopKeyHelp"))
		}
		s.list.Axis = layout.Vertical
		return s.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

// gamepadGuide is the remote-control guide's gamepad section, in the UI language.
func gamepadGuide(lang string) string {
	if lang == "zh" {
		return "https://github.com/TNTcraftHIM/Piik/blob/main/docs/guide/remote-control.zh-CN.md#手柄"
	}
	return "https://github.com/TNTcraftHIM/Piik/blob/main/docs/guide/remote-control.md#gamepads"
}

func valueBoxCentered(gtx layout.Context, u *UI, value string) layout.Dimensions {
	p := &u.P
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(38))
	shape := boxOf(size, px(gtx, 12))
	fillBox(gtx.Ops, shape, p.Wall)
	strokeBox(gtx.Ops, shape, px(gtx, 1), p.ControlEdge)
	cg := gtx
	cg.Constraints = layout.Exact(size)
	layout.Center.Layout(cg, func(gtx layout.Context) layout.Dimensions {
		return u.label(gtx, value, textStyle{size: 13, mono: true, weight: font.Bold, color: p.Ink})
	})
	return layout.Dimensions{Size: size}
}

func (r *Room) remoteRequestCard(gtx layout.Context, u *UI, snap roomSnapshot, request remoteRequest) layout.Dimensions {
	s := &r.remote
	p := &u.P
	buttons, ok := s.decisions[request.ID]
	if !ok {
		buttons = &[2]Button{}
		s.decisions[request.ID] = buttons
	}
	if buttons[0].Clicked(gtx) {
		r.host.DecideRemote(request.ID, false)
	}
	if buttons[1].Clicked(gtx) {
		r.host.DecideRemote(request.ID, true)
	}
	name := request.PeerID
	for _, viewer := range snap.Viewers {
		if viewer.ID == request.PeerID {
			name = viewer.Name
		}
	}
	pending := request.State == "pending"
	statusKey, icon := "remote.controlling", "mouse"
	switch request.State {
	case "pending":
		statusKey, icon = "remote.waiting", "clock"
	case "suspended":
		statusKey, icon = "remote.hostFocus", "pause"
	}
	return deckRow(gtx, u, true, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return nameTag(gtx, u, name, participantColor(request.PeerID))
					}),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if pending {
							return layout.Dimensions{}
						}
						return buttons[0].Layout(gtx, u, ButtonStyle{Icon: "x", Size: sizeDense})
					}))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, icon, 14, p.Muted) }),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, u.T(statusKey), textStyle{size: 11.5, color: p.Muted, maxLines: 2})
					}))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !pending {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Spacing: layout.SpaceStart}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return buttons[0].Layout(gtx, u, ButtonStyle{Icon: "x", Caption: u.T("remote.decline"), Size: sizeCompact})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return buttons[1].Layout(gtx, u, ButtonStyle{Icon: "check", Caption: u.T("remote.allow"), Size: sizeCompact, Tone: tonePrimary})
						}))
				})
			}))
	})
}

// voiceContent is the Voice tab: who is in room voice and who is speaking.
func (r *Room) voiceContent(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	s := &r.interact
	p := &u.P
	voice := snap.Voice
	if s.voiceMutes == nil {
		s.voiceMutes = map[string]*widget.Clickable{}
	}
	names := map[string]string{snap.HostID: displayName(u, snap.Name)}
	for _, viewer := range snap.Viewers {
		names[viewer.ID] = viewer.Name
	}
	if !voice.Enabled || !voice.Joined {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(300))
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "headphones", 22, p.Muted) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, u.T("voice.openHint"), textStyle{size: 12.5, color: p.Muted, align: text.Middle, lineH: 1.5})
				}))
		})
	}
	s.voiceList.Axis = layout.Vertical
	if s.voiceRows == nil {
		s.voiceRows, s.voicePerson = map[string]*widget.Clickable{}, map[string]*Slider{}
	}
	return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				// One scrolling list: settings and the participants heading first.
				return s.voiceList.Layout(gtx, len(voice.Members)+1, func(gtx layout.Context, i int) layout.Dimensions {
					if i == 0 {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.voiceSettings.Layout(gtx, u, "sliders", u.T("voice.settings"), itoa(int(voice.Volume*100+0.5))+"%")
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !s.voiceSettings.Open {
									return layout.Dimensions{}
								}
								return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return r.voiceSettings(gtx, u, voice)
								})
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, u.T("voice.participants"), textStyle{size: 12, weight: font.Bold, color: p.Ink})
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout))
					}
					member := voice.Members[i-1]
					self := member.PeerID == snap.HostID
					mute, ok := s.voiceMutes[member.PeerID]
					if !ok {
						mute = &widget.Clickable{}
						s.voiceMutes[member.PeerID] = mute
					}
					if mute.Clicked(gtx) && !self {
						r.host.VoiceMute(member.PeerID)
					}
					rowClick, ok := s.voiceRows[member.PeerID]
					if !ok {
						rowClick = &widget.Clickable{}
						s.voiceRows[member.PeerID] = rowClick
						s.voicePerson[member.PeerID] = &Slider{}
					}
					if rowClick.Clicked(gtx) && !self {
						if s.voiceOpenRow == member.PeerID {
							s.voiceOpenRow = ""
						} else {
							s.voiceOpenRow = member.PeerID
						}
					}
					expanded := s.voiceOpenRow == member.PeerID && !self
					return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(48))
						full := size
						if expanded {
							full.Y += gtx.Dp(40)
						}
						fillBox(gtx.Ops, boxOf(full, px(gtx, 14)), p.Wall)
						if !self {
							rowClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								pointerHand(gtx, size)
								return layout.Dimensions{Size: size}
							})
						}
						if expanded {
							so := op.Offset(image.Pt(gtx.Dp(12), size.Y)).Push(gtx.Ops)
							sg := gtx
							sg.Constraints = layout.Constraints{Max: image.Pt(full.X-gtx.Dp(24), gtx.Dp(34))}
							layout.Flex{Alignment: layout.Middle}.Layout(sg,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.label(gtx, u.T("voice.personVolume"), textStyle{size: 11, color: p.Muted, maxLines: 1})
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									d, value, moved := s.voicePerson[member.PeerID].Layout(gtx, u, float32(member.Volume*100), 0, 200, 5)
									if moved && int(value) != int(member.Volume*100+0.5) {
										r.host.VoicePersonVolume(member.PeerID, float64(value)/100)
									}
									return d
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.label(gtx, itoa(int(member.Volume*100+0.5))+"%", textStyle{size: 11.5, mono: true, weight: font.Bold, color: p.Ink})
								}))
							so.Pop()
						}
						if member.Speaking {
							strokeBox(gtx.Ops, boxOf(size, px(gtx, 14)), px(gtx, 2), p.Live)
						}
						row := gtx
						row.Constraints = layout.Constraints{Min: image.Pt(size.X-gtx.Dp(20), 0), Max: image.Pt(size.X-gtx.Dp(20), size.Y)}
						o := op.Offset(image.Pt(gtx.Dp(10), gtx.Dp(7))).Push(gtx.Ops)
						layout.Flex{Alignment: layout.Middle}.Layout(row,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if self {
									return miniPerson(gtx, 34, participantColor(member.PeerID))
								}
								// The avatar opens the person menu (reactions to them).
								avatar := r.person.click(&r.person.avatars, "voice:"+member.PeerID)
								if avatar.Clicked(gtx) {
									r.person.open(member.PeerID)
								}
								return avatar.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									d := miniPerson(gtx, 34, participantColor(member.PeerID))
									pointerHand(gtx, d.Size)
									return d
								})
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								name := names[member.PeerID]
								if name == "" {
									name = u.T("voice.participant")
								}
								if self {
									name += " (" + u.T("voice.self") + ")"
								}
								if member.PeerID == snap.HostID {
									name = "♛ " + name // the Host (voice.participants marks it)
								}
								return u.label(gtx, name, textStyle{size: 13, weight: font.SemiBold, color: p.Ink, maxLines: 1})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !member.Deafened {
									return layout.Dimensions{}
								}
								return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return glyph(gtx, "headphonesOff", 18, p.Muted)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								icon, c := "microphoneOff", p.Muted
								if member.Publishing {
									icon, c = "microphone", p.Ink
								}
								if member.Speaking {
									c = p.Live
								}
								if member.Muted {
									icon, c = "speakerOff", p.Danger
								}
								// Microphone state, level meter and its hint (voice.microphoneActive / speaking).
								tip := u.T("voice.microphoneInactive")
								if member.Publishing {
									tip = u.T("voice.microphoneActive")
								}
								if member.Speaking {
									tip = u.T("voice.speaking")
								}
								if !self {
									if member.Muted {
										tip += "\n" + u.T("voice.unmuteForMe")
									} else {
										tip += "\n" + u.T("voice.muteForMe")
									}
								}
								return mute.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									d := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if !member.Publishing || member.Muted {
												return layout.Dimensions{}
											}
											return layout.Inset{Right: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return voiceMeter(gtx, member.Level, p.Live, withAlpha(p.Muted, .3))
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, icon, 18, c) }))
									if !self {
										pointerHand(gtx, d.Size)
									}
									if s.voiceTips == nil {
										s.voiceTips = map[string]time.Time{}
									}
									s.voiceTips[member.PeerID] = hoverTooltip(gtx, u, mute.Hovered(), s.voiceTips[member.PeerID], tip, d.Size)
									return d
								})
							}))
						o.Pop()
						return layout.Dimensions{Size: full}
					})
				})
			}))
	})
}

// voiceSettings is the Voice tab's settings: master listening volume and,
// for the host, the room voice bitrate (AudioBitrateControl purpose="voice").
func (r *Room) voiceSettings(gtx layout.Context, u *UI, voice voiceSnapshot) layout.Dimensions {
	s := &r.interact
	p := &u.P
	presets := []struct {
		key  string
		kbps int
	}{{"voice.quality.saver", 32}, {"voice.quality.standard", 64}, {"voice.quality.clear", 96}}
	for i, preset := range presets {
		if s.voiceRates[i].Clicked(gtx) {
			s.voiceRateBox.Invalid = false
			r.host.VoiceBitrate(preset.kbps * 1000)
		}
	}
	percent := int(voice.Volume*100 + 0.5)
	if r.needDevices(&r.voiceDevs) {
		r.loadDevices(&r.voiceDevs, r.host.VoiceDevices)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return heading(gtx, u, "microphone", u.T("host.device.microphone"))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			r.mu.Lock()
			list := r.voiceDevs
			r.mu.Unlock()
			d, changed, chosen, refresh := s.voiceDevice.Layout(gtx, u, list, voice.Device)
			if changed {
				r.host.VoiceDevice(chosen)
			}
			if refresh {
				r.loadDevices(&r.voiceDevs, r.host.VoiceDevices)
			}
			return d
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			level := float32(-1)
			if voice.Microphone {
				level = voice.MicLevel
			}
			return volumeRow(gtx, u, &s.voiceMicVol, "microphone", u.T("host.microphone.volume"), voice.MicGain, level, r.host.VoiceMicGain)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			on := voice.Processing
			d := switchRow(gtx, u, &s.voiceProcess, u.T("host.microphone.voiceProcessing"), &on, false)
			if on != voice.Processing {
				r.host.VoiceProcessing(on)
			}
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.Visual() {
				return layout.Dimensions{}
			}
			return u.label(gtx, u.T("native.voice.processingHint"), textStyle{size: 11, color: p.Muted, lineH: 1.4})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			level := float32(-1)
			if voice.Microphone {
				level = voice.MicLevel
			}
			return gateRow(gtx, u, &s.voiceGate, voice.Gate, voice.GateOpen, level, r.host.VoiceGate)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return heading(gtx, u, "headphones", u.T("voice.listeningVolume"))
				}),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return valueBox(gtx, u, itoa(percent), "%") }))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d, value, moved := s.voiceVolume.Layout(gtx, u, float32(percent), 0, 200, 5)
			if moved && int(value) != percent {
				r.host.VoiceVolume(float64(value) / 100)
			}
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.Visual() {
				return layout.Dimensions{}
			}
			return u.label(gtx, u.T("voice.listeningVolumeHint"), textStyle{size: 11, color: p.Muted, lineH: 1.4})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return heading(gtx, u, "microphone", u.T("voice.bitrate")) }),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					d, typed, ok := s.voiceRateBox.Layout(gtx, u, itoa(voice.Bitrate/1000), "kbps", 100, voice.Bitrate == 0)
					if ok {
						value, valid := parseNumber(typed)
						if !valid || value != math.Trunc(value) || value < minAudioBitrate/1000 || value > maxAudioSend/1000 {
							s.voiceRateBox.Invalid = true
						} else if int(value)*1000 != voice.Bitrate {
							s.voiceRateBox.Invalid = false
							r.host.VoiceBitrate(int(value) * 1000)
						}
					}
					return d
				}))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var items []layout.Widget
			for i, preset := range presets {
				items = append(items, func(gtx layout.Context) layout.Dimensions {
					return s.voiceRates[i].Layout(gtx, u, "", u.T(preset.key), itoa(preset.kbps), voice.Bitrate == preset.kbps*1000, voice.Bitrate == 0)
				})
			}
			return flowRow(gtx, 8, items)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.Visual() {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T("voice.bitrateHint", "min", itoa(minAudioBitrate/1000), "max", itoa(maxAudioSend/1000)),
					textStyle{size: 11, color: p.Muted, lineH: 1.4})
			})
		}))
}

// voiceControls is the Voice tab's bottom row: microphone, listening and
// open/close room voice.
func (r *Room) voiceControls(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	s := &r.interact
	voice := snap.Voice
	joined := voice.Enabled && voice.Joined
	if s.voiceOpen.Clicked(gtx) {
		if joined {
			r.host.VoiceClose()
		} else {
			r.host.VoiceOpen()
		}
	}
	if s.voiceMic.Clicked(gtx) {
		r.host.VoiceMicrophone(!(voice.Microphone || voice.Pending))
	}
	if s.voiceEar.Clicked(gtx) {
		r.host.VoiceDeafen(!voice.Deafened)
	}
	return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !joined {
					return layout.Dimensions{}
				}
				icon, title := "microphoneOff", "voice.microphoneOn"
				if voice.Microphone || voice.Pending {
					icon, title = "microphone", "voice.microphoneOff"
				}
				_ = title
				return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.voiceMic.Layout(gtx, u, ButtonStyle{Icon: icon, Size: sizeCompact, Pressed: voice.Microphone, Busy: voice.Pending})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !joined {
					return layout.Dimensions{}
				}
				icon := "headphones"
				if voice.Deafened {
					icon = "headphonesOff"
				}
				return s.voiceEar.Layout(gtx, u, ButtonStyle{Icon: icon, Size: sizeCompact, Pressed: !voice.Deafened})
			}),
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				key, tone := "voice.open", tonePrimary
				if joined {
					key, tone = "voice.close", toneDanger
				}
				return s.voiceOpen.Layout(gtx, u, ButtonStyle{Comic: map[bool]string{true: "hint-voice-close", false: "hint-voice-open"}[joined], Icon: "headphones", Caption: u.T(key), Size: sizeCompact, Tone: tone})
			}))
	})
}

// pickerReactions are the untargeted reactions: throws need a target person.
var pickerReactions = reactionIDs[:14]

// helpPanel is the header Help card (version, troubleshooting) and, after a
// failure, ShareFailureHelp's checks.
type helpPanel struct {
	open   bool
	close  Button
	guide  Button
	github Button
	copy   Button
	copied time.Time
	list   widget.List

	report     Button
	show       Button
	reportPath string
	reportErr  bool
}

// failureChecks is hostFailureChecks: what to try for a failure code.
func failureChecks(code string) []string {
	parts := strings.SplitN(code, "/", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	scope, operation, reason := parts[0], parts[1], parts[2]
	switch scope {
	case "site", "room":
		return []string{"shareHelp.site"}
	case "app":
		switch {
		case reason == "unavailable" || reason == "timeout":
			return []string{"shareHelp.appConnection"}
		case operation == "prepare-edge":
			return []string{"shareHelp.mediaConnection"}
		case operation == "start-share" || operation == "capture" || operation == "replace-share-source":
			return []string{"shareHelp.source", "shareHelp.compare"}
		}
	}
	return nil
}

func (r *Room) helpContent(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	s := &r.helpPanel
	p := &u.P
	if s.close.Clicked(gtx) {
		s.open = false
	}
	if s.guide.Clicked(gtx) && r.onHelp != nil {
		r.onHelp()
	}
	if s.github.Clicked(gtx) && r.onExternal != nil {
		r.onExternal(projectPage())
	}
	version := buildinfo.Version()
	if s.report.Clicked(gtx) {
		path, err := saveReport(r.host.Diagnostics(), u.Lang)
		s.reportPath, s.reportErr = path, err != nil
	}
	if s.show.Clicked(gtx) && s.reportPath != "" {
		showInFolder(s.reportPath)
	}
	if s.copy.Clicked(gtx) {
		details := fmt.Sprintf("%s\n%s\nVibePiik %s (Piik App %s, native UI)", noticeText(u, snap.Notice), snap.FailureCode, vibeVersion, version)
		gtx.Execute(clipboardWrite(details))
		s.copied = gtx.Now
	}
	s.list.Axis = layout.Vertical
	return floatingCard(gtx, u, gtx.Constraints.Max, "help", u.T("help.title"), &s.close, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return brandMark(gtx, 28, p.Ink) }),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, "VibePiik", textStyle{size: 14, weight: font.Bold, color: p.Ink})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, vibeVersion+" · Piik "+version, textStyle{size: 11, mono: true, color: p.Muted})
					}))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
		}
		if notice := appRelease("", "", r.invalidate); notice != nil {
			// AppReleaseNotice inside the App card.
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.release.Layout(gtx, u, notice, r.onExternal)
				})
			}))
		}
		if snap.FailureCode != "" {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, u.T("shareHelp.title"), textStyle{size: 13, weight: font.Bold, color: p.Ink})
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, noticeText(u, snap.Notice), textStyle{size: 13, weight: font.SemiBold, color: p.Danger, lineH: 1.5})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, snap.FailureCode, textStyle{size: 10.5, mono: true, color: p.Muted})
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout))
			checks := append(failureChecks(snap.FailureCode), "shareHelp.report")
			for i, key := range checks {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, fmt.Sprintf("%d. %s", i+1, u.T(key)), textStyle{size: 12.5, color: p.Ink, lineH: 1.5})
					})
				}))
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				caption := u.T("shareHelp.copy")
				if !s.copied.IsZero() && gtx.Now.Sub(s.copied) < 2*time.Second {
					caption = u.T("shareHelp.copied")
				}
				gtx.Constraints.Min.X = 0
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.copy.Layout(gtx, u, ButtonStyle{Comic: "hint-copy-code", Icon: "copy", Caption: caption, Size: sizeCompact})
				})
			}))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.report.Layout(gtx, u, ButtonStyle{Comic: "hint-debug-export", Icon: "save", Caption: u.T("native.report"), Size: sizeCompact,
									Tip: u.T("native.reportHint")})
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if s.reportPath == "" {
									return layout.Dimensions{}
								}
								return s.show.Layout(gtx, u, ButtonStyle{Icon: "arrowRight", Caption: u.T("native.reportShow"), Size: sizeCompact, Tone: toneQuiet})
							}))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						switch {
						case s.reportErr:
							return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, u.T("native.reportFailed"), textStyle{size: 11.5, color: p.DangerText})
							})
						case s.reportPath != "":
							return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, u.T("native.reportSaved", "path", s.reportPath), textStyle{size: 11, color: p.Muted, lineH: 1.4})
							})
						case !u.Visual():
							return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, u.T("native.reportHint"), textStyle{size: 11, color: p.Muted, lineH: 1.4})
							})
						}
						return layout.Dimensions{}
					}))
			})
		}))
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.guide.Layout(gtx, u, ButtonStyle{Icon: "book", Caption: u.T("shareHelp.guide"), Size: sizeCompact, Trailing: "arrowRight"})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.github.Layout(gtx, u, ButtonStyle{Caption: "GitHub ↗", Size: sizeCompact, Tone: toneQuiet})
				}))
		}))
		return s.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

// throwFlightShare is the part of a throw's life spent flying to the target.
const throwFlightShare = 0.35

// throwFlight is a tomato or poop on its way from one seat to another.
type throwFlight struct {
	from, to int // pawn indexes
	reaction string
	t        float32 // 0..1 of the flight
}

// throwsFor lists the throws in flight between the pawns (by peer id).
func throwsFor(snap roomSnapshot, ids []string, now time.Time) []throwFlight {
	index := map[string]int{}
	for i, id := range ids {
		index[id] = i
	}
	var flights []throwFlight
	for _, reaction := range snap.Reactions {
		if reaction.Target == "" || !isThrow(reaction.Reaction) || !reaction.Until.After(now) {
			continue
		}
		from, okFrom := index[reaction.From]
		to, okTo := index[reaction.Target]
		progress := 1 - float32(reaction.Until.Sub(now))/float32(reactionDuration)
		if okFrom && okTo && progress < throwFlightShare {
			flights = append(flights, throwFlight{from: from, to: to, reaction: reaction.Reaction, t: max(progress, 0) / throwFlightShare})
		}
	}
	return flights
}
