package nativeui

import (
	"context"
	"errors"
	"github.com/TNTcraftHIM/Piik/internal/app/buildinfo"
	"image"
	"image/color"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

// launcherBackend is the App launcher API (launcherClient, or a fake in tests).
type launcherBackend interface {
	State(context.Context) (LauncherState, error)
	Launch(ctx context.Context, mode, language, password, lanAddress string, debug bool) (string, error)
	SendTV(ctx context.Context, link string) ([]string, int, error)
	EnableDebug(context.Context) error
	SetSite(ctx context.Context, site string, allowed bool) error
	SetStartupSite(ctx context.Context, site string) error
}

// Launcher is the App start page (AppLauncherPage + LauncherForm).
type Launcher struct {
	backend    launcherBackend
	invalidate func()
	onTarget   func(target string) // a room is ready: show it
	onTheme    func()
	onLanguage func(string)
	onExternal func(string) // system browser

	mu        sync.Mutex
	state     LauncherState
	loaded    bool
	loadError error
	starting  bool
	launchErr *LaunchError
	debugBusy bool
	tvState   string // "", "busy", "sent", "failed"
	tvMessage string
	tvExpire  time.Time

	mode           string
	welcome        int
	list           widget.List
	modeCards      [2]RadioCard
	password       Input
	access         Disclosure
	lanOpen        Disclosure
	lanPick        []widget.Clickable
	guide          Disclosure
	addSite        Button
	open           Button
	retry          Button
	tvInput        Input
	tvSend         Button
	debug          Button
	siteStar       []siteStar
	brandAt        time.Time        // BrandMark motion="once" start
	brandHover     widget.Clickable // replays it on pointer enter
	brandWasHover  bool
	release        releaseView
	siteOnStart    bool // star the typed site once approved (approval page "Open on start")
	siteOnStartBox widget.Clickable
	errorDetails   Disclosure
	debugErr       bool
	langs          Segmented
	theme          Button
	github         Button
	passwordSynced bool
	siteOpen       bool
	siteInput      Input
	siteAdd        Button
	siteError      string
	siteLinks      []widget.Clickable
	siteRemove     []widget.Clickable
	loadRetry      Button
	lanSelected    string
	focusFrom      int // site count when the removal started
	focusRemove    int // after removing a site, focus the next row's remove button (KB-8); -1 none
}

func newLauncher(backend launcherBackend, invalidate func()) *Launcher {
	l := &Launcher{backend: backend, invalidate: invalidate, welcome: rand.IntN(20), focusRemove: -1}
	l.list.Axis = layout.Vertical
	go l.reload()
	return l
}

func (l *Launcher) reload() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state, err := l.backend.State(ctx)
	l.mu.Lock()
	l.loadError = err
	if err == nil {
		l.state, l.loaded = state, true
		if l.mode == "" {
			l.mode = state.DefaultMode
			if l.mode == "" && len(state.Modes) > 0 {
				l.mode = state.Modes[0]
			}
		}
		if state.LAN != nil && l.lanSelected == "" {
			l.lanSelected = state.LAN.Selected
		}
	}
	starting := err == nil && state.Starting
	l.mu.Unlock()
	l.invalidate()
	if starting {
		time.AfterFunc(750*time.Millisecond, l.reload)
	}
}

func (l *Launcher) launch(u *UI) {
	l.mu.Lock()
	if l.starting {
		l.mu.Unlock()
		return
	}
	l.starting, l.launchErr = true, nil
	mode, password, lan := l.mode, l.password.Editor.Text(), l.lanSelected
	debug := l.state.Debug != nil && *l.state.Debug
	l.mu.Unlock()
	language := u.Lang
	if language != "zh" && language != "vis" {
		language = "en" // the App launcher knows en, zh and vis (Italian is native only)
	}
	go func() {
		target, err := l.backend.Launch(context.Background(), mode, language, password, lan, debug)
		l.mu.Lock()
		l.starting = false
		var launchErr *LaunchError
		if err != nil {
			if !errors.As(err, &launchErr) {
				launchErr = &LaunchError{Message: err.Error()}
			}
			if launchErr.Status == 409 {
				// Another launch owns the App: read its state instead of failing.
				l.loaded = false
				l.mu.Unlock()
				l.reload()
				return
			}
			l.launchErr = launchErr
		} else {
			l.state.Target = target
		}
		l.mu.Unlock()
		l.invalidate()
		if err == nil && l.onTarget != nil {
			l.onTarget(target)
		}
	}()
}

