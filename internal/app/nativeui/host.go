package nativeui

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/TNTcraftHIM/Piik/internal/app/loopback"
	"github.com/TNTcraftHIM/Piik/internal/app/nativecapture"
	"github.com/TNTcraftHIM/Piik/internal/server/protocol"
)

// HostEnv is what the App gives the native host: its native capture control
// and its saved sharing quality.
type HostEnv struct {
	NewControl   func() ControlSession
	Capabilities loopback.NativeMediaCapabilities
	Quality      QualityStore
	OpenExternal func(string) error // system browser, for help links
}

type QualityStore interface {
	Read() *protocol.QualitySettings
	Save(protocol.QualitySettings) error
}

const signalingProtocol = "piik-v24"

// Quality presets (src/client/media/quality.ts); 1080p60 is the host default.
var qualityPresets = map[string]protocol.QualitySettings{
	"720p30":  {Resolution: protocol.QualityResolution{Width: 1280, Height: 720}, MaxFramerate: 30, MaxBitrate: 3_000_000, DegradationPreference: "balanced", MaxAudioBitrate: 128000},
	"1080p30": {Resolution: protocol.QualityResolution{Width: 1920, Height: 1080}, MaxFramerate: 30, MaxBitrate: 5_000_000, DegradationPreference: "balanced", MaxAudioBitrate: 128000},
	"1080p60": {Resolution: protocol.QualityResolution{Width: 1920, Height: 1080}, MaxFramerate: 60, MaxBitrate: 12_000_000, DegradationPreference: "balanced", MaxAudioBitrate: 128000},
}

// edge is one viewer connection served by the App (mediaedge), relayed here.
type edge struct {
	child     string
	conn      string
	prepared  bool  // route-update prepare candidate not yet promoted
	revision  int64 // prepare revision
	offerSent bool
	pending   []json.RawMessage
	connected bool
	reported  bool
	closed    bool
}

type iceServerConfig struct {
	URLs []string `json:"urls"`
}

// hostSession is the host page's logic for the native room (HOST-FLOW.md).
type hostSession struct {
	api        *roomAPI
	env        HostEnv
	invalidate func()
	ctx        context.Context
	cancel     context.CancelFunc

	mu sync.Mutex
	// UI facts.
	signal  string
	phase   string
	notice  string
	busy    bool
	viewers []roomPeer
	source  nativecapture.CaptureTarget
	preview image.Image
	name    string

	// Stage 3: settings, stats and access.
	peerID      string
	stats       map[string]viewerStats
	codecMode   string // sharing settings codec: "h264", "vp8" or "auto"
	topology    bool
	microphone  bool
	sourceAudio *bool
	policy      string

	// Stage 4: interactions and remote control.
	interactionsReady  bool
	pendingInteraction string
	interactionError   string
	chat               []chatMessage
	reactions          []activeReaction
	unread             int
	remote             remoteSnapshot
	remotePeers        json.RawMessage
	voice              *voiceEndpoint
	selfAudio          *nativecapture.CaptureTarget // VibePiik's own process, for leaving out the room voice
	previewWidth       int                          // its width on screen
	previewSeen        time.Time                    // the screen preview was last drawn
	voiceOn            bool                         // room voice joined when the share's sound was set up

	// Batch 1 parity: shared sound, start cancel, source switching, notices.
	shareVolume float64
	shareMuted  bool
	switching   bool
	applying    int // sharing settings being applied to the live share
	cancelStart context.CancelFunc
	noticeAt    time.Time
	capture     captureStats
	passwordOn  bool
	failureCode string // last share failure (hostFailureCode), for the help panel

	// Room.
	roomID, hostToken, invite string
	clientID                  string
	roomCancel                context.CancelFunc
	natPrediction             bool

	// Signaling.
	conn          *websocket.Conn
	authenticated bool
	everAuthed    bool
	iceServers    []iceServerConfig
	capacity      int

	// Share.
	control      *controlClient
	adapter      uint32
	encoder      uint32
	codec        string
	codecInUse   string
	share        string // shareGeneration == App shareId
	serverShare  string
	paused       bool
	audio        bool
	border       bool
	exclude      *nativecapture.CaptureTarget // application whose audio a screen share leaves out
	commentDev   string                       // commentary microphone ("" = default)
	commentVol   float64                      // commentary volume 0–2
	quality      protocol.QualitySettings
	lastRevision int64
	lastPhase    string
	activeRev    int64
	children     []string
	edges        map[string]*edge  // by connectionId
	active       map[string]string // child → connectionId
	prepared     *edge

	writeMu sync.Mutex
	// trace sees every native control event (tests).
	trace func(string, json.RawMessage)
	// sender replaces the signaling socket in tests.
	sender func(map[string]any) bool
}

func newHostSession(target roomTarget, env HostEnv, invalidate func()) *hostSession {
	ctx, cancel := context.WithCancel(context.Background())
	h := &hostSession{api: newRoomAPI(target), env: env, invalidate: invalidate, ctx: ctx, cancel: cancel,
		signal: "offline", phase: phaseIdle, capacity: 2, edges: map[string]*edge{}, active: map[string]string{},
		clientID: randomHex(16), quality: qualityPresets["1080p60"], stats: map[string]viewerStats{}, codecMode: "h264",
		topology: true, policy: protocol.CodeEntryOpen, name: loadPreferences().Name,
		commentDev: loadPreferences().CommentDevice, commentVol: savedPercent(loadPreferences().CommentVolume)}
	if env.Quality != nil {
		if saved := env.Quality.Read(); saved != nil {
			h.quality = *saved
		}
	}
	h.voice = newVoiceEndpoint(h)
	go h.boot()
	return h
}

func randomHex(n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 2*n)
	for i := range b {
		b[i] = digits[rand.IntN(16)]
	}
	return string(b)
}

func (h *hostSession) changed() {
	if h.invalidate != nil {
		h.invalidate()
	}
}

func (h *hostSession) setNotice(text string) {
	h.mu.Lock()
	h.notice = text
	h.noticeAt = time.Now()
	h.mu.Unlock()
	h.changed()
}

