package nativeui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/TNTcraftHIM/Piik/internal/app/nativecapture"
	"github.com/TNTcraftHIM/Piik/internal/server/protocol"
)

// settingsLocked returns the sharing settings as the panel shows them.
func (h *hostSession) settingsLocked() shareSettings {
	return shareSettings{Quality: h.quality, Codec: h.codecMode, Topology: h.topology, Microphone: h.microphone,
		MicAvail: h.env.Capabilities.Microphone, Limits: limitsFrom(h.env.Capabilities),
		MicDevice: h.commentDev, MicVolume: h.commentVol}
}

// SetSettings applies the sharing settings panel: quality goes to the live
// share (update-share + set-quality-settings) and is remembered; codec and
// route policy apply to the next share, the microphone right away.
func (h *hostSession) SetSettings(next shareSettings) {
	h.mu.Lock()
	previous := h.settingsLocked()
	h.quality, h.codecMode, h.topology = next.Quality, next.Codec, next.Topology
	share, control := h.share, h.control
	live := share != "" && h.phase == phaseLive
	micChanged := next.Microphone != previous.Microphone
	h.microphone = next.Microphone
	h.mu.Unlock()
	h.changed()
	if next.Quality != previous.Quality {
		if h.env.Quality != nil {
			_ = h.env.Quality.Save(next.Quality)
		}
		if live && control != nil {
			h.busyApplying(1)
			go func() {
				defer h.busyApplying(-1)
				ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
				defer cancel()
				if _, err := control.request(ctx, "update-share", map[string]any{"shareId": share, "profile": next.Quality}); err != nil {
					h.setNotice("host.fail.quality")
					return
				}
				h.send(map[string]any{"type": "set-quality-settings", "qualitySettings": next.Quality})
				h.setNotice("host.notice.qualityApplied")
			}()
		}
	}
	if micChanged && live && control != nil && h.env.Capabilities.Microphone {
		go func() {
			ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
			defer cancel()
			fields := map[string]any{"shareId": share, "enabled": next.Microphone}
			if next.Microphone {
				h.mu.Lock()
				if h.commentDev != "" {
					fields["deviceId"] = h.commentDev
				}
				fields["volume"] = h.commentVol
				h.mu.Unlock()
			}
			if _, err := control.request(ctx, "set-microphone", fields); err != nil && next.Microphone {
				h.setNotice("host.commentary.failed")
			}
		}()
	}
}

// busyApplying counts settings requests in flight (SC-5 busy spinner).
func (h *hostSession) busyApplying(delta int) {
	h.mu.Lock()
	h.applying += delta
	h.mu.Unlock()
	h.changed()
}

// SetName changes the host's display name (set-display-name).
func (h *hostSession) SetName(name string) error {
	name = norm.NFC.String(strings.TrimSpace(name))
	if name == "" || utf8.RuneCountInString(name) > 24 || !protocol.ValidDisplayName(name) {
		return errors.New("invalid display name")
	}
	h.mu.Lock()
	h.name = name
	offline := h.signal != "connected"
	h.mu.Unlock()
	if offline {
		h.setNotice("host.nameOffline")
	}
	updatePreferences(func(p *preferences) { p.Name = name })
	h.changed()
	h.send(map[string]any{"type": "set-display-name", "displayName": name})
	return nil
}

func (h *hostSession) access(body map[string]any, out any) error {
	h.mu.Lock()
	roomID, token := h.roomID, h.hostToken
	h.mu.Unlock()
	if roomID == "" {
		return errors.New("no room")
	}
	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()
	_, err := h.api.do(ctx, http.MethodPost, "/api/rooms/"+roomID+"/access", body, token, out)
	return err
}

// SetPolicy sets the room code entry: "open" (code alone) or "private".
func (h *hostSession) SetPolicy(policy string) {
	go func() {
		var result struct {
			CodeEntryPolicy string `json:"codeEntryPolicy"`
		}
		if err := h.access(map[string]any{"action": "set-code-entry-policy", "policy": policy}, &result); err != nil {
			h.setNotice("host.accessFailed")
			return
		}
		h.mu.Lock()
		h.policy = result.CodeEntryPolicy
		passwordOn := h.passwordOn
		h.mu.Unlock()
		switch {
		case result.CodeEntryPolicy == protocol.CodeEntryOpen:
			h.setNotice("host.policy.setOpen")
		case passwordOn:
			h.setNotice("host.policy.setPrivatePassword")
		default:
			h.setNotice("host.policy.setPrivateInvite")
		}
	}()
}

