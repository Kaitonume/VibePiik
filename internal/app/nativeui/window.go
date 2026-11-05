package nativeui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/TNTcraftHIM/Piik/internal/app/appwindow"
)

// WebWindow is the WebView2 window (appwindow.Manager) that still shows the
// pages not yet drawn natively.
type WebWindow interface {
	Available() bool
	Open(string) error
	Close()
}

// Manager shows App pages: the start page and the host room natively, other
// pages in the web window. It has the console's window interface (Available,
// Open, Close).
type Manager struct {
	mu          sync.Mutex
	web         WebWindow
	onUserClose func()
	native      *nativeWindow
	host        *HostEnv
}

func NewManager(web WebWindow, onUserClose func()) *Manager {
	return &Manager{web: web, onUserClose: onUserClose}
}

// SetHost enables the native host room with the App's capture control.
func (m *Manager) SetHost(env HostEnv) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.host = &env
}

func (m *Manager) Available() bool { return true }

// Open shows target: the launcher and the App's own host room natively,
// anything else in the web window.
func (m *Manager) Open(target string) error {
	m.mu.Lock()
	nativeRoom := m.host != nil && isAppRoomPage(target)
	m.mu.Unlock()
	switch {
	case isLauncherPage(target):
		if m.web != nil {
			m.web.Close()
		}
		return m.openNative(target, "")
	case nativeRoom:
		if m.web != nil {
			m.web.Close()
		}
		return m.openNative("", target)
	}
	m.closeNative()
	if m.web == nil || !m.web.Available() {
		return appwindow.ErrUnavailable
	}
	return m.web.Open(target)
}

// Close closes every VibePiik window without quitting the App.
func (m *Manager) Close() {
	m.closeNative()
	if m.web != nil {
		m.web.Close()
	}
}

func isLauncherPage(target string) bool {
	parsed, err := url.Parse(target)
	return err == nil && parsed.Path == "/client"
}

// isAppRoomPage is the App's own host entry on this computer.
func isAppRoomPage(target string) bool {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "http" || (parsed.Path != "" && parsed.Path != "/") {
		return false
	}
	host := parsed.Hostname()
	return (host == "127.0.0.1" || host == "localhost") && parsed.Query().Get("piik-client-port") != ""
}

type nativeWindow struct {
	window *app.Window
	quiet  bool
	done   chan struct{}
	room   string // host room entry to show next

	sharing    func() bool // a share is running in the room (ConfirmBrowser)
	askBrowser chan bool   // question for the room, picked up by the next frame
}

func (m *Manager) openNative(launcherURL, roomURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.native != nil {
		if roomURL != "" {
			m.native.room = roomURL
			m.native.window.Invalidate()
		}
		m.native.window.Perform(system.ActionRaise)
		return nil
	}
	native := &nativeWindow{window: new(app.Window), done: make(chan struct{}), room: roomURL}
	m.native = native
	host := m.host
	go m.run(native, launcherURL, host)
	return nil
}

func (m *Manager) closeNative() {
	m.mu.Lock()
	native := m.native
	if native != nil {
		native.quiet = true
	}
	m.mu.Unlock()
	if native == nil {
		return
	}
	native.window.Perform(system.ActionClose)
	select {
	case <-native.done:
	case <-time.After(3 * time.Second):
	}
}