// boot admits this App to its room server and reads its capabilities.
func (h *hostSession) boot() {
	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()
	if err := h.api.Admit(ctx); err != nil {
		slog.Debug("vibepiik-native", "event", "site-access-failed", "error", err)
	}
	var capabilities struct {
		NatPrediction bool `json:"natPrediction"`
	}
	if _, err := h.api.do(ctx, http.MethodGet, "/api/capabilities", nil, "", &capabilities); err == nil {
		h.mu.Lock()
		h.natPrediction = capabilities.NatPrediction
		h.mu.Unlock()
	}
}

func (h *hostSession) Snapshot() roomSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	phase := h.phase
	if phase == phaseLive && h.paused {
		phase = phasePaused
	}
	snap := roomSnapshot{Signal: h.signal, Phase: phase, Code: h.roomID, Invite: h.invite, Source: h.source.Title,
		Preview: h.preview, Viewers: append([]roomPeer(nil), h.viewers...), Name: h.name, Notice: h.notice, Busy: h.busy,
		HostID: h.peerID, Stats: copyStats(h.stats), Settings: h.settingsLocked(), SourceAudio: h.sourceAudio,
		CodecInUse: h.codecInUse, Policy: h.policy, Canonical: canonicalInvite(h.invite), Target: h.source,
		ShareVolume: h.shareVolume, ShareMuted: h.shareMuted, Switching: h.switching, Applying: h.applying > 0, NoticeAt: h.noticeAt,
		ShareVolumeOK: h.env.Capabilities.ShareVolume, Capture: h.capture, Audio: h.audio, PasswordOn: h.passwordOn, FailureCode: h.failureCode,
		Exclude: h.exclude, CanExclude: h.env.Capabilities.ProcessAudioExclusion && h.env.Capabilities.SystemAudio}
	h.extrasSnapshotLocked(&snap)
	// Same names get a peer ID suffix, like the page.
	ids, names := []string{h.peerID}, []string{h.name}
	for _, viewer := range snap.Viewers {
		ids, names = append(ids, viewer.ID), append(names, viewer.Name)
	}
	for i, label := range labelNames(ids, names)[1:] {
		snap.Viewers[i].Name = label
	}
	return snap
}

// ---------------------------------------------------------------------------
// Native control
// ---------------------------------------------------------------------------

func (h *hostSession) ensureControl() (*controlClient, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.control != nil {
		return h.control, nil
	}
	if h.env.NewControl == nil {
		return nil, errors.New("native capture is unavailable")
	}
	h.control = newControlClient(h.env.NewControl(), h.controlEvent)
	return h.control, nil
}

