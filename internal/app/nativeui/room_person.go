package nativeui

import (
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

// personMenu is the couch's person menu (lr-person-menu): reactions to one
// person, throws included, and a shortcut to their connection details.
type personMenu struct {
	target    string // peerId; "" = closed
	close     Button
	reactions [16]widget.Clickable
	details   Button
	pawns     map[string]*widget.Clickable
	avatars   map[string]*widget.Clickable
	rect      image.Rectangle
}

func (m *personMenu) click(table *map[string]*widget.Clickable, id string) *widget.Clickable {
	if *table == nil {
		*table = map[string]*widget.Clickable{}
	}
	click, ok := (*table)[id]
	if !ok {
		click = &widget.Clickable{}
		(*table)[id] = click
	}
	return click
}

// open toggles the menu for a person.
func (m *personMenu) open(id string) {
	if m.target == id {
		m.target = ""
	} else {
		m.target = id
	}
}

// isThrow reports the reactions that fly to someone (tomato, poop).
func isThrow(reaction string) bool {
	for _, id := range reactionIDs[len(pickerReactions):] {
		if id == reaction {
			return true
		}
	}
	return false
}

func (r *Room) personContent(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	m := &r.person
	p := &u.P
	if m.close.Clicked(gtx) {
		m.target = ""
	}
	self := m.target == snap.HostID
	name := displayName(u, snap.Name)
	viewer := false
	for _, v := range snap.Viewers {
		if v.ID == m.target {
			name, viewer = v.Name, true
		}
	}
	if name == "" {
		name = u.T("interaction.participant")
	}
	choices := reactionIDs
	if self {
		choices = pickerReactions
	}
	ready := snap.InteractionsReady && !snap.InteractionPending
	for i, id := range choices {
		if m.reactions[i].Clicked(gtx) && ready {
			target := m.target
			if self {
				target = ""
			}
			r.host.SendReaction(id, target)
			m.target = ""
		}
	}
	if m.details.Clicked(gtx) {
		r.detailsOpen = true
		if viewer {
			r.selectedViewer = m.target
		}
		m.target = ""
	}
	return floatingCard(gtx, u, gtx.Constraints.Max, "smile", u.T("interaction.express"), &m.close, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return miniPerson(gtx, 34, participantColor(m.target)) }),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						key := "interaction.toPerson"
						if self {
							key = "interaction.express"
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if u.Visual() {
									return layout.Dimensions{}
								}
								return u.label(gtx, u.T(key), textStyle{size: 10.5, color: p.Muted, maxLines: 1})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, name, textStyle{size: 14, weight: font.Bold, color: p.Ink, maxLines: 1})
							}))
					}))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var items []layout.Widget
				for i, id := range choices {
					items = append(items, func(gtx layout.Context) layout.Dimensions {
						return reactionChoice(gtx, u, &m.reactions[i], id, isThrow(id), ready)
					})
				}
				return flowRow(gtx, 6, items)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				status := ""
				switch {
				case !snap.InteractionsReady:
					status = u.T("interaction.reconnecting")
				case snap.InteractionError != "":
					status = u.T(snap.InteractionError)
				}
				if status == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, status, textStyle{size: 11, color: p.Muted, maxLines: 2})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !viewer && !self {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return m.details.Layout(gtx, u, ButtonStyle{Icon: "gauge", Caption: u.T("host.details"), Size: sizeCompact, Tone: toneQuiet})
				})
			}))
	})
}

// reactionChoice is one reaction button; throws are wider with their name.
func reactionChoice(gtx layout.Context, u *UI, click *widget.Clickable, id string, throw, enabled bool) layout.Dimensions {
	p := &u.P
	cell := gtx.Dp(44)
	var caption layout.Dimensions
	var captionCall op.CallOp
	if throw && !u.Visual() {
		macro := op.Record(gtx.Ops)
		caption = u.label(gtx, u.T("interaction.reaction."+id), textStyle{size: 12, weight: font.SemiBold, color: p.Ink, maxLines: 1})
		captionCall = macro.Stop()
	}
	size := image.Pt(cell, cell)
	if throw {
		size.X = cell + caption.Size.X + gtx.Dp(14)
	}
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		shape := boxOf(size, px(gtx, 12))
		switch {
		case throw:
			fillBox(gtx.Ops, shape, p.Wall)
			strokeBox(gtx.Ops, shape, px(gtx, 1.5), p.ControlEdge)
		case click.Hovered() && enabled:
			fillBox(gtx.Ops, shape, p.ControlHover)
		}
		if throw && click.Hovered() && enabled {
			fillBox(gtx.Ops, shape, p.ControlHover)
			strokeBox(gtx.Ops, shape, px(gtx, 1.5), p.Action)
		}
		io := op.Offset(image.Pt((cell-gtx.Dp(28))/2, (cell-gtx.Dp(28))/2)).Push(gtx.Ops)
		reactionIcon(gtx, id, 28)
		io.Pop()
		if throw {
			co := op.Offset(image.Pt(cell, (size.Y-caption.Size.Y)/2)).Push(gtx.Ops)
			captionCall.Add(gtx.Ops)
			co.Pop()
		}
		if !enabled {
			fillBox(gtx.Ops, shape, withAlpha(p.Paper, .5))
		} else {
			pointerHand(gtx, size)
		}
		return layout.Dimensions{Size: size}
	})
}