// preferences are the native UI's own copy of the web client's theme and
// language choices (localStorage piik:ui-theme / piik:ui-lang).
type preferences struct {
	Theme    string `json:"theme,omitempty"`          // "light", "dark" or "" (follow Windows)
	Language string `json:"language,omitempty"`       // "en", "zh", "it", "vis" or "" (follow Windows)
	Spoken   string `json:"spokenLanguage,omitempty"` // language kept under ✦ ("" = Windows)
	RoomCode string `json:"roomCode,omitempty"`       // last room code, asked for again (piik:host-room-preference)
	Quality  string `json:"quality,omitempty"`        // quality preset of the sharing settings
	Name     string `json:"name,omitempty"`           // host display name (piik:display-name)

	RemoteMode         string  `json:"remoteControlMode,omitempty"`    // last remote-control permission (ask/room/password)
	RemoteStopKey      int     `json:"remoteStopKey,omitempty"`        // host stop shortcut Ctrl+Alt+F<key>
	VoiceVolume        int     `json:"voiceListeningVolume,omitempty"` // master voice volume in % (0 = 100)
	MicVolume          int     `json:"microphoneVolume,omitempty"`     // room voice microphone volume in % (0 = 100, -1 = 0)
	VoiceDevice        string  `json:"voiceInput,omitempty"`           // room voice microphone (Windows endpoint ID)
	CommentDevice      string  `json:"commentaryInput,omitempty"`      // commentary microphone mixed into the share
	CommentVolume      int     `json:"commentaryVolume,omitempty"`     // commentary volume in % (0 = 100, -1 = 0)
	VoiceRaw           bool    `json:"voiceProcessingOff,omitempty"`   // noise & echo reduction turned off
	VoiceGate          float64 `json:"voiceNoiseGate,omitempty"`       // noise gate, meter position 0–1 (0 = off)
	ChatOverlay        bool    `json:"chatOverlay,omitempty"`          // on-screen chat over the TV picture
	ChatScale          float64 `json:"chatScale,omitempty"`
	ChatOpacity        float64 `json:"chatOpacity,omitempty"`
	ChatSpeed          float64 `json:"chatSpeed,omitempty"`
	ChatColors         bool    `json:"chatColorBySender,omitempty"`
	DeskOverlay        bool    `json:"chatOverOtherApps,omitempty"` // chat window above games and other apps
	DeskMonitor        int     `json:"chatOverlayScreen,omitempty"`
	DeskCorner         string  `json:"chatOverlayCorner,omitempty"`
	DeskSeconds        int     `json:"chatOverlaySeconds,omitempty"`
	PickNoSound        bool    `json:"pickerSoundOff,omitempty"`  // source picker: share sound off
	PickNoBorder       bool    `json:"pickerBorderOff,omitempty"` // source picker: capture border off
	PreviewFit         string  `json:"pictureFit,omitempty"`      // contain, cover or fill
	PreviewNoAutoPause bool    `json:"previewAutoPauseOff,omitempty"`
	PreviewSharp       bool    `json:"previewSharp,omitempty"` // live full-detail screen preview
	PreviewFPS         int     `json:"previewFps,omitempty"`   // sharp preview rate: 30 or 60
}

func preferencesPath() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "VibePiik", "ui.json")
}

func loadPreferences() preferences {
	var value preferences
	if data, err := os.ReadFile(preferencesPath()); err == nil {
		_ = json.Unmarshal(data, &value)
	}
	return value
}

func savePreferences(value preferences) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(preferencesPath()), 0o700)
	_ = os.WriteFile(preferencesPath(), data, 0o600)
}

var preferencesMu sync.Mutex

// updatePreferences changes the saved preferences under a lock.
func updatePreferences(change func(*preferences)) {
	preferencesMu.Lock()
	defer preferencesMu.Unlock()
	value := loadPreferences()
	change(&value)
	savePreferences(value)
}

