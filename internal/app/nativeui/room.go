package nativeui

import (
	"context"
	"github.com/TNTcraftHIM/Piik/internal/app/buildinfo"
	"image"
	"io"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/TNTcraftHIM/Piik/internal/app/nativecapture"
	"github.com/TNTcraftHIM/Piik/internal/server/protocol"
)

// Share phases of the native host.
const (
	phaseIdle     = "idle"
	phaseStarting = "starting"
	phaseLive     = "live"
	phasePaused   = "paused"
)

type roomPeer struct {
	ID, Name   string
	Upstream   string // peerId this viewer receives from; "" = the host (or not yet routed)
	Routed     bool   // upstream kind is not "none"
	MediaReady bool
}

// viewerStats is the latest viewer-quality-evidence of one viewer.
type viewerStats struct {
	Width, Height int
	FPS, Kbps     float64
	LossPercent   float64
	RTTms         float64
	HasRTT        bool
	Path          string // natTraversalPath
	At            time.Time
	Stutter       bool       // a freeze in the latest sample
	Detail        []statLine // detailed metrics (stats.more)
}

// shareSettings are the sharing settings panel's values (HostPage quality,
// codec mode, route policy, audio).
type shareSettings struct {
	Quality    protocol.QualitySettings
	Codec      string // "h264", "vp8" or "auto"
	Topology   bool
	Microphone bool
	MicAvail   bool
	Limits     videoLimits // App capture ceilings (zero: local defaults)
	MicDevice  string      // commentary microphone ("" = default)
	MicVolume  float64     // commentary volume 0–2
}

func (s shareSettings) limits() videoLimits {
	if s.Limits.MaxWidth == 0 {
		return localVideoLimits
	}
	return s.Limits
}

// roomSnapshot is what the room screen shows; roomHost owns it.
type roomSnapshot struct {
	Signal  string // "offline", "connecting", "connected"
	Phase   string
	Code    string
	Invite  string
	Source  string
	Preview image.Image
	Viewers []roomPeer
	Name    string
	Notice  string
	Busy    bool // room creation in flight

	HostID      string // host peerId (colour)
	Stats       map[string]viewerStats
	Settings    shareSettings
	SourceAudio *bool
	CodecInUse  string
	Policy      string // "open" or "private"
	Canonical   string // invite without the #v= grant
	Target      nativecapture.CaptureTarget

	Chat               []chatMessage
	Reactions          []activeReaction
	Unread             int
	InteractionsReady  bool
	InteractionPending bool
	InteractionError   string
	Remote             remoteSnapshot
	Voice              voiceSnapshot
	ShareVolume        float64
	ShareMuted         bool
	Switching          bool
	Applying           bool // settings going to the live share (busy spinner)
	NoticeAt           time.Time
	ShareVolumeOK      bool
	Capture            captureStats
	Audio              bool
	PasswordOn         bool
	FailureCode        string
	Exclude            *nativecapture.CaptureTarget // application left out of screen sound
	CanExclude         bool
}

// roomHost is the native host logic (hostSession, or a fake in tests).
type roomHost interface {
	Snapshot() roomSnapshot
	Sources(ctx context.Context) ([]nativecapture.CaptureTarget, error)
	SourcePreview(ctx context.Context, target nativecapture.CaptureTarget) (image.Image, error)
	Start(target nativecapture.CaptureTarget, audio, border bool)
	SetPaused(paused bool)
	Stop()
	CreateRoom()
	SendTV(ctx context.Context) ([]string, int, error)
	SetSettings(shareSettings)
	SetName(name string) error
	SetPolicy(policy string)
	RotateInvite()
	RevokeInvite()
	ReplaceRoom()
	SendChat(text string) error
	SendReaction(reaction, target string)
	MarkChatRead()
	SetRemotePolicy(mode, password string)
	SetRemoteSimultaneous(enabled bool)
	DecideRemote(id string, allow bool)
	SetStopShortcut(key int)
	VoiceOpen()
	VoiceClose()
	VoiceMicrophone(on bool)
	VoiceDeafen(on bool)
	VoiceMute(peer string)
	VoiceVolume(volume float64)
	VoicePersonVolume(peer string, volume float64)
	VoiceBitrate(bitrate int)
	Diagnostics() map[string]any
	VoiceLinks() []voiceLinkView
	Microphones(ctx context.Context) ([]audioDevice, error)
	SetCommentary(device string, volume float64)
	VoiceDevices() ([]audioDevice, error)
	VoiceDevice(id string)
	VoiceMicGain(gain float64)
	VoiceProcessing(on bool)
	VoiceGate(position float64)
	PreviewShown(width int)
	EndShare() // stops sharing and returns when done
	SetShareVolume(volume float64, muted bool)
	CancelStart()
	SetSourceAudio(on bool)
	SetViewerPassword(password string)
	SetExcludeAudio(target *nativecapture.CaptureTarget)
	Close()
}