func (h *hostSession) grantAction(action, notice string) {
	go func() {
		var result struct {
			InviteURL *string `json:"inviteUrl"`
		}
		if err := h.access(map[string]any{"action": action}, &result); err != nil {
			h.setNotice("host.accessFailed")
			return
		}
		h.mu.Lock()
		if result.InviteURL != nil {
			h.invite = *result.InviteURL
		} else {
			h.invite = canonicalInvite(h.invite)
		}
		h.notice = notice
		h.mu.Unlock()
		h.changed()
	}()
}

// RotateInvite gives the room a new invite link; RevokeInvite removes it and
// disconnects viewers who joined with it.
func (h *hostSession) RotateInvite() { h.grantAction("rotate-viewer-grant", "host.invite.updated") }
func (h *hostSession) RevokeInvite() { h.grantAction("revoke-viewer-grant", "host.invite.revoked") }

// ReplaceRoom gives the room a new code: the share ends and viewers leave.
func (h *hostSession) ReplaceRoom() {
	go func() {
		h.mu.Lock()
		roomID, token, policy := h.roomID, h.hostToken, h.policy
		h.mu.Unlock()
		if roomID == "" {
			return
		}
		h.endSharing("", true)
		ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
		defer cancel()
		if policy == "" {
			policy = protocol.CodeEntryOpen
		}
		var room protocol.CreateRoomResponse
		_, err := h.api.do(ctx, http.MethodPost, "/api/rooms/"+roomID+"/replacement", map[string]any{"codeEntryPolicy": policy}, token, &room)
		if err != nil {
			h.forgetRoom()
			if err := h.ensureRoom(); err != nil {
				h.setNotice("host.roomInvalid")
				return
			}
		} else {
			h.adoptRoom(room)
		}
		h.setNotice("host.roomReplaced")
	}()
}

// canonicalInvite drops the #v= grant (and query) from an invite link.
func canonicalInvite(invite string) string {
	if i := strings.IndexAny(invite, "#?"); i >= 0 {
		return invite[:i]
	}
	return invite
}

func (h *hostSession) viewerEvidence(message serverMessage) {
	if message.ViewerPeerID == "" {
		return
	}
	metrics := message.Metrics
	stats := viewerStats{Path: metrics.NatTraversalPath, At: time.Now(), Detail: metrics.evidenceDetail.lines()}
	if metrics.FreezeCountDelta != nil && *metrics.FreezeCountDelta > 0 {
		stats.Stutter = true
	}
	if metrics.Width != nil && metrics.Height != nil {
		stats.Width, stats.Height = int(*metrics.Width), int(*metrics.Height)
	}
	if metrics.FramesPerSecond != nil {
		stats.FPS = *metrics.FramesPerSecond
	}
	if metrics.BitrateKbps != nil {
		stats.Kbps = *metrics.BitrateKbps
	}
	if metrics.PacketsReceived != nil && metrics.PacketsLost != nil && *metrics.PacketsReceived+*metrics.PacketsLost > 0 {
		stats.LossPercent = 100 * *metrics.PacketsLost / (*metrics.PacketsReceived + *metrics.PacketsLost)
	}
	if metrics.RttMs != nil {
		stats.RTTms, stats.HasRTT = *metrics.RttMs, true
	}
	h.mu.Lock()
	if h.stats == nil {
		h.stats = map[string]viewerStats{}
	}
	h.stats[message.ViewerPeerID] = stats
	h.mu.Unlock()
	h.changed()
}

// SetShareVolume sets the shared sound for every viewer (0–2, muted = 0).
func (h *hostSession) SetShareVolume(volume float64, muted bool) {
	h.mu.Lock()
	h.shareVolume, h.shareMuted = volume, muted
	share, control := h.share, h.control
	h.mu.Unlock()
	h.changed()
	if share == "" || control == nil || !h.env.Capabilities.ShareVolume {
		return
	}
	value := volume
	if muted {
		value = 0
	}
	go func() {
		ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
		defer cancel()
		if _, err := control.request(ctx, "set-share-volume", map[string]any{"shareId": share, "volume": value}); err != nil {
			h.setNotice("host.shareVolume.unavailable")
		}
	}()
}

