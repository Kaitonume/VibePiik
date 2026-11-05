package nativeui

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	gopus "github.com/thesyncim/gopus"
)

// Native room voice (room-voice/session.ts + browser-endpoint.ts): the room
// server plans a tree of peer connections; each participant sends its own
// microphone and forwards the publications the plan routes through it.

type voiceMember struct {
	PeerID        string `json:"peerId"`
	Connected     bool   `json:"connected"`
	Joined        bool   `json:"joined"`
	PublicationID string `json:"publicationId"`
	Deafened      *bool  `json:"deafened"`
}

type voicePublication struct {
	ID     string `json:"id"`
	PeerID string `json:"peerId"`
}

type voiceConnConfig struct {
	ConnectionID string   `json:"connectionId"`
	ParentPeerID string   `json:"parentPeerId"`
	ChildPeerID  string   `json:"childPeerId"`
	Transport    string   `json:"transport"`
	Pending      bool     `json:"pending"`
	Revision     int64    `json:"revision"`
	Send         []string `json:"send"`
	Receive      []string `json:"receive"`
}

type voiceState struct {
	Bitrate      int                `json:"bitrate"`
	Revision     int64              `json:"revision"`
	Enabled      bool               `json:"enabled"`
	HostPeerID   *string            `json:"hostPeerId"`
	Members      []voiceMember      `json:"members"`
	Publications []voicePublication `json:"publications"`
	Connections  []voiceConnConfig  `json:"connections"`
	Unavailable  []string           `json:"unavailable"`
}

type voiceTrack struct {
	MID           string `json:"mid"`
	PublicationID string `json:"publicationId"`
}

type voiceDescription struct {
	Revision int64 `json:"revision"`
	SDP      struct {
		Type string `json:"type"`
		SDP  string `json:"sdp"`
	} `json:"sdp"`
	Tracks []voiceTrack `json:"tracks"`
}

type voiceSignal struct {
	ConnectionID string            `json:"connectionId"`
	Description  *voiceDescription `json:"description"`
	Candidate    json.RawMessage   `json:"candidate"`
	Renegotiate  bool              `json:"renegotiate"`
}

type voiceConn struct {
	config     voiceConnConfig
	pc         *webrtc.PeerConnection
	senders    map[string]*webrtc.RTPSender
	remote     map[string]string // mid → publication
	dirty      bool
	requested  bool
	descSent   bool
	offerRev   int64
	remoteRev  int64
	localCands []map[string]any
	queued     []webrtc.ICECandidateInit
	reported   string
	closed     bool
}

// playback is one received publication being decoded for the speakers.
type playback struct {
	decoder *gopus.Decoder
	frames  [][]int16
	level   float32
	heard   time.Time
}

// voiceSnapshot is what the Voice tab shows.
type voiceSnapshot struct {
	Enabled     bool
	Joined      bool
	Microphone  bool
	Pending     bool
	Deafened    bool
	Error       string
	Members     []voiceMemberView
	Volume      float64     // master listening volume, 0–2
	Bitrate     int         // room voice bitrate set by the host (bit/s)
	Device      string      // chosen microphone ("" = Windows default)
	MicGain     float64     // microphone volume, 0–2
	MicLevel    float32     // latest microphone level (RMS, 0–1)
	Gate        float64     // noise gate position on the meter (0 = off)
	GateOpen    bool        // the gate lets the voice through now
	Processing  bool        // noise & echo reduction on
	Unavailable bool        // some voice paths are unavailable
	Edges       []voiceEdge // the voice tree's connections (topology)
}

// voiceEdge is one room-voice connection between two members.
type voiceEdge struct {
	Parent, Child string
	Pending       bool
}

type voiceMemberView struct {
	PeerID     string
	Joined     bool
	Publishing bool
	Deafened   bool
	Speaking   bool
	Muted      bool
	Volume     float64 // this person's volume, 0–2
	Level      float32 // latest voice level (RMS, 0–1) for the meter
}

type voiceEndpoint struct {
	host *hostSession
	api  *webrtc.API

	mu         sync.Mutex
	peerID     string
	state      *voiceState
	conns      map[string]*voiceConn
	forward    map[string]*webrtc.TrackLocalStaticRTP
	plays      map[string]*playback
	mic        interface{ Close() }
	micTrack   *webrtc.TrackLocalStaticSample
	micPub     string
	encoder    *gopus.Encoder
	micLevel   float32
	micHeard   time.Time
	gateOpen   bool
	gate       float64 // noise gate (meter position 0–1, 0 = off)
	publishing bool
	retired    map[string]bool
	speaker    *speaker
	deafened   bool
	muted      map[string]bool
	volume     float64            // master listening volume
	volumes    map[string]float64 // per person (peerId)
	device     string             // chosen microphone (Windows endpoint ID, "" = default)
	micGain    float64            // microphone volume, 0–2
	processing bool               // noise & echo reduction (Voice Capture DSP)
	errorKey   string
	busy       bool
	again      bool
}

