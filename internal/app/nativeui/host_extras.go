package nativeui

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Room interactions (chat, reactions) and remote control for the native host
// (lib/room-interactions.ts, HostRemoteControl.tsx).

const (
	chatHistoryLimit   = 1000
	maxChatCodePoints  = 280
	reactionDuration   = 2400 * time.Millisecond
	interactionTimeout = 6 * time.Second
)

var reactionIDs = []string{"thumbsup", "thumbsdown", "laugh", "clown", "eyeroll", "eyes", "wow", "wave", "heart", "clap", "party", "fire", "star", "sleep", "tomato", "poop"}

// reactionGlyph is the emoji of each reaction (shared/room-interactions.ts).
var reactionGlyph = map[string]string{
	"wave": "👋", "heart": "❤", "clap": "👏", "laugh": "😂", "wow": "🤯", "party": "🎉",
	"fire": "🔥", "eyes": "👀", "star": "⭐", "sleep": "😴", "tomato": "🍅", "poop": "💩",
	"clown": "🤡", "eyeroll": "🙄", "thumbsup": "👍", "thumbsdown": "👎",
}

type chatMessage struct {
	ID, PeerID, Name, Text string
	Host, Self             bool
	At                     time.Time
}

type activeReaction struct {
	ID, From, Target, Reaction string
	Until                      time.Time
}

type remoteRequest struct {
	ID         string `json:"id"`
	PeerID     string `json:"peerId"`
	State      string `json:"state"` // pending, active, suspended
	EndpointID string `json:"endpointId,omitempty"`
}

type remoteSnapshot struct {
	Available    bool
	Mode         string // off, ask, room, password
	Requests     []remoteRequest
	CoPlay       bool
	Simultaneous *bool
	Shortcut     int
	ShortcutOK   bool
	Gamepad      *bool // virtual gamepads available (nil: not reported)
}

type interactionMessage struct {
	ID         string  `json:"id"`
	RequestID  string  `json:"requestId"`
	OccurredAt float64 `json:"occurredAt"`
	Reason     string  `json:"reason"`
	Sender     struct {
		PeerID      string `json:"peerId"`
		Role        string `json:"role"`
		DisplayName string `json:"displayName"`
	} `json:"sender"`
	Payload struct {
		Kind         string `json:"kind"`
		Text         string `json:"text"`
		Reaction     string `json:"reaction"`
		TargetPeerID string `json:"targetPeerId"`
	} `json:"payload"`
	Peers json.RawMessage `json:"peers"`
}

// afterAuthenticated subscribes to room data like the page's signaling client.
func (h *hostSession) afterAuthenticated() {
	h.mu.Lock()
	peerID := h.peerID
	h.mu.Unlock()
	h.voice.authenticated(peerID)
	h.send(map[string]any{"type": "subscribe-room-interactions"})
	if h.env.Capabilities.RemoteInput {
		h.send(map[string]any{"type": "subscribe-remote-input"})
	}
	h.flushCoPlay()
}

// extraMessage handles the stage 4 signaling messages; false when not one.
func (h *hostSession) extraMessage(kind string, data []byte) bool {
	switch kind {
	case "room-voice-state", "room-voice-signal", "room-voice-error":
		h.voice.receive(kind, data)
		return true
	case "room-interactions-ready", "room-interaction", "room-interaction-rejected", "remote-input-peers":
	default:
		return false
	}
	if h.trace != nil {
		h.trace("remote-signal:"+kind, data)
	}
	var message interactionMessage
	if json.Unmarshal(data, &message) != nil {
		return true
	}
	switch kind {
	case "room-interactions-ready":
		h.mu.Lock()
		h.interactionsReady = true
		h.mu.Unlock()
	case "room-interaction-rejected":
		h.mu.Lock()
		if message.RequestID == h.pendingInteraction {
			h.pendingInteraction = ""
			h.interactionError = "interaction.error." + message.Reason
		}
		h.mu.Unlock()
	case "room-interaction":
		h.roomInteraction(message)
	case "remote-input-peers":
		h.mu.Lock()
		h.remotePeers = message.Peers
		share, control := h.share, h.control
		h.mu.Unlock()
		if share != "" && control != nil && h.env.Capabilities.RemoteInput {
			go h.pushRemotePeers(share, control, message.Peers)
		}
	}
	h.changed()
	return true
}