// Room is the host room screen (HostPage).
type Room struct {
	host       roomHost
	invalidate func()
	onTheme    func()
	onLanguage func(string)
	onHelp     func()
	onJoin     func()
	onExternal func(string)

	mu       sync.Mutex
	picking  bool
	loading  bool
	status   string
	sources  []*pickerSource
	border   bool
	audio    bool
	welcome  int
	tvState  string
	tvText   string
	tvExpire time.Time
	copied   time.Time
	previews map[string]image.Image
	pictures map[image.Image]paint.ImageOp

	list     widget.List
	picker   SourcePicker
	start    RoundAction
	join     RoundAction
	settings Button
	pause    Button
	stop     Button
	switchSr Button
	lcd      LCD
	edit     Button
	details  Button
	topology Button
	tvSend   Button
	appBtn   Button
	langs    Segmented
	theme    Button
	help     Button

	// Stage 3 panels.
	panel             sharingPanel
	settingsOpen      bool
	pendingSettings   *shareSettings
	includeCredential bool
	accessOpen        bool
	detailsOpen       bool
	topologyOpen      bool
	name              nameEditor
	copyBtn           Button
	rotateBtn         Button
	revokeBtn         Button
	replaceBtn        Button
	entryBtn          Button
	credential        Switch
	policyToggle      Segmented
	switchFlat        FlatButton
	pauseFlat         FlatButton
	stopFlat          FlatButton
	settingsFlat      FlatButton

	// Batch 1 parity.
	cancelFlat     FlatButton
	helpPanel      helpPanel
	chinHelp       widget.Clickable
	passwordInput  Input
	passwordSet    Button
	passwordRemove Button
	replaceArmed   time.Time

	// Batch 2 parity.
	pickExclude    exclusionPicker
	liveExclude    exclusionPicker
	apps           []nativecapture.CaptureTarget
	appsBusy       bool
	appsFailed     bool
	micList        deviceList // commentary microphones (from the App)
	voiceDevs      deviceList // room voice microphones (Windows)
	person         personMenu
	chat           chatOverlayState
	previewCtl     previewControls
	inviteSel      widget.Selectable
	wasReady       map[string]bool // viewers that had the picture (host.notice.reconnecting)
	testDetached   bool
	detached       detachedPanels
	chinHoverAt    time.Time
	ledClick       widget.Clickable // header connection LEDs (LedStrip tooltip)
	ledHoverAt     time.Time
	ledPinned      bool                  // toggleOnClick
	pawnTips       map[string]*time.Time // hover start per pawn
	browser        *browserAsk           // open in browser? (ConfirmBrowser)
	release        releaseView
	pipClose       func() // closes the picture-in-picture window
	theater        bool
	fullscreen     bool
	fullClick      gesture.Click
	onFullscreen   func(bool)
	testTVSent     bool
	voiceLinkList  []voiceLinkView
	voiceLinksAt   time.Time
	voiceLinksBusy bool
	showPassword   bool
	passwordEye    Button
	noticeClose    Button
	noticeHidden   time.Time // the notice dismissed by the host
	fit            string    // picture fit of the local preview
	previewOff     bool      // local preview paused on this PC
	autoPause      bool      // pause the preview while VibePiik is in the background
	focused        bool      // VibePiik window has the focus
	card           viewerCard
	selectedViewer string
	viewerRows     map[string]*widget.Clickable

	// Stage 4 floating panels.
	interact interactPanel
	remote   remotePanel

	// Live preview of the shared window (DWM thumbnail), set by the window.
	preview livePreview
}

// livePreview shows a shared window live inside the TV (dwmPreview on Windows).
type livePreview interface {
	// Show draws source into dest (window pixels), cropped to clip; Hide removes it.
	Show(source nativecapture.CaptureTarget, dest, clip image.Rectangle) bool
	Hide()
	// SetFit is the picture fit: contain, cover or fill (playback.fit.*).
	SetFit(mode string)
}

func newRoom(host roomHost, invalidate func()) *Room {
	room := &Room{host: host, invalidate: invalidate, welcome: rand.IntN(len(welcome_en)), audio: true, border: true,
		previews: map[string]image.Image{}, pictures: map[image.Image]paint.ImageOp{}, includeCredential: true, accessOpen: true}
	room.list.Axis = layout.Vertical
	if prefs := loadPreferences(); prefs.PickNoSound || prefs.PickNoBorder {
		room.audio, room.border = !prefs.PickNoSound, !prefs.PickNoBorder
	}
	room.chat = loadOverlaySettings()
	if prefs := loadPreferences(); true {
		room.fit, room.autoPause, room.focused = prefs.PreviewFit, !prefs.PreviewNoAutoPause, true
		if room.fit == "" {
			room.fit = "contain"
		}
	}
	if os.Getenv("VIBEPIIK_TEST_PANELS") == "1" {
		// Automated tests only: open the deck panels for a screenshot.
		room.detailsOpen, room.topologyOpen = true, true
	}
	if os.Getenv("VIBEPIIK_TEST_AUTOSTART") == "1" && os.Getenv("VIBEPIIK_TEST_SOURCES") != "" {
		go func() { time.Sleep(2 * time.Second); room.openPicker() }()
	}
	return room
}

func (r *Room) openPicker() {
	r.mu.Lock()
	r.picking, r.loading, r.status = true, true, ""
	r.mu.Unlock()
	r.invalidate()
	go r.loadSources()
}

func (r *Room) loadSources() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	targets, err := r.host.Sources(ctx)
	if filter := os.Getenv("VIBEPIIK_TEST_SOURCES"); filter != "" {
		// Automated tests only: list just the test window, never anyone else's.
		var kept []nativecapture.CaptureTarget
		for _, target := range targets {
			if strings.Contains(target.Title, filter) {
				kept = append(kept, target)
			}
		}
		targets = kept
	}
	r.mu.Lock()
	r.loading = false
	if err != nil {
		r.status = "host.sourcePicker.failed"
	} else if len(targets) == 0 {
		r.status = "host.sourcePicker.empty"
	}
	r.sources = r.sources[:0]
	for _, target := range targets {
		r.sources = append(r.sources, &pickerSource{Target: target, Preview: r.previews[captureKey(target)]})
	}
	sources := append([]*pickerSource(nil), r.sources...)
	r.mu.Unlock()
	r.invalidate()
	if os.Getenv("VIBEPIIK_TEST_AUTOSTART") == "1" && len(sources) == 1 {
		// Automated tests only: share the single test window after a look at the picker.
		go func() {
			time.Sleep(3 * time.Second)
			r.mu.Lock()
			r.picking = false
			r.mu.Unlock()
			r.host.Start(sources[0].Target, false, true)
		}()
	}
	// Previews one at a time, like the web picker.
	for _, source := range sources {
		r.mu.Lock()
		still := r.picking
		r.mu.Unlock()
		if !still {
			return
		}
		previewCtx, cancelPreview := context.WithTimeout(context.Background(), 5*time.Second)
		picture, err := r.host.SourcePreview(previewCtx, source.Target)
		cancelPreview()
		if err != nil || picture == nil {
			continue
		}
		r.mu.Lock()
		source.Preview = picture
		source.opSet = false
		r.previews[captureKey(source.Target)] = picture
		r.mu.Unlock()
		r.invalidate()
	}
}

