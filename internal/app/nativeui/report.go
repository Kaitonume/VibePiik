package nativeui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/TNTcraftHIM/Piik/internal/app/buildinfo"
)

// Diagnostics is the host side of the native diagnostics report: what a
// maintainer needs to understand a failure, without credentials (no invite
// grant, host token or passwords) and without other people's names.
func (h *hostSession) Diagnostics() map[string]any {
	snap := h.Snapshot()
	h.mu.Lock()
	defer h.mu.Unlock()
	viewers := []map[string]any{}
	for i, viewer := range snap.Viewers {
		entry := map[string]any{"index": i + 1, "mediaReady": viewer.MediaReady, "relayed": viewer.Upstream != "" && viewer.Upstream != snap.HostID}
		if stats, ok := snap.Stats[viewer.ID]; ok {
			entry["stats"] = map[string]any{"width": stats.Width, "height": stats.Height, "fps": stats.FPS, "kbps": stats.Kbps,
				"lossPercent": stats.LossPercent, "rttMs": stats.RTTms, "path": stats.Path, "ageSeconds": time.Since(stats.At).Seconds(),
				"detail": stats.Detail}
		}
		viewers = append(viewers, entry)
	}
	return map[string]any{
		"signal":        snap.Signal,
		"phase":         snap.Phase,
		"roomCode":      snap.Code,
		"policy":        snap.Policy,
		"passwordOn":    snap.PasswordOn,
		"notice":        snap.Notice,
		"failureCode":   snap.FailureCode,
		"source":        map[string]any{"kind": snap.Target.Kind, "audio": snap.Audio, "sourceAudio": snap.SourceAudio, "excludeAudio": snap.Exclude != nil},
		"codecMode":     h.codecMode,
		"codecInUse":    snap.CodecInUse,
		"encoder":       map[string]any{"adapter": h.adapter, "encoder": h.encoder},
		"quality":       snap.Settings.Quality,
		"topology":      snap.Settings.Topology,
		"capture":       snap.Capture,
		"capabilities":  h.env.Capabilities,
		"natPrediction": h.natPrediction,
		"viewers":       viewers,
		"voice": map[string]any{"enabled": snap.Voice.Enabled, "joined": snap.Voice.Joined, "microphone": snap.Voice.Microphone,
			"members": len(snap.Voice.Members), "bitrate": snap.Voice.Bitrate, "error": snap.Voice.Error},
		"remoteControl": map[string]any{"mode": snap.Remote.Mode, "requests": len(snap.Remote.Requests), "shortcut": snap.Remote.Shortcut,
			"shortcutAvailable": snap.Remote.ShortcutOK},
	}
}

// downloadsDir is the user's Downloads folder (home when missing).
func downloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		dir = home
	}
	return dir, nil
}

// saveChat writes the chat kept by this window as text (interaction.export).
func saveChat(messages []chatMessage, hostLabel string) (string, error) {
	dir, err := downloadsDir()
	if err != nil {
		return "", err
	}
	var text strings.Builder
	text.WriteString("VibePiik chat " + time.Now().Format("2006-01-02 15:04") + "\r\n\r\n")
	for _, message := range messages {
		name := message.Name
		if message.Host && name == "" {
			name = hostLabel
		}
		at := ""
		if !message.At.IsZero() {
			at = "[" + message.At.Format("15:04:05") + "] "
		}
		text.WriteString(at + name + ": " + strings.ReplaceAll(message.Text, "\n", " ") + "\r\n")
	}
	path := filepath.Join(dir, "vibepiik-chat-"+time.Now().Format("20060102-150405")+".txt")
	// UTF-8 with BOM so Notepad shows Chinese and emoji correctly.
	if err := os.WriteFile(path, append([]byte("\ufeff"), text.String()...), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// saveReport writes the diagnostics report to the Downloads folder.
func saveReport(host map[string]any, lang string) (string, error) {
	dir, err := downloadsDir()
	if err != nil {
		return "", err
	}
	report := map[string]any{
		"report":    "vibepiik-native-diagnostics",
		"createdAt": time.Now().Format(time.RFC3339),
		"app":       map[string]any{"version": buildinfo.Version(), "ui": "native", "os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version()},
		"language":  lang,
		"host":      host,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "vibepiik-report-"+time.Now().Format("20060102-150405")+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// showInFolder opens Explorer with the file selected.
func showInFolder(path string) {
	if runtime.GOOS == "windows" {
		_ = exec.Command("explorer.exe", "/select,", path).Start()
	}
}