func (l *Launcher) sendTV(u *UI) {
	l.mu.Lock()
	if l.tvState == "busy" {
		l.mu.Unlock()
		return
	}
	link, ok := piikTvLink(l.tvInput.Editor.Text())
	if !ok {
		l.tvState, l.tvMessage = "failed", piikTvText(u.textLang(), "card.invalidExample")
		l.tvExpire = time.Now().Add(8 * time.Second)
		l.mu.Unlock()
		return
	}
	l.tvInput.Editor.SetText(link)
	l.tvState, l.tvMessage = "busy", piikTvText(u.textLang(), "card.searching")
	l.mu.Unlock()
	uiLanguage := u.textLang()
	go func() {
		names, status, err := l.backend.SendTV(context.Background(), link)
		state, message := "failed", ""
		switch {
		case err == nil && len(names) > 0:
			state, message = "sent", piikTvText(uiLanguage, "send.sentTo", "names", strings.Join(names, ", "))
		case status == 404:
			message = piikTvText(uiLanguage, "send.noTv")
		case status == 400:
			message = piikTvText(uiLanguage, "card.invalid")
		case status == 0:
			message = piikTvText(uiLanguage, "send.appDown")
		default:
			message = piikTvText(uiLanguage, "send.failed", "detail", strconv.Itoa(status))
		}
		l.mu.Lock()
		l.tvState, l.tvMessage, l.tvExpire = state, message, time.Now().Add(8*time.Second)
		l.mu.Unlock()
		l.invalidate()
		time.AfterFunc(8*time.Second+50*time.Millisecond, l.invalidate)
	}()
}

func (l *Launcher) Layout(gtx layout.Context, u *UI) layout.Dimensions {
	p := &u.P
	fillRect(gtx.Ops, gtx.Constraints.Max, p.Wall)
	l.mu.Lock()
	state, loaded, starting := l.state, l.loaded, l.starting || l.state.Starting
	if !l.passwordSynced && loaded {
		l.password.Editor.SetText(state.LocalAccessPassword)
		l.passwordSynced = true
	}
	if !l.tvExpire.IsZero() && gtx.Now.After(l.tvExpire) {
		l.tvState, l.tvMessage, l.tvExpire = "", "", time.Time{}
	}
	l.mu.Unlock()

	// Events.
	for i, mode := range l.modes(state) {
		if i < len(l.modeCards) && l.modeCards[i].Clicked(gtx) {
			l.mu.Lock()
			l.mode = mode
			l.mu.Unlock()
		}
	}
	if l.open.Clicked(gtx) || l.retry.Clicked(gtx) {
		if state.Target != "" && l.onTarget != nil {
			go l.onTarget(state.Target)
		} else {
			l.launch(u)
		}
	}
	if l.tvSend.Clicked(gtx) {
		l.sendTV(u)
	}
	for {
		event, ok := l.tvInput.Editor.Update(gtx)
		if !ok {
			break
		}
		if _, submit := event.(widget.SubmitEvent); submit {
			l.sendTV(u)
		}
		if l.tvState == "failed" {
			l.tvState, l.tvMessage = "", ""
		}
	}
	if l.theme.Clicked(gtx) && l.onTheme != nil {
		l.onTheme()
	}
	if l.addSite.Clicked(gtx) {
		l.siteOpen = !l.siteOpen
	}
	if l.siteAdd.Clicked(gtx) {
		l.addSiteNow()
	}
	if l.github.Clicked(gtx) && l.onExternal != nil {
		go l.onExternal(projectPage())
	}
	if l.loadRetry.Clicked(gtx) {
		go l.reload()
	}
	if l.debug.Clicked(gtx) {
		l.mu.Lock()
		l.debugBusy = true
		l.mu.Unlock()
		go func() {
			err := l.backend.EnableDebug(context.Background())
			l.mu.Lock()
			l.debugBusy, l.debugErr = false, err != nil
			l.mu.Unlock()
			l.reload()
		}()
	}

	notice := appRelease(state.Version, state.Revision, l.invalidate)
	autoOpenUpdate(notice, false)
	layoutUpdateModal(gtx, u, notice, false, nil, l.onExternal, l.invalidate)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return l.header(gtx, u, state) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			l.mu.Lock()
			failed := !loaded && l.loadError != nil
			l.mu.Unlock()
			if failed {
				return l.loadFailed(gtx, u)
			}
			if !loaded || starting {
				return l.loading(gtx, u, starting)
			}
			return material(gtx, &l.list, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(36), Bottom: unit.Dp(40), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(620))
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return l.panel(gtx, u, state)
						})
					})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return l.footer(gtx, u, state) }),
	)
}