func newVoiceEndpoint(host *hostSession) *voiceEndpoint {
	engine := &webrtc.MediaEngine{}
	_ = engine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2,
			SDPFmtpLine: "minptime=10;useinbandfec=1"},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio)
	return &voiceEndpoint{host: host, api: webrtc.NewAPI(webrtc.WithMediaEngine(engine)),
		conns: map[string]*voiceConn{}, forward: map[string]*webrtc.TrackLocalStaticRTP{}, plays: map[string]*playback{},
		retired: map[string]bool{}, muted: map[string]bool{}, volume: savedVoiceVolume(), volumes: map[string]float64{},
		device: loadPreferences().VoiceDevice, micGain: savedMicGain(), processing: !loadPreferences().VoiceRaw, gate: loadPreferences().VoiceGate}
}

func (v *voiceEndpoint) signal(message map[string]any) bool { return v.host.send(message) }

func (v *voiceEndpoint) command(command map[string]any) bool {
	return v.signal(map[string]any{"type": "room-voice-command", "command": command})
}

// authenticated restarts voice for a (re)connected signaling session.
func (v *voiceEndpoint) authenticated(peerID string) {
	v.closeConnections()
	v.mu.Lock()
	v.peerID, v.state = peerID, nil
	v.mu.Unlock()
	v.signal(map[string]any{"type": "subscribe-room-voice", "deafened": v.deafened, "bitrate": true})
}

func (v *voiceEndpoint) me() *voiceMember {
	if v.state == nil {
		return nil
	}
	for i := range v.state.Members {
		if v.state.Members[i].PeerID == v.peerID {
			return &v.state.Members[i]
		}
	}
	return nil
}

func (v *voiceEndpoint) myPublication() string {
	if v.state == nil {
		return ""
	}
	for _, publication := range v.state.Publications {
		if publication.PeerID == v.peerID {
			return publication.ID
		}
	}
	return ""
}

// receive handles room-voice-state, room-voice-signal and room-voice-error.
func (v *voiceEndpoint) receive(kind string, data []byte) {
	switch kind {
	case "room-voice-state":
		var state voiceState
		if json.Unmarshal(data, &state) != nil {
			return
		}
		v.mu.Lock()
		if v.state != nil && state.Revision < v.state.Revision {
			v.mu.Unlock()
			return
		}
		v.state = &state
		me := v.me()
		joined := state.Enabled && me != nil && me.Joined
		publication := v.myPublication()
		if !joined {
			v.publishing = false
		}
		startMic := false
		switch {
		case publication != "" && v.publishing:
			v.publishing = false
			v.micPub = publication
			startMic = true
		case publication != "" && v.mic == nil && v.micPub != publication && !v.retired[publication]:
			// A former session's publication cannot revive a microphone.
			v.retired[publication] = true
			go v.command(map[string]any{"action": "unpublish", "publicationId": publication})
		}
		stopMic := v.mic != nil && (!joined || publication == "" || publication != v.micPub)
		if state.Bitrate > 0 && v.encoder != nil {
			_ = v.encoder.SetBitrate(state.Bitrate)
		}
		v.mu.Unlock()
		if stopMic {
			v.stopMicrophone(false)
		}
		if startMic {
			if err := v.startCapture(); err != nil {
				// No microphone at all, or one that would not start (voice.error.*).
				key := "voice.error.microphoneFailed"
				if devices, listErr := listCaptureDevices(); listErr == nil && len(devices) == 0 {
					key = "voice.error.microphoneMissing"
				}
				v.setError(key)
				v.StopMicrophone()
			}
		}
		v.retireStale()
		v.updateSpeaker()
		v.reconcile()
	case "room-voice-signal":
		var message voiceSignal
		if json.Unmarshal(data, &message) != nil {
			return
		}
		go func() {
			if err := v.receiveSignal(message); err != nil {
				slog.Debug("vibepiik-native", "event", "voice-signal-failed", "error", err)
			}
			v.reconcile()
		}()
	case "room-voice-error":
		var message struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(data, &message)
		v.mu.Lock()
		if message.Code == "not-allowed" {
			v.publishing = false
		}
		v.mu.Unlock()
		key := map[string]string{"connection-unavailable": "voice.error.transport", "not-enabled": "voice.error.closed"}[message.Code]
		if key == "" {
			key = "voice.error.rejected"
		}
		v.setError(key)
	}
	v.host.changed()
}

func (v *voiceEndpoint) setError(key string) {
	v.mu.Lock()
	v.errorKey = key
	v.mu.Unlock()
	v.host.changed()
}

