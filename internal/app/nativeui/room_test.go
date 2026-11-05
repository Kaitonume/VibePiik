package nativeui

import (
	"context"
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"gioui.org/layout"

	"github.com/TNTcraftHIM/Piik/internal/app/nativecapture"
)

type fakeHost struct {
	mu      sync.Mutex
	snap    roomSnapshot
	started []nativecapture.CaptureTarget
	paused  []bool
	stopped int
	created int
}

func (h *fakeHost) Snapshot() roomSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.snap
}
func (h *fakeHost) Sources(context.Context) ([]nativecapture.CaptureTarget, error) {
	return []nativecapture.CaptureTarget{
		{Kind: "window", SourceID: "1", PID: 11, Title: "Sketchbook"},
		{Kind: "window", SourceID: "2", PID: 12, Title: "A short film with a rather long window title that wraps"},
		{Kind: "window", SourceID: "3", PID: 13, Title: "Voice call"},
		{Kind: "display", SourceID: "4", Title: "Screen 1"},
	}, nil
}
func (h *fakeHost) SourcePreview(_ context.Context, target nativecapture.CaptureTarget) (image.Image, error) {
	return testPicture(320, 180, color.NRGBA{R: 0xe2, G: 0xc4, B: 0xa5, A: 255}), nil
}
func (h *fakeHost) Start(target nativecapture.CaptureTarget, audio, border bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.started = append(h.started, target)
}
func (h *fakeHost) SetPaused(paused bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.paused = append(h.paused, paused)
}
func (h *fakeHost) Stop()       { h.mu.Lock(); h.stopped++; h.mu.Unlock() }
func (h *fakeHost) CreateRoom() { h.mu.Lock(); h.created++; h.mu.Unlock() }
func (h *fakeHost) SendTV(context.Context) ([]string, int, error) {
	return []string{"TV"}, 200, nil
}
func (h *fakeHost) Close()                      {}
func (h *fakeHost) SetSettings(s shareSettings) { h.mu.Lock(); h.snap.Settings = s; h.mu.Unlock() }
func (h *fakeHost) SetName(name string) error {
	h.mu.Lock()
	h.snap.Name = name
	h.mu.Unlock()
	return nil
}
func (h *fakeHost) SetPolicy(policy string) { h.mu.Lock(); h.snap.Policy = policy; h.mu.Unlock() }
func (h *fakeHost) RotateInvite()           {}
func (h *fakeHost) RevokeInvite()           {}
func (h *fakeHost) ReplaceRoom()            {}

func testPicture(w, h int, c color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := c
			if (x/40+y/40)%2 == 0 {
				v = color.NRGBA{R: 0x3a, G: 0x8f, B: 0x6e, A: 255}
			}
			img.SetNRGBA(x, y, v)
		}
	}
	return img
}