// CancelStart abandons a share that is still starting.
func (h *hostSession) CancelStart() {
	h.mu.Lock()
	cancel := h.cancelStart
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// captureStats is what the App reports while sending (edge-quality).
type captureStats struct {
	Width, Height int
	FPS, Kbps     float64
	At            time.Time
}

func (h *hostSession) edgeQuality(raw []byte) {
	var sample struct {
		FramesPerSecond float64 `json:"framesPerSecond"`
		BitrateKbps     float64 `json:"bitrateKbps"`
		Width           int     `json:"width"`
		Height          int     `json:"height"`
	}
	if json.Unmarshal(raw, &sample) != nil || sample.Width == 0 {
		return
	}
	h.mu.Lock()
	h.capture = captureStats{Width: sample.Width, Height: sample.Height, FPS: sample.FramesPerSecond, Kbps: sample.BitrateKbps, At: time.Now()}
	h.mu.Unlock()
	h.changed()
}

// watchDisplay refreshes the still preview of a shared screen every 3 s:
// Windows has no live thumbnail of a whole monitor.
func (h *hostSession) watchDisplay(share string) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
		}
		h.mu.Lock()
		current, target, paused := h.share == share, h.source, h.paused
		h.mu.Unlock()
		if !current {
			return
		}
		if target.Kind == "display" && !paused && !sharpPreview() {
			h.refreshPreview(target)
		}
		// Joining or leaving the room voice changes which sound is left out.
		h.mu.Lock()
		audio, border, applied, live := h.audio, h.border, h.voiceOn, h.phase == phaseLive && !h.switching
		h.mu.Unlock()
		if target.Kind == "display" && audio && live && h.voice != nil && h.voice.Snapshot().Joined != applied {
			h.switchSource(target, audio, border)
		}
	}
}

// SetSourceAudio turns the shared source's sound on or off while sharing.
func (h *hostSession) SetSourceAudio(on bool) {
	h.mu.Lock()
	target, border := h.source, h.border
	h.mu.Unlock()
	go h.switchSource(target, on, border)
}

// excludeFlag adds excludeAudio to a start/replace request: only for a screen
// share with sound, when the App can leave one application out. h.mu held.
func (h *hostSession) excludeFlag(fields map[string]any, target nativecapture.CaptureTarget, audio bool) {
	caps := h.env.Capabilities
	if !audio || target.Kind != "display" || !caps.ProcessAudioExclusion || !caps.SystemAudio {
		return
	}
	// Windows leaves out one application only. In room voice that is VibePiik
	// itself, which plays the others' voices: otherwise they would hear
	// themselves back in the shared sound.
	if h.voiceOn && h.selfAudio != nil {
		fields["excludeAudio"] = *h.selfAudio
		return
	}
	if h.exclude != nil {
		fields["excludeAudio"] = *h.exclude
	}
}

// SetExcludeAudio picks the application whose sound a screen share leaves
// out (nil: none). While sharing a screen with sound it applies right away.
func (h *hostSession) SetExcludeAudio(target *nativecapture.CaptureTarget) {
	h.mu.Lock()
	if target != nil {
		copied := *target
		target = &copied
	}
	same := (h.exclude == nil) == (target == nil) && (target == nil || audioAppKey(*h.exclude) == audioAppKey(*target))
	h.exclude = target
	source, audio, border, live := h.source, h.audio, h.border, h.share != "" && h.phase == phaseLive
	h.mu.Unlock()
	h.changed()
	if !same && live && audio && source.Kind == "display" {
		go h.switchSource(source, audio, border)
	}
}

// audioAppKey is audioApplicationKey: one entry per process.
func audioAppKey(target nativecapture.CaptureTarget) string {
	if target.Kind != "window" {
		return ""
	}
	return fmt.Sprintf("%d:%s", target.PID, target.CreationTime)
}

