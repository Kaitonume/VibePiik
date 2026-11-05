package nativeui

import (
	"image"
	"os"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
)

// Opening VibePiik in the browser (the console's [o]) closes this window. A
// running share would be cut off without a word, so the window comes to the
// front and asks first; "Yes" stops the share, and the browser page starts
// it again.

// ConfirmBrowser is asked by the console before the page moves to the
// browser. It returns once the host has answered (true: go ahead).
func (m *Manager) ConfirmBrowser() bool {
	m.mu.Lock()
	native := m.native
	m.mu.Unlock()
	if native == nil || native.sharing == nil || !native.sharing() {
		return true
	}
	answer := make(chan bool, 1)
	m.mu.Lock()
	native.askBrowser = answer
	m.mu.Unlock()
	native.window.Perform(system.ActionRaise)
	native.window.Invalidate()
	select {
	case ok := <-answer:
		return ok
	case <-native.done:
		return true
	}
}

// browserAsk is the open question on the room (nil: none).
type browserAsk struct {
	answer   chan bool
	yes, no  Button
	answered bool
}

func (r *Room) askBrowser(answer chan bool) {
	r.browser = &browserAsk{answer: answer}
	if os.Getenv("VIBEPIIK_TEST_BROWSER") == "yes" {
		// Automated tests only: answer "Yes" after a moment.
		go func() { time.Sleep(2500 * time.Millisecond); r.answerBrowser(true) }()
	}
}

// answerBrowser closes the question; "Yes" stops the share first.
func (r *Room) answerBrowser(ok bool) {
	ask := r.browser
	if ask == nil || ask.answered {
		return
	}
	ask.answered = true
	r.browser = nil
	go func() {
		if ok {
			r.host.EndShare()
		}
		ask.answer <- ok
	}()
	r.invalidate()
}

// browserModal draws the question over the whole room.
func (r *Room) browserModal(gtx layout.Context, u *UI) {
	ask := r.browser
	if ask == nil {
		return
	}
	if ask.yes.Clicked(gtx) {
		r.answerBrowser(true)
		return
	}
	if ask.no.Clicked(gtx) {
		r.answerBrowser(false)
		return
	}
	p := &u.P
	size := gtx.Constraints.Max
	macro := op.Record(gtx.Ops)
	// Dim the room and catch every click outside the card.
	fillRect(gtx.Ops, size, withAlpha(p.Shade, .55))
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, ask)
	pointer.CursorDefault.Add(gtx.Ops)
	area.Pop()
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape}, pointer.Filter{Target: ask, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, isKey := ev.(key.Event); isKey && e.State == key.Press {
			r.answerBrowser(false)
		}
	}
	cg := gtx
	cg.Constraints = layout.Constraints{Max: image.Pt(min(gtx.Dp(460), size.X-gtx.Dp(32)), size.Y)}
	cardMacro := op.Record(gtx.Ops)
	card := layout.UniformInset(unit.Dp(24)).Layout(cg, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "alert", 30, p.Warn) }),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T("native.browserStop"), textStyle{size: 14, weight: font.SemiBold, color: p.Ink, align: text.Middle, lineH: 1.5})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = 0 // the two buttons centred under the text
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ask.no.Layout(gtx, u, ButtonStyle{Icon: "x", Caption: u.T("native.no"), Size: sizeCompact})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ask.yes.Layout(gtx, u, ButtonStyle{Icon: "check", Caption: u.T("native.yes"), Size: sizeCompact, Tone: tonePrimary})
					}))
			}))
	})
	cardCall := cardMacro.Stop()
	at := image.Pt((size.X-card.Size.X)/2, (size.Y-card.Size.Y)/2)
	o := op.Offset(at).Push(gtx.Ops)
	shape := boxOf(card.Size, px(gtx, 22))
	shadow(gtx.Ops, shape, px(gtx, 10), px(gtx, 30), withAlpha(p.Shade, .35))
	fillBox(gtx.Ops, shape, p.Paper)
	strokeBox(gtx.Ops, shape, px(gtx, 1.5), p.ControlEdge)
	cardCall.Add(gtx.Ops)
	o.Pop()
	op.Defer(gtx.Ops, macro.Stop())
	_ = paint.ColorOp{}
}

func init() {
	nativeCopy_en["native.browserStop"] = "To open VibePiik in the browser, the current stream must stop: you will have to start it again from the browser. Continue?"
	nativeCopy_en["native.yes"] = "Yes"
	nativeCopy_en["native.no"] = "No"
	nativeCopy_zh["native.browserStop"] = "要在浏览器中打开 VibePiik，必须先停止当前的直播，之后需要在浏览器中重新开始。确定吗？"
	nativeCopy_zh["native.yes"] = "是"
	nativeCopy_zh["native.no"] = "否"
	nativeCopy_it["native.browserStop"] = "Se vuoi aprire VibePiik su browser bisogna fermare la stream in corso, dovrai avviarla nuovamente da browser. Confermi?"
	nativeCopy_it["native.yes"] = "Sì"
	nativeCopy_it["native.no"] = "No"
}