func TestRoomShots(t *testing.T) {
	for _, palette := range []Palette{DarkPalette(), LightPalette()} {
		theme := map[bool]string{true: "dark", false: "light"}[palette.Dark]
		ui := &UI{P: palette, Shaper: newShaper(false), Lang: "en"}
		host := &fakeHost{snap: roomSnapshot{Signal: "offline", Phase: phaseIdle}}
		room := newRoom(host, func() {})
		room.welcome = 4
		renderPNG(t, "room-idle-"+theme, image.Pt(1185, 1057), func(gtx layout.Context) { room.Layout(gtx, ui) })
		renderPNG(t, "room-scene-"+theme, image.Pt(1100, 1000), func(gtx layout.Context) { gtx.Constraints.Min = image.Point{}; room.scene(gtx, ui, host.snap) })

		host.snap = roomSnapshot{Signal: "connected", Phase: phaseLive, Code: "9527", Invite: "http://192.168.0.5:8787/r/9527#v=x",
			Preview: testPicture(1920, 1080, color.NRGBA{R: 0x8f, G: 0xb8, B: 0x7d, A: 255}),
			Viewers: []roomPeer{{ID: "viewer-aaaaaaaa", Name: "Leeroy"}, {ID: "viewer-bbbbbbbb", Name: "PIIK TV"}}}
		renderPNG(t, "room-live-"+theme, image.Pt(1185, 1057), func(gtx layout.Context) { room.Layout(gtx, ui) })

		host.snap.HostID = "host-peer-1"
		host.snap.Settings = shareSettings{Quality: qualityPresets["1080p60"], Codec: "h264", Topology: true}
		host.snap.Canonical = "http://192.168.0.5:8787/r/9527"
		host.snap.Policy = "open"
		host.snap.CodecInUse = "h264"
		host.snap.Viewers[0].MediaReady, host.snap.Viewers[1].MediaReady = true, true
		host.snap.Viewers[1].Upstream = "viewer-aaaaaaaa"
		host.snap.Stats = map[string]viewerStats{"viewer-aaaaaaaa": {Width: 1920, Height: 1080, FPS: 58.7, Kbps: 4210, HasRTT: true, RTTms: 4}}
		room.settingsOpen, room.panel.fine.Open, room.panel.conn.Open = true, true, true
		room.detailsOpen, room.topologyOpen = true, true
		renderPNG(t, "room-panels-"+theme, image.Pt(1185, 2300), func(gtx layout.Context) { room.Layout(gtx, ui) })
		room.panel.sound = true
		room.settingsOpen, room.detailsOpen, room.topologyOpen = true, false, false
		renderPNG(t, "room-sound-"+theme, image.Pt(1185, 1500), func(gtx layout.Context) { room.Layout(gtx, ui) })
		room.settingsOpen = false
		host.snap.InteractionsReady = true
		host.snap.Chat = []chatMessage{{ID: "m1", PeerID: "viewer-aaaaaaaa", Name: "Leeroy", Text: "Ciao! Si vede benissimo sulla TV"},
			{ID: "m2", PeerID: "host-peer-1", Name: "Host", Text: "Perfetto, parto con la partita", Self: true, Host: true},
			{ID: "m3", PeerID: "viewer-aaaaaaaa", Name: "Leeroy", Text: "Vai!"}}
		host.snap.Reactions = []activeReaction{{ID: "r1", From: "viewer-aaaaaaaa", Reaction: "heart", Until: time.Now().Add(1500 * time.Millisecond)}}
		host.snap.Remote = remoteSnapshot{Available: true, Mode: "ask", Shortcut: 10, ShortcutOK: true,
			Requests: []remoteRequest{{ID: "q1", PeerID: "viewer-bbbbbbbb", State: "pending"}, {ID: "q2", PeerID: "viewer-aaaaaaaa", State: "active"}}}
		room.interact.open, room.interact.picking, room.remote.open = true, true, true
		renderPNG(t, "room-interact-"+theme, image.Pt(1185, 1100), func(gtx layout.Context) { room.Layout(gtx, ui) })
		host.snap.Voice = voiceSnapshot{Enabled: true, Joined: true, Microphone: true, Members: []voiceMemberView{
			{PeerID: "host-peer-1", Joined: true, Publishing: true, Speaking: true},
			{PeerID: "viewer-aaaaaaaa", Joined: true, Publishing: true},
			{PeerID: "viewer-bbbbbbbb", Joined: true, Deafened: true}}}
		room.interact.voiceTab, room.remote.open = true, false
		renderPNG(t, "room-voice-"+theme, image.Pt(1185, 1100), func(gtx layout.Context) { room.Layout(gtx, ui) })
		room.interact.voiceTab = false
		host.snap.FailureCode, host.snap.Notice, host.snap.NoticeAt = "app/start-share/rejected", "native.fail.captureStart", time.Now()
		host.snap.ShareVolumeOK, host.snap.ShareVolume, host.snap.Audio = true, 1.2, true
		host.snap.Policy, host.snap.PasswordOn = "private", true
		room.interact.open, room.helpPanel.open, room.settingsOpen, room.panel.sound = false, true, true, true
		renderPNG(t, "room-help-"+theme, image.Pt(1185, 1700), func(gtx layout.Context) { room.Layout(gtx, ui) })
		room.helpPanel.open, room.settingsOpen, room.panel.sound = false, false, false
		host.snap.Notice, host.snap.FailureCode = "", ""
		room.interact.open, room.remote.open = false, false

		// Batch 2: typed quality values, per-viewer card, consent gate, voice settings.
		host.snap.Settings.Quality.Resolution.Width, host.snap.Settings.Quality.Resolution.Height = 2560, 1440
		host.snap.Settings.Quality.MaxFramerate, host.snap.Settings.Quality.MaxBitrate = 72, 25_000_000
		stats := host.snap.Stats["viewer-aaaaaaaa"]
		jitter, dropped, codec := 3.4, 2.0, "video/H264"
		stats.Detail = evidenceDetail{JitterMs: &jitter, FramesDroppedDelta: &dropped, Codec: &codec}.lines()
		host.snap.Stats["viewer-aaaaaaaa"] = stats
		host.snap.Remote = remoteSnapshot{Available: true, Mode: "off", Shortcut: 10, ShortcutOK: true}
		room.settingsOpen, room.panel.sound, room.panel.fine.Open = true, false, true
		room.detailsOpen, room.selectedViewer, room.card.more.Open = true, "viewer-aaaaaaaa", true
		room.remote.open, room.remote.preparing, room.remote.draft = true, true, "room"
		renderPNG(t, "room-batch2-"+theme, image.Pt(1185, 2300), func(gtx layout.Context) { room.Layout(gtx, ui) })
		room.settingsOpen, room.detailsOpen, room.remote.open, room.remote.preparing = false, false, false, false
		host.snap.Voice.Volume, host.snap.Voice.Bitrate = 1.3, 64000
		host.snap.Voice.Members[1].Volume = 0.6
		room.interact.open, room.interact.voiceTab, room.interact.voiceSettings.Open = true, true, true
		room.interact.voiceOpenRow = "viewer-aaaaaaaa"
		renderPNG(t, "room-voiceset-"+theme, image.Pt(1185, 1300), func(gtx layout.Context) { room.Layout(gtx, ui) })
		room.interact.open, room.interact.voiceTab = false, false

		// Batch 3: person menu over the chat, commentary microphone choice.
		room.interact.open, room.person.target = true, "viewer-aaaaaaaa"
		room.settingsOpen, room.panel.sound = true, true
		host.snap.Settings.MicAvail, host.snap.Settings.Microphone = true, true
		host.snap.Settings.MicDevice, host.snap.Settings.MicVolume = "m1", 1.4
		room.micList = deviceList{devices: []audioDevice{{ID: "m1", Label: "Headset Microphone (USB)"}, {ID: "m2", Label: "Webcam"}}, loaded: true}
		renderPNG(t, "room-person-"+theme, image.Pt(1185, 1500), func(gtx layout.Context) { room.Layout(gtx, ui) })
		room.interact.open, room.person.target, room.settingsOpen, room.panel.sound = false, "", false, false

		// Chat settings: on-screen chat and the chat over other apps.
		room.interact.open, room.chat.settings, room.chat.on = true, true, true
		room.chat.desk = desktopOverlay{On: true, Monitor: 1, Corner: "top-right", Seconds: 15}
		room.chat.screenList = []screenChoice{{Width: 2560, Height: 1440, Primary: true}, {Width: 2560, Height: 1440}}
		room.chat.screensAt = time.Now().Add(time.Hour)
		renderPNG(t, "room-chatsettings-"+theme, image.Pt(1185, 1300), func(gtx layout.Context) { room.Layout(gtx, ui) })
		room.interact.open, room.chat.settings, room.chat.on, room.chat.desk.On = false, false, false, false

		host.snap.CanExclude = true
		host.snap.Exclude = &nativecapture.CaptureTarget{Kind: "window", SourceID: "3", PID: 13, Title: "Voice call"}
		room.picking = true
		room.loadSources()
		room.picker.Tab, room.pickExclude.disc.Open = "display", true
		renderPNG(t, "room-exclude-"+theme, image.Pt(1185, 1057), func(gtx layout.Context) { room.Layout(gtx, ui) })
		room.picker.Tab = "window"
		host.snap.CanExclude = false

		room.picking = true
		room.loadSources()
		renderPNG(t, "room-picker-"+theme, image.Pt(1185, 1057), func(gtx layout.Context) { room.Layout(gtx, ui) })
	}
}