func captureKey(target nativecapture.CaptureTarget) string {
	return target.Kind + "\x00" + target.SourceID + "\x00" + target.Title
}

func (r *Room) sendTV(u *UI) {
	r.mu.Lock()
	if r.tvState == "busy" {
		r.mu.Unlock()
		return
	}
	r.tvState, r.tvText = "busy", piikTvText(u.textLang(), "card.searching")
	r.mu.Unlock()
	lang := u.Lang
	go func() {
		names, status, err := r.host.SendTV(context.Background())
		state, message := "failed", ""
		switch {
		case err == nil && len(names) > 0:
			state, message = "sent", piikTvText(lang, "send.sentTo", "names", strings.Join(names, ", "))
		case status == 404:
			message = piikTvText(lang, "send.noTv")
		case status == 409:
			message = piikTvText(lang, "send.noRoom")
		case status == 0:
			message = piikTvText(lang, "send.appDown")
		default:
			message = piikTvText(lang, "send.failed", "detail", itoa(status))
		}
		r.mu.Lock()
		r.tvState, r.tvText, r.tvExpire = state, message, time.Now().Add(6*time.Second)
		r.mu.Unlock()
		r.invalidate()
		time.AfterFunc(6*time.Second+50*time.Millisecond, r.invalidate)
	}()
}

func (r *Room) Layout(gtx layout.Context, u *UI) layout.Dimensions {
	p := &u.P
	fillRect(gtx.Ops, gtx.Constraints.Max, p.Wall)
	snap := r.host.Snapshot()
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape}, key.Filter{Name: key.NameSpace}, key.Filter{Name: key.NameF11})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			switch {
			case e.Name == key.NameEscape && r.fullscreen:
				r.setFullscreen(false)
			case e.Name == key.NameF11 && (snap.Phase == phaseLive || snap.Phase == phasePaused):
				r.setFullscreen(!r.fullscreen)
			case e.Name == key.NameEscape:
				r.escape()
			case snap.Phase == phaseLive || snap.Phase == phasePaused:
				r.togglePreview() // Space: the local preview, like the page's player
			}
		}
	}
	if !r.testDetached && os.Getenv("VIBEPIIK_TEST_DETACH") == "1" && snap.Phase == phaseLive && r.detached.frame != nil {
		// Automated tests only: detach Interactions once sharing is live.
		r.testDetached = true
		go func() {
			r.detached.frame.Lock()
			defer r.detached.frame.Unlock()
			r.detachPanel("interact")
		}()
	}
	if !r.testTVSent && os.Getenv("VIBEPIIK_TEST_SENDTV") == "1" && snap.Phase == phaseLive && snap.Invite != "" {
		// Automated tests only: press "Send URL to PIIK TV app" once sharing is live.
		r.testTVSent = true
		r.sendTV(u)
	}
	r.mu.Lock()
	if !r.tvExpire.IsZero() && gtx.Now.After(r.tvExpire) {
		r.tvState, r.tvText, r.tvExpire = "", "", time.Time{}
	}
	if r.picking && snap.Phase != phaseIdle && snap.Phase != phaseLive && snap.Phase != phasePaused {
		r.picking = false
	}
	r.mu.Unlock()

	// Events.
	if r.start.Clicked(gtx) || r.switchFlat.Clicked(gtx) {
		r.openPicker()
	}
	if r.join.Clicked(gtx) && r.onJoin != nil {
		r.onJoin()
	}
	if r.pauseFlat.Clicked(gtx) {
		r.host.SetPaused(snap.Phase != phasePaused)
	}
	if r.settingsFlat.Clicked(gtx) {
		r.settingsOpen = !r.settingsOpen
	}
	if r.edit.Clicked(gtx) {
		r.name.editing = true
		r.name.input.SetText(displayName(u, snap.Name))
		gtx.Execute(key.FocusCmd{Tag: &r.name.input})
	}
	if r.details.Clicked(gtx) {
		r.detailsOpen = !r.detailsOpen
	}
	if r.topology.Clicked(gtx) {
		r.topologyOpen = !r.topologyOpen
	}
	if r.entryBtn.Clicked(gtx) {
		r.accessOpen = !r.accessOpen
	}
	if r.replaceBtn.Clicked(gtx) {
		// One click arms, a second within 5 s replaces (it ends the share).
		if !r.replaceArmed.IsZero() && gtx.Now.Sub(r.replaceArmed) < 5*time.Second {
			r.replaceArmed = time.Time{}
			r.host.ReplaceRoom()
		} else {
			r.replaceArmed = gtx.Now
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(5*time.Second + 10*time.Millisecond)})
		}
	}
	if r.cancelFlat.Clicked(gtx) {
		r.host.CancelStart()
	}
	r.flushSettings(gtx)
	if r.stopFlat.Clicked(gtx) {
		r.host.Stop()
	}
	if r.lcd.Clicked(gtx) {
		switch {
		case snap.Code != "":
			gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: ioNopCloser(snap.Code)})
			r.copied = gtx.Now
		case snap.Code == "" && !snap.Busy:
			r.host.CreateRoom()
		}
	}
	if r.tvSend.Clicked(gtx) {
		r.sendTV(u)
	}
	if r.theme.Clicked(gtx) && r.onTheme != nil {
		r.onTheme()
	}
	if r.help.Clicked(gtx) || r.appBtn.Clicked(gtx) || r.chinHelp.Clicked(gtx) {
		r.helpPanel.open = !r.helpPanel.open
	}
	if false && r.onHelp != nil {
		r.onHelp()
	}

	r.browserModal(gtx, u) // on top of everything (deferred)
	if r.fullscreen {
		if snap.Phase == phaseLive || snap.Phase == phasePaused {
			return r.fullscreenLayout(gtx, u, snap)
		}
		r.setFullscreen(false)
	}
	var headerH int
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := r.header(gtx, u, snap)
			headerH = d.Size.Y
			return d
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material(gtx, &r.list, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(64), Left: unit.Dp(40), Right: unit.Dp(40)}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, r.contentCap(gtx))
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return r.scene(gtx, u, snap)
						})
					})
			})
		}),
	)
	r.autoOpenRemote(snap)
	r.overlays(gtx, u, snap)
	r.placePreview(gtx, snap, headerH)
	if dest, clip, ok := r.tvPicture(gtx, snap, headerH); ok {
		r.feedOverlay(gtx, u, snap, dest, dest.Intersect(clip))
	} else {
		r.feedOverlay(gtx, u, snap, image.Rectangle{}, image.Rectangle{})
	}
	return dims
}