// retireStale closes connections the plan no longer has.
func (v *voiceEndpoint) retireStale() {
	v.mu.Lock()
	allowed := map[string]voiceConnConfig{}
	if v.state != nil {
		for _, config := range v.state.Connections {
			allowed[config.ConnectionID] = config
		}
	}
	var retire []*voiceConn
	for id, conn := range v.conns {
		config, ok := allowed[id]
		if !ok {
			retire = append(retire, conn)
			delete(v.conns, id)
			continue
		}
		if joinIDs(conn.config.Send) != joinIDs(config.Send) || joinIDs(conn.config.Receive) != joinIDs(config.Receive) {
			conn.dirty = true
		}
		conn.config = config
	}
	v.mu.Unlock()
	for _, conn := range retire {
		conn.closed = true
		_ = conn.pc.Close()
	}
}

func joinIDs(ids []string) string {
	out := ""
	for _, id := range ids {
		out += id + ","
	}
	return out
}

func contains2(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func (v *voiceEndpoint) isOfferer(config voiceConnConfig) bool {
	return config.Transport == "peer" && config.ParentPeerID == v.peerID
}

// localTrack is the track that carries a publication on our side: our own
// microphone, or the forwarder of a publication we receive.
func (v *voiceEndpoint) localTrack(publication string) webrtc.TrackLocal {
	if publication == v.micPub && v.micTrack != nil {
		return v.micTrack
	}
	if track, ok := v.forward[publication]; ok {
		return track
	}
	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"voice-"+publication, "voice-"+publication)
	if err != nil {
		return nil
	}
	v.forward[publication] = track
	return track
}

func (v *voiceEndpoint) create(config voiceConnConfig) (*voiceConn, error) {
	v.host.mu.Lock()
	servers := v.host.iceServers
	v.host.mu.Unlock()
	var ice []webrtc.ICEServer
	if config.Transport != "sfu" {
		for _, server := range servers {
			ice = append(ice, webrtc.ICEServer{URLs: server.URLs})
		}
	}
	pc, err := v.api.NewPeerConnection(webrtc.Configuration{ICEServers: ice})
	if err != nil {
		return nil, err
	}
	conn := &voiceConn{config: config, pc: pc, senders: map[string]*webrtc.RTPSender{}, remote: map[string]string{}, dirty: v.isOfferer(config)}
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		init := candidate.ToJSON()
		value := map[string]any{"candidate": init.Candidate}
		if init.SDPMid != nil {
			value["sdpMid"] = *init.SDPMid
		}
		if init.SDPMLineIndex != nil {
			value["sdpMLineIndex"] = *init.SDPMLineIndex
		}
		v.mu.Lock()
		if conn.closed || len(conn.localCands) >= 64 {
			v.mu.Unlock()
			return
		}
		conn.localCands = append(conn.localCands, value)
		v.mu.Unlock()
		v.flushCandidates(conn)
	})
	pc.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		mid := ""
		for _, transceiver := range pc.GetTransceivers() {
			if transceiver.Receiver() == receiver {
				mid = transceiver.Mid()
			}
		}
		go v.pump(conn, mid, remote)
	})
	pc.OnConnectionStateChange(func(webrtc.PeerConnectionState) { go v.reconcile() })
	if v.isOfferer(config) {
		if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
			_ = pc.Close()
			return nil, err
		}
		_, _ = pc.CreateDataChannel("piik-room-voice", nil)
	}
	return conn, nil
}

func (v *voiceEndpoint) flushCandidates(conn *voiceConn) {
	v.mu.Lock()
	if !conn.descSent || conn.closed {
		v.mu.Unlock()
		return
	}
	pending := conn.localCands
	conn.localCands = nil
	v.mu.Unlock()
	for _, candidate := range pending {
		v.signal(map[string]any{"type": "room-voice-signal", "connectionId": conn.config.ConnectionID, "candidate": candidate})
	}
}

// pump reads one received publication: forward it where the plan says, and
// decode it for the speakers.
func (v *voiceEndpoint) pump(conn *voiceConn, mid string, remote *webrtc.TrackRemote) {
	for {
		packet, _, err := remote.ReadRTP()
		if err != nil {
			return
		}
		v.mu.Lock()
		publication := conn.remote[mid]
		if publication == "" || conn.closed || !contains2(conn.config.Receive, publication) {
			v.mu.Unlock()
			continue
		}
		forward := v.forward[publication]
		play := v.plays[publication]
		if play == nil && !conn.config.Pending {
			decoder, err := gopus.NewDecoder(gopus.DecoderConfig{SampleRate: audioRate, Channels: 1})
			if err == nil {
				play = &playback{decoder: decoder}
				v.plays[publication] = play
			}
		}
		v.mu.Unlock()
		if forward != nil {
			_ = forward.WriteRTP(packet)
		}
		if play != nil && len(packet.Payload) > 0 {
			frame := make([]int16, audioFrame*6)
			n, err := play.decoder.DecodeInt16(packet.Payload, frame)
			if err != nil || n <= 0 {
				continue
			}
			level := rms(frame[:n])
			v.mu.Lock()
			for start := 0; start+audioFrame <= n; start += audioFrame {
				play.frames = append(play.frames, frame[start:start+audioFrame])
			}
			if len(play.frames) > 6 {
				play.frames = play.frames[len(play.frames)-3:] // keep latency bounded
			}
			play.level = level
			if level > 0.02 {
				play.heard = time.Now()
			}
			v.mu.Unlock()
		}
	}
}