// material lays out a scrollable single-item list filling the space.
func material(gtx layout.Context, list *widget.List, body layout.Widget) layout.Dimensions {
	return list.List.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return body(gtx)
	})
}

func (l *Launcher) modes(state LauncherState) []string {
	order := []string{"link", "local"}
	if len(state.Modes) == 0 {
		return order
	}
	var modes []string
	for _, mode := range order {
		for _, available := range state.Modes {
			if available == mode {
				modes = append(modes, mode)
			}
		}
	}
	return modes
}

func (l *Launcher) header(gtx layout.Context, u *UI, state LauncherState) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(6), Left: unit.Dp(44), Right: unit.Dp(44)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					options := []string{"glyph:zhong", "EN", "IT", "✦"}
					languages := []string{"zh", "en", "it", "vis"}
					selected := 1
					for i, language := range languages {
						if language == u.Lang {
							selected = i
						}
					}
					dims, chosen := l.langs.Layout(gtx, u, options, selected)
					if chosen != selected && l.onLanguage != nil {
						l.onLanguage(languages[chosen])
					}
					return dims
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					icon, caption := "moon", u.T("theme.dark.short")
					if u.P.Dark {
						icon, caption = "sun", u.T("theme.light.short")
					}
					return l.theme.Layout(gtx, u, ButtonStyle{Icon: icon, Caption: caption, Size: sizeCompact, IconSize: 16})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if state.Debug == nil {
						return layout.Dimensions{}
					}
					return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						tip := u.T("client.launch.debug") + "\n" + u.T("client.launch.debugHint")
						if l.debugErr {
							tip = u.T("client.launch.debugFailed")
						}
						return l.debug.Layout(gtx, u, ButtonStyle{Comic: "hint-debug-export", Icon: "cpu", Size: sizeCompact, Pressed: *state.Debug,
							Disabled: *state.Debug || l.debugBusy || state.Target != "", Busy: l.debugBusy, Tip: tip, Tone: map[bool]tone{true: toneDanger, false: toneDefault}[l.debugErr]})
					})
				}),
			)
		})
}

func (l *Launcher) loading(gtx layout.Context, u *UI, starting bool) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// The brand loader breathes while the room starts.
				phase := float32(gtx.Now.UnixMilli()%1600) / 1600
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(33 * time.Millisecond)})
				alpha := 0.55 + 0.45*float32(1-abs32(phase*2-1))
				return brandMark(gtx, 64, withAlpha(u.P.Ink, alpha))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				key := "common.loading"
				if starting {
					key = "client.launch.starting"
				}
				return u.label(gtx, u.T(key), textStyle{size: 14, weight: font.SemiBold, color: u.P.Ink})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !starting {
					return layout.Dimensions{}
				}
				return waitingCaptionLabel(gtx, u, u.P.Muted, 0)
			}))
	})
}