// placePreview puts the live window preview over the TV screen, which sits
// at a fixed place in the scrolled scene (Layout's insets, layoutTV's frame).
func (r *Room) placePreview(gtx layout.Context, snap roomSnapshot, headerH int) {
	if r.preview == nil {
		return
	}
	r.mu.Lock()
	picking := r.picking
	r.mu.Unlock()
	live := snap.Phase == phaseLive && !picking && snap.Target.Kind == "window"
	if r.preview != nil {
		r.preview.SetFit(r.fit)
	}
	// The live picture is drawn by Windows above the page: hide it under a question.
	if !live || r.list.Position.First != 0 || r.previewPaused() || r.browser != nil {
		r.preview.Hide()
		return
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
	dest := image.Rect(tvX+pad, tvY+pad, tvX+pad+screenW, tvY+pad+screenH)
	clip := image.Rect(0, headerH, window.X, window.Y)
	// Floating panels sit above the page: keep the live preview out of them.
	for _, panel := range []image.Rectangle{r.interact.panelRect, r.interact.remoteRect, r.interact.helpRect, r.person.rect} {
		if !panel.Empty() && panel.Min.Y < dest.Max.Y && panel.Max.Y > dest.Min.Y && panel.Min.X < clip.Max.X {
			clip.Max.X = max(clip.Min.X, panel.Min.X)
		}
	}
	if !r.preview.Show(snap.Target, dest, clip) {
		r.preview.Hide()
	}
}

func (r *Room) header(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	p := &u.P
	// header.more: narrow windows keep icons, the words move to the tooltips.
	compact := gtx.Constraints.Max.X < gtx.Dp(1020)
	short := func(caption string) string {
		if compact {
			return ""
		}
		return caption
	}
	return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(6), Left: unit.Dp(44), Right: unit.Dp(44)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Top: unit.Dp(5), Bottom: unit.Dp(5)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions { return brandMark(gtx, 34, p.Ink) })
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							key, state, comic := "state.signal.offline", "off", "signal-offline"
							switch snap.Signal {
							case "connected":
								key, state, comic = "state.signal.connected", "live", "signal-connected"
							case "connecting":
								key, state, comic = "state.signal.connecting", "busy", "signal-connecting"
							case "reconnecting":
								key, state, comic = "state.signal.reconnecting", "busy", "signal-recovering"
							}
							if r.ledClick.Clicked(gtx) {
								r.ledPinned = !r.ledPinned // the web strip toggles its tooltip on click
							}
							return r.ledClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								d := leds(gtx, u, u.T(key), state)
								hovered := r.ledClick.Hovered()
								if r.ledPinned && !hovered {
									hovered = true
									if r.ledHoverAt.IsZero() {
										r.ledHoverAt = gtx.Now.Add(-500 * time.Millisecond)
									}
								}
								r.ledHoverAt = hoverTooltipComic(gtx, u, hovered, r.ledHoverAt, u.T(key), comic, d.Size)
								return d
							})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							caption, icon := piikTvText(u.textLang(), "send.label"), "tv"
							r.mu.Lock()
							state, message := r.tvState, r.tvText
							r.mu.Unlock()
							if state == "sent" {
								caption, icon = piikTvText(u.textLang(), "send.sentLabel"), "check"
							} else if state == "failed" && message != "" {
								caption, icon = message, "alert"
							}
							return r.tvSend.Layout(gtx, u, ButtonStyle{Icon: icon, Caption: short(caption), Size: sizeCompact,
								Busy: state == "busy", Disabled: snap.Invite == "", Tip: map[bool]string{true: piikTvText(u.textLang(), "send.noRoom"), false: piikTvText(u.textLang(), "send.hint")}[snap.Invite == ""]})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							// AppConnectionButton: ready, or a newer Piik release.
							icon, tip, comic := "check", u.T("client.connect.manage"), "hint-app-ready"
							if notice := appRelease("", "", r.invalidate); notice != nil && (notice.Kind == "update-available" || notice.Kind == "major-update") {
								icon, comic = "arrowUp", "update-available"
								tip = u.T("client.connect.update", "current", buildinfo.Version(), "version", notice.Version)
							}
							return r.appBtn.Layout(gtx, u, ButtonStyle{Comic: comic, Icon: icon, Caption: short("App"), Size: sizeCompact, Tip: tip})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							options := []string{"glyph:zhong", "EN", "IT", "✦"}
							languages := []string{"zh", "en", "it", "vis"}
							selected := 1
							for i, language := range languages {
								if language == u.Lang {
									selected = i
								}
							}
							dims, chosen := r.langs.Layout(gtx, u, options, selected)
							if chosen != selected && r.onLanguage != nil {
								r.onLanguage(languages[chosen])
							}
							return dims
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							icon, caption := "moon", u.T("theme.dark.short")
							if u.P.Dark {
								icon, caption = "sun", u.T("theme.light.short")
							}
							tip := u.T("theme.dark")
							if u.P.Dark {
								tip = u.T("theme.light")
							}
							return r.theme.Layout(gtx, u, ButtonStyle{Icon: icon, Caption: short(caption), Size: sizeCompact, IconSize: 16, Tip: tip})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.help.Layout(gtx, u, ButtonStyle{Comic: "hint-help", Icon: "help", Size: sizeCompact, Tone: toneQuiet, Tip: u.T("help.hint")})
						}),
					)
				}))
		})
}