func rms(samples []int16) float32 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		f := float64(s) / 32768
		sum += f * f
	}
	return float32(math.Sqrt(sum / float64(len(samples))))
}

// mix fills one speaker block from every received publication.
func (v *voiceEndpoint) mix(out []int16) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	any := false
	acc := make([]int32, len(out))
	for publication, play := range v.plays {
		if len(play.frames) == 0 {
			continue
		}
		frame := play.frames[0]
		play.frames = play.frames[1:]
		peer := v.publisher(publication)
		if v.deafened || v.muted[peer] {
			continue
		}
		any = true
		gain := v.volume * v.personVolume(peer)
		if gain == 1 {
			for i := range acc {
				acc[i] += int32(frame[i])
			}
			continue
		}
		for i := range acc {
			acc[i] += int32(float64(frame[i]) * gain)
		}
	}
	if !any {
		return false
	}
	for i, value := range acc {
		out[i] = int16(max(-32768, min(32767, value)))
	}
	return true
}

func (v *voiceEndpoint) publisher(publication string) string {
	if v.state == nil {
		return ""
	}
	for _, p := range v.state.Publications {
		if p.ID == publication {
			return p.PeerID
		}
	}
	return ""
}

// updateSpeaker opens the speakers while joined and listening.
func (v *voiceEndpoint) updateSpeaker() {
	v.mu.Lock()
	me := v.me()
	want := v.state != nil && v.state.Enabled && me != nil && me.Joined
	have := v.speaker != nil
	v.mu.Unlock()
	switch {
	case want && !have:
		s, err := openSpeaker(v.mix)
		if err != nil {
			v.setError("voice.error.transport")
			return
		}
		v.mu.Lock()
		v.speaker = s
		v.mu.Unlock()
	case !want && have:
		v.mu.Lock()
		s := v.speaker
		v.speaker = nil
		v.mu.Unlock()
		s.Close()
	}
}

// reconcile brings every planned connection in line (one pass at a time).
func (v *voiceEndpoint) reconcile() {
	v.mu.Lock()
	if v.busy {
		v.again = true
		v.mu.Unlock()
		return
	}
	v.busy = true
	v.mu.Unlock()
	for {
		v.reconcileOnce()
		v.mu.Lock()
		if !v.again {
			v.busy = false
			v.mu.Unlock()
			v.host.changed()
			return
		}
		v.again = false
		v.mu.Unlock()
	}
}

func (v *voiceEndpoint) reconcileOnce() {
	v.mu.Lock()
	if v.state == nil {
		v.mu.Unlock()
		return
	}
	configs := append([]voiceConnConfig(nil), v.state.Connections...)
	v.mu.Unlock()
	for _, config := range configs {
		v.mu.Lock()
		conn := v.conns[config.ConnectionID]
		v.mu.Unlock()
		if conn == nil {
			created, err := v.create(config)
			if err != nil {
				continue
			}
			v.mu.Lock()
			v.conns[config.ConnectionID] = created
			v.mu.Unlock()
			conn = created
		}
		if err := v.reconcileConn(conn); err != nil {
			slog.Debug("vibepiik-native", "event", "voice-reconcile-failed", "error", err)
			v.report(conn, "failed")
		}
	}
}

func (v *voiceEndpoint) reconcileConn(conn *voiceConn) error {
	v.mu.Lock()
	config := conn.config
	// Senders follow the plan's send list.
	for publication, sender := range conn.senders {
		if !contains2(config.Send, publication) || (publication == v.micPub && v.micTrack == nil) {
			_ = conn.pc.RemoveTrack(sender)
			delete(conn.senders, publication)
			conn.dirty = true
		}
	}
	for _, publication := range config.Send {
		if _, ok := conn.senders[publication]; ok {
			continue
		}
		if publication != v.micPub && v.receivingLocked(publication) == false {
			continue
		}
		track := v.localTrack(publication)
		if track == nil {
			continue
		}
		sender, err := conn.pc.AddTrack(track)
		if err != nil {
			v.mu.Unlock()
			return err
		}
		go drainRTCP(sender)
		conn.senders[publication] = sender
		conn.dirty = true
	}
	offerer := v.isOfferer(config)
	if offerer {
		receiving := 0
		for _, transceiver := range conn.pc.GetTransceivers() {
			direction := transceiver.Direction()
			if direction == webrtc.RTPTransceiverDirectionRecvonly || direction == webrtc.RTPTransceiverDirectionSendrecv {
				receiving++
			}
		}
		for i := receiving; i < len(config.Receive); i++ {
			if _, err := conn.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err == nil {
				conn.dirty = true
			}
		}
	}
	state := conn.pc.ConnectionState()
	dirty := conn.dirty && conn.pc.SignalingState() == webrtc.SignalingStateStable
	requested := conn.requested
	v.mu.Unlock()
	switch state {
	case webrtc.PeerConnectionStateConnected:
		v.report(conn, "connected")
	case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
		v.report(conn, "failed")
		return nil
	}
	if !dirty {
		return nil
	}
	if offerer {
		v.mu.Lock()
		conn.dirty = false
		conn.offerRev++
		revision := conn.offerRev
		v.mu.Unlock()
		offer, err := conn.pc.CreateOffer(nil)
		if err != nil {
			return err
		}
		return v.sendDescription(conn, offer, revision)
	}
	if !requested {
		if v.signal(map[string]any{"type": "room-voice-signal", "connectionId": conn.config.ConnectionID, "renegotiate": true}) {
			v.mu.Lock()
			conn.requested = true
			v.mu.Unlock()
		}
	}
	return nil
}

