package nativeui

import (
	"image"
	"os"
	"strconv"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

// updateModal is the update window: changelog, "Install now" / "Later",
// download and install progress. One for the whole App (launcher and room).
var updateModal struct {
	open     bool
	install  Button
	later    Button
	page     Button
	list     widget.List
	tested   bool
	backdrop int // event tag
}

func openUpdateModal() { updateModal.open = true }

// autoOpenUpdate opens an important update by itself once per version (not
// while sharing).
func autoOpenUpdate(notice *releaseNotice, sharing bool) {
	if notice == nil || sharing || updateModal.open {
		return
	}
	if !notice.Important && os.Getenv("VIBEPIIK_TEST_UPDATE") == "" {
		return
	}
	shown := loadPreferences().UpdateShown
	if shown == notice.Version && os.Getenv("VIBEPIIK_TEST_UPDATE") == "" {
		return
	}
	updateModal.open = true
	version := notice.Version
	go updatePreferences(func(p *preferences) { p.UpdateShown = version })
}

// layoutUpdateModal draws the window over everything (deferred). stopShare
// ends a running share before installing (nil in the launcher); sharing says
// whether one is running; open opens a page in the browser.
func layoutUpdateModal(gtx layout.Context, u *UI, notice *releaseNotice, sharing bool, stopShare func(), open func(string), invalidate func()) {
	if !updateModal.open || notice == nil {
		return
	}
	m := &updateModal
	state, progress, failure := updateState()
	busy := state == "download" || state == "apply"
	start := func() {
		if busy {
			return
		}
		go func() {
			if sharing && stopShare != nil {
				stopShare()
			}
			installUpdate(notice, invalidate)
		}()
		setUpdateState("download", 0, "")
	}
	if m.install.Clicked(gtx) && notice.Package {
		start()
	}
	if m.later.Clicked(gtx) && !busy {
		m.open = false
		if state == "failed" {
			setUpdateState("", 0, "")
		}
		return
	}
	if m.page.Clicked(gtx) && open != nil {
		go open(notice.ReleaseURL)
	}
	if os.Getenv("VIBEPIIK_TEST_UPDATE") == "install" && os.Getenv("VIBEPIIK_RESTARTED") == "" && !m.tested {
		// Automated tests only: press "Install now" once.
		m.tested = true
		start()
	}
	p := &u.P
	size := gtx.Constraints.Max
	macro := op.Record(gtx.Ops)
	fillRect(gtx.Ops, size, withAlpha(p.Shade, .55))
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &m.backdrop)
	pointer.CursorDefault.Add(gtx.Ops)
	area.Pop()
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape}, pointer.Filter{Target: &m.backdrop, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, isKey := ev.(key.Event); isKey && e.State == key.Press && !busy {
			m.open = false
		}
	}
	width := min(gtx.Dp(560), size.X-gtx.Dp(32))
	cg := gtx
	cg.Constraints = layout.Constraints{Max: image.Pt(width, size.Y-gtx.Dp(64))}
	cardMacro := op.Record(gtx.Ops)
	card := layout.UniformInset(unit.Dp(24)).Layout(cg, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		title := u.T("native.update.title", "version", notice.Version)
		if notice.Important {
			title += " · " + u.T("native.update.important")
		}
		status := ""
		switch state {
		case "download":
			status = u.T("native.update.downloading", "percent", strconv.Itoa(progress))
		case "apply":
			status = u.T("native.update.applying")
		case "failed":
			status = u.T("native.update.failed", "error", failure)
		default:
			if sharing && notice.Package {
				status = u.T("native.update.stopsShare")
			}
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						ink := p.Live
						if notice.Important {
							ink = p.Warn
						}
						return glyph(gtx, "arrowUp", 22, ink)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, title, textStyle{size: 16, weight: font.Bold, color: p.Ink})
					}))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// The changelog scrolls inside at most half the window.
				gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, max(size.Y/2, gtx.Dp(160)))
				m.list.Axis = layout.Vertical
				changelog := releaseChangelog(notice.Body, u.textLang())
				if changelog == "" {
					changelog = u.T(releaseLabels[notice.Kind]) + " " + notice.Version
				}
				return material(gtx, &m.list, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, changelog, textStyle{size: 13, color: p.Ink, lineH: 1.5})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if status == "" {
					return layout.Dimensions{}
				}
				ink := p.Muted
				switch {
				case state == "failed":
					ink = p.DangerText
				case state == "" && sharing:
					ink = p.Warn
				}
				return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, status, textStyle{size: 12.5, weight: font.SemiBold, color: ink, lineH: 1.4})
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = 0
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return m.page.Layout(gtx, u, ButtonStyle{Caption: u.T("native.update.page"), Size: sizeDense, Tone: toneQuiet})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return m.later.Layout(gtx, u, ButtonStyle{Icon: "x", Caption: u.T("native.update.later"), Size: sizeCompact, Disabled: busy})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !notice.Package {
							return layout.Dimensions{}
						}
						caption := u.T("native.update.install")
						if state == "failed" {
							caption = u.T("native.update.retry")
						}
						return m.install.Layout(gtx, u, ButtonStyle{Icon: "arrowDown", Caption: caption, Size: sizeCompact, Tone: tonePrimary,
							Busy: busy, Disabled: busy})
					}))
			}))
	})
	cardCall := cardMacro.Stop()
	o := op.Offset(image.Pt((size.X-card.Size.X)/2, (size.Y-card.Size.Y)/2)).Push(gtx.Ops)
	shape := boxOf(card.Size, px(gtx, 22))
	shadow(gtx.Ops, shape, px(gtx, 10), px(gtx, 30), withAlpha(p.Shade, .35))
	fillBox(gtx.Ops, shape, p.Paper)
	strokeBox(gtx.Ops, shape, px(gtx, 1.5), p.ControlEdge)
	cardCall.Add(gtx.Ops)
	o.Pop()
	op.Defer(gtx.Ops, macro.Stop())
	if busy {
		gtx.Execute(op.InvalidateCmd{})
	}
	_ = text.Middle
}