func (h *hostSession) pushRemotePeers(share string, control *controlClient, peers json.RawMessage) {
	if len(peers) == 0 {
		peers = json.RawMessage("[]")
	}
	ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
	defer cancel()
	if _, err := control.request(ctx, "set-remote-input-peers", map[string]any{"shareId": share, "peers": peers}); err != nil {
		slog.Warn("vibepiik-native", "event", "remote-input-peers-failed", "error", err)
	}
}

func (h *hostSession) roomInteraction(message interactionMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.interactionsReady {
		return
	}
	self := message.Sender.PeerID == h.peerID
	if self && message.RequestID == h.pendingInteraction {
		h.pendingInteraction = ""
		h.interactionError = ""
	}
	switch message.Payload.Kind {
	case "chat":
		h.chat = append(h.chat, chatMessage{ID: message.ID, PeerID: message.Sender.PeerID, Name: message.Sender.DisplayName,
			Text: message.Payload.Text, Host: message.Sender.Role == "host", Self: self, At: time.Now()})
		if len(h.chat) > chatHistoryLimit {
			h.chat = h.chat[len(h.chat)-chatHistoryLimit:]
		}
		if !self {
			h.unread++
		}
	case "reaction":
		if _, ok := reactionGlyph[message.Payload.Reaction]; !ok {
			return
		}
		now := time.Now()
		kept := h.reactions[:0]
		for _, reaction := range h.reactions {
			if reaction.Until.After(now) {
				kept = append(kept, reaction)
			}
		}
		h.reactions = append(kept, activeReaction{ID: message.ID, From: message.Sender.PeerID, Target: message.Payload.TargetPeerID,
			Reaction: message.Payload.Reaction, Until: now.Add(reactionDuration)})
		go func() {
			time.Sleep(reactionDuration + 50*time.Millisecond)
			h.changed()
		}()
	}
}

func (h *hostSession) sendInteraction(payload map[string]any) error {
	h.mu.Lock()
	if !h.interactionsReady {
		h.mu.Unlock()
		return errors.New("interactions are not ready")
	}
	if h.pendingInteraction != "" {
		h.mu.Unlock()
		return errors.New("an interaction is pending")
	}
	requestID := randomHex(16)
	h.pendingInteraction = requestID
	h.interactionError = ""
	h.mu.Unlock()
	if !h.send(map[string]any{"type": "send-room-interaction", "requestId": requestID, "payload": payload}) {
		h.mu.Lock()
		h.pendingInteraction = ""
		h.interactionError = "interaction.error.offline"
		h.mu.Unlock()
		h.changed()
		return errors.New("offline")
	}
	go func() {
		time.Sleep(interactionTimeout)
		h.mu.Lock()
		if h.pendingInteraction == requestID {
			h.pendingInteraction = ""
			h.interactionError = "interaction.error.unconfirmed"
		}
		h.mu.Unlock()
		h.changed()
	}()
	h.changed()
	return nil
}

// SendChat posts a chat message to the room.
func (h *hostSession) SendChat(text string) error {
	text = norm.NFC.String(strings.TrimSpace(text))
	if text == "" || utf8.RuneCountInString(text) > maxChatCodePoints || strings.ContainsAny(text, "\r\n\t\u200b\u2028\u2029\ufeff") {
		return errors.New("invalid chat text")
	}
	return h.sendInteraction(map[string]any{"kind": "chat", "text": text})
}