// receivingLocked reports whether a publication arrives on some active connection.
func (v *voiceEndpoint) receivingLocked(publication string) bool {
	for _, conn := range v.conns {
		if !conn.config.Pending && contains2(conn.config.Receive, publication) {
			return true
		}
	}
	return false
}

func drainRTCP(sender *webrtc.RTPSender) {
	buffer := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buffer); err != nil {
			return
		}
	}
}

func (v *voiceEndpoint) sendDescription(conn *voiceConn, description webrtc.SessionDescription, revision int64) error {
	v.mu.Lock()
	conn.descSent = false
	v.mu.Unlock()
	if err := conn.pc.SetLocalDescription(description); err != nil {
		return err
	}
	local := conn.pc.LocalDescription()
	tracks := []voiceTrack{}
	v.mu.Lock()
	for publication, sender := range conn.senders {
		if !contains2(conn.config.Send, publication) {
			continue
		}
		for _, transceiver := range conn.pc.GetTransceivers() {
			if transceiver.Sender() == sender && transceiver.Mid() != "" {
				tracks = append(tracks, voiceTrack{MID: transceiver.Mid(), PublicationID: publication})
			}
		}
	}
	v.mu.Unlock()
	message := map[string]any{"type": "room-voice-signal", "connectionId": conn.config.ConnectionID, "description": map[string]any{
		"revision": revision, "sdp": map[string]any{"type": local.Type.String(), "sdp": local.SDP}, "tracks": tracks}}
	if !v.signal(message) {
		return errors.New("voice signaling unavailable")
	}
	v.mu.Lock()
	conn.descSent = true
	v.mu.Unlock()
	v.flushCandidates(conn)
	return nil
}

func (v *voiceEndpoint) receiveSignal(message voiceSignal) error {
	v.mu.Lock()
	conn := v.conns[message.ConnectionID]
	v.mu.Unlock()
	if conn == nil {
		return nil
	}
	if message.Renegotiate {
		v.mu.Lock()
		if v.isOfferer(conn.config) {
			conn.dirty = true
		}
		v.mu.Unlock()
		return nil
	}
	if len(message.Candidate) > 0 && string(message.Candidate) != "null" {
		var candidate webrtc.ICECandidateInit
		if json.Unmarshal(message.Candidate, &candidate) != nil {
			return nil
		}
		if conn.pc.RemoteDescription() != nil {
			return conn.pc.AddICECandidate(candidate)
		}
		v.mu.Lock()
		if len(conn.queued) < 64 {
			conn.queued = append(conn.queued, candidate)
		}
		v.mu.Unlock()
		return nil
	}
	description := message.Description
	if description == nil {
		return nil
	}
	offer := description.SDP.Type == "offer"
	v.mu.Lock()
	offerer := v.isOfferer(conn.config)
	if offer == offerer || description.Revision <= conn.remoteRev ||
		(!offer && (description.Revision != conn.offerRev || conn.pc.SignalingState() != webrtc.SignalingStateHaveLocalOffer)) {
		v.mu.Unlock()
		return nil
	}
	allowed := conn.config.Receive
	remote := map[string]string{}
	for _, track := range description.Tracks {
		if contains2(allowed, track.PublicationID) {
			remote[track.MID] = track.PublicationID
		}
	}
	conn.remote = remote
	v.mu.Unlock()
	sdpType := webrtc.SDPTypeAnswer
	if offer {
		sdpType = webrtc.SDPTypeOffer
	}
	if err := conn.pc.SetRemoteDescription(webrtc.SessionDescription{Type: sdpType, SDP: description.SDP.SDP}); err != nil {
		return err
	}
	v.mu.Lock()
	conn.remoteRev = description.Revision
	queued := conn.queued
	conn.queued = nil
	v.mu.Unlock()
	for _, candidate := range queued {
		_ = conn.pc.AddICECandidate(candidate)
	}
	if offer {
		v.mu.Lock()
		conn.requested = false
		v.mu.Unlock()
		if err := v.reconcileConn(conn); err != nil {
			return err
		}
		v.mu.Lock()
		conn.dirty = false
		v.mu.Unlock()
		answer, err := conn.pc.CreateAnswer(nil)
		if err != nil {
			return err
		}
		return v.sendDescription(conn, answer, description.Revision)
	}
	return nil
}