func init() {
	for lang, texts := range map[string]map[string]string{
		"en": {
			"native.update.title":       "VibePiik {version}",
			"native.update.important":   "important update",
			"native.update.install":     "Install now",
			"native.update.later":       "Later",
			"native.update.retry":       "Try again",
			"native.update.page":        "Release page ↗",
			"native.update.button":      "Update to {version}",
			"native.update.downloading": "Downloading the update… {percent}%",
			"native.update.applying":    "Installing: VibePiik restarts by itself…",
			"native.update.failed":      "Could not update: {error}",
			"native.update.stopsShare":  "Installing stops the current stream; VibePiik then restarts by itself.",
		},
		"it": {
			"native.update.title":       "VibePiik {version}",
			"native.update.important":   "aggiornamento importante",
			"native.update.install":     "Installa ora",
			"native.update.later":       "Più tardi",
			"native.update.retry":       "Riprova",
			"native.update.page":        "Pagina della versione ↗",
			"native.update.button":      "Aggiorna a {version}",
			"native.update.downloading": "Download dell'aggiornamento… {percent}%",
			"native.update.applying":    "Installazione: VibePiik si riavvia da solo…",
			"native.update.failed":      "Impossibile aggiornare: {error}",
			"native.update.stopsShare":  "L'installazione ferma la stream in corso; poi VibePiik si riavvia da solo.",
		},
		"zh": {
			"native.update.title":       "VibePiik {version}",
			"native.update.important":   "重要更新",
			"native.update.install":     "立即安装",
			"native.update.later":       "稍后",
			"native.update.retry":       "重试",
			"native.update.page":        "版本页面 ↗",
			"native.update.button":      "更新到 {version}",
			"native.update.downloading": "正在下载更新… {percent}%",
			"native.update.applying":    "正在安装：VibePiik 将自动重启…",
			"native.update.failed":      "无法更新：{error}",
			"native.update.stopsShare":  "安装会停止当前直播，之后 VibePiik 会自动重启。",
		},
	} {
		catalog := map[string]map[string]string{"en": nativeCopy_en, "it": nativeCopy_it, "zh": nativeCopy_zh}[lang]
		for key, value := range texts {
			catalog[key] = value
		}
	}
}