func (r *Room) scene(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	r.mu.Lock()
	picking := r.picking
	r.mu.Unlock()
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutTV(gtx, u, r.tvCap(gtx), func(gtx layout.Context) layout.Dimensions {
				if picking {
					return r.pickerScreen(gtx, u)
				}
				return r.screen(gtx, u, snap)
			}, func(gtx layout.Context) layout.Dimensions {
				layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return r.chinTools(gtx, u, snap) })
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					color, icon := u.P.Idle, "moon"
					switch snap.Phase {
					case phaseLive:
						color, icon = u.P.Live, "cast"
					case phasePaused:
						color, icon = u.P.Warn, "pause"
					case phaseStarting:
						color, icon = u.P.Busy, "loader"
					}
					// The status light opens troubleshooting, like the web page.
					return r.chinHelp.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						d := glyph(gtx, icon, 17, color)
						pointerHand(gtx, d.Size)
						// The status light's hint: state and its comic (StatusIndicator).
						tip, kind := u.T("host.idle"), "waiting-for-host"
						switch snap.Phase {
						case phaseLive:
							tip, kind = u.T("host.live"), "share-live"
						case phasePaused:
							tip, kind = u.T("host.paused"), "host-paused"
						case phaseStarting:
							tip, kind = u.T("host.starting"), "source-starting"
						}
						r.chinHoverAt = hoverTooltipComic(gtx, u, r.chinHelp.Hovered(), r.chinHoverAt, tip, kind, d.Size)
						return d
					})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return r.liveNotice(gtx, u, snap) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return r.toolbar(gtx, u, snap) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !r.settingsOpen {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return r.sharingSettings(gtx, u, snap) })
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(720))
			own := hostColor
			if snap.HostID != "" {
				own = participantColor(snap.HostID)
			}
			pawns := []pawn{{name: displayName(u, snap.Name), color: own, host: true, you: true}}
			for _, viewer := range snap.Viewers {
				stats, ok := snap.Stats[viewer.ID]
				stutter := ok && stats.Stutter && time.Since(stats.At) < 10*time.Second
				pawns = append(pawns, pawn{name: viewer.Name, color: participantColor(viewer.ID), ready: viewer.MediaReady, stutter: stutter, id: viewer.ID})
			}
			pawns[0].id = snap.HostID
			for i := range pawns {
				voiceKey, deafened := "", false
				for _, member := range snap.Voice.Members {
					if member.PeerID != pawns[i].id {
						continue
					}
					voiceKey, deafened = "voice.microphoneInactive", member.Deafened
					if member.Publishing {
						pawns[i].mic = 1
						voiceKey = "voice.microphoneActive"
						if member.Speaking {
							pawns[i].mic = 2
							voiceKey = "voice.speaking"
						}
					}
				}
				for _, request := range snap.Remote.Requests {
					if request.PeerID == pawns[i].id && request.State != "pending" {
						pawns[i].remote = true
					}
				}
				pawns[i].reaction, pawns[i].progress = reactionFor(snap, pawns[i].id, gtx.Now)
				pawns[i].gifts = giftsFor(snap, pawns[i].id, gtx.Now, pawns[i].reaction)
				if pawns[i].id != "" && snap.Phase != phaseIdle {
					click := r.person.click(&r.person.pawns, pawns[i].id)
					if click.Clicked(gtx) {
						r.person.open(pawns[i].id)
					}
					pawns[i].click = click
					// personHint: the full name (overflow), host/you, voice state,
					// remote control and what a click does.
					lines := []string{pawns[i].name}
					if pawns[i].host {
						lines[0] += " · " + u.T("common.host")
					}
					status := ""
					if voiceKey != "" {
						status = u.T(voiceKey)
						if deafened {
							status += ", " + u.T("voice.listeningPaused")
						}
					}
					if pawns[i].remote {
						if status != "" {
							status += ", "
						}
						status += u.T("remote.participant.control")
					}
					if status != "" {
						lines = append(lines, status)
					}
					lines = append(lines, u.T("interaction.express"))
					pawns[i].tip, pawns[i].tipComic = strings.Join(lines, "\n"), "hint-reaction-picker"
					if r.pawnTips == nil {
						r.pawnTips = map[string]*time.Time{}
					}
					if r.pawnTips[pawns[i].id] == nil {
						r.pawnTips[pawns[i].id] = new(time.Time)
					}
					pawns[i].tipAt = r.pawnTips[pawns[i].id]
				}
			}
			ids := make([]string, len(pawns))
			for i := range pawns {
				ids[i] = pawns[i].id
			}
			return layoutCouch(gtx, u, pawns, throwsFor(snap, ids, gtx.Now), func(gtx layout.Context) layout.Dimensions { return r.couchFooter(gtx, u, snap) })
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return r.deck(gtx, u, snap) }),
	)
}

func displayName(u *UI, name string) string {
	if name != "" {
		return name
	}
	return u.T("common.host")
}

func (r *Room) pickerScreen(gtx layout.Context, u *UI) layout.Dimensions {
	r.mu.Lock()
	sources := append([]*pickerSource(nil), r.sources...)
	loading, status := r.loading, r.status
	r.mu.Unlock()
	if status != "" {
		status = u.T(status)
	}
	snap := r.host.Snapshot()
	var exclusion layout.Widget
	if snap.CanExclude {
		var targets []nativecapture.CaptureTarget
		for _, source := range sources {
			targets = append(targets, source.Target)
		}
		exclusion = func(gtx layout.Context) layout.Dimensions {
			r.pickExclude.VoiceOn = snap.Voice.Joined
			d, result := r.pickExclude.Layout(gtx, u, audioApps(targets), snap.Exclude, loading, status != "" && len(targets) == 0, false)
			if result.Changed {
				r.host.SetExcludeAudio(result.Target)
			}
			if result.Refresh {
				r.openPicker()
			}
			return d
		}
	}
	dims, action := r.picker.Layout(gtx, u, sources, loading, status, &r.border, &r.audio, exclusion)
	switch {
	case action.Close:
		r.mu.Lock()
		r.picking = false
		r.mu.Unlock()
	case action.Refresh:
		r.openPicker()
	case action.Chosen != nil:
		r.mu.Lock()
		r.picking = false
		audio, border := r.audio, r.border
		r.mu.Unlock()
		go updatePreferences(func(p *preferences) { p.PickNoSound, p.PickNoBorder = !audio, !border })
		r.host.Start(action.Chosen.Target, audio, border)
	}
	return dims
}