func (v *voiceEndpoint) report(conn *voiceConn, state string) {
	v.mu.Lock()
	if conn.reported == state || conn.closed {
		v.mu.Unlock()
		return
	}
	v.mu.Unlock()
	if v.signal(map[string]any{"type": "room-voice-transport", "connectionId": conn.config.ConnectionID, "state": state}) {
		v.mu.Lock()
		conn.reported = state
		v.mu.Unlock()
	}
}

// startCapture opens the microphone for our authorized publication.
func (v *voiceEndpoint) startCapture() error {
	v.mu.Lock()
	if v.mic != nil {
		v.mu.Unlock()
		return nil
	}
	bitrate := 32_000
	if v.state != nil && v.state.Bitrate > 0 {
		bitrate = v.state.Bitrate
	}
	v.mu.Unlock()
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"voice-mic", "voice-mic")
	if err != nil {
		return err
	}
	// The encoder follows the source rate: 16 kHz from the echo canceller,
	// 48 kHz from the plain microphone. Opus RTP keeps its 48 kHz clock.
	var encoder *gopus.Encoder
	var encoderMu sync.Mutex
	newEncoder := func(rate int) error {
		created, err := gopus.NewEncoder(gopus.EncoderConfig{SampleRate: rate, Channels: 1, Application: gopus.ApplicationVoIP})
		if err != nil {
			return err
		}
		_ = created.SetBitrate(bitrate)
		_ = created.SetInBandFEC(gopus.InBandFECEnabled)
		encoderMu.Lock()
		encoder = created
		encoderMu.Unlock()
		return nil
	}
	packet := make([]byte, 1500)
	var mic interface{ Close() }
	var gate voiceGate
	capture := func(frame []int16) {
		v.mu.Lock()
		gain, gatePosition := v.micGain, v.gate
		v.mu.Unlock()
		if gain != 1 {
			applyGain(frame, gain)
		}
		level := rms(frame)
		now := time.Now()
		open := gate.apply(frame, level, gatePosition, now)
		v.mu.Lock()
		v.micLevel, v.gateOpen = level, open
		if level > 0.02 && open {
			v.micHeard = now
		}
		v.mu.Unlock()
		encoderMu.Lock()
		current := encoder
		encoderMu.Unlock()
		if current == nil {
			return
		}
		n, err := current.EncodeInt16(frame, packet)
		if err != nil || n <= 0 {
			return
		}
		_ = track.WriteSample(media.Sample{Data: append([]byte(nil), packet[:n]...), Duration: 20 * time.Millisecond})
	}
	switch {
	case os.Getenv("VIBEPIIK_TEST_TONE") == "1":
		// Automated tests only: a 440 Hz tone instead of the real microphone.
		if err := newEncoder(audioRate); err != nil {
			return err
		}
		mic = startTone(capture)
	default:
		if err := newEncoder(aecRate); err != nil {
			return err
		}
		v.mu.Lock()
		deviceID, processing := v.device, v.processing
		v.mu.Unlock()
		if processing {
			if echo, err := openEchoCancelledMicrophone(captureIndex(deviceID), capture); err == nil {
				mic = echo
				slog.Debug("vibepiik-native", "event", "voice-echo-cancellation", "enabled", true)
				break
			} else {
				slog.Debug("vibepiik-native", "event", "voice-echo-cancellation", "enabled", false, "error", err)
			}
		}
		if err := newEncoder(audioRate); err != nil {
			return err
		}
		plain, err := openMicrophone(waveInDevice(deviceLabel(deviceID)), capture)
		if err != nil {
			return err
		}
		mic = plain
	}
	encoderMu.Lock()
	current := encoder
	encoderMu.Unlock()
	v.mu.Lock()
	v.mic, v.micTrack, v.encoder = mic, track, current
	v.mu.Unlock()
	v.host.changed()
	go v.reconcile()
	return nil
}

func (v *voiceEndpoint) stopMicrophone(unpublish bool) {
	v.mu.Lock()
	mic := v.mic
	publication := v.micPub
	v.mic, v.micTrack, v.encoder, v.micPub, v.publishing = nil, nil, nil, "", false
	if publication != "" {
		v.retired[publication] = true
	}
	v.mu.Unlock()
	if mic != nil {
		mic.Close()
	}
	if unpublish && publication != "" {
		v.command(map[string]any{"action": "unpublish", "publicationId": publication})
	}
	go v.reconcile()
}

// Public controls (the Voice tab).

// Open joins (and for the host opens) room voice; Close leaves it.
func (v *voiceEndpoint) Open() {
	v.setError("")
	v.command(map[string]any{"action": "join"})
}

func (v *voiceEndpoint) Close() {
	v.stopMicrophone(true)
	v.command(map[string]any{"action": "leave"})
}

