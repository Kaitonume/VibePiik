package nativeui

import (
	"image"
	"os"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/TNTcraftHIM/Piik/internal/app/loopback"
	"github.com/TNTcraftHIM/Piik/internal/app/nativecapture"
	"github.com/TNTcraftHIM/Piik/internal/server/protocol"
)

func TestFramerateCeiling(t *testing.T) {
	limits := localVideoLimits
	cases := []struct {
		w, h, want int
	}{
		{1920, 1080, 240}, // 8160 macroblocks: 254 > 240 cap
		{2560, 1440, 144}, // 14400 macroblocks
		{3840, 2160, 64},  // 32400 macroblocks
		{1921, 1080, 0},   // odd width
		{2, 1080, 0},      // too small
		{9000, 1080, 0},   // wider than 8192
		{8192, 8192, 0},   // more pixels than allowed
	}
	for _, c := range cases {
		if got := framerateCeiling(c.w, c.h, limits); got != c.want {
			t.Errorf("framerateCeiling(%d, %d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
	narrow := limitsFrom(loopback.NativeMediaCapabilities{MaxVideoResolution: protocol.QualityResolution{Width: 1920, Height: 1080},
		MaxVideoFramerate: 60, MaxVideoBitrate: 20_000_000})
	if narrow.MaxWidth != 1920 || narrow.MaxFramerate != 60 || narrow.MaxBitrate != 20_000_000 || narrow.MaxPixels != localVideoLimits.MaxPixels {
		t.Fatalf("limitsFrom = %+v", narrow)
	}
	if framerateCeiling(2560, 1440, narrow) != 0 || framerateCeiling(1920, 1080, narrow) != 60 {
		t.Fatal("App limits not applied")
	}
}

func TestSliderRange(t *testing.T) {
	// FPS: common range 15–60 step 5.
	if top, common, custom := sliderRange(30, 15, 60, 5, 144); top != 60 || common != 30 || custom {
		t.Fatalf("30 fps: %v %v %v", top, common, custom)
	}
	if _, common, custom := sliderRange(72, 15, 60, 5, 144); !custom || common != 60 {
		t.Fatalf("72 fps should be custom, back to 60: %v %v", common, custom)
	}
	if top, _, custom := sliderRange(24, 15, 60, 5, 24); top != 20 || !custom {
		t.Fatalf("ceiling 24: top %v custom %v", top, custom)
	}
	if top, _, custom := sliderRange(10, 15, 60, 5, 12); top >= 15 || !custom {
		t.Fatalf("no common range: top %v custom %v", top, custom)
	}
	// Bitrate: 2–12 Mbps step 0.5.
	if _, common, custom := sliderRange(25, 2, 12, 0.5, 240); !custom || common != 12 {
		t.Fatalf("25 Mbps: %v %v", common, custom)
	}
}

func TestAudioApps(t *testing.T) {
	apps := audioApps([]nativecapture.CaptureTarget{
		{Kind: "window", SourceID: "1", PID: 7, CreationTime: "a", Title: "Game"},
		{Kind: "window", SourceID: "2", PID: 7, CreationTime: "a", Title: "Game launcher"},
		{Kind: "window", SourceID: "3", PID: 8, CreationTime: "b", Title: "Call"},
		{Kind: "display", SourceID: "4", Title: "Screen 1"},
	})
	if len(apps) != 2 || apps[0].Title != "Game" || apps[1].Title != "Call" {
		t.Fatalf("audioApps = %+v", apps)
	}
}

func TestExcludeFlag(t *testing.T) {
	excluded := nativecapture.CaptureTarget{Kind: "window", SourceID: "3", PID: 8, Title: "Call"}
	h := &hostSession{exclude: &excluded}
	h.env.Capabilities.ProcessAudioExclusion, h.env.Capabilities.SystemAudio = true, true
	screen := nativecapture.CaptureTarget{Kind: "display", SourceID: "1", Title: "Screen"}
	fields := map[string]any{}
	h.excludeFlag(fields, screen, true)
	if fields["excludeAudio"] != excluded {
		t.Fatalf("screen with sound should exclude: %v", fields)
	}
	for _, c := range []struct {
		target nativecapture.CaptureTarget
		audio  bool
	}{{screen, false}, {excluded, true}} {
		fields := map[string]any{}
		h.excludeFlag(fields, c.target, c.audio)
		if _, ok := fields["excludeAudio"]; ok {
			t.Fatalf("unexpected exclusion for %+v audio=%v", c.target, c.audio)
		}
	}
	h.env.Capabilities.ProcessAudioExclusion = false
	fields = map[string]any{}
	h.excludeFlag(fields, screen, true)
	if _, ok := fields["excludeAudio"]; ok {
		t.Fatal("exclusion without capability")
	}
}

func TestEvidenceDetailAndState(t *testing.T) {
	jitter, freezes, codec, audio := 2.5, 1.0, "video/AV1", "audio/opus"
	lines := evidenceDetail{JitterMs: &jitter, FreezeCountDelta: &freezes, Codec: &codec, AudioCodec: &audio}.lines()
	found := map[string]statLine{}
	for _, line := range lines {
		found[line.Label] = line
	}
	if found["stats.jitter"].Value != "2.5 ms" || found["stats.freezeCount"].Value != "1" {
		t.Fatalf("lines = %+v", lines)
	}
	if !found["stats.codec"].Warn || found["stats.codec"].Value != "AV1" {
		t.Fatalf("unexpected codec should warn: %+v", found["stats.codec"])
	}
	if _, warned := found["stats.warn.audioCodec"]; warned {
		t.Fatal("opus must not warn")
	}
	now := viewerStats{At: time.Now(), Stutter: true}
	cases := []struct {
		peer roomPeer
		want string
	}{
		{roomPeer{ID: "a"}, "state.peer.connecting"},
		{roomPeer{ID: "a", Upstream: "host"}, "state.peer.routing"},
		{roomPeer{ID: "a", Upstream: "host", MediaReady: true}, "state.peer.stuttering"},
	}
	for _, c := range cases {
		if got, _ := viewerState(c.peer, now, true); got != c.want {
			t.Errorf("viewerState(%+v) = %s, want %s", c.peer, got, c.want)
		}
	}
	if got, _ := viewerState(roomPeer{MediaReady: true}, viewerStats{}, false); got != "state.peer.connected" {
		t.Errorf("ready viewer = %s", got)
	}
}

func TestVoiceGain(t *testing.T) {
	v := newVoiceEndpoint(&hostSession{})
	v.volume = 0.5
	v.volumes["peer-b"] = 2
	v.state = &voiceState{Publications: []voicePublication{{ID: "pa", PeerID: "peer-a"}, {ID: "pb", PeerID: "peer-b"}}}
	v.plays["pa"] = &playback{frames: [][]int16{make([]int16, 4)}}
	v.plays["pb"] = &playback{frames: [][]int16{make([]int16, 4)}}
	for i := range 4 {
		v.plays["pa"].frames[0][i] = 1000
		v.plays["pb"].frames[0][i] = 1000
	}
	out := make([]int16, 4)
	if !v.mix(out) {
		t.Fatal("nothing mixed")
	}
	// peer-a: 1000 × 0.5; peer-b: 1000 × 0.5 × 2.
	if out[0] != 1500 {
		t.Fatalf("mixed sample = %d, want 1500", out[0])
	}
}

func TestNativeCopyFallback(t *testing.T) {
	u := &UI{Lang: "zh"}
	if u.T("native.report") != "保存报告" {
		t.Fatal("zh native copy")
	}
	u.Lang = "vis"
	if u.T("native.report") != "Save report" {
		t.Fatal("native copy falls back to English")
	}
	if u.T("remote.apply") == "" {
		t.Fatal("web copy still reachable")
	}
}

func TestEscapeClosesTopmost(t *testing.T) {
	host := &fakeHost{snap: roomSnapshot{Signal: "connected", Phase: phaseLive, Code: "1234", HostID: "h",
		Viewers: []roomPeer{{ID: "v1", Name: "Leeroy"}}, InteractionsReady: true}}
	room := newRoom(host, func() {})
	ui := &UI{P: DarkPalette(), Shaper: newShaper(false), Lang: "en"}
	var router input.Router
	frame := func() {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(),
			Constraints: layout.Exact(image.Pt(1185, 1057))}
		room.Layout(gtx, ui)
		room.overlays(gtx, ui, host.Snapshot())
		router.Frame(&ops)
	}
	room.interact.open, room.person.target = true, "v1"
	frame()
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	if room.person.target != "" || !room.interact.open {
		t.Fatalf("first Escape: person=%q interact=%v", room.person.target, room.interact.open)
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	if room.interact.open {
		t.Fatal("second Escape should close Interactions")
	}
}

func TestPersonMenuTargetsReactions(t *testing.T) {
	if !isThrow("tomato") || !isThrow("poop") || isThrow("heart") {
		t.Fatal("throws")
	}
	var m personMenu
	m.open("v1")
	if m.target != "v1" {
		t.Fatal("open")
	}
	m.open("v1")
	if m.target != "" {
		t.Fatal("toggle closes")
	}
}

func TestSaveChatText(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	path, err := saveChat([]chatMessage{{Name: "Leeroy", Text: "Ciao\nTV", At: time.Date(2026, 11, 5, 18, 0, 0, 0, time.Local)},
		{Host: true, Text: "你好"}}, "Host")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "[18:00:00] Leeroy: Ciao TV") || !strings.Contains(text, "Host: 你好") {
		t.Fatalf("chat file:\n%s", text)
	}
}

func TestApplyGain(t *testing.T) {
	frame := []int16{1000, -1000, 30000}
	applyGain(frame, 2)
	if frame[0] != 2000 || frame[1] != -2000 || frame[2] != 32767 {
		t.Fatalf("gain = %v", frame)
	}
}

func TestLabelNames(t *testing.T) {
	got := labelNames([]string{"host-1", "peer-aaaaaa1234", "peer-bbbbbb1234", "x"}, []string{"Host", "Leo", "Leo", "Ann"})
	if got[0] != "Host" || got[1] != "Leo (aa1234)" || got[2] != "Leo (bb1234)" || got[3] != "Ann" {
		t.Fatalf("labels = %q", got)
	}
	got = labelNames([]string{"aaaa-xx1234", "bbbb-xx1234"}, []string{"Leo", "Leo"})
	if got[0] != "Leo (a-xx1234)" || got[1] != "Leo (b-xx1234)" {
		t.Fatalf("extended labels = %q", got)
	}
}

func TestSegmentedArrowKeys(t *testing.T) {
	ui := &UI{P: DarkPalette(), Shaper: newShaper(false), Lang: "en"}
	var s Segmented
	var router input.Router
	selected := 0
	frame := func() {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(400, 60))}
		_, selected = s.LayoutWide(gtx, ui, nil, []string{"A", "B", "C"}, selected)
		router.Frame(&ops)
	}
	frame()
	router.Queue(key.FocusEvent{Focus: true})
	frame()
	// Focus the first option, then press Right twice and Left once.
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Source: router.Source()}
	gtx.Execute(key.FocusCmd{Tag: &s.clicks[0]})
	frame()
	for _, name := range []key.Name{key.NameRightArrow, key.NameRightArrow, key.NameLeftArrow, key.NameLeftArrow, key.NameLeftArrow} {
		router.Queue(key.Event{Name: name, State: key.Press})
		frame()
	}
	if selected != 2 {
		t.Fatalf("selected = %d, want 2 (0→1→2→1→0→2)", selected)
	}
}