func (m *Manager) run(native *nativeWindow, launcherURL string, hostEnv *HostEnv) {
	defer close(native.done)
	window := native.window
	window.Option(app.Title("VibePiik"), app.Size(unit.Dp(1200), unit.Dp(windowHeightDp)), app.MinSize(unit.Dp(720), unit.Dp(560)))
	prefs := loadPreferences()
	dark := systemDark()
	switch prefs.Theme {
	case "dark":
		dark = true
	case "light":
		dark = false
	}
	language := prefs.Language
	if language == "" {
		language = systemLanguage()
	}
	spoken := prefs.Spoken
	if spoken == "" {
		spoken = systemLanguage()
	}
	if language != "vis" {
		spoken = language
	}
	ui := &UI{Lang: language, Spoken: spoken}
	ui.Shaper = newShaper(ui.textLang() == "zh")
	setPalette := func() {
		if dark {
			ui.P = DarkPalette()
		} else {
			ui.P = LightPalette()
		}
	}
	setPalette()
	var hwnd uintptr
	focused := true
	var frameMu sync.Mutex // the main window and detached panels draw one at a time
	var current *Room
	toggleTheme := func() {
		dark = !dark
		setPalette()
		theme := map[bool]string{true: "dark", false: "light"}[dark]
		updatePreferences(func(p *preferences) { p.Theme = theme })
		if hwnd != 0 {
			appwindow.SetDarkTitleBar(hwnd, dark)
		}
	}
	setLanguage := func(next string) {
		before := ui.textLang()
		ui.Lang = next
		if next != "vis" {
			ui.Spoken = next // ✦ keeps the last spoken language
		}
		if (ui.textLang() == "zh") != (before == "zh") {
			ui.Shaper = newShaper(ui.textLang() == "zh")
		}
		spoken := ui.Spoken
		updatePreferences(func(p *preferences) { p.Language, p.Spoken = next, spoken })
	}

	var launcher *Launcher
	if launcherURL != "" {
		launcher = newLauncher(newLauncherClient(launcherURL), window.Invalidate)
		launcher.onTarget = func(target string) { go func() { _ = m.Open(target) }() }
		launcher.onTheme = toggleTheme
		launcher.onLanguage = setLanguage
		if hostEnv != nil && hostEnv.OpenExternal != nil {
			open := hostEnv.OpenExternal
			launcher.onExternal = func(url string) { _ = open(url) }
		}
	}
	var room *Room
	var host *hostSession
	defer func() {
		if host != nil {
			host.Close()
		}
	}()
	openRoom := func(target string) {
		parsed, err := parseRoomTarget(target)
		if err != nil || hostEnv == nil {
			return
		}
		if host != nil {
			host.Close()
		}
		host = newHostSession(parsed, *hostEnv, func() {
			if current != nil {
				current.detached.invalidateAll()
			} else {
				window.Invalidate()
			}
		})
		if room != nil && room.preview != nil {
			room.preview.Hide()
		}
		if room != nil && room.chat.window != nil {
			room.chat.window.Close()
		}
		room = newRoom(host, window.Invalidate)
		room.invalidate = func() { room.detached.invalidateAll() }
		room.detached.frame, room.detached.ui, room.detached.main, room.detached.mainWnd = &frameMu, ui, window, hwnd
		current = room
		room.preview = newLivePreview(hwnd)
		room.chat.window = newChatOverlay(hwnd)
		room.onFullscreen = func(on bool) {
			if on {
				window.Option(app.Fullscreen.Option())
			} else {
				window.Option(app.Windowed.Option())
			}
		}
		room.onTheme = toggleTheme
		room.onLanguage = setLanguage
		if hostEnv.OpenExternal != nil {
			open := hostEnv.OpenExternal
			room.onHelp = func() {
				url := piikGuideURL
				if ui.textLang() == "zh" {
					url = strings.Replace(url, "troubleshooting.md", "troubleshooting.zh-CN.md", 1)
				}
				go func() { _ = open(url) }()
			}
			room.onExternal = func(url string) { go func() { _ = open(url) }() }
		}
		// Joining another room opens the viewer page in the browser: the native
		// room keeps hosting.
		joinURL := parsed.Origin + "/join"
		if hostEnv.OpenExternal != nil {
			open := hostEnv.OpenExternal
			room.onJoin = func() {
				// The page takes the window's language, ✦ mode and theme
				// (clientLaunchURL's piik-lang / piik-mode / piik-theme).
				lang, mode := ui.Lang, "text"
				if lang == "vis" {
					lang, mode = ui.textLang(), "vis"
				}
				if lang != "zh" {
					lang = "en" // the web page has English and Chinese
				}
				theme := map[bool]string{true: "dark", false: "light"}[dark]
				target := joinURL + "?piik-preferences=copy,theme#piik-client=1&piik-lang=" + lang + "&piik-mode=" + mode + "&piik-theme=" + theme
				go func() { _ = open(target) }()
			}
		}
	}
	testBrowserDone := false
	native.sharing = func() bool {
		frameMu.Lock()
		defer frameMu.Unlock()
		if current == nil {
			return false
		}
		phase := current.host.Snapshot().Phase
		return phase == phaseLive || phase == phasePaused || phase == phaseStarting
	}
	var ops op.Ops
	title := "VibePiik"
	for {
		switch event := window.Event().(type) {
		case app.Win32ViewEvent:
			if event.HWND != 0 && hwnd == 0 {
				hwnd = event.HWND
				if room != nil {
					room.detached.mainWnd = hwnd
				}
				appwindow.SetDarkTitleBar(hwnd, dark)
				if room != nil {
					room.preview = newLivePreview(hwnd)
					room.chat.window = newChatOverlay(hwnd)
				}
				// Gio reports the handle after sizing the window: back where the
				// last VibePiik window was.
				go func(hwnd uintptr) {
					time.Sleep(30 * time.Millisecond)
					appwindow.RestoreWindowPlacement(hwnd, windowHeightDp)
				}(hwnd)
			} else if event.HWND == 0 && hwnd != 0 {
				appwindow.SaveWindowPlacement(hwnd)
				hwnd = 0
				if room != nil && room.preview != nil {
					room.preview.Hide()
					room.preview = nil
				}
				if room != nil && room.chat.window != nil {
					room.chat.window.Close()
					room.chat.window = nil
				}
			}
		case app.ConfigEvent:
			if room != nil && room.focused != event.Config.Focused {
				room.focused = event.Config.Focused
				window.Invalidate()
			}
			if room == nil && launcher != nil && event.Config.Focused && !focused {
				go launcher.reload() // back to the window: refresh the App state
			}
			focused = event.Config.Focused
		case app.DestroyEvent:
			if room != nil {
				room.detached.closeAll()
			}
			if host != nil {
				// The window is gone: leave the room (a page in the browser
				// takes over), never keep sharing with no window.
				go host.Close()
			}
			m.mu.Lock()
			quiet := native.quiet
			if m.native == native {
				m.native = nil
			}
			m.mu.Unlock()
			if !quiet && m.onUserClose != nil {
				m.onUserClose()
			}
			return
		case app.FrameEvent:
			m.mu.Lock()
			next := native.room
			native.room = ""
			ask := native.askBrowser
			native.askBrowser = nil
			m.mu.Unlock()
			if os.Getenv("VIBEPIIK_TEST_BROWSER") == "yes" && !testBrowserDone && current != nil && current.host.Snapshot().Phase == phaseLive {
				// Automated tests only: the console's [o] while sharing (no browser opens).
				testBrowserDone = true
				go func() {
					time.Sleep(3 * time.Second)
					fmt.Println("test-browser: asking")
					ok := m.ConfirmBrowser()
					fmt.Println("test-browser: answer", ok)
					if ok {
						m.Close()
						fmt.Println("test-browser: window closed")
					}
				}()
			}
			if ask != nil {
				if current != nil {
					current.askBrowser(ask)
				} else {
					ask <- true
				}
			}
			if next != "" {
				openRoom(next)
			}
			gtx := app.NewContext(&ops, event)
			beginFrame(hwnd, gtx.Constraints.Max)
			frameMu.Lock()
			switch {
			case room != nil:
				room.Layout(gtx, ui)
				if next := room.windowTitle(ui, room.host.Snapshot()); next != title {
					title = next
					window.Option(app.Title(title))
				}
			case launcher != nil:
				launcher.Layout(gtx, ui)
			}
			frameMu.Unlock()
			event.Frame(gtx.Ops)
		}
	}
}

// windowHeightDp is the window's starting height (taller than the first
// builds' 880: the TV, toolbar and couch fit without scrolling).
const windowHeightDp = 1000

const piikGuideURL = "https://github.com/TNTcraftHIM/Piik/blob/main/docs/guide/troubleshooting.md"