// SendReaction sends a reaction, to a participant when target is set.
func (h *hostSession) SendReaction(reaction, target string) {
	payload := map[string]any{"kind": "reaction", "reaction": reaction}
	if target != "" {
		payload["targetPeerId"] = target
	}
	_ = h.sendInteraction(payload)
}

// MarkChatRead clears the unread counter (the chat is open).
func (h *hostSession) MarkChatRead() {
	h.mu.Lock()
	changed := h.unread != 0
	h.unread = 0
	h.mu.Unlock()
	if changed {
		h.changed()
	}
}

// remoteControlState stores the App's remote-control-state event.
func (h *hostSession) remoteControlState(raw json.RawMessage) {
	var state struct {
		Mode         string          `json:"mode"`
		Requests     []remoteRequest `json:"requests"`
		CoPlay       bool            `json:"coPlay"`
		Simultaneous *bool           `json:"simultaneous"`
		Gamepad      *bool           `json:"gamepad"`
	}
	if json.Unmarshal(raw, &state) != nil {
		return
	}
	h.mu.Lock()
	h.remote.Mode, h.remote.Requests, h.remote.CoPlay, h.remote.Simultaneous = state.Mode, state.Requests, state.CoPlay, state.Simultaneous
	h.remote.Gamepad = state.Gamepad
	if state.Mode != "" && state.Mode != "off" {
		mode := state.Mode
		go updatePreferences(func(p *preferences) { p.RemoteMode = mode })
	}
	h.mu.Unlock()
	h.flushCoPlay()
	h.changed()
}

// flushCoPlay tells the room which approved controllers co-play.
func (h *hostSession) flushCoPlay() {
	h.mu.Lock()
	share := h.share
	coPlay := h.remote.CoPlay
	grants := []map[string]any{}
	for _, request := range h.remote.Requests {
		if coPlay && request.State != "pending" && request.EndpointID != "" {
			grants = append(grants, map[string]any{"endpointId": request.EndpointID})
		}
	}
	h.mu.Unlock()
	if share != "" && h.env.Capabilities.RemoteCoPlay {
		h.send(map[string]any{"type": "set-co-play-presence", "shareGeneration": share, "grants": grants})
	}
}

func (h *hostSession) remoteRequest(kind string, fields map[string]any) error {
	h.mu.Lock()
	share, control := h.share, h.control
	h.mu.Unlock()
	if share == "" || control == nil {
		return errors.New("not sharing")
	}
	fields["shareId"] = share
	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()
	_, err := control.request(ctx, kind, fields)
	return err
}

// SetRemotePolicy turns remote control off or on (ask, room or password).
func (h *hostSession) SetRemotePolicy(mode, password string) {
	go func() {
		fields := map[string]any{"mode": mode}
		if mode == "password" && password != "" {
			fields["password"] = password
		}
		if h.env.Capabilities.RemoteCoPlay {
			fields["coPlay"] = true
		}
		if h.env.Capabilities.RemoteStopShortcut && mode != "off" {
			h.mu.Lock()
			key := h.remote.Shortcut
			h.mu.Unlock()
			if key == 0 {
				key = savedStopKey()
			}
			if err := h.stopShortcut(&key); err != nil {
				// Like the page: no emergency stop, no remote control.
				slog.Debug("vibepiik-native", "event", "remote-stop-shortcut-failed", "error", err)
				h.setNotice("remote.shortcutFailed")
				return
			}
		}
		if err := h.remoteRequest("set-remote-control-policy", fields); err != nil {
			slog.Warn("vibepiik-native", "event", "remote-policy-failed", "mode", mode, "error", err)
			h.setNotice("remote.decisionFailed")
		}
	}()
}

func (h *hostSession) SetRemoteSimultaneous(enabled bool) {
	go func() { _ = h.remoteRequest("set-remote-control-simultaneous", map[string]any{"enabled": enabled}) }()
}

