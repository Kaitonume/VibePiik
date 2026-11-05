package nativeui

import (
	"image"
	"image/color"
	"os"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func TestFlightLanesAndSpeed(t *testing.T) {
	now := time.Now()
	look := overlayLook{Speed: 1.5}
	var flights []overlayFlight
	for i := range 10 {
		flights = addFlight(flights, chatMessage{ID: itoa(i), PeerID: "p", Text: "m"}, look, now)
	}
	if len(flights) > overlayLanes {
		t.Fatalf("one flight per lane, got %d", len(flights))
	}
	if flights[len(flights)-1].Duration != 4*time.Second {
		t.Fatalf("speed 1.5 → 4 s, got %v", flights[len(flights)-1].Duration)
	}
	if len(activeFlights(flights, now.Add(5*time.Second))) != 0 {
		t.Fatal("flights end")
	}
	if (overlayLook{Scale: 3, Opacity: .1}).clamped() != (overlayLook{Scale: 1.3, Opacity: .4, Speed: 1}) {
		t.Fatal("clamp")
	}
}

func TestChatLinesExpire(t *testing.T) {
	now := time.Now()
	lines := []overlayLine{{ID: "a", Until: now.Add(time.Second)}, {ID: "b", Until: now.Add(3 * time.Second)}}
	if nextExpiry(lines, now) != time.Second || len(activeLines(lines, now.Add(2*time.Second))) != 1 {
		t.Fatal("expiry")
	}
}

// TestOverlayRender draws the flights and the chat box headless: the
// background must stay transparent (the overlay window shows what is below).
func TestOverlayRender(t *testing.T) {
	size := image.Pt(900, 506)
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skipf("headless GPU unavailable: %v", err)
	}
	defer window.Release()
	ui := &UI{P: DarkPalette(), Shaper: newShaper(false), Lang: "en"}
	now := time.Now()
	flights := []overlayFlight{
		{ID: "1", PeerID: "viewer-aaaaaaaa", Text: "Ciao! Si vede benissimo", Lane: 0, Start: now.Add(-2 * time.Second), Duration: overlayDuration},
		{ID: "2", PeerID: "viewer-bbbbbbbb", Text: "Vai!!", Lane: 1, Start: now.Add(-4 * time.Second), Duration: overlayDuration},
	}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size), Source: input.Source{}}
	if !drawFlights(gtx, ui, flights, overlayLook{ColorBySender: true}, now) {
		t.Fatal("flights should be moving")
	}
	if err := window.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
	if err := window.Screenshot(img); err != nil {
		t.Fatal(err)
	}
	if a := img.RGBAAt(size.X-2, size.Y-2).A; a != 0 {
		t.Fatalf("background alpha %d, want transparent", a)
	}
	opaque := 0
	for y := 12; y < 40; y++ {
		for x := 0; x < size.X; x++ {
			if c := img.RGBAAt(x, y); c.A > 0 {
				opaque++
				if c.R > c.A || c.G > c.A || c.B > c.A {
					t.Fatalf("pixel %v not premultiplied", c)
				}
			}
		}
	}
	if opaque == 0 {
		t.Fatal("lane 0 pill not drawn")
	}
	if os.Getenv("NATIVEUI_SHOTS") != "" {
		renderPNG(t, "overlay-flights", size, func(gtx layout.Context) {
			fillRect(gtx.Ops, size, color.NRGBA{R: 0x3a, G: 0x6e, B: 0x4f, A: 255})
			drawFlights(gtx, ui, flights, overlayLook{ColorBySender: true}, now)
		})
		lines := []overlayLine{
			{ID: "1", PeerID: "viewer-aaaaaaaa", Name: "Leeroy", Text: "GG! That last round was insane", Until: now.Add(time.Minute)},
			{ID: "2", PeerID: "viewer-bbbbbbbb", Name: "PIIK TV", Reaction: "tomato", TargetName: "Host", Until: now.Add(time.Minute)},
			{ID: "3", PeerID: "host-peer-1", Name: "Host", Text: "Ahah, next one I win", Until: now.Add(time.Minute)},
		}
		for _, corner := range []string{"top-right", "bottom-left"} {
			renderPNG(t, "overlay-box-"+corner, image.Pt(420, 520), func(gtx layout.Context) {
				paint.Fill(gtx.Ops, color.NRGBA{R: 0x50, G: 0x60, B: 0x80, A: 255})
				drawChatBox(gtx, ui, lines, overlayLook{}, corner)
			})
		}
	}
}