func (h *fakeHost) SendChat(string) error          { return nil }
func (h *fakeHost) SendReaction(string, string)    {}
func (h *fakeHost) MarkChatRead()                  {}
func (h *fakeHost) SetRemotePolicy(string, string) {}
func (h *fakeHost) SetRemoteSimultaneous(bool)     {}
func (h *fakeHost) DecideRemote(string, bool)      {}
func (h *fakeHost) SetStopShortcut(int)            {}

func (h *fakeHost) VoiceOpen()           {}
func (h *fakeHost) VoiceClose()          {}
func (h *fakeHost) VoiceMicrophone(bool) {}
func (h *fakeHost) VoiceDeafen(bool)     {}
func (h *fakeHost) VoiceMute(string)     {}

func (h *fakeHost) SetShareVolume(float64, bool) {}
func (h *fakeHost) CancelStart()                 {}

func (h *fakeHost) SetSourceAudio(bool) {}

func (h *fakeHost) SetViewerPassword(string)          {}
func (h *fakeHost) VoiceVolume(float64)               {}
func (h *fakeHost) VoicePersonVolume(string, float64) {}
func (h *fakeHost) VoiceBitrate(int)                  {}
func (h *fakeHost) Diagnostics() map[string]any       { return map[string]any{"phase": h.Snapshot().Phase} }
func (h *fakeHost) Microphones(context.Context) ([]audioDevice, error) {
	return []audioDevice{{ID: "m1", Label: "Headset Microphone (USB)"}, {ID: "m2", Label: "Webcam"}}, nil
}
func (h *fakeHost) SetCommentary(device string, volume float64) {
	h.mu.Lock()
	h.snap.Settings.MicDevice, h.snap.Settings.MicVolume = device, volume
	h.mu.Unlock()
}
func (h *fakeHost) VoiceDevices() ([]audioDevice, error) { return h.Microphones(nil) }
func (h *fakeHost) VoiceLinks() []voiceLinkView {
	return []voiceLinkView{{Peer: "viewer-aaaaaaaa", State: "connected", RTTms: 6, JitterMs: 2.1, HasStats: true}}
}
func (h *fakeHost) VoiceDevice(string)   {}
func (h *fakeHost) VoiceMicGain(float64) {}
func (h *fakeHost) VoiceProcessing(bool) {}
func (h *fakeHost) VoiceGate(float64)    {}
func (h *fakeHost) SetExcludeAudio(target *nativecapture.CaptureTarget) {
	h.mu.Lock()
	h.snap.Exclude = target
	h.mu.Unlock()
}