func (l *Launcher) panel(gtx layout.Context, u *UI, state LauncherState) layout.Dimensions {
	p := &u.P
	// A centred grid column: children keep their own width unless they fill it.
	gtx.Constraints.Min.X = 0
	gap := layout.Spacer{Height: unit.Dp(20)}.Layout
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return l.heading(gtx, u) }),
		layout.Rigid(gap),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return l.standby(gtx, u, state) }),
		layout.Rigid(gap),
	}
	if l.launchErr == nil && state.Reason != "" && state.Target == "" && !state.Starting {
		l.launchErr = &LaunchError{Message: state.Detail, Detail: state.Detail, Reason: state.Reason}
	}
	if l.launchErr != nil {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return l.launchError(gtx, u) }),
			layout.Rigid(gap))
	}
	if notice := appRelease(state.Version, state.Revision, l.invalidate); notice != nil {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return l.release.Layout(gtx, u, notice, l.onExternal)
		}), layout.Rigid(gap))
	}
	if state.Target != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return l.open.Layout(gtx, u, ButtonStyle{Comic: "hint-create-room", Icon: "arrowRight", Caption: u.T("client.launch.go"), Tone: tonePrimary, IconSize: 19})
		}))
	} else {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if u.Visual() {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(-8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return u.label(gtx, u.T("client.launch.room"), textStyle{size: 13, weight: font.Bold, color: p.Muted})
				})
			}),
			layout.Rigid(gap),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return l.modeChoice(gtx, u, state) }),
			layout.Rigid(gap),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return l.roomSettings(gtx, u, state) }),
		)
		if l.mode == "local" && !u.Visual() {
			children = append(children, layout.Rigid(gap), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T("client.launch.localHTTPS"), textStyle{size: 12, color: p.Muted, align: text.Middle, lineH: 1.6})
			}))
		}
		children = append(children, layout.Rigid(gap), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			icon, caption, button := "arrowRight", u.T("client.launch.go"), &l.open
			if l.launchErr != nil {
				icon, caption, button = "refresh", u.T("common.retry"), &l.retry
			}
			lanMissing := l.mode == "local" && state.LAN != nil && l.lanSelected == ""
			return button.Layout(gtx, u, ButtonStyle{Comic: "hint-client-" + l.mode, Icon: icon, Caption: caption, Tone: tonePrimary, IconSize: 18, Disabled: lanMissing})
		}))
	}
	children = append(children, layout.Rigid(gap), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return l.tvCard(gtx, u, state)
	}))
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (l *Launcher) heading(gtx layout.Context, u *UI) layout.Dimensions {
	p := &u.P
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// The mascot's feet (y=29 of 32) sit on the wordmark baseline.
						// It draws itself once, and again when the pointer enters.
						if l.brandAt.IsZero() {
							l.brandAt = gtx.Now
						}
						hovered := l.brandHover.Hovered()
						if hovered && !l.brandWasHover && gtx.Now.Sub(l.brandAt) > 1100*time.Millisecond {
							l.brandAt = gtx.Now
						}
						l.brandWasHover = hovered
						dims := l.brandHover.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return brandMarkOnce(gtx, 52, p.Ink, l.brandAt) })
						dims.Baseline = int(float32(dims.Size.Y) * (1 - 29.0/32 - .09375))
						return dims
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, "VibePiik", textStyle{size: 36, weight: font.Bold, color: p.Ink, maxLines: 1})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, ".", textStyle{size: 36, weight: font.Bold, color: p.BrandDot, maxLines: 1})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, "App", textStyle{size: 15, weight: font.SemiBold, color: p.Muted, maxLines: 1})
					}))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lines := welcome_en
				switch u.Lang {
				case "zh":
					lines = welcome_zh
				case "it":
					lines = welcome_it
				}
				line := ""
				if !u.Visual() && len(lines) > 0 {
					line = lines[l.welcome%len(lines)]
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, "✦", textStyle{size: 15, weight: font.SemiBold, color: p.WelcomeStar, maxLines: 1})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if u.Visual() {
							return welcomeCipher(gtx, u, l.welcome) // WelcomeCipher: two pictures instead of words
						}
						return u.label(gtx, line, textStyle{size: 15, weight: font.SemiBold, color: p.Ink, maxLines: 1, lineH: 1.7})
					}))
			}))
	})
}