// StartMicrophone publishes our microphone (capture starts once authorized).
func (v *voiceEndpoint) StartMicrophone() {
	v.mu.Lock()
	me := v.me()
	if v.state == nil || !v.state.Enabled || me == nil || !me.Joined || v.mic != nil || v.publishing {
		v.mu.Unlock()
		return
	}
	v.publishing = true
	v.errorKey = ""
	v.mu.Unlock()
	if !v.command(map[string]any{"action": "publish"}) {
		v.mu.Lock()
		v.publishing = false
		v.mu.Unlock()
		v.setError("voice.error.transport")
	}
	v.host.changed()
}

func (v *voiceEndpoint) StopMicrophone() { v.stopMicrophone(true) }

func (v *voiceEndpoint) Deafen(deafened bool) {
	v.mu.Lock()
	v.deafened = deafened
	v.mu.Unlock()
	v.command(map[string]any{"action": "deafen", "deafened": deafened})
	v.host.changed()
}

func (v *voiceEndpoint) Mute(peerID string) {
	v.mu.Lock()
	v.muted[peerID] = !v.muted[peerID]
	v.mu.Unlock()
	v.host.changed()
}

func (v *voiceEndpoint) personVolume(peer string) float64 {
	if volume, ok := v.volumes[peer]; ok {
		return volume
	}
	return 1
}

// SetVolume sets the master listening volume (0–2) and remembers it.
func (v *voiceEndpoint) SetVolume(volume float64) {
	volume = min(max(volume, 0), 2)
	v.mu.Lock()
	v.volume = volume
	v.mu.Unlock()
	percent := int(volume*100 + 0.5)
	if percent == 0 {
		percent = -1 // 0 in the file means "never set" (100%)
	}
	go updatePreferences(func(p *preferences) { p.VoiceVolume = percent })
	v.host.changed()
}

// SetPersonVolume sets one person's volume (0–2) on this computer only.
func (v *voiceEndpoint) SetPersonVolume(peer string, volume float64) {
	v.mu.Lock()
	v.volumes[peer] = min(max(volume, 0), 2)
	v.mu.Unlock()
	v.host.changed()
}

// SetBitrate is the host's room voice bitrate for everyone (bit/s).
func (v *voiceEndpoint) SetBitrate(bitrate int) {
	v.command(map[string]any{"action": "bitrate", "bitrate": bitrate})
}

// savedVoiceVolume is voiceListeningVolume: 100% unless changed.
func savedVoiceVolume() float64 {
	switch percent := loadPreferences().VoiceVolume; {
	case percent < 0:
		return 0
	case percent == 0:
		return 1
	default:
		return min(float64(percent)/100, 2)
	}
}

func (v *voiceEndpoint) Snapshot() voiceSnapshot {
	v.mu.Lock()
	defer v.mu.Unlock()
	snap := voiceSnapshot{Microphone: v.mic != nil, Pending: v.publishing, Deafened: v.deafened, Error: v.errorKey, Volume: v.volume,
		Device: v.device, MicGain: v.micGain, MicLevel: v.micLevel, Processing: v.processing, Gate: v.gate, GateOpen: v.gateOpen}
	if v.state == nil {
		return snap
	}
	snap.Enabled, snap.Bitrate = v.state.Enabled, v.state.Bitrate
	snap.Unavailable = len(v.state.Unavailable) > 0
	if me := v.me(); me != nil {
		snap.Joined = me.Joined
	}
	now := time.Now()
	for _, member := range v.state.Members {
		if !member.Joined {
			continue
		}
		view := voiceMemberView{PeerID: member.PeerID, Joined: true, Publishing: member.PublicationID != "",
			Deafened: member.Deafened != nil && *member.Deafened, Muted: v.muted[member.PeerID],
			Volume: v.personVolume(member.PeerID)}
		if member.PeerID == v.peerID {
			view.Publishing = v.mic != nil
			view.Speaking = v.mic != nil && now.Sub(v.micHeard) < 400*time.Millisecond
			if view.Speaking {
				view.Level = v.micLevel
			}
		} else if member.PublicationID != "" {
			if play := v.plays[member.PublicationID]; play != nil {
				view.Speaking = now.Sub(play.heard) < 400*time.Millisecond
				if view.Speaking {
					view.Level = play.level
				}
			}
		}
		snap.Members = append(snap.Members, view)
	}
	for _, conn := range v.state.Connections {
		snap.Edges = append(snap.Edges, voiceEdge{Parent: conn.ParentPeerID, Child: conn.ChildPeerID, Pending: conn.Pending})
	}
	return snap
}

func (v *voiceEndpoint) closeConnections() {
	v.mu.Lock()
	conns := v.conns
	v.conns = map[string]*voiceConn{}
	v.plays = map[string]*playback{}
	v.forward = map[string]*webrtc.TrackLocalStaticRTP{}
	v.mu.Unlock()
	for _, conn := range conns {
		conn.closed = true
		_ = conn.pc.Close()
	}
}