// screen is the TV picture: welcome and entry actions when idle, the shared
// source while sharing.
func (r *Room) screen(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	size := gtx.Constraints.Max
	switch snap.Phase {
	case phaseLive, phasePaused, phaseStarting:
		if snap.Phase != phaseStarting && r.previewPaused() {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return stateComic(gtx, u, "preview-paused", u.T("host.localPreviewPaused"))
			})
		}
		if snap.Preview != nil {
			picture, ok := r.pictures[snap.Preview]
			if !ok {
				picture = paint.NewImageOp(snap.Preview)
				picture.Filter = paint.FilterLinear
				r.pictures = map[image.Image]paint.ImageOp{snap.Preview: picture}
			}
			drawFit(gtx.Ops, picture, size, r.fit)
			if snap.Target.Kind == "display" {
				r.host.PreviewShown(size.X)
			}
		}
		if snap.Phase == phasePaused {
			fillRect(gtx.Ops, size, rgba(0, 0, 0, .55))
		}
		if snap.Phase == phaseStarting {
			staticNoise(gtx, size) // StaticNoise while the share starts
		}

		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			message, kind := "", ""
			switch {
			case snap.Phase == phaseStarting:
				message, kind = u.T("host.starting"), "source-starting"
			case snap.Switching:
				message, kind = u.T("host.switchingSource"), "source-switching"
			case snap.Phase == phasePaused:
				message, kind = u.T("host.paused"), "host-paused"
			}
			if message == "" {
				return layout.Dimensions{}
			}
			if snap.Phase == phasePaused {
				return stateComic(gtx, u, kind, message)
			}
			// Waiting overlays carry a playful line (WaitingCaption).
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return stateComic(gtx, u, kind, message) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return waitingCaptionLabel(gtx, u, rgba(0xd8, 0xe2, 0xf2, .7), 1)
				}))
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if snap.Notice != "" {
				return layout.Inset{Top: unit.Dp(24), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					kind := noticeComic(snap)
					if kind != "" {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return stateComic(gtx, u, kind, noticeText(u, snap.Notice))
						})
					}
					return u.label(gtx, noticeText(u, snap.Notice), textStyle{size: 14, weight: font.SemiBold, color: rgb(0xf0a39a), align: text.Middle})
				})
			}
			return layout.Inset{Top: unit.Dp(32), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return r.welcomeLine(gtx, u)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return r.start.Layout(gtx, u, "cast", u.T("host.start"), true, false)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(18)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Viewing stays in the browser: say so before the click.
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return r.join.Layout(gtx, u, "door", u.T("host.join"), false, r.onJoin == nil)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "globe", 13, withAlpha(tvInk, .7)) }),
										layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if u.Visual() {
												return layout.Dimensions{}
											}
											return u.label(gtx, u.T("native.join.browser"), textStyle{size: 11.5, color: withAlpha(tvInk, .75)})
										}))
								})
							}))
					}))
			})
		}))
}

func (r *Room) welcomeLine(gtx layout.Context, u *UI) layout.Dimensions {
	lines := welcome_en
	switch u.Lang {
	case "zh":
		lines = welcome_zh
	case "it":
		lines = welcome_it
	}
	line := lines[r.welcome%len(lines)]
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, "✦", textStyle{size: 16, weight: font.SemiBold, color: welcomeStar})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if u.Visual() {
					gtx := gtx
					return welcomeCipherInk(gtx, r.welcome, tvInk)
				}
				return u.label(gtx, line, textStyle{size: 16, weight: font.SemiBold, color: tvInk, lineH: 1.7})
			}))
	})
}