func TestComicShots(t *testing.T) {
	if !hasComic("hint-pause") || !hasComic("source-starting") || hasComic("nope") {
		t.Fatal("embedded comics")
	}
	for _, palette := range []Palette{DarkPalette(), LightPalette()} {
		theme := map[bool]string{true: "dark", false: "light"}[palette.Dark]
		for _, lang := range []string{"en", "vis"} {
			ui := &UI{P: palette, Shaper: newShaper(false), Lang: lang}
			renderPNG(t, "comic-tooltip-"+theme+"-"+lang, image.Pt(360, 260), func(gtx layout.Context) {
				fillRect(gtx.Ops, gtx.Constraints.Max, palette.Wall)
				hoverTooltipComic(gtx, ui, true, time.Time{}.Add(-time.Second), ui.T("host.pause"), "hint-pause", image.Pt(360, 10))
			})
		}
		ui := &UI{P: palette, Shaper: newShaper(false), Lang: "en"}
		host := &fakeHost{snap: roomSnapshot{Signal: "connected", Phase: phaseStarting, Code: "9527"}}
		room := newRoom(host, func() {})
		renderPNG(t, "room-starting-"+theme, image.Pt(1185, 760), func(gtx layout.Context) { room.Layout(gtx, ui) })
		host.snap = roomSnapshot{Signal: "connected", Phase: phaseIdle, Code: "9527", Notice: "host.stopNotice", NoticeAt: time.Now()}
		renderPNG(t, "room-stopped-"+theme, image.Pt(1185, 760), func(gtx layout.Context) { room.Layout(gtx, ui) })
	}
}

func TestItalianShots(t *testing.T) {
	ui := &UI{P: DarkPalette(), Shaper: newShaper(false), Lang: "it"}
	if ui.T("host.start") == copy_en["host.start"] || ui.T("native.join.browser") == nativeCopy_en["native.join.browser"] {
		t.Fatal("Italian texts not used")
	}
	host := &fakeHost{snap: roomSnapshot{Signal: "connected", Phase: phaseIdle, Code: "9527"}}
	room := newRoom(host, func() {})
	renderPNG(t, "room-idle-it", image.Pt(1185, 1057), func(gtx layout.Context) { room.Layout(gtx, ui) })
	host.snap = roomSnapshot{Signal: "connected", Phase: phaseLive, Code: "9527", Invite: "http://x/r/9527#v=a", HostID: "h",
		Viewers: []roomPeer{{ID: "viewer-aaaaaaaa", Name: "Leo", MediaReady: true}}, Settings: shareSettings{Quality: qualityPresets["1080p60"], Codec: "h264"}}
	room.settingsOpen = true
	renderPNG(t, "room-live-it", image.Pt(1185, 1500), func(gtx layout.Context) { room.Layout(gtx, ui) })
}

func TestComicStrips(t *testing.T) {
	for _, kind := range []string{"hint-quality", "source-starting", "share-ended"} {
		for _, dark := range []bool{true, false} {
			strip := comicStripFor(kind, dark)
			if strip == nil || len(strip.frames) != comicFrames {
				t.Fatalf("%s dark=%v: strip missing", kind, dark)
			}
			if size := strip.frames[0].Size(); size.X != 320 || size.Y != 96 {
				t.Fatalf("%s frame size %v", kind, size)
			}
		}
	}
}
