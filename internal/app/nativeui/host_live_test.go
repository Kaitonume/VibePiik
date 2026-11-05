package nativeui

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/TNTcraftHIM/Piik/internal/app/loopback"
)

// wsControl drives a running App's native control over its loopback socket,
// so the live test exercises the same nativecontrol as the in-process host.
type wsControl struct {
	conn   *websocket.Conn
	events chan any
}

func dialControl(t *testing.T, port string) (*wsControl, loopback.NativeMediaCapabilities) {
	t.Helper()
	response, err := http.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		t.Fatal(err)
	}
	var health struct {
		InstanceToken string                           `json:"instanceToken"`
		NativeMedia   loopback.NativeMediaCapabilities `json:"nativeMedia"`
	}
	_ = json.NewDecoder(response.Body).Decode(&health)
	response.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws://127.0.0.1:"+port+"/control",
		&websocket.DialOptions{Subprotocols: []string{"piik-client-v10." + health.InstanceToken}})
	if err != nil {
		t.Fatal(err)
	}
	conn.SetReadLimit(1 << 20)
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"version":10,"id":"hello-native-test","type":"hello"}`)); err != nil {
		t.Fatal(err)
	}
	if _, data, err := conn.Read(ctx); err != nil || !strings.Contains(string(data), `"ready"`) {
		t.Fatalf("hello: %s %v", data, err)
	}
	control := &wsControl{conn: conn, events: make(chan any, 256)}
	go func() {
		defer close(control.events)
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			control.events <- json.RawMessage(data)
		}
	}()
	return control, health.NativeMedia
}

func (c *wsControl) Handle(ctx context.Context, payload []byte) (any, error) {
	return nil, c.conn.Write(ctx, websocket.MessageText, payload)
}
func (c *wsControl) Events() <-chan any { return c.events }
func (c *wsControl) Close() error       { return c.conn.Close(websocket.StatusNormalClosure, "") }

// TestLiveSourceIndex prints only where NATIVEUI_LIVE_WINDOW sits among the
// window sources (other titles are never printed).
func TestLiveSourceIndex(t *testing.T) {
	entry := os.Getenv("NATIVEUI_LIVE")
	if entry == "" {
		t.Skip("NATIVEUI_LIVE not set")
	}
	port := strings.SplitN(strings.SplitN(entry, "piik-client-port=", 2)[1], "#", 2)[0]
	control, _ := dialControl(t, port)
	client := newControlClient(control, nil)
	defer client.Close()
	raw, err := client.request(context.Background(), "list-sources", nil)
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Sources []struct{ Kind, Title string } `json:"sources"`
	}
	_ = json.Unmarshal(raw, &list)
	index, windows := -1, 0
	for _, source := range list.Sources {
		if source.Kind != "window" {
			continue
		}
		if strings.Contains(source.Title, os.Getenv("NATIVEUI_LIVE_WINDOW")) {
			index = windows
		}
		windows++
	}
	t.Logf("window index %d of %d", index, windows)
}

// TestLiveWindowShot saves a capture of one process's own window
// (NATIVEUI_LIVE_PID) to NATIVEUI_LIVE_SHOT, through the App's source preview.
func TestLiveWindowShot(t *testing.T) {
	entry, pid := os.Getenv("NATIVEUI_LIVE"), os.Getenv("NATIVEUI_LIVE_PID")
	if entry == "" || pid == "" {
		t.Skip("NATIVEUI_LIVE / NATIVEUI_LIVE_PID not set")
	}
	port := strings.SplitN(strings.SplitN(entry, "piik-client-port=", 2)[1], "#", 2)[0]
	control, _ := dialControl(t, port)
	h := &hostSession{env: HostEnv{NewControl: func() ControlSession { return control }}, ctx: context.Background()}
	sources, err := h.Sources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if source.Kind == "window" && itoa(int(source.PID)) == pid {
			picture, err := h.SourcePreview(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(os.Getenv("NATIVEUI_LIVE_SHOT"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := png.Encode(file, picture); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("window of that process not found")
}

// TestLiveHost shares a window through a running App (NATIVEUI_LIVE=<room entry
// URL>, NATIVEUI_LIVE_WINDOW=<window title>) and keeps the room open for
// NATIVEUI_LIVE_SECONDS while viewers join through the printed invite link.
func TestLiveHost(t *testing.T) {
	entry := os.Getenv("NATIVEUI_LIVE")
	if entry == "" {
		t.Skip("NATIVEUI_LIVE not set")
	}
	// Keep the user's own VibePiik preferences out of the test.
	t.Setenv("LOCALAPPDATA", t.TempDir())
	target, err := parseRoomTarget(entry)
	if err != nil {
		t.Fatal(err)
	}
	port := strings.TrimPrefix(strings.SplitN(strings.SplitN(entry, "piik-client-port=", 2)[1], "#", 2)[0], "")
	control, capabilities := dialControl(t, port)
	host := newHostSession(target, HostEnv{NewControl: func() ControlSession { return control }, Capabilities: capabilities}, func() {})
	defer host.Close()
	host.trace = func(kind string, raw json.RawMessage) {
		if strings.HasPrefix(kind, "remote") || kind == "request-failed" {
			t.Logf("control event %s %s", kind, raw)
		}
	}
	sources, err := host.Sources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	title := os.Getenv("NATIVEUI_LIVE_WINDOW")
	found := false
	for _, source := range sources {
		if source.Kind == "window" && strings.Contains(source.Title, title) {
			host.Start(source, false, false)
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("window %q not among %d sources", title, len(sources))
	}
	seconds := 120
	if value := os.Getenv("NATIVEUI_LIVE_SECONDS"); value != "" {
		_ = json.Unmarshal([]byte(value), &seconds)
	}
	start := time.Now()
	deadline := start.Add(time.Duration(seconds) * time.Second)
	last := ""
	script := os.Getenv("NATIVEUI_LIVE_SCRIPT") == "pause"
	step := 0
	for time.Now().Before(deadline) {
		snap := host.Snapshot()
		host.mu.Lock()
		edges := len(host.edges)
		host.mu.Unlock()
		line := snap.Signal + " " + snap.Phase + " code=" + snap.Code + " viewers=" + itoa(len(snap.Viewers)) + " edges=" + itoa(edges) + " notice=" + snap.Notice
		if line != last {
			t.Logf("%s %s invite=%s", time.Now().Format("15:04:05"), line, snap.Invite)
			last = line
		}
		if os.Getenv("NATIVEUI_LIVE_TV") == "1" && snap.Phase == phaseLive && snap.Invite != "" {
			names, status, err := host.SendTV(context.Background())
			t.Logf("send to TV: %v %d %v", names, status, err)
			os.Setenv("NATIVEUI_LIVE_TV", "sent")
		}
		if os.Getenv("NATIVEUI_LIVE_SCRIPT") == "access" && snap.Phase == phaseLive {
			elapsed := time.Since(start)
			step3 := func(at time.Duration, name string, action func()) {
				if step < 100 && elapsed > at && step == int(at/time.Second) {
					return
				}
			}
			_ = step3
			actions := []struct {
				at     time.Duration
				name   string
				action func()
			}{
				{8 * time.Second, "name", func() { t.Log("set name:", host.SetName("Tester")) }},
				{11 * time.Second, "quality 720p30", func() {
					s := host.Snapshot().Settings
					s.Quality = qualityPresets["720p30"]
					host.SetSettings(s)
				}},
				{14 * time.Second, "private", func() { host.SetPolicy("private") }},
				{17 * time.Second, "rotate", func() { host.RotateInvite() }},
				{20 * time.Second, "revoke", func() { host.RevokeInvite() }},
				{23 * time.Second, "open", func() { host.SetPolicy("open") }},
				{26 * time.Second, "replace", func() { host.ReplaceRoom() }},
			}
			if step < len(actions) && elapsed > actions[step].at {
				t.Logf("action %s", actions[step].name)
				actions[step].action()
				step++
			}
			line := snap.Phase + " code=" + snap.Code + " policy=" + snap.Policy + " name=" + snap.Name + " invite=" + snap.Invite +
				" q=" + itoa(int(snap.Settings.Quality.Resolution.Height)) + "p" + itoa(int(snap.Settings.Quality.MaxFramerate))
			if line != last {
				t.Log(line)
				last = line
			}
		}
		if os.Getenv("NATIVEUI_LIVE_SCRIPT") == "batch2" && snap.Phase == phaseLive {
			elapsed := time.Since(start)
			actions := []struct {
				at     time.Duration
				name   string
				action func()
			}{
				{6 * time.Second, "custom quality 2560x1440 72 fps 25 Mbps", func() {
					s := host.Snapshot().Settings
					s.Quality.Resolution.Width, s.Quality.Resolution.Height = 2560, 1440
					s.Quality.MaxFramerate, s.Quality.MaxBitrate = 72, 25_000_000
					host.SetSettings(s)
				}},
				{12 * time.Second, "stop shortcut F9", func() { host.SetStopShortcut(9) }},
				{15 * time.Second, "open voice", func() { host.VoiceOpen() }},
				{20 * time.Second, "voice bitrate 48 kbps", func() { host.VoiceBitrate(48_000) }},
				{23 * time.Second, "list microphones (App and Windows, none opened)", func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					app, err := host.Microphones(ctx)
					local, localErr := host.VoiceDevices()
					same := 0
					for _, a := range app {
						for _, l := range local {
							if a.ID == l.ID {
								same++
							}
						}
					}
					t.Logf("App microphones: %d (err %v); Windows: %d (err %v); same IDs: %d", len(app), err, len(local), localErr, same)
				}},
				{25 * time.Second, "diagnostics", func() {
					data, err := json.Marshal(host.Diagnostics())
					t.Logf("diagnostics: %d bytes, err=%v, has invite=%v", len(data), err, strings.Contains(string(data), "#v="))
				}},
				{27 * time.Second, "check", func() {
					s := host.Snapshot()
					t.Logf("quality=%dx%d@%v %v notice=%q shortcut=%d saved=%d voiceBitrate=%d capture=%+v", s.Settings.Quality.Resolution.Width,
						s.Settings.Quality.Resolution.Height, s.Settings.Quality.MaxFramerate, s.Settings.Quality.MaxBitrate, s.Notice,
						s.Remote.Shortcut, loadPreferences().RemoteStopKey, s.Voice.Bitrate, s.Capture)
				}},
			}
			if step < len(actions) && elapsed > actions[step].at {
				t.Logf("action %s", actions[step].name)
				actions[step].action()
				step++
			}
		}
		if os.Getenv("NATIVEUI_LIVE_SCRIPT") == "voice" && snap.Phase == phaseLive {
			elapsed := time.Since(start)
			if step == 0 && elapsed > 4*time.Second {
				t.Log("open room voice")
				host.VoiceOpen()
				step++
			}
			if step == 1 && len(snap.Voice.Members) > 1 {
				t.Log("viewer joined voice: start microphone (test tone)")
				host.VoiceMicrophone(true)
				step++
			}
			host.voice.mu.Lock()
			conns := ""
			for id, conn := range host.voice.conns {
				conns += id[:6] + ":" + conn.pc.ConnectionState().String() + "/" + itoa(len(conn.senders)) + " "
			}
			host.voice.mu.Unlock()
			members := ""
			for _, member := range snap.Voice.Members {
				members += fmt.Sprintf("%s(pub=%v speak=%v) ", member.PeerID[:6], member.Publishing, member.Speaking)
			}
			line := fmt.Sprintf("voice enabled=%v joined=%v mic=%v err=%s conns=[%s] members=[%s]", snap.Voice.Enabled, snap.Voice.Joined,
				snap.Voice.Microphone, snap.Voice.Error, conns, members)
			if line != last {
				t.Log(line)
				last = line
			}
		}
		if os.Getenv("NATIVEUI_LIVE_SCRIPT") == "interact" && snap.Phase == phaseLive {
			elapsed := time.Since(start)
			if step == 0 && elapsed > 5*time.Second {
				host.SetRemotePolicy("ask", "")
				step++
			}
			if step == 1 && elapsed > 40*time.Second {
				t.Log("send chat:", host.SendChat("Ciao dall'host nativo"))
				step++
			}
			if step == 2 && elapsed > 44*time.Second {
				host.SendReaction("party", "")
				step++
			}
			for _, request := range snap.Remote.Requests {
				if request.State == "pending" && step < 10 {
					t.Logf("remote request %s from %s: declining", request.ID, request.PeerID)
					host.DecideRemote(request.ID, false)
					step = 10
				}
			}
			line := fmt.Sprintf("chat=%d reactions=%d remote=%s requests=%d ready=%v err=%s", len(snap.Chat), len(snap.Reactions),
				snap.Remote.Mode, len(snap.Remote.Requests), snap.InteractionsReady, snap.InteractionError)
			if line != last {
				t.Log(line)
				for _, message := range snap.Chat {
					t.Logf("  chat %s: %s", message.Name, message.Text)
				}
				for _, reaction := range snap.Reactions {
					t.Logf("  reaction %s from %s", reaction.Reaction, reaction.From)
				}
				last = line
			}
		}
		if script {
			elapsed := time.Since(start)
			switch {
			case step == 0 && elapsed > 30*time.Second:
				t.Log("pause")
				host.SetPaused(true)
				step++
			case step == 1 && elapsed > 45*time.Second:
				t.Log("resume")
				host.SetPaused(false)
				step++
			case step == 2 && elapsed > 80*time.Second:
				t.Log("stop")
				host.Stop()
				step++
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}