func (l *Launcher) standby(gtx layout.Context, u *UI, state LauncherState) layout.Dimensions {
	p := &u.P
	if l.guide.Button.Clicked(gtx) {
		l.guide.Open = !l.guide.Open
	}
	angle := float32(0)
	if l.guide.Open {
		angle = 3.14159
	}
	rotation := l.guide.rotate.set(gtx, angle, 180*time.Millisecond, spring)
	return card(gtx, u, 22, layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(16), Left: unit.Dp(20), Right: unit.Dp(20)},
		func(gtx layout.Context) layout.Dimensions {
			rows := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return l.guide.Button.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "check", 18, p.Ink) }),
							layout.Rigid(layout.Spacer{Width: unit.Dp(9)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, u.T("client.connect.standby"), textStyle{size: 14, weight: font.Bold, color: p.Ink})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return dimsOf(gtx.Constraints.Max.X, gtx.Dp(40))
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								ink := p.Muted
								if l.guide.Button.click.Hovered() {
									ink = p.Ink
								}
								return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return u.label(gtx, u.T("client.connect.guide"), textStyle{size: 12, color: ink})
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										side := px(gtx, 14)
										defer op.Affine(rotateAbout(side/2, side/2, rotation)).Push(gtx.Ops).Pop()
										return glyph(gtx, "chevron", 14, ink)
									}))
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return l.addSite.Layout(gtx, u, ButtonStyle{Comic: "hint-client-site", Icon: "plus", Size: sizeDense, Tone: toneQuiet})
							}))
					})
				}),
			}
			if l.guide.Open {
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, u.T("client.connect.standbyHint"), textStyle{size: 12, color: p.Muted, lineH: 1.65})
					})
				}))
			}
			if l.siteOpen {
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return l.siteForm(gtx, u) }))
			}
			if len(l.siteLinks) != len(state.Sites) {
				l.siteLinks = make([]widget.Clickable, len(state.Sites))
				l.siteRemove = make([]widget.Clickable, len(state.Sites))
				l.siteStar = make([]siteStar, len(state.Sites))
			}
			for i, site := range state.Sites {
				i, site := i, site
				if l.siteLinks[i].Clicked(gtx) && l.onExternal != nil {
					go l.onExternal(site + "/#piik-client=1")
				}
				if l.siteStar[i].click.Clicked(gtx) {
					next := site
					if state.StartupSite == site {
						next = ""
					}
					go func() {
						if err := l.backend.SetStartupSite(context.Background(), next); err != nil {
							l.mu.Lock()
							l.siteError = "client.connect.saveFailed"
							l.mu.Unlock()
						}
						l.reload()
					}()
				}
				if l.siteRemove[i].Clicked(gtx) {
					l.focusRemove, l.focusFrom = i, len(state.Sites)
					go func() {
						if err := l.backend.SetSite(context.Background(), site, false); err != nil {
							l.mu.Lock()
							l.siteError = "client.connect.saveFailed"
							l.mu.Unlock()
						}
						l.reload()
					}()
				}
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return l.siteLinks[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									d := u.label(gtx, site+" ↗", textStyle{size: 12, mono: true, color: p.Ink, lineH: 1.6})
									pointerHand(gtx, d.Size)
									return d
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								// ★: open this site when the App starts (client.connect.openOnStart).
								star := &l.siteStar[i]
								return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return star.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										ink := withAlpha(p.Muted, .6)
										if state.StartupSite == site {
											ink = rgb(0xefb23f)
										}
										d := glyph(gtx, "star", 15, ink)
										pointerHand(gtx, d.Size)
										star.hoverAt = hoverTooltip(gtx, u, star.click.Hovered(), star.hoverAt, u.T("client.connect.openOnStart"), d.Size)
										return d
									})
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return l.siteRemove[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									d := glyph(gtx, "x", 14, p.Muted)
									pointerHand(gtx, d.Size)
									return d
								})
							}))
					})
				}))
			}
			if l.focusRemove >= 0 && len(state.Sites) < l.focusFrom && len(l.siteRemove) == len(state.Sites) {
				// The list has been reread: focus the row now in that place.
				if len(state.Sites) > 0 {
					gtx.Execute(key.FocusCmd{Tag: &l.siteRemove[min(l.focusRemove, len(state.Sites)-1)]})
				} else {
					gtx.Execute(key.FocusCmd{Tag: &l.addSite.click})
				}
				l.focusRemove = -1
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		})
}