// toolbar is .lr-host-share-controls: one bar (paper, 1px edge, radius 20,
// padding 6) with flat buttons; while sharing: switch, pause, stop, a divider,
// then the sharing settings toggle.
func (r *Room) toolbar(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	p := &u.P
	sharing := snap.Phase == phaseLive || snap.Phase == phasePaused
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		var children []layout.FlexChild
		if snap.Phase == phaseStarting {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					r.cancelFlat.Comic = "hint-close"
					return r.cancelFlat.Layout(gtx, u, "x", u.T("host.cancelStart"), p.Danger, false)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
		}
		if sharing {
			pauseIcon, pauseKey := "pause", "host.pause"
			if snap.Phase == phasePaused {
				pauseIcon, pauseKey = "play", "host.resume"
			}
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					caption := u.T("host.switchSource")
					if snap.Switching {
						caption = u.T("host.switchingSource")
					}
					r.switchFlat.Comic = "hint-switch-source"
					return r.switchFlat.Layout(gtx, u, "switchSource", caption, p.Ink, snap.Switching)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					r.pauseFlat.Comic = map[bool]string{true: "hint-resume", false: "hint-pause"}[snap.Phase == phasePaused]
					return r.pauseFlat.Layout(gtx, u, pauseIcon, u.T(pauseKey), p.Ink, false)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					r.stopFlat.Comic = "hint-share-stop"
					return r.stopFlat.Layout(gtx, u, "stop", u.T("host.stop"), p.Danger, false)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(int(px(gtx, 1)), int(px(gtx, 28)))
					o := op.Offset(image.Pt(0, int(px(gtx, 8)))).Push(gtx.Ops)
					fillRect(gtx.Ops, size, p.ControlEdge)
					o.Pop()
					return layout.Dimensions{Size: image.Pt(size.X+int(px(gtx, 8)), int(px(gtx, 44)))}
				}),
			)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			r.settingsFlat.Comic = "hint-advanced"
			icon := "sliders"
			if snap.Applying {
				icon = "loader" // host.settings.button busy while applying
			}
			return r.settingsFlat.Layout(gtx, u, icon, u.T("host.settings.button"), p.Ink, r.settingsOpen)
		}))
		if sharing && snap.Remote.Available {
			if r.remote.toolbar.Clicked(gtx) {
				if r.detached.isDetached("remote") {
					r.detached.raise("remote")
				} else {
					r.remote.open = !r.remote.open
				}
			}
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				pending, controller := 0, ""
				for _, request := range snap.Remote.Requests {
					if request.State == "pending" {
						pending++
					} else if controller == "" {
						controller = request.State
					}
				}
				// The trigger's hint follows the state (remote.requests / hostFocus / controlling).
				switch {
				case pending > 0:
					r.remote.toolbar.Tip = u.T("remote.requests")
				case controller == "suspended":
					r.remote.toolbar.Tip = u.T("remote.hostFocus")
				case controller != "":
					r.remote.toolbar.Tip = u.T("remote.controlling")
				default:
					r.remote.toolbar.Tip = u.T("remote.title")
				}
				r.remote.toolbar.Comic = "hint-remote-control"
				on := snap.Remote.Mode != "" && snap.Remote.Mode != "off"
				d := r.remote.toolbar.Layout(gtx, u, "mouse", u.T("remote.title"), p.Ink, r.remote.open || on)
				if pending > 0 {
					countBadge(gtx, u, d.Size, pending)
				}
				return d
			}))
		}
		content := layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
		call := macro.Stop()
		pad := int(px(gtx, 7))
		size := image.Pt(content.Size.X+2*pad, content.Size.Y+2*pad)
		shape := boxOf(size, px(gtx, 20))
		shadow(gtx.Ops, shape, px(gtx, 3), 0, withAlpha(p.SurfaceRim, .6))
		fillBox(gtx.Ops, shape, p.Paper)
		glaze(gtx.Ops, p, shape)
		strokeBox(gtx.Ops, shape, px(gtx, 1), p.ControlEdge)
		o := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		o.Pop()
		return layout.Dimensions{Size: size}
	})
}

// deck is .lr-deck: the summary row (room code, name, connection buttons),
// then the access row and the details/topology rows when open; gap 14.
func (r *Room) deck(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	rows := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions { return r.summaryRow(gtx, u, snap) })}
	add := func(show bool, w layout.Widget) {
		if show {
			rows = append(rows, layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout), layout.Rigid(w))
		}
	}
	add(snap.Code != "" && r.accessOpen, func(gtx layout.Context) layout.Dimensions { return r.accessRow(gtx, u, snap) })
	add(r.detailsOpen, func(gtx layout.Context) layout.Dimensions { return r.detailsRows(gtx, u, snap) })
	add(r.topologyOpen, func(gtx layout.Context) layout.Dimensions { return r.topologyRow(gtx, u, snap) })
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func (r *Room) summaryRow(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	p := &u.P
	own := hostColor
	if snap.HostID != "" {
		own = participantColor(snap.HostID)
	}
	identity := func(gtx layout.Context) layout.Dimensions {
		if r.name.editing {
			return r.name.Layout(gtx, u, displayName(u, snap.Name), r.host.SetName)
		}
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return nameTag(gtx, u, displayName(u, snap.Name), own) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.edit.Layout(gtx, u, ButtonStyle{Comic: "hint-rename", Icon: "pencil", Caption: u.T("common.edit"), Tip: u.T("host.nameEdit")})
			}))
	}
	connection := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.details.Layout(gtx, u, ButtonStyle{Comic: "hint-details", Icon: "gauge", Caption: u.T("host.details"), Pressed: r.detailsOpen,
					Disabled: snap.Code == ""})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.topology.Layout(gtx, u, ButtonStyle{Comic: "hint-topology", Icon: "branch", Caption: u.T("host.topology"), Pressed: r.topologyOpen})
			}))
	}
	return deckRow(gtx, u, false, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		firstRow := func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, u.T("common.roomCode"), textStyle{size: 12, weight: font.SemiBold, color: p.Ink})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(9)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.lcd.Layout(gtx, u, snap.Code, snap.Busy)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if snap.Code == "" {
						return layout.Dimensions{}
					}
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							armed := !r.replaceArmed.IsZero() && gtx.Now.Sub(r.replaceArmed) < 5*time.Second
							style := ButtonStyle{Icon: "refresh", Caption: u.T("host.roomReplace"), Size: sizeCompact}
							if armed {
								style.Tone, style.Icon = toneDanger, "alert"
							}
							style.Tip = u.T("host.roomReplaceConfirm")
							return r.replaceBtn.Layout(gtx, u, style)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							key, icon := "host.policy.currentOpen", "globe"
							switch {
							case snap.Policy == "private" && snap.PasswordOn:
								key, icon = "host.policy.currentPassword", "key"
							case snap.Policy == "private":
								key, icon = "host.policy.currentInvite", "lock"
							}
							hint := "host.policy.openHint"
							switch {
							case snap.Policy == "private" && snap.PasswordOn:
								hint = "host.policy.privatePasswordHint"
							case snap.Policy == "private":
								hint = "host.policy.privateHint"
							}
							return r.entryBtn.Layout(gtx, u, ButtonStyle{Icon: icon, Caption: u.T(key), Size: sizeCompact, Tip: u.T(hint)})
						}))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if r.copied.IsZero() || gtx.Now.Sub(r.copied) > 2*time.Second {
						return layout.Dimensions{}
					}
					gtx.Execute(op.InvalidateCmd{At: r.copied.Add(2*time.Second + 10*time.Millisecond)})
					return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, u.T("common.copied"), textStyle{size: 12, weight: font.SemiBold, color: p.Live})
					})
				}),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(identity),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if snap.Code != "" {
						return layout.Dimensions{}
					}
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(connection))
				}),
			)
		}
		if snap.Code == "" {
			return firstRow(gtx)
		}
		// With a room the connection buttons wrap to a second line, right-aligned.
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(firstRow),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Flex{Spacing: layout.SpaceStart, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							size := image.Pt(int(px(gtx, 1)), int(px(gtx, 28)))
							fillRect(gtx.Ops, size, p.ControlEdge)
							return layout.Dimensions{Size: image.Pt(size.X+int(px(gtx, 8)), size.Y)}
						}),
						layout.Rigid(connection))
				})
			}))
	})
}