// Shutdown releases audio devices and connections (room left or window closed).
func (v *voiceEndpoint) Shutdown() {
	v.stopMicrophone(false)
	v.closeConnections()
	v.mu.Lock()
	s := v.speaker
	v.speaker = nil
	v.state = nil
	v.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

type testTone struct{ stop chan struct{} }

func (t *testTone) Close() { close(t.stop) }

// startTone feeds a soft 440 Hz tone in 20 ms frames (test hook).
func startTone(onFrame func([]int16)) *testTone {
	t := &testTone{stop: make(chan struct{})}
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		frame := make([]int16, audioFrame)
		phase := 0.0
		for {
			select {
			case <-t.stop:
				return
			case <-ticker.C:
			}
			for i := range frame {
				frame[i] = int16(6000 * math.Sin(phase))
				phase += 2 * math.Pi * 440 / audioRate
			}
			onFrame(frame)
		}
	}()
	return t
}

// applyGain scales PCM samples in place, clipping at full scale.
func applyGain(frame []int16, gain float64) {
	for i, sample := range frame {
		frame[i] = int16(max(-32768, min(32767, float64(sample)*gain)))
	}
}

// savedMicGain is the remembered microphone volume (100% unless changed).
func savedMicGain() float64 {
	switch percent := loadPreferences().MicVolume; {
	case percent < 0:
		return 0
	case percent == 0:
		return 1
	default:
		return min(float64(percent)/100, 2)
	}
}

// SetMicGain sets the room voice microphone volume (0–2) and remembers it.
func (v *voiceEndpoint) SetMicGain(gain float64) {
	gain = min(max(gain, 0), 2)
	v.mu.Lock()
	v.micGain = gain
	v.mu.Unlock()
	percent := int(gain*100 + 0.5)
	if percent == 0 {
		percent = -1
	}
	go updatePreferences(func(p *preferences) { p.MicVolume = percent })
	v.host.changed()
}

// SetDevice picks the room voice microphone; a running microphone restarts on it.
func (v *voiceEndpoint) SetDevice(id string) {
	v.mu.Lock()
	changed := v.device != id
	v.device = id
	running := v.mic != nil
	v.mu.Unlock()
	go updatePreferences(func(p *preferences) { p.VoiceDevice = id })
	v.host.changed()
	if changed && running {
		go func() {
			v.stopMicrophone(true)
			v.StartMicrophone()
		}()
	}
}

// SetProcessing turns noise & echo reduction on or off; a running microphone restarts.
func (v *voiceEndpoint) SetProcessing(on bool) {
	v.mu.Lock()
	changed := v.processing != on
	v.processing = on
	running := v.mic != nil
	v.mu.Unlock()
	go updatePreferences(func(p *preferences) { p.VoiceRaw = !on })
	v.host.changed()
	if changed && running {
		go func() {
			v.stopMicrophone(true)
			v.StartMicrophone()
		}()
	}
}

// deviceLabel is the friendly name of a microphone ID ("" when unknown).
func deviceLabel(id string) string {
	if id == "" {
		return ""
	}
	devices, err := listCaptureDevices()
	if err != nil {
		return ""
	}
	for _, device := range devices {
		if device.ID == id {
			return device.Label
		}
	}
	return ""
}

// voiceLinkView is one room-voice connection in Connection details (voice.details.*).
type voiceLinkView struct {
	Peer        string // the other end (peerId)
	State       string // webrtc connection state
	RTTms       float64
	LossPercent float64
	JitterMs    float64
	HasStats    bool
}

// voiceLinks reads each voice connection's state and latest statistics.
func (v *voiceEndpoint) voiceLinks() []voiceLinkView {
	v.mu.Lock()
	conns := make([]*voiceConn, 0, len(v.conns))
	for _, conn := range v.conns {
		conns = append(conns, conn)
	}
	me := v.peerID
	v.mu.Unlock()
	var links []voiceLinkView
	for _, conn := range conns {
		if conn.pc == nil {
			continue
		}
		peer := conn.config.ChildPeerID
		if peer == me {
			peer = conn.config.ParentPeerID
		}
		link := voiceLinkView{Peer: peer, State: conn.pc.ConnectionState().String()}
		var received, lost float64
		for _, stat := range conn.pc.GetStats() {
			switch s := stat.(type) {
			case webrtc.ICECandidatePairStats:
				if s.Nominated && s.CurrentRoundTripTime > 0 {
					link.RTTms, link.HasStats = s.CurrentRoundTripTime*1000, true
				}
			case webrtc.InboundRTPStreamStats:
				received += float64(s.PacketsReceived)
				lost += float64(s.PacketsLost)
				link.JitterMs = max(link.JitterMs, s.Jitter*1000)
				link.HasStats = true
			}
		}
		if received+lost > 0 {
			link.LossPercent = 100 * lost / (received + lost)
		}
		links = append(links, link)
	}
	return links
}