func (l *Launcher) modeChoice(gtx layout.Context, u *UI, state LauncherState) layout.Dimensions {
	modes := l.modes(state)
	info := map[string][3]string{
		"link":  {"globe", "client.launch.link", "client.launch.linkHint"},
		"local": {"users", "client.launch.local", "client.launch.localHint"},
	}
	// Arrow keys move the choice like a radio group (LauncherForm onKeyDown).
	for i := range modes {
		if i >= len(l.modeCards) {
			break
		}
		for {
			ev, ok := gtx.Event(key.Filter{Focus: &l.modeCards[i].click, Name: key.NameLeftArrow},
				key.Filter{Focus: &l.modeCards[i].click, Name: key.NameRightArrow},
				key.Filter{Focus: &l.modeCards[i].click, Name: key.NameUpArrow},
				key.Filter{Focus: &l.modeCards[i].click, Name: key.NameDownArrow})
			if !ok {
				break
			}
			e, ok := ev.(key.Event)
			if !ok || e.State != key.Press {
				continue
			}
			next := (i + 1) % len(modes)
			if e.Name == key.NameLeftArrow || e.Name == key.NameUpArrow {
				next = (i + len(modes) - 1) % len(modes)
			}
			l.mu.Lock()
			l.mode = modes[next]
			l.mu.Unlock()
			gtx.Execute(key.FocusCmd{Tag: &l.modeCards[next].click})
		}
	}
	children := make([]layout.FlexChild, 0, 2*len(modes))
	for i, mode := range modes {
		i, mode := i, mode
		if i > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout))
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			entry := info[mode]
			l.modeCards[i].Comic = "hint-client-" + mode
			l.modeCards[i].Tip = ""
			if !u.Visual() {
				l.modeCards[i].Tip = u.T(entry[2])
			}
			return l.modeCards[i].Layout(gtx, u, entry[0], u.T(entry[1]), u.T(entry[2]), l.mode == mode)
		}))
	}
	return layout.Flex{}.Layout(gtx, children...)
}

func (l *Launcher) roomSettings(gtx layout.Context, u *UI, state LauncherState) layout.Dimensions {
	p := &u.P
	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(460))
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	marker := ""
	if l.password.Editor.Text() != "" {
		marker = u.T("client.launch.accessSet")
	}
	access := func(gtx layout.Context) layout.Dimensions {
		return l.access.Layout(gtx, u, "key", u.T("client.launch.accessSettings"), marker, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					placeholder := u.T("client.launch.localAccess")
					if u.Visual() {
						placeholder = ""
					}
					return l.password.Layout(gtx, u, "key", placeholder, true)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if u.Visual() {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, u.T("client.launch.localAccessHint"), textStyle{size: 12, color: p.Muted, lineH: 1.6})
					})
				}))
		})
	}
	showLAN := l.mode == "local" && state.LAN != nil && (len(state.LAN.Addresses) != 1 || state.LAN.Selected == "")
	if !showLAN {
		return access(gtx)
	}
	lan := func(gtx layout.Context) layout.Dimensions {
		if state.LAN.Selected == "" && !l.lanOpen.Open && l.lanSelected == "" {
			l.lanOpen.Open = true
		}
		if len(l.lanPick) != len(state.LAN.Addresses) {
			l.lanPick = make([]widget.Clickable, len(state.LAN.Addresses))
		}
		return l.lanOpen.Layout(gtx, u, "users", u.T("client.launch.lanAddress"), "", func(gtx layout.Context) layout.Dimensions {
			if len(state.LAN.Addresses) == 0 {
				return u.label(gtx, u.T("client.launch.lanUnavailable"), textStyle{size: 12, color: p.Warn})
			}
			rows := []layout.FlexChild{}
			for i, address := range state.LAN.Addresses {
				i, address := i, address
				if l.lanPick[i].Clicked(gtx) {
					l.lanSelected = address.Address
				}
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return l.lanPick[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						ink := p.Ink
						mark := "○ "
						if l.lanSelected == address.Address {
							ink, mark = p.ActionText, "● "
						}
						return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, mark+address.Address+" · "+address.Name, textStyle{size: 13, color: ink})
						})
					})
				}))
			}
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T("client.launch.lanHint"), textStyle{size: 11, color: p.Muted})
			}))
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		})
	}
	return layout.Flex{Alignment: layout.Start}.Layout(gtx,
		layout.Flexed(1, access), layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout), layout.Flexed(1, lan))
}