// noticeText translates a notice key; other text is shown as it is.
func noticeText(u *UI, notice string) string {
	if value := u.T(notice); value != "" {
		return value
	}
	return notice
}

// clipboardWrite copies s.
func clipboardWrite(s string) clipboard.WriteCmd {
	return clipboard.WriteCmd{Type: "application/text", Data: ioNopCloser(s)}
}

func ioNopCloser(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

var _ = f32.Point{}

// autoOpenRemote opens the remote control panel when a new request waits.
func (r *Room) autoOpenRemote(snap roomSnapshot) {
	if r.remote.seen == nil {
		r.remote.seen = map[string]bool{}
	}
	for _, request := range snap.Remote.Requests {
		if request.State == "pending" && !r.remote.seen[request.ID] {
			r.remote.seen[request.ID] = true
			r.remote.open = true
		}
	}
}

// liveNotice shows the latest notice under the TV while a share runs (the
// idle TV shows notices itself). It fades after 8 s.
func (r *Room) liveNotice(gtx layout.Context, u *UI, snap roomSnapshot) layout.Dimensions {
	p := &u.P
	armed := !r.replaceArmed.IsZero() && gtx.Now.Sub(r.replaceArmed) < 5*time.Second
	text, tone := "", p.Warn
	switch {
	case armed:
		text, tone = u.T("host.roomReplaceConfirm"), p.Danger
	case r.viewersReconnecting(snap):
		text = u.T("common.viewers") + ": " + u.T("host.notice.reconnecting")
	case snap.Phase != phaseIdle && snap.Notice != "" && gtx.Now.Sub(snap.NoticeAt) < 8*time.Second && !r.noticeHidden.Equal(snap.NoticeAt):
		text = noticeText(u, snap.Notice)
		gtx.Execute(op.InvalidateCmd{At: snap.NoticeAt.Add(8*time.Second + 10*time.Millisecond)})
	}
	if text == "" {
		return layout.Dimensions{}
	}
	return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{}
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(720))
			macro := op.Record(gtx.Ops)
			content := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return glyph(gtx, "alert", 16, tone) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X -= gtx.Dp(100)
					return u.label(gtx, text, textStyle{size: 12.5, weight: font.SemiBold, color: p.Ink, maxLines: 2})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if armed {
						return layout.Dimensions{}
					}
					if r.noticeClose.Clicked(gtx) {
						r.noticeHidden = snap.NoticeAt
					}
					return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return r.noticeClose.Layout(gtx, u, ButtonStyle{Icon: "x", Size: sizeDense, Tone: toneQuiet, Tip: u.T("notice.dismiss"), Comic: "hint-close"})
					})
				}))
			call := macro.Stop()
			pad := gtx.Dp(14)
			size := image.Pt(content.Size.X+2*pad, content.Size.Y+gtx.Dp(16))
			shape := boxOf(size, float32(min(size.Y/2, gtx.Dp(18))))
			fillBox(gtx.Ops, shape, p.Paper)
			strokeBox(gtx.Ops, shape, px(gtx, 1.5), tone)
			o := op.Offset(image.Pt(pad, gtx.Dp(8))).Push(gtx.Ops)
			call.Add(gtx.Ops)
			o.Pop()
			return layout.Dimensions{Size: size}
		})
	})
}

// windowTitle is the page title: VibePiik | code · state.
func (r *Room) windowTitle(u *UI, snap roomSnapshot) string {
	title := "VibePiik"
	if snap.Code != "" {
		title += " | " + snap.Code
	}
	frame := "hostIdle"
	switch snap.Phase {
	case phaseLive:
		title += " · " + u.T("host.live")
		frame = "hostActive"
	case phasePaused:
		title += " · " + u.T("host.paused")
		frame = ""
	case phaseStarting:
		title += " · " + u.T("host.starting")
		frame = "hostStarting"
	default:
		if snap.Code != "" {
			title += " · " + u.T("host.roomReady")
			frame = "hostReady"
		}
	}
	// Title frames: a playful variation after the state (useDocumentTitle).
	if decoration := u.titleDecoration(frame, snap.Code); decoration != "" {
		title += " · " + decoration
	}
	return title
}

// escape closes the topmost open layer (Escape key, KB-3).
func (r *Room) escape() {
	r.mu.Lock()
	picking := r.picking
	r.picking = false
	r.mu.Unlock()
	switch {
	case picking:
	case r.person.target != "":
		r.person.target = ""
	case r.helpPanel.open:
		r.helpPanel.open = false
	case r.remote.open:
		r.remote.open, r.remote.preparing, r.remote.draft = false, false, ""
		r.remote.password.Editor.SetText("")
	case r.interact.picking:
		r.interact.picking = false
	case r.interact.open:
		r.interact.open = false
	case r.selectedViewer != "":
		r.selectedViewer = ""
	case r.settingsOpen:
		r.settingsOpen = false
	}
	r.invalidate()
}

// viewersReconnecting reports a viewer that had the picture and lost it.
func (r *Room) viewersReconnecting(snap roomSnapshot) bool {
	if r.wasReady == nil {
		r.wasReady = map[string]bool{}
	}
	reconnecting := false
	present := map[string]bool{}
	for _, viewer := range snap.Viewers {
		present[viewer.ID] = true
		if viewer.MediaReady {
			r.wasReady[viewer.ID] = true
		} else if r.wasReady[viewer.ID] && snap.Phase == phaseLive {
			reconnecting = true
		}
	}
	for id := range r.wasReady {
		if !present[id] {
			delete(r.wasReady, id)
		}
	}
	return reconnecting
}