// DecideRemote allows or declines (or ends) one control request.
func (h *hostSession) DecideRemote(id string, allow bool) {
	go func() {
		if err := h.remoteRequest("set-remote-control", map[string]any{"requestId": id, "allow": allow}); err != nil {
			h.setNotice("remote.decisionFailed")
		}
	}()
}

// SetStopShortcut picks Ctrl+Alt+F<key> as the host's emergency stop.
func (h *hostSession) SetStopShortcut(key int) {
	go updatePreferences(func(p *preferences) { p.RemoteStopKey = key })
	go func() {
		if err := h.stopShortcut(&key); err != nil {
			h.setNotice("remote.shortcutFailed")
		}
		h.changed()
	}()
}

func (h *hostSession) stopShortcut(key *int) error {
	if !h.env.Capabilities.RemoteStopShortcut {
		return errors.New("unavailable")
	}
	kind, fields := "get-remote-stop-shortcut", map[string]any{}
	if key != nil {
		kind, fields = "set-remote-stop-shortcut", map[string]any{"key": *key}
	}
	h.mu.Lock()
	share, control := h.share, h.control
	h.mu.Unlock()
	if share == "" || control == nil {
		return errors.New("not sharing")
	}
	fields["shareId"] = share
	ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
	defer cancel()
	raw, err := control.request(ctx, kind, fields)
	if err != nil {
		return err
	}
	var result struct {
		Key       int  `json:"key"`
		Available bool `json:"available"`
	}
	_ = json.Unmarshal(raw, &result)
	h.mu.Lock()
	h.remote.Shortcut, h.remote.ShortcutOK = result.Key, result.Available
	h.mu.Unlock()
	if !result.Available {
		return errors.New("shortcut unavailable")
	}
	return nil
}

// extrasSnapshotLocked fills the stage 4 part of a snapshot.
func (h *hostSession) extrasSnapshotLocked(snap *roomSnapshot) {
	snap.Chat = append([]chatMessage(nil), h.chat...)
	now := time.Now()
	for _, reaction := range h.reactions {
		if reaction.Until.After(now) {
			snap.Reactions = append(snap.Reactions, reaction)
		}
	}
	snap.Unread = h.unread
	snap.InteractionsReady = h.interactionsReady
	snap.InteractionPending = h.pendingInteraction != ""
	snap.InteractionError = h.interactionError
	remote := h.remote
	remote.Available = h.env.Capabilities.RemoteInput
	remote.Requests = append([]remoteRequest(nil), h.remote.Requests...)
	if remote.Mode == "" {
		remote.Mode = "off"
	}
	snap.Remote = remote
	snap.Voice = h.voice.Snapshot()
}

// Voice controls (the Interactions panel's Voice tab).
func (h *hostSession) VoiceOpen()                 { h.voice.Open() }
func (h *hostSession) VoiceClose()                { h.voice.Close() }
func (h *hostSession) VoiceDeafen(on bool)        { h.voice.Deafen(on) }
func (h *hostSession) VoiceMute(peer string)      { h.voice.Mute(peer) }
func (h *hostSession) VoiceVolume(volume float64) { h.voice.SetVolume(volume) }
func (h *hostSession) VoicePersonVolume(peer string, volume float64) {
	h.voice.SetPersonVolume(peer, volume)
}
func (h *hostSession) VoiceBitrate(bitrate int) { h.voice.SetBitrate(bitrate) }
func (h *hostSession) VoiceMicrophone(on bool) {
	if on {
		h.voice.StartMicrophone()
	} else {
		h.voice.StopMicrophone()
	}
}

// savedStopKey is the host stop shortcut chosen last time (F10 by default).
func savedStopKey() int {
	if key := loadPreferences().RemoteStopKey; key >= 1 && key <= 11 {
		return key
	}
	return 10
}

// VoiceLinks is the room-voice connections for Connection details.
func (h *hostSession) VoiceLinks() []voiceLinkView { return h.voice.voiceLinks() }