func (h *hostSession) Sources(ctx context.Context) ([]nativecapture.CaptureTarget, error) {
	control, err := h.ensureControl()
	if err != nil {
		return nil, err
	}
	raw, err := control.request(ctx, "list-sources", nil)
	if err != nil {
		return nil, err
	}
	var list struct {
		Sources []nativecapture.CaptureTarget `json:"sources"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	var targets []nativecapture.CaptureTarget
	for _, source := range list.Sources {
		if source.Kind == "window" || source.Kind == "display" {
			targets = append(targets, source)
		}
	}
	return targets, nil
}

func (h *hostSession) SourcePreview(ctx context.Context, target nativecapture.CaptureTarget) (image.Image, error) {
	control, err := h.ensureControl()
	if err != nil {
		return nil, err
	}
	raw, err := control.request(ctx, "source-preview", map[string]any{"source": target})
	if err != nil {
		return nil, err
	}
	var preview struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &preview); err != nil || preview.Data == "" {
		return nil, errors.New("no preview")
	}
	picture := decodePreview(preview.Data)
	if picture == nil {
		return nil, errors.New("preview is not a bitmap")
	}
	return picture, nil
}

// capturePath picks the encoder like defaultNativeCapturePath, falling back
// to software VP8 when there is no hardware H.264 encoder.
func (h *hostSession) capturePath(ctx context.Context, control *controlClient) error {
	raw, err := control.request(ctx, "capture-options", nil)
	if err != nil {
		return err
	}
	var options struct {
		Adapters []struct {
			Index        uint32 `json:"index"`
			HardwareH264 []struct {
				Index uint32 `json:"index"`
			} `json:"hardwareH264"`
		} `json:"adapters"`
	}
	if err := json.Unmarshal(raw, &options); err != nil {
		return err
	}
	h.mu.Lock()
	mode := h.codecMode
	h.mu.Unlock()
	if mode != "vp8" {
		for _, adapter := range options.Adapters {
			if len(adapter.HardwareH264) > 0 && h.env.Capabilities.HardwareH264 {
				h.adapter, h.encoder, h.codec = adapter.Index, adapter.HardwareH264[0].Index, "h264"
				if mode == "auto" {
					h.codec = "auto"
				}
				return nil
			}
		}
	}
	if h.env.Capabilities.SoftwareVP8 && len(options.Adapters) > 0 {
		h.adapter, h.encoder, h.codec = options.Adapters[0].Index, 0, "vp8"
		return nil
	}
	return errors.New("no video encoder")
}

// Start begins sharing target, or switches to it while sharing.
func (h *hostSession) Start(target nativecapture.CaptureTarget, audio, border bool) {
	h.mu.Lock()
	switch h.phase {
	case phaseStarting:
		h.mu.Unlock()
		return
	case phaseLive:
		h.mu.Unlock()
		go h.switchSource(target, audio, border)
		return
	}
	h.phase, h.notice, h.source, h.audio, h.border = phaseStarting, "", target, audio, border
	h.mu.Unlock()
	h.changed()
	go h.startShare(target, audio, border)
}

func (h *hostSession) startFlags(fields map[string]any, border bool) {
	capabilities := h.env.Capabilities
	if capabilities.CaptureBorderControl {
		fields["showCaptureBorder"] = border
	}
	if capabilities.SourceCancellation {
		fields["reportSourceCancellation"] = true
	}
}

func (h *hostSession) startShare(target nativecapture.CaptureTarget, audio, border bool) {
	fail := func(err error, key string) {
		slog.Debug("vibepiik-native", "event", "share-start-failed", "error", err)
		h.mu.Lock()
		h.phase, h.share = phaseIdle, ""
		h.notice, h.noticeAt = key, time.Now()
		h.mu.Unlock()
		h.changed()
	}
	control, err := h.ensureControl()
	if err != nil {
		h.mu.Lock()
		h.failureCode = "app/capture/unavailable"
		h.mu.Unlock()
		fail(err, "host.sourcePicker.unsupported")
		return
	}
	ctx, cancel := context.WithTimeout(h.ctx, 20*time.Second)
	defer cancel()
	h.mu.Lock()
	h.cancelStart = cancel
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.cancelStart = nil
		h.mu.Unlock()
	}()
	if err := h.capturePath(ctx, control); err != nil {
		fail(err, "host.sourcePicker.unsupported")
		return
	}
	share := randomHex(16)
	h.mu.Lock()
	quality := h.quality
	h.share, h.paused, h.serverShare = share, false, ""
	h.lastRevision, h.lastPhase, h.activeRev = 0, "", 0
	h.mu.Unlock()
	fields := map[string]any{"shareId": share, "source": target, "audio": audio, "adapterIndex": h.adapter,
		"encoderIndex": h.encoder, "edgeCapacity": 3, "profile": quality, "codec": h.codec}
	if h.env.Capabilities.Microphone || h.env.Capabilities.ShareVolume {
		fields["microphoneMixing"] = true
	}
	if h.env.Capabilities.CaptureFailureDetails {
		fields["reportCaptureFailure"] = true
	}
	if h.env.Capabilities.RemoteInput {
		fields["remoteInput"] = true
	}
	h.startFlags(fields, border)
	joined := h.prepareSelfAudio(target, audio)
	h.mu.Lock()
	h.voiceOn = joined
	h.excludeFlag(fields, target, audio)
	if h.env.Capabilities.Microphone && h.microphone {
		fields["microphoneMixing"] = true
	}
	h.mu.Unlock()
	started, err := control.request(ctx, "start-share", fields)
	if err != nil {
		var failure controlFailure
		if errors.As(err, &failure) && failure.Code == "source-canceled" {
			fail(err, "")
			return
		}
		if errors.Is(err, context.Canceled) {
			// The host cancelled: stop whatever the App had begun.
			go h.stopShare(share)
			fail(err, "host.startCancelled")
			return
		}
		h.mu.Lock()
		h.failureCode = "app/start-share/rejected"
		if errors.Is(err, context.DeadlineExceeded) {
			h.failureCode = "app/start-share/timeout"
		}
		timedOut := errors.Is(err, context.DeadlineExceeded)
		h.mu.Unlock()
		if timedOut {
			fail(err, "native.fail.requestTimeout")
		} else {
			fail(err, "native.fail.captureStart")
		}
		return
	}
	var result struct {
		SourceAudio *bool  `json:"sourceAudio"`
		Codec       string `json:"codec"`
	}
	_ = json.Unmarshal(started, &result)
	h.mu.Lock()
	h.sourceAudio, h.codecInUse = result.SourceAudio, result.Codec
	peers := h.remotePeers
	h.mu.Unlock()
	if h.env.Capabilities.RemoteInput && len(peers) > 0 {
		go h.pushRemotePeers(share, control, peers)
	}
	if h.env.Capabilities.RemoteStopShortcut {
		go func() {
			// The stop shortcut chosen last time (remoteStopKey).
			if key := savedStopKey(); key != 10 {
				_ = h.stopShortcut(&key)
			} else {
				_ = h.stopShortcut(nil)
			}
			h.changed()
		}()
	}
	go h.refreshPreview(target)
	go h.watchDisplay(share)
	go h.sharpLoop(share)
	if err := h.ensureRoom(); err != nil {
		h.stopShare(share)
		h.mu.Lock()
		h.failureCode = "room/create/failed"
		h.mu.Unlock()
		fail(err, "host.fail.room")
		return
	}
	h.mu.Lock()
	h.shareVolume, h.shareMuted = 1, false
	ready := h.authenticated
	h.mu.Unlock()
	if ready {
		h.sendStartSharing()
	}
}

func (h *hostSession) refreshPreview(target nativecapture.CaptureTarget) {
	ctx, cancel := context.WithTimeout(h.ctx, 6*time.Second)
	defer cancel()
	picture, err := h.SourcePreview(ctx, target)
	if err != nil {
		return
	}
	h.mu.Lock()
	if h.source == target {
		h.preview = picture
	}
	h.mu.Unlock()
	h.changed()
}

func (h *hostSession) switchSource(target nativecapture.CaptureTarget, audio, border bool) {
	h.mu.Lock()
	share, control := h.share, h.control
	h.mu.Unlock()
	if share == "" || control == nil {
		return
	}
	fields := map[string]any{"shareId": share, "source": target, "audio": audio,
		"adapterIndex": h.adapter, "encoderIndex": h.encoder}
	h.startFlags(fields, border)
	joined := h.prepareSelfAudio(target, audio)
	h.mu.Lock()
	h.voiceOn = joined
	h.excludeFlag(fields, target, audio)
	h.mu.Unlock()
	ctx, cancel := context.WithTimeout(h.ctx, 20*time.Second)
	defer cancel()
	h.mu.Lock()
	h.switching = true
	h.mu.Unlock()
	h.changed()
	defer func() {
		h.mu.Lock()
		h.switching = false
		h.mu.Unlock()
		h.changed()
	}()
	if _, err := control.request(ctx, "replace-share-source", fields); err != nil {
		h.mu.Lock()
		h.failureCode = "app/replace-share-source/rejected"
		h.mu.Unlock()
		h.setNotice("host.fail.source")
		return
	}
	h.mu.Lock()
	newSource := h.source != target
	h.source, h.audio, h.border, h.preview = target, audio, border, nil
	h.mu.Unlock()
	if newSource {
		h.setNotice("host.notice.sourceSwitch.ok")
	}
	h.changed()
	h.refreshPreview(target)
}

func (h *hostSession) SetPaused(paused bool) {
	h.mu.Lock()
	share, control := h.share, h.control
	h.mu.Unlock()
	if share == "" || control == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
		defer cancel()
		if _, err := control.request(ctx, "pause-share", map[string]any{"shareId": share, "paused": paused}); err != nil {
			if paused {
				h.setNotice("host.pause.noTracksPause")
			} else {
				h.setNotice("host.pause.noTracksResume")
			}
			return
		}
		h.mu.Lock()
		h.paused = paused
		h.mu.Unlock()
		h.changed()
		h.send(map[string]any{"type": "set-sharing-paused", "shareGeneration": share, "paused": paused})
	}()
}

// Stop ends the share; the room and its code stay.
func (h *hostSession) Stop() {
	go h.endSharing("host.stopNotice", true)
}

// EndShare stops sharing and returns once the App has stopped it.
func (h *hostSession) EndShare() { h.endSharing("host.stopNotice", true) }

func (h *hostSession) endSharing(notice string, notifyServer bool) {
	h.mu.Lock()
	share := h.share
	if share == "" {
		h.mu.Unlock()
		return
	}
	edges := make([]string, 0, len(h.edges))
	for connection := range h.edges {
		edges = append(edges, connection)
	}
	h.share, h.serverShare, h.phase, h.paused, h.preview = "", "", phaseIdle, false, nil
	h.remote.Mode, h.remote.Requests = "off", nil
	h.edges, h.active, h.prepared, h.children = map[string]*edge{}, map[string]string{}, nil, nil
	h.notice = notice
	h.mu.Unlock()
	h.changed()
	if notifyServer {
		h.send(map[string]any{"type": "stop-sharing", "shareGeneration": share})
	}
	for _, connection := range edges {
		h.closeEdgeRequest(share, connection)
	}
	h.stopShare(share)
}

func (h *hostSession) stopShare(share string) {
	h.mu.Lock()
	control := h.control
	h.mu.Unlock()
	if control == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, _ = control.request(ctx, "stop-share", map[string]any{"shareId": share})
}

func (h *hostSession) closeEdgeRequest(share, connection string) {
	h.mu.Lock()
	control := h.control
	h.mu.Unlock()
	if control == nil || share == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_, _ = control.request(ctx, "close-edge", map[string]any{"shareId": share, "connectionId": connection})
	}()
}

// controlEvent handles the App's media events (HOST-FLOW §4.9).
func (h *hostSession) controlEvent(kind string, raw json.RawMessage) {
	if h.trace != nil {
		h.trace(kind, raw)
	}
	var event struct {
		ShareID      string          `json:"shareId"`
		ConnectionID string          `json:"connectionId"`
		State        string          `json:"state"`
		Candidate    json.RawMessage `json:"candidate"`
		Failed       bool            `json:"failed"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return
	}
	h.mu.Lock()
	current := event.ShareID != "" && event.ShareID == h.share
	h.mu.Unlock()
	if !current {
		if kind == "share-started" && event.ShareID != "" {
			// A start the host cancelled finished anyway: stop it.
			go h.stopShare(event.ShareID)
		}
		return
	}
	switch kind {
	case "share-ended":
		notice := "host.stopNotice"
		if event.Failed {
			notice = "host.shareEnded"
		}
		h.endSharing(notice, true)
	case "edge-candidate":
		h.outboundCandidate(event.ConnectionID, event.Candidate)
	case "edge-state":
		h.edgeState(event.ConnectionID, event.State)
	case "edge-quality":
		h.edgeQuality(raw)
	case "remote-control-state":
		h.remoteControlState(raw)
	}
}