func (l *Launcher) launchError(gtx layout.Context, u *UI) layout.Dimensions {
	p := &u.P
	err := l.launchErr
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return pill(gtx, u, "alert", u.T("client.launch.error"), p.Danger)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			key := "client.launch.failure." + err.Reason
			message := u.T(key)
			if message == "" {
				message = err.Message
			}
			return u.label(gtx, message, textStyle{size: 14, weight: font.SemiBold, color: p.Ink, align: text.Middle})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, u.T("client.launch.retryHint"), textStyle{size: 14, weight: font.SemiBold, color: p.Ink, align: text.Middle})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if err.Detail == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return l.errorDetails.Layout(gtx, u, "alert", u.T("client.launch.errorDetails"), "", func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, err.Detail, textStyle{size: 13, mono: true, color: p.Muted})
				})
			})
		}))
}

// pill is .lr-pill: a rounded status label with an icon.
func pill(gtx layout.Context, u *UI, icon, caption string, tint color.NRGBA) layout.Dimensions {
	p := &u.P
	macro := op.Record(gtx.Ops)
	measure := gtx
	measure.Constraints.Min = image.Point{}
	dims := layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(14), Right: unit.Dp(16)}.Layout(measure,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, icon, 18, tint) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, caption, textStyle{size: 13, weight: font.SemiBold, color: p.Ink})
				}))
		})
	call := macro.Stop()
	shape := boxOf(dims.Size, float32(dims.Size.Y)/2)
	fillBox(gtx.Ops, shape, p.NoticeDanger)
	strokeBox(gtx.Ops, shape, px(gtx, 2), tint)
	call.Add(gtx.Ops)
	return dims
}

func (l *Launcher) tvCard(gtx layout.Context, u *UI, state LauncherState) layout.Dimensions {
	p := &u.P
	tv := func(key string, vars ...string) string { return piikTvText(u.textLang(), key, vars...) }
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.Visual() {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, tv("card.title"), textStyle{size: 13, weight: font.Bold, color: p.Muted})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card(gtx, u, 22, layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(16), Left: unit.Dp(20), Right: unit.Dp(20)},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return l.tvInput.Layout(gtx, u, "link", tv("card.placeholder"), false)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									icon := "tv"
									if l.tvState == "sent" {
										icon = "check"
									}
									return l.tvSend.Layout(gtx, u, ButtonStyle{Icon: icon, Caption: tv("card.button"), Tone: tonePrimary,
										IconSize: 19, Busy: l.tvState == "busy", Disabled: l.tvState == "busy"})
								}))
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if l.tvMessage == "" {
								return layout.Dimensions{}
							}
							ink := p.Ink
							switch l.tvState {
							case "sent":
								ink = p.Live
							case "failed":
								ink = p.Danger
							}
							return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, l.tvMessage, textStyle{size: 13, weight: font.SemiBold, color: ink})
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if u.Visual() {
								return layout.Dimensions{}
							}
							return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, tv("card.hint"), textStyle{size: 12, color: p.Muted, lineH: 1.65})
							})
						}))
				})
		}))
}

func (l *Launcher) footer(gtx layout.Context, u *UI, state LauncherState) layout.Dimensions {
	p := &u.P
	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(16), Left: unit.Dp(44), Right: unit.Dp(44)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return l.github.Layout(gtx, u, ButtonStyle{Icon: "", Caption: "GitHub ↗", Size: sizeDense, Tone: toneQuiet})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					version := state.Version
					if version == "" {
						version = buildinfo.Version()
					}
					return u.label(gtx, "VibePiik "+vibeVersion+" · Piik "+version,
						textStyle{size: 11.5, color: p.Muted, align: text.End})
				}))
		})
}

