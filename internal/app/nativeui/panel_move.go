package nativeui

import (
	"image"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
)

// Floating panels move by dragging their title bar (floating.move).

type panelMove struct {
	drag   gesture.Drag
	start  f32.Point
	offset image.Point

	resize      gesture.Drag
	resizeStart f32.Point
	grow        image.Point // size change from the default (floating.resize)

	// The drag follows the mouse on the screen: positions inside the moving
	// panel jump as it moves, which made the drag jerky.
	screenStart image.Point
	offsetStart image.Point
	growStart   image.Point
	screenOK    bool
}

// screenDrag is off in tests, where the pointer is simulated.
var screenDrag = true

// pressed remembers where a drag started on the screen.
func (m *panelMove) pressed() {
	m.screenStart, m.screenOK = cursorOnScreen()
	m.screenOK = m.screenOK && screenDrag
	m.offsetStart, m.growStart = m.offset, m.grow
}

// moved is how far the mouse went on the screen since the press.
func (m *panelMove) moved() (image.Point, bool) {
	if !m.screenOK {
		return image.Point{}, false
	}
	now, ok := cursorOnScreen()
	if !ok {
		return image.Point{}, false
	}
	return now.Sub(m.screenStart), true
}

// panelMoves holds each panel's drag state, by its close button (UI goroutine only).
var panelMoves = map[*Button]*panelMove{}

func movesOf(close *Button) *panelMove {
	m, ok := panelMoves[close]
	if !ok {
		m = &panelMove{}
		panelMoves[close] = m
	}
	return m
}

// arrowStep reads the arrow keys sent to tag (focused handle) as a 24 px step
// (FloatingPanel onKeyDown).
func arrowStep(gtx layout.Context, tag event.Tag) image.Point {
	var step image.Point
	for {
		ev, ok := gtx.Event(key.FocusFilter{Target: tag},
			key.Filter{Focus: tag, Name: key.NameLeftArrow}, key.Filter{Focus: tag, Name: key.NameRightArrow},
			key.Filter{Focus: tag, Name: key.NameUpArrow}, key.Filter{Focus: tag, Name: key.NameDownArrow})
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		d := gtx.Dp(24)
		switch e.Name {
		case key.NameLeftArrow:
			step.X -= d
		case key.NameRightArrow:
			step.X += d
		case key.NameUpArrow:
			step.Y -= d
		case key.NameDownArrow:
			step.Y += d
		}
	}
	return step
}

// focusRing marks a focused handle.
func focusRing(gtx layout.Context, u *UI, tag event.Tag, size image.Point) {
	if gtx.Focused(tag) {
		strokeBox(gtx.Ops, boxOf(size, px(gtx, 8)), px(gtx, 2), withAlpha(u.P.Action, .85))
	}
}

// dragArea makes the title bar (size) a drag handle and applies the movement;
// focused, the arrow keys move the panel (floating.move).
func (m *panelMove) dragArea(gtx layout.Context, size image.Point) {
	m.offset = m.offset.Add(arrowStep(gtx, m))
	for {
		e, ok := m.drag.Update(gtx.Metric, gtx.Source, gesture.Both)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			m.start = e.Position
			m.pressed()
			gtx.Execute(key.FocusCmd{Tag: m})
		case pointer.Drag:
			if d, ok := m.moved(); ok {
				m.offset = m.offsetStart.Add(d)
				continue
			}
			delta := e.Position.Sub(m.start)
			m.offset = m.offset.Add(image.Pt(int(delta.X), int(delta.Y)))
		}
	}
	if gtx.Focused(m) {
		fillBox(gtx.Ops, boxOf(image.Pt(size.X, gtx.Dp(2)), 0).offset(0, float32(size.Y-gtx.Dp(2))), withAlpha(currentAction, .85))
	}
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	m.drag.Add(gtx.Ops)
	event.Op(gtx.Ops, m)
	pointer.CursorGrab.Add(gtx.Ops)
}

// placePanel moves a panel's default place by its drag offset, kept inside window.
func placePanel(at, size, window image.Point, close *Button) image.Point {
	m := movesOf(close)
	moved := at.Add(m.offset)
	moved.X = min(max(moved.X, 0), window.X-size.X)
	moved.Y = min(max(moved.Y, 0), window.Y-size.Y)
	m.offset = moved.Sub(at)
	return moved
}

// resizeGrip is the lower-right corner handle (side px square at the card's corner).
func (m *panelMove) resizeGrip(gtx layout.Context, u *UI, size image.Point) {
	side := gtx.Dp(18)
	// Focused, the arrow keys resize (floating.resize).
	if step := arrowStep(gtx, &m.resizeStart); step != (image.Point{}) {
		m.grow = m.grow.Add(step)
		m.offset = m.offset.Add(step)
	}
	currentAction = u.P.Action
	for {
		e, ok := m.resize.Update(gtx.Metric, gtx.Source, gesture.Both)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			m.resizeStart = e.Position
			m.pressed()
		case pointer.Drag:
			if d, ok := m.moved(); ok {
				// The default place is anchored at the lower right: keep the top-left still.
				m.grow, m.offset = m.growStart.Add(d), m.offsetStart.Add(d)
				continue
			}
			delta := e.Position.Sub(m.resizeStart)
			d := image.Pt(int(delta.X), int(delta.Y))
			m.grow = m.grow.Add(d)
			// The default place is anchored at the lower right: keep the top-left still.
			m.offset = m.offset.Add(d)
		}
	}
	o := op.Offset(size.Sub(image.Pt(side, side))).Push(gtx.Ops)
	defer o.Pop()
	glyph(gtx, "grip", 14, withAlpha(u.P.Muted, .7))
	focusRing(gtx, u, &m.resizeStart, image.Pt(side, side))
	defer clip.Rect{Max: image.Pt(side, side)}.Push(gtx.Ops).Pop()
	m.resize.Add(gtx.Ops)
	event.Op(gtx.Ops, &m.resizeStart)
	pointer.CursorNorthWestResize.Add(gtx.Ops)
}

// panelSize applies the resize to a panel's default size.
func panelSize(size, window image.Point, close *Button, minW, minH int) image.Point {
	m := movesOf(close)
	grown := size.Add(m.grow)
	grown.X = min(max(grown.X, minW), window.X)
	grown.Y = min(max(grown.Y, minH), window.Y)
	m.grow = grown.Sub(size)
	return grown
}

// currentAction is the palette's action colour for the title-bar focus mark
// (dragArea has no UI; resizeGrip of the same frame sets it).
var currentAction = rgb(0x3d7bd9)