func TestCrowdShot(t *testing.T) {
	ui := &UI{P: DarkPalette(), Shaper: newShaper(false), Lang: "it"}
	host := &fakeHost{snap: roomSnapshot{Signal: "connected", Phase: phaseLive, Code: "9527", HostID: "host-peer-1",
		Invite: "http://192.168.0.5:8787/r/9527#v=x", Settings: shareSettings{Topology: true}}}
	names := []string{"Leeroy", "PIIK TV", "Anna", "Bruno", "Carla", "Dario", "Elena", "Franco", "Giulia", "Hugo", "Irene", "Un nome davvero molto lungo"}
	for i, name := range names {
		id := "viewer-" + string(rune('a'+i)) + "aaaaaaa"
		peer := roomPeer{ID: id, Name: name, MediaReady: i%3 != 2}
		if i > 3 {
			peer.Upstream = "viewer-aaaaaaaa"
		}
		host.snap.Viewers = append(host.snap.Viewers, peer)
	}
	host.snap.Voice = voiceSnapshot{Enabled: true, Joined: true, Members: []voiceMemberView{{PeerID: "host-peer-1", Joined: true}},
		Edges: []voiceEdge{{Parent: "host-peer-1", Child: "viewer-aaaaaaaa"}, {Parent: "host-peer-1", Child: "viewer-caaaaaaa", Pending: true}}}
	host.snap.Reactions = []activeReaction{
		{ID: "r1", From: "viewer-aaaaaaaa", Reaction: "heart", Until: time.Now().Add(2 * time.Second)},
		{ID: "r2", From: "viewer-baaaaaaa", Target: "viewer-aaaaaaaa", Reaction: "star", Until: time.Now().Add(2 * time.Second)}}
	room := newRoom(host, func() {})
	room.detailsOpen, room.topologyOpen = true, true
	renderPNG(t, "room-crowd-dark", image.Pt(1185, 2500), func(gtx layout.Context) { room.Layout(gtx, ui) })
	if room.windowTitle(ui, host.snap) == "VibePiik | 9527 · "+ui.T("host.live") {
		t.Fatal("the window title should carry a title frame")
	}
	ui.Lang, ui.Spoken = "vis", "it"
	if got := ui.T("common.host"); got != extraCatalogs["it"]["common.host"] {
		t.Fatalf("✦ keeps the spoken language: %q", got)
	}
}

func (h *fakeHost) PreviewShown(int) {}

func TestPreviewRateShot(t *testing.T) {
	ui := &UI{P: DarkPalette(), Shaper: newShaper(false), Lang: "it"}
	host := &fakeHost{snap: roomSnapshot{Signal: "connected", Phase: phaseLive, Code: "9527", Audio: true,
		Target:  nativecapture.CaptureTarget{Kind: "display", SourceID: "1", Title: "Screen"},
		Preview: testPicture(1920, 1080, color.NRGBA{R: 0x8f, G: 0xb8, B: 0x7d, A: 255})}}
	room := newRoom(host, func() {})
	room.previewCtl.fpsUntil = time.Now().Add(time.Hour)
	renderPNG(t, "preview-rate", image.Pt(1185, 760), func(gtx layout.Context) { gtx.Now = time.Now(); room.Layout(gtx, ui) })
}

func (h *fakeHost) EndShare() {}