// ---------------------------------------------------------------------------
// Room and signaling
// ---------------------------------------------------------------------------

// CreateRoom makes the room before sharing (LCD "+").
func (h *hostSession) CreateRoom() {
	go func() {
		if err := h.ensureRoom(); err != nil {
			h.setNotice("host.roomInvalid")
		}
	}()
}

func (h *hostSession) ensureRoom() error {
	h.mu.Lock()
	if h.roomID != "" {
		h.mu.Unlock()
		return nil
	}
	if h.busy {
		h.mu.Unlock()
		return errors.New("room creation in flight")
	}
	h.busy = true
	h.mu.Unlock()
	h.changed()
	defer func() {
		h.mu.Lock()
		h.busy = false
		h.mu.Unlock()
		h.changed()
	}()
	ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
	defer cancel()
	_ = h.api.Admit(ctx)
	prefs := loadPreferences()
	room, err := h.api.CreateRoom(ctx, prefs.RoomCode)
	if err != nil && prefs.RoomCode != "" {
		room, err = h.api.CreateRoom(ctx, "")
	}
	if err != nil {
		return err
	}
	h.adoptRoom(room)
	return nil
}

func (h *hostSession) adoptRoom(room protocol.CreateRoomResponse) {
	roomCtx, roomCancel := context.WithCancel(h.ctx)
	h.mu.Lock()
	if h.roomCancel != nil {
		h.roomCancel()
	}
	h.roomID, h.hostToken, h.invite, h.roomCancel = room.RoomID, room.HostToken, room.InviteURL, roomCancel
	if room.CodeEntryPolicy != "" {
		h.policy = room.CodeEntryPolicy
	}
	h.clientID = randomHex(16)
	h.mu.Unlock()
	updatePreferences(func(p *preferences) { p.RoomCode = room.RoomID })
	h.changed()
	go h.signalLoop(roomCtx, room.RoomID)
}

// forgetRoom drops the room (closed or invalid).
func (h *hostSession) forgetRoom() {
	h.mu.Lock()
	if h.roomCancel != nil {
		h.roomCancel()
	}
	h.roomID, h.hostToken, h.invite, h.roomCancel = "", "", "", nil
	h.viewers = nil
	h.mu.Unlock()
	h.voice.Shutdown()
	h.changed()
}

