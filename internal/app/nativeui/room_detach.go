package nativeui

import (
	"sync"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// Detached panels (floating.detach): Interactions and Remote control can
// leave the room for their own always-on-top window, like the web page's
// document picture-in-picture. Both windows lay out under one frame lock, so
// the Room is never drawn from two goroutines at once. Closing the window
// puts the panel back in the room.

type detachedPanels struct {
	mu      sync.Mutex
	windows map[string]*app.Window // "interact", "remote"
	frame   *sync.Mutex            // shared with the main window's frames
	ui      *UI
	main    *app.Window
	mainWnd uintptr // the main window's handle: detached windows open on its screen
}

// drawingDetached is true while a detached window lays out (frame lock held):
// panels then hide their detach button and move/resize handles.
var drawingDetached bool

func (d *detachedPanels) isDetached(name string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.windows[name] != nil
}

// invalidateAll redraws the main window and every detached one.
func (d *detachedPanels) invalidateAll() {
	if d.main != nil {
		d.main.Invalidate()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, w := range d.windows {
		w.Invalidate()
	}
}

// raise brings a detached panel's window to the front (floating.focus).
func (d *detachedPanels) raise(name string) {
	d.mu.Lock()
	w := d.windows[name]
	d.mu.Unlock()
	if w != nil {
		w.Perform(system.ActionRaise)
	}
}

// open detaches panel name into its own window (no-op without a main window).
func (r *Room) detachPanel(name string) {
	d := &r.detached
	if d.frame == nil || d.ui == nil {
		return
	}
	d.mu.Lock()
	if d.windows == nil {
		d.windows = map[string]*app.Window{}
	}
	if d.windows[name] != nil {
		d.mu.Unlock()
		d.raise(name)
		return
	}
	w := new(app.Window)
	d.windows[name] = w
	d.mu.Unlock()
	title := "VibePiik · " + d.ui.T("interaction.title")
	if name == "remote" {
		title = "VibePiik · " + d.ui.T("remote.title")
	}
	w.Option(app.Title(title), app.Size(unit.Dp(420), unit.Dp(600)), app.MinSize(unit.Dp(300), unit.Dp(320)), app.TopMost(true))
	go func() {
		var ops op.Ops
		var own uintptr
		placed := 0
		for {
			switch e := w.Event().(type) {
			case app.DestroyEvent:
				d.mu.Lock()
				delete(d.windows, name)
				d.mu.Unlock()
				d.frame.Lock()
				// The panel goes back into the room, still open.
				if name == "interact" {
					r.interact.open = true
				} else {
					r.remote.open = true
				}
				d.frame.Unlock()
				r.invalidate()
				return
			case app.Win32ViewEvent:
				own = e.HWND
			case app.FrameEvent:
				d.frame.Lock()
				gtx := app.NewContext(&ops, e)
				beginFrame(own, gtx.Constraints.Max)
				closed := r.detachedLayout(gtx, d.ui, name)
				d.frame.Unlock()
				e.Frame(gtx.Ops)
				if placed < 4 && own != 0 {
					// After Gio's own setup (it may move the window during its first
					// frames): VibePiik's screen, always on top.
					placed++
					placeBeside(own, d.mainWnd)
					w.Invalidate()
				}
				if closed {
					w.Perform(system.ActionClose)
				}
			}
		}
	}()
	r.invalidate()
}

// detachedLayout draws a detached panel filling its window; it reports a
// click on the panel's own close button.
func (r *Room) detachedLayout(gtx layout.Context, u *UI, name string) bool {
	fillRect(gtx.Ops, gtx.Constraints.Max, u.P.Wall)
	snap := r.host.Snapshot()
	drawingDetached = true
	defer func() { drawingDetached = false }()
	gtx.Constraints = layout.Exact(gtx.Constraints.Max)
	switch name {
	case "interact":
		r.interact.open = true
		r.interactionsPanel(gtx, u, snap)
		return !r.interact.open
	case "remote":
		r.remote.open = true
		r.remotePanel(gtx, u, snap)
		return !r.remote.open
	}
	return true
}

// panelDetacher is the detach button of a panel, by its close button.
type panelDetacher struct {
	button Button
	open   func()
}

var panelDetachers = map[*Button]*panelDetacher{}

// registerDetachers makes Interactions and Remote control detachable.
func (r *Room) registerDetachers() {
	if r.detached.frame == nil {
		return
	}
	for close, name := range map[*Button]string{&r.interact.close: "interact", &r.remote.close: "remote"} {
		if panelDetachers[close] == nil {
			name := name
			panelDetachers[close] = &panelDetacher{open: func() {
				r.detached.frame.Lock()
				defer r.detached.frame.Unlock()
				r.detachPanel(name)
			}}
		}
	}
}

// closeAll closes every detached window (the main window is going away).
func (d *detachedPanels) closeAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, w := range d.windows {
		w.Perform(system.ActionClose)
	}
}