func TestPanelDrag(t *testing.T) {
	host := &fakeHost{snap: roomSnapshot{Signal: "connected", Phase: phaseLive, Code: "1234", HostID: "h", InteractionsReady: true}}
	room := newRoom(host, func() {})
	ui := &UI{P: DarkPalette(), Shaper: newShaper(false), Lang: "en"}
	room.interact.open = true
	var router input.Router
	frame := func() {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1185, 1057))}
		room.Layout(gtx, ui)
		router.Frame(&ops)
	}
	frame()
	before := room.interact.panelRect
	at := f32.Pt(float32(before.Min.X+60), float32(before.Min.Y+20))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at})
	frame()
	for i := 1; i <= 5; i++ {
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at.Sub(f32.Pt(float32(40*i), float32(30*i)))})
		frame()
	}
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at.Sub(f32.Pt(200, 150))})
	frame()
	after := room.interact.panelRect
	if after.Min.X >= before.Min.X-100 || after.Min.Y >= before.Min.Y-60 {
		t.Fatalf("panel did not follow the drag: %v → %v", before, after)
	}
}

func TestThrowFlight(t *testing.T) {
	now := time.Now()
	start := now.Add(-time.Duration(float64(reactionDuration) * 0.2)) // 20% into its life: flying
	snap := roomSnapshot{Reactions: []activeReaction{{ID: "r", From: "a", Target: "b", Reaction: "tomato", Until: start.Add(reactionDuration)}}}
	flights := throwsFor(snap, []string{"h", "a", "b"}, now)
	if len(flights) != 1 || flights[0].from != 1 || flights[0].to != 2 || flights[0].t < .5 || flights[0].t > .65 {
		t.Fatalf("flights = %+v", flights)
	}
	if id, _ := reactionFor(snap, "b", now); id != "" {
		t.Fatal("the tomato lands only after its flight")
	}
	later := start.Add(time.Duration(float64(reactionDuration) * 0.6))
	if id, _ := reactionFor(snap, "b", later); id != "tomato" || len(throwsFor(snap, []string{"h", "a", "b"}, later)) != 0 {
		t.Fatal("after the flight the tomato sits on its target")
	}
	ui := &UI{P: DarkPalette(), Shaper: newShaper(false), Lang: "en"}
	pawns := []pawn{{name: "Host", host: true, id: "h"}, {name: "Leo", id: "a"}, {name: "TV", id: "b"}}
	renderPNG(t, "couch-throw", image.Pt(400, 160), func(gtx layout.Context) {
		gtx.Now = now
		fillRect(gtx.Ops, gtx.Constraints.Max, ui.P.Wall)
		layoutCouch(gtx, ui, pawns, flights, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })
	})
}