func (h *hostSession) signalLoop(ctx context.Context, roomID string) {
	attempt := 0
	for ctx.Err() == nil {
		h.mu.Lock()
		if h.roomID != roomID {
			h.mu.Unlock()
			return
		}
		h.signal = map[bool]string{false: "connecting", true: "reconnecting"}[attempt > 0]
		h.mu.Unlock()
		h.changed()
		code, err := h.signalOnce(ctx)
		h.mu.Lock()
		authed := h.everAuthed
		h.conn, h.authenticated, h.everAuthed = nil, false, false
		h.interactionsReady, h.pendingInteraction = false, ""
		h.signal = "offline"
		h.mu.Unlock()
		h.changed()
		if ctx.Err() != nil {
			return
		}
		switch code {
		case 4001, 4003, 1008, 4004:
			slog.Debug("vibepiik-native", "event", "signaling-stopped", "code", int(code), "error", err)
			if code == 4001 {
				h.endSharing("host.roomReplaced", false)
			}
			h.forgetRoom()
			return
		}
		if authed {
			attempt = 0
		}
		delay := time.Duration(min(8000, 500<<min(attempt, 4))+rand.IntN(250)) * time.Millisecond
		attempt++
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (h *hostSession) signalOnce(ctx context.Context) (websocket.StatusCode, error) {
	header := http.Header{}
	header.Set("Origin", h.api.target.Origin)
	if cookie := h.api.cookieHeader(); cookie != "" {
		header.Set("Cookie", cookie)
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	conn, _, err := websocket.Dial(dialCtx, h.api.signalURL(), &websocket.DialOptions{HTTPHeader: header})
	cancel()
	if err != nil {
		return -1, err
	}
	conn.SetReadLimit(1 << 20)
	defer conn.CloseNow()
	h.mu.Lock()
	h.conn = conn
	h.mu.Unlock()
	if err := h.writeRaw(conn, h.authenticateMessage()); err != nil {
		return websocket.CloseStatus(err), err
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return websocket.CloseStatus(err), err
		}
		h.signalMessage(data)
	}
}

func (h *hostSession) authenticateMessage() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	message := map[string]any{"type": "authenticate", "protocol": signalingProtocol, "roomId": h.roomID, "role": "host",
		"token": h.hostToken, "clientId": h.clientID, "roomSession": true, "sharingPaused": h.paused,
		"qualitySettings": h.quality, "routePolicy": h.routePolicy(), "viewerPresence": true,
		"displayName": h.displayName()}
	if h.share != "" {
		message["shareGeneration"] = h.share
	} else {
		message["roomOnly"] = true
	}
	return message
}

func (h *hostSession) displayName() string {
	if h.name != "" {
		return h.name
	}
	return "Host"
}

func (h *hostSession) routePolicy() map[string]any {
	return map[string]any{"peerOnly": true, "topologyOptimization": h.topology, "natPrediction": h.natPrediction}
}

// writeRaw validates message against the server's schema, then writes it.
func (h *hostSession) writeRaw(conn *websocket.Conn, message map[string]any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err := protocol.DecodeClientMessage(data); err != nil {
		slog.Warn("vibepiik-native", "event", "signal-message-invalid", "type", message["type"], "error", err)
		return nil
	}
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, data)
}

// send writes an authenticated signaling message; false when signaling is down.
func (h *hostSession) send(message map[string]any) bool {
	if h.sender != nil {
		return h.sender(message)
	}
	h.mu.Lock()
	conn, ready := h.conn, h.authenticated
	h.mu.Unlock()
	if conn == nil || !ready {
		return false
	}
	return h.writeRaw(conn, message) == nil
}

func (h *hostSession) sendStartSharing() {
	h.mu.Lock()
	share := h.share
	message := map[string]any{"type": "start-sharing", "shareGeneration": share, "qualitySettings": h.quality,
		"routePolicy": h.routePolicy()}
	if h.paused {
		message["sharingPaused"] = true
	}
	h.mu.Unlock()
	if share != "" {
		h.send(message)
	}
}

type routeAssignment struct {
	ChildPeerIDs []string `json:"childPeerIds"`
}

type serverMessage struct {
	Type                      string           `json:"type"`
	Code                      string           `json:"code"`
	Message                   string           `json:"message"`
	PeerID                    string           `json:"peerId"`
	EndpointMediaCopyCapacity int              `json:"endpointMediaCopyCapacity"`
	ViewerPasswordEnabled     bool             `json:"viewerPasswordEnabled"`
	CodeEntryPolicy           string           `json:"codeEntryPolicy"`
	IceConfig                 *json.RawMessage `json:"iceConfig"`
	ShareGeneration           *string          `json:"shareGeneration"`
	RouteRevision             int64            `json:"routeRevision"`
	RouteAssignment           routeAssignment  `json:"routeAssignment"`
	HostPaused                *bool            `json:"hostPaused"`
	Paused                    bool             `json:"paused"`
	Revision                  int64            `json:"revision"`
	Phase                     string           `json:"phase"`
	Assignment                routeAssignment  `json:"assignment"`
	Candidate                 struct {
		ChildPeerID  string `json:"childPeerId"`
		ConnectionID string `json:"connectionId"`
		Transport    string `json:"transport"`
	} `json:"candidate"`
	FromPeerID   string          `json:"fromPeerId"`
	ConnectionID string          `json:"connectionId"`
	Rebuild      bool            `json:"rebuild"`
	Payload      json.RawMessage `json:"payload"`
	Viewers      []struct {
		Role        string `json:"role"`
		PeerID      string `json:"peerId"`
		DisplayName string `json:"displayName"`
		MediaReady  bool   `json:"mediaReady"`
		Upstream    struct {
			Kind   string `json:"kind"`
			PeerID string `json:"peerId"`
		} `json:"upstream"`
	} `json:"viewers"`
	ViewerPeerID string `json:"viewerPeerId"`
	Metrics      struct {
		NatTraversalPath string   `json:"natTraversalPath"`
		Width            *float64 `json:"width"`
		Height           *float64 `json:"height"`
		FramesPerSecond  *float64 `json:"framesPerSecond"`
		BitrateKbps      *float64 `json:"bitrateKbps"`
		PacketsReceived  *float64 `json:"packetsReceivedDelta"`
		PacketsLost      *float64 `json:"packetsLostDelta"`
		RttMs            *float64 `json:"rttMs"`
		evidenceDetail
	} `json:"metrics"`
}