// audioApps lists the processes behind windows, once each.
func audioApps(sources []nativecapture.CaptureTarget) []nativecapture.CaptureTarget {
	seen := map[string]bool{}
	var apps []nativecapture.CaptureTarget
	for _, target := range sources {
		key := audioAppKey(target)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		apps = append(apps, target)
	}
	return apps
}

// SetViewerPassword sets (or with "" removes) the room password viewers type
// with the room code.
func (h *hostSession) SetViewerPassword(password string) {
	go func() {
		var value any
		if password != "" {
			value = password
		}
		var result struct {
			Enabled bool `json:"enabled"`
		}
		if err := h.access(map[string]any{"action": "set-viewer-password", "password": value}, &result); err != nil {
			h.setNotice("host.accessFailed")
			return
		}
		h.mu.Lock()
		was := h.passwordOn
		h.passwordOn = result.Enabled
		h.mu.Unlock()
		switch {
		case !result.Enabled:
			h.setNotice("host.password.removed")
		case was:
			h.setNotice("host.password.updated")
		default:
			h.setNotice("host.password.saved")
		}
	}()
}

// savedPercent reads a remembered 0–200 % volume (0 = never set = 100 %, -1 = 0 %).
func savedPercent(percent int) float64 {
	switch {
	case percent < 0:
		return 0
	case percent == 0:
		return 1
	default:
		return min(float64(percent)/100, 2)
	}
}

func storedPercent(volume float64) int {
	percent := int(volume*100 + 0.5)
	if percent == 0 {
		return -1
	}
	return percent
}

// Microphones lists the App's microphones for the commentary mix.
func (h *hostSession) Microphones(ctx context.Context) ([]audioDevice, error) {
	control, err := h.ensureControl()
	if err != nil {
		return nil, err
	}
	raw, err := control.request(ctx, "list-microphones", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Devices []audioDevice `json:"devices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result.Devices, nil
}

// SetCommentary picks the commentary microphone and its volume (0–2); both
// are remembered and apply right away while the microphone is mixed in.
func (h *hostSession) SetCommentary(device string, volume float64) {
	volume = min(max(volume, 0), 2)
	h.mu.Lock()
	deviceChanged := device != h.commentDev
	volumeChanged := volume != h.commentVol
	h.commentDev, h.commentVol = device, volume
	share, control, on := h.share, h.control, h.microphone && h.phase == phaseLive
	h.mu.Unlock()
	go updatePreferences(func(p *preferences) { p.CommentDevice, p.CommentVolume = device, storedPercent(volume) })
	h.changed()
	if !on || share == "" || control == nil || (!deviceChanged && !volumeChanged) {
		return
	}
	fields := map[string]any{"shareId": share}
	if volumeChanged {
		fields["volume"] = volume
	}
	if deviceChanged {
		fields["deviceId"] = device
	}
	go func() {
		ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
		defer cancel()
		if _, err := control.request(ctx, "set-microphone", fields); err != nil {
			h.setNotice("host.commentary.failed")
		}
	}()
}

func (h *hostSession) VoiceDevices() ([]audioDevice, error) { return listCaptureDevices() }
func (h *hostSession) VoiceDevice(id string)                { h.voice.SetDevice(id) }
func (h *hostSession) VoiceMicGain(gain float64)            { h.voice.SetMicGain(gain) }
func (h *hostSession) VoiceProcessing(on bool)              { h.voice.SetProcessing(on) }

// prepareSelfAudio finds VibePiik's own process among the capture sources
// (once) when a screen share with sound may need to leave the room voice
// out, and reports whether the room voice is joined.
func (h *hostSession) prepareSelfAudio(target nativecapture.CaptureTarget, audio bool) bool {
	if h.voice == nil || !h.voice.Snapshot().Joined {
		return false
	}
	h.mu.Lock()
	known := h.selfAudio != nil
	h.mu.Unlock()
	if !known && audio && target.Kind == "display" {
		ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
		defer cancel()
		if sources, err := h.Sources(ctx); err == nil {
			for _, source := range sources {
				if source.Kind == "window" && source.PID == uint32(os.Getpid()) && source.Valid() {
					found := source
					h.mu.Lock()
					h.selfAudio = &found
					h.mu.Unlock()
					break
				}
			}
		}
	}
	return true
}