// addSiteNow approves the typed Piik Site and opens it in the browser.
func (l *Launcher) addSiteNow() {
	site := strings.TrimRight(strings.TrimSpace(l.siteInput.Editor.Text()), "/")
	if site == "" {
		return
	}
	if !strings.Contains(site, "://") {
		site = "https://" + site
	}
	star := l.siteOnStart
	go func() {
		err := l.backend.SetSite(context.Background(), site, true)
		if err == nil && star {
			// "Open on start" chosen on the approval: star it right away.
			err = l.backend.SetStartupSite(context.Background(), site)
		}
		l.mu.Lock()
		if err != nil {
			l.siteError = "client.connect.invalid"
		} else {
			l.siteError, l.siteOpen, l.siteOnStart = "", false, false
			l.siteInput.Editor.SetText("")
		}
		l.mu.Unlock()
		if err == nil && l.onExternal != nil {
			l.onExternal(site + "/#piik-client=1")
		}
		l.reload()
	}()
}

func (l *Launcher) siteForm(gtx layout.Context, u *UI) layout.Dimensions {
	p := &u.P
	for {
		event, ok := l.siteInput.Editor.Update(gtx)
		if !ok {
			break
		}
		if _, submit := event.(widget.SubmitEvent); submit {
			l.addSiteNow()
		}
	}
	l.siteInput.Editor.SingleLine, l.siteInput.Editor.Submit = true, true
	l.mu.Lock()
	siteError := l.siteError
	l.mu.Unlock()
	return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T("client.connect.addSiteHint"), textStyle{size: 12, color: p.Muted, lineH: 1.6})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return l.siteInput.Layout(gtx, u, "globe", u.T("client.connect.siteAddress"), false)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return l.siteAdd.Layout(gtx, u, ButtonStyle{Icon: "arrowRight", Caption: u.T("client.connect.approveAndOpen"), Size: sizeCompact, Tone: tonePrimary})
					}))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// The approval page's "Open on start" check box.
				if l.siteOnStartBox.Clicked(gtx) {
					l.siteOnStart = !l.siteOnStart
				}
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return l.siteOnStartBox.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						ink, mark := p.Muted, "check"
						if l.siteOnStart {
							ink = rgb(0xefb23f)
						}
						d := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								side := gtx.Dp(18)
								box := boxOf(image.Pt(side, side), float32(gtx.Dp(5)))
								strokeBox(gtx.Ops, box, px(gtx, 1.5), withAlpha(p.Muted, .8))
								if l.siteOnStart {
									fillBox(gtx.Ops, box, p.Action)
									gtx.Constraints = layout.Exact(image.Pt(side, side))
									layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return glyph(gtx, mark, 13, p.ActionText) })
								}
								return dimsOf(side, side)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "star", 15, ink) }),
							layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if u.Visual() {
									return layout.Dimensions{}
								}
								return u.label(gtx, u.T("client.connect.openOnStart"), textStyle{size: 12.5, color: p.Ink})
							}))
						pointerHand(gtx, d.Size)
						return d
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if siteError == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, u.T(siteError), textStyle{size: 12, color: p.Danger})
				})
			}))
	})
}

// loadFailed replaces the endless loader when the App launcher cannot be read.
func (l *Launcher) loadFailed(gtx layout.Context, u *UI) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return brandMark(gtx, 64, u.P.Muted) }),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T("gate.connectFailed"), textStyle{size: 14, weight: font.SemiBold, color: u.P.Ink})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return l.loadRetry.Layout(gtx, u, ButtonStyle{Icon: "refresh", Caption: u.T("common.retry"), Size: sizeCompact})
			}))
	})
}

// siteStar is the ★ toggle of a site row.
type siteStar struct {
	click   widget.Clickable
	hoverAt time.Time
}

var welcomeSymbols = [][2]string{{"couch", "popcorn"}, {"gamepad", "trophy"}, {"clapperboard", "star"}, {"gift", "heart"},
	{"tv", "users"}, {"flag", "zap"}, {"smile", "bulb"}}

// welcomeCipher is the ✦ welcome line: two glyphs, no words.
func welcomeCipher(gtx layout.Context, u *UI, index int) layout.Dimensions {
	return welcomeCipherInk(gtx, index, u.P.Ink)
}

func welcomeCipherInk(gtx layout.Context, index int, ink color.NRGBA) layout.Dimensions {
	pair := welcomeSymbols[index%len(welcomeSymbols)]
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, pair[0], 18, ink) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, pair[1], 18, ink) }))
}