func (h *hostSession) signalMessage(data []byte) {
	var message serverMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return
	}
	if h.extraMessage(message.Type, data) {
		return
	}
	switch message.Type {
	case "authenticated":
		h.onAuthenticated(message)
	case "sharing-started":
		h.mu.Lock()
		matches := message.ShareGeneration != nil && *message.ShareGeneration == h.share && h.share != ""
		if matches {
			h.serverShare, h.phase, h.paused = h.share, phaseLive, message.Paused
			h.activeRev = message.RouteRevision
		}
		h.mu.Unlock()
		h.changed()
		if matches {
			h.reconcile(message.RouteAssignment.ChildPeerIDs, message.RouteRevision)
		}
	case "sharing-start-failed":
		h.mu.Lock()
		matches := message.ShareGeneration != nil && *message.ShareGeneration == h.share
		h.mu.Unlock()
		if matches {
			h.endSharing("host.shareEnded", false)
		}
	case "route-update":
		h.routeUpdate(message)
	case "signal":
		h.inboundSignal(message.FromPeerID, message.Payload)
	case "restart-request":
		h.restartRequest(message.FromPeerID, message.ConnectionID, message.Rebuild)
	case "pause-sharing-source":
		h.mu.Lock()
		share := h.share
		matches := message.ShareGeneration != nil && *message.ShareGeneration == share
		control := h.control
		h.mu.Unlock()
		if matches && control != nil {
			go func() {
				ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
				defer cancel()
				_, _ = control.request(ctx, "pause-share", map[string]any{"shareId": share, "paused": true})
				h.mu.Lock()
				h.paused = true
				h.mu.Unlock()
				h.changed()
				h.send(map[string]any{"type": "set-sharing-paused", "shareGeneration": share, "paused": true})
			}()
		}
	case "viewer-presence":
		var viewers []roomPeer
		for _, entry := range message.Viewers {
			if entry.Role == "viewer" {
				viewers = append(viewers, roomPeer{ID: entry.PeerID, Name: entry.DisplayName, MediaReady: entry.MediaReady,
					Upstream: entry.Upstream.PeerID, Routed: entry.Upstream.Kind != "none" && entry.Upstream.Kind != ""})
			}
		}
		h.mu.Lock()
		h.viewers = viewers
		h.mu.Unlock()
		h.changed()
	case "viewer-quality-evidence":
		h.viewerEvidence(message)
	case "room-closed":
		h.endSharing("host.roomClosed", false)
		h.forgetRoom()
		h.setNotice("host.roomClosed")
	case "error":
		h.onError(message)
	}
}

func (h *hostSession) onAuthenticated(message serverMessage) {
	var ice struct {
		IceServers []struct {
			URLs json.RawMessage `json:"urls"`
		} `json:"iceServers"`
	}
	if message.IceConfig != nil {
		_ = json.Unmarshal(*message.IceConfig, &ice)
	}
	var servers []iceServerConfig
	for _, server := range ice.IceServers {
		var urls []string
		if json.Unmarshal(server.URLs, &urls) != nil {
			var single string
			if json.Unmarshal(server.URLs, &single) == nil {
				urls = []string{single}
			}
		}
		if len(urls) > 0 {
			servers = append(servers, iceServerConfig{URLs: urls})
		}
	}
	h.mu.Lock()
	reauth := h.serverShare != ""
	h.authenticated, h.everAuthed, h.signal = true, true, "connected"
	h.peerID = message.PeerID
	h.passwordOn = message.ViewerPasswordEnabled
	if message.CodeEntryPolicy != "" {
		h.policy = message.CodeEntryPolicy
	}
	h.iceServers = servers
	if message.EndpointMediaCopyCapacity > 0 {
		h.capacity = message.EndpointMediaCopyCapacity
	}
	share := h.share
	resumed := share != "" && message.ShareGeneration != nil && *message.ShareGeneration == share
	var stale []string
	if resumed {
		h.serverShare, h.phase = share, phaseLive
		h.activeRev = message.RouteRevision
		h.lastRevision, h.lastPhase = 0, ""
		if reauth {
			// Offers or answers may have been lost: drop edges that never connected.
			for connection, e := range h.edges {
				if !e.connected {
					stale = append(stale, connection)
				}
			}
		}
	}
	h.mu.Unlock()
	h.changed()
	for _, connection := range stale {
		h.dropEdge(connection, false)
	}
	h.afterAuthenticated()
	switch {
	case resumed:
		h.reconcile(message.RouteAssignment.ChildPeerIDs, message.RouteRevision)
	case share != "":
		h.sendStartSharing()
	}
}

func (h *hostSession) onError(message serverMessage) {
	slog.Debug("vibepiik-native", "event", "signaling-error", "code", message.Code)
	switch message.Code {
	case "INVALID_TOKEN", "ROOM_NOT_FOUND":
		h.mu.Lock()
		sharing := h.share != ""
		h.mu.Unlock()
		h.forgetRoom()
		if sharing {
			// Keep the capture: publish it in a new room.
			go func() {
				if err := h.ensureRoom(); err != nil {
					h.endSharing("host.roomInvalid", false)
				}
			}()
		} else {
			h.setNotice("host.roomInvalid")
		}
	case "HOST_ALREADY_CONNECTED":
		// Another page or App took this room over (host.terminated.session).
		h.endSharing("host.terminated.session", false)
		h.forgetRoom()
	case "ROOM_ACCESS_DENIED", "AUTH_REQUIRED":
		h.setNotice("host.err.accessDenied")
	case "PEER_NOT_FOUND":
		h.setNotice("host.err.peerGone")
	case "SERVER_ERROR":
		h.setNotice("host.err.serverError")
	case "ROOM_FULL":
		h.setNotice("host.err.roomCapacity")
	default:
		// Unknown codes: the generic text, never raw server wording (NO-13).
		if message.Code != "INVALID_MESSAGE" && (message.Code != "" || message.Message != "") {
			h.setNotice("host.err.serverError")
		}
	}
}

// ---------------------------------------------------------------------------
// Viewer edges (HOST-FLOW §5)
// ---------------------------------------------------------------------------

func (h *hostSession) routeUpdate(message serverMessage) {
	h.mu.Lock()
	if h.share == "" || h.serverShare != h.share {
		h.mu.Unlock()
		return
	}
	switch {
	case message.Revision < h.lastRevision,
		message.Revision == h.lastRevision && message.Phase == h.lastPhase,
		message.Revision == h.lastRevision && h.lastPhase == "active" && message.Phase == "prepare":
		h.mu.Unlock()
		return
	}
	h.lastRevision, h.lastPhase = message.Revision, message.Phase
	h.children = message.Assignment.ChildPeerIDs
	h.mu.Unlock()

	switch message.Phase {
	case "prepare":
		candidate := message.Candidate
		if candidate.Transport != "direct" {
			h.discardPrepared()
			return
		}
		if !h.preparePlanValid(message.Assignment.ChildPeerIDs, candidate.ChildPeerID, candidate.ConnectionID) {
			h.discardPrepared()
			h.send(map[string]any{"type": "route-failed", "revision": message.Revision, "phase": "prepare", "connectionId": candidate.ConnectionID})
			return
		}
		h.discardPrepared()
		e := &edge{child: candidate.ChildPeerID, conn: candidate.ConnectionID, prepared: true, revision: message.Revision}
		h.mu.Lock()
		h.edges[e.conn] = e
		h.prepared = e
		h.mu.Unlock()
		go h.startEdge(e)
	case "active":
		h.mu.Lock()
		h.activeRev = message.Revision
		prepared := h.prepared
		promote := prepared != nil && prepared.revision == message.Revision && contains(message.Assignment.ChildPeerIDs, prepared.child)
		var replaced string
		if promote {
			replaced = h.active[prepared.child]
			h.active[prepared.child] = prepared.conn
			prepared.prepared = false
			h.prepared = nil
		}
		h.mu.Unlock()
		if promote {
			if replaced != "" && replaced != prepared.conn {
				h.dropEdge(replaced, true)
			}
		} else {
			h.discardPrepared()
		}
		h.reconcile(message.Assignment.ChildPeerIDs, message.Revision)
	}
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (h *hostSession) preparePlanValid(children []string, child, connection string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	seen := map[string]bool{}
	for _, id := range children {
		if seen[id] {
			return false
		}
		seen[id] = true
	}
	if !seen[child] || len(children) > h.capacity {
		return false
	}
	newOnes := 0
	for _, id := range children {
		if _, ok := h.active[id]; !ok {
			newOnes++
		}
	}
	for activeChild := range h.active {
		if !seen[activeChild] {
			return false
		}
	}
	if newOnes > 1 {
		return false
	}
	return h.active[child] != connection
}

func (h *hostSession) discardPrepared() {
	h.mu.Lock()
	prepared := h.prepared
	h.prepared = nil
	h.mu.Unlock()
	if prepared != nil && prepared.prepared {
		h.dropEdge(prepared.conn, true)
	}
}

// reconcile keeps exactly the assigned children (up to capacity) served.
func (h *hostSession) reconcile(children []string, revision int64) {
	h.mu.Lock()
	if h.share == "" {
		h.mu.Unlock()
		return
	}
	h.children = children
	var wanted []string
	seen := map[string]bool{}
	for _, child := range children {
		if !seen[child] && len(wanted) < h.capacity {
			seen[child] = true
			wanted = append(wanted, child)
		}
	}
	var drop []string
	for child, connection := range h.active {
		if !seen[child] {
			drop = append(drop, connection)
			delete(h.active, child)
		}
	}
	var start []*edge
	for _, child := range wanted {
		if _, ok := h.active[child]; ok {
			continue
		}
		e := &edge{child: child, conn: randomHex(16)}
		h.edges[e.conn] = e
		h.active[child] = e.conn
		start = append(start, e)
	}
	h.mu.Unlock()
	for _, connection := range drop {
		h.dropEdge(connection, true)
	}
	for _, e := range start {
		go h.startEdge(e)
	}
	_ = revision
}

// startEdge asks the App for an offer and sends it to the viewer.
func (h *hostSession) startEdge(e *edge) {
	h.mu.Lock()
	share, control := h.share, h.control
	servers := h.iceServers
	h.mu.Unlock()
	if share == "" || control == nil {
		return
	}
	if servers == nil {
		servers = []iceServerConfig{}
	}
	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()
	raw, err := control.request(ctx, "prepare-edge", map[string]any{"shareId": share, "connectionId": e.conn, "iceServers": servers})
	if err != nil {
		h.edgeFailed(e)
		return
	}
	var offer struct {
		SDP string `json:"sdp"`
	}
	if json.Unmarshal(raw, &offer) != nil || offer.SDP == "" {
		h.edgeFailed(e)
		return
	}
	sent := h.send(map[string]any{"type": "signal", "targetPeerId": e.child, "payload": map[string]any{
		"kind": "description", "connectionId": e.conn, "description": map[string]any{"type": "offer", "sdp": offer.SDP}}})
	if !sent {
		h.edgeFailed(e)
		return
	}
	h.mu.Lock()
	e.offerSent = true
	pending := e.pending
	e.pending = nil
	h.mu.Unlock()
	for _, candidate := range pending {
		h.sendCandidate(e, candidate)
	}
}

func (h *hostSession) outboundCandidate(connection string, candidate json.RawMessage) {
	h.mu.Lock()
	e := h.edges[connection]
	if e == nil || e.closed {
		h.mu.Unlock()
		return
	}
	if !e.offerSent {
		if len(e.pending) < 64 {
			e.pending = append(e.pending, candidate)
		}
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()
	h.sendCandidate(e, candidate)
}

func (h *hostSession) sendCandidate(e *edge, candidate json.RawMessage) {
	var value any
	if len(candidate) == 0 || string(candidate) == "null" {
		value = nil
	} else {
		var fields map[string]any
		if json.Unmarshal(candidate, &fields) != nil {
			return
		}
		// The signaling schema allows only these keys.
		clean := map[string]any{"candidate": fields["candidate"]}
		for _, key := range []string{"sdpMid", "sdpMLineIndex", "usernameFragment"} {
			if v, ok := fields[key]; ok && v != nil {
				clean[key] = v
			}
		}
		value = clean
	}
	h.send(map[string]any{"type": "signal", "targetPeerId": e.child, "payload": map[string]any{
		"kind": "candidate", "connectionId": e.conn, "candidate": value}})
}

func (h *hostSession) edgeState(connection, state string) {
	h.mu.Lock()
	e := h.edges[connection]
	if e == nil || e.closed {
		h.mu.Unlock()
		return
	}
	report := false
	if state == "connected" {
		e.connected = true
		if e.prepared && !e.reported {
			e.reported, report = true, true
		}
	}
	revision := e.revision
	h.mu.Unlock()
	if report {
		h.send(map[string]any{"type": "route-transport-connected", "revision": revision, "connectionId": connection})
	}
	if state == "failed" || state == "closed" {
		h.edgeFailed(e)
	}
}

// edgeFailed reports a failed edge to the route controller and closes it.
func (h *hostSession) edgeFailed(e *edge) {
	h.mu.Lock()
	if e.closed {
		h.mu.Unlock()
		return
	}
	var message map[string]any
	if e.prepared {
		message = map[string]any{"type": "route-failed", "revision": e.revision, "phase": "prepare", "connectionId": e.conn}
		if h.prepared == e {
			h.prepared = nil
		}
	} else if h.active[e.child] == e.conn {
		delete(h.active, e.child)
		if contains(h.children, e.child) {
			message = map[string]any{"type": "route-failed", "revision": h.activeRev, "phase": "active", "connectionId": e.conn}
		}
	}
	h.mu.Unlock()
	if message != nil {
		h.send(message)
	}
	h.dropEdge(e.conn, true)
}

// dropEdge forgets an edge and asks the App to close it.
func (h *hostSession) dropEdge(connection string, close bool) {
	h.mu.Lock()
	e := h.edges[connection]
	share := h.share
	if e != nil {
		e.closed = true
		delete(h.edges, connection)
		if h.active[e.child] == connection {
			delete(h.active, e.child)
		}
	}
	h.mu.Unlock()
	if e != nil && close {
		h.closeEdgeRequest(share, connection)
	}
}

func (h *hostSession) inboundSignal(from string, payload json.RawMessage) {
	var signal struct {
		Kind         string `json:"kind"`
		ConnectionID string `json:"connectionId"`
		Description  *struct {
			Type string `json:"type"`
			SDP  string `json:"sdp"`
		} `json:"description"`
		Candidate json.RawMessage `json:"candidate"`
	}
	if json.Unmarshal(payload, &signal) != nil {
		return
	}
	h.mu.Lock()
	e := h.edges[signal.ConnectionID]
	share, control := h.share, h.control
	h.mu.Unlock()
	if e == nil || e.child != from || control == nil {
		return
	}
	var kind string
	var fields map[string]any
	switch signal.Kind {
	case "description":
		if signal.Description == nil || signal.Description.Type != "answer" {
			return
		}
		kind = "edge-answer"
		fields = map[string]any{"shareId": share, "connectionId": e.conn, "sdp": signal.Description.SDP}
	case "candidate":
		kind = "edge-candidate"
		var candidate any
		if len(signal.Candidate) > 0 && string(signal.Candidate) != "null" {
			var parsed map[string]any
			if json.Unmarshal(signal.Candidate, &parsed) != nil {
				return
			}
			clean := map[string]any{"candidate": parsed["candidate"]}
			for _, key := range []string{"sdpMid", "sdpMLineIndex", "usernameFragment"} {
				if v, ok := parsed[key]; ok && v != nil {
					clean[key] = v
				}
			}
			candidate = clean
		}
		fields = map[string]any{"shareId": share, "connectionId": e.conn, "candidate": candidate}
	default:
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
		defer cancel()
		if _, err := control.request(ctx, kind, fields); err != nil {
			var failure controlFailure
			if errors.As(err, &failure) {
				h.edgeFailed(e)
			}
		}
	}()
}

func (h *hostSession) restartRequest(from, connection string, rebuild bool) {
	h.mu.Lock()
	current, ok := h.active[from]
	assigned := contains(h.children, from)
	h.mu.Unlock()
	if !assigned || (ok && current != connection) || (!ok && !rebuild) {
		return
	}
	if ok {
		h.dropEdge(current, true)
	}
	h.mu.Lock()
	children := h.children
	revision := h.activeRev
	h.mu.Unlock()
	h.reconcile(children, revision)
}

// SendTV sends the invite link to the PIIK TV apps on this network.
func (h *hostSession) SendTV(ctx context.Context) ([]string, int, error) {
	h.mu.Lock()
	link := h.invite
	h.mu.Unlock()
	if link == "" {
		return nil, 409, errors.New("no room")
	}
	return h.api.SendTV(ctx, link)
}

// Close ends sharing and leaves the room (the window closed).
func (h *hostSession) Close() {
	h.endSharing("", true)
	h.mu.Lock()
	control := h.control
	h.control = nil
	h.mu.Unlock()
	h.voice.Shutdown()
	h.cancel()
	if control != nil {
		control.Close()
	}
}

// copyStats copies the viewer stats map for a snapshot.
func copyStats(stats map[string]viewerStats) map[string]viewerStats {
	out := make(map[string]viewerStats, len(stats))
	for id, value := range stats {
		out[id] = value
	}
	return out
}
