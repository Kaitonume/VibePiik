package nativeui

import (
	"sync/atomic"
	"time"
)

// sharpOn is the "sharp preview" choice for screen shares (remembered),
// sharpFPS its rate (30 or 60).
var sharpOn atomic.Bool
var sharpFPS atomic.Int32

func init() {
	prefs := loadPreferences()
	sharpOn.Store(prefs.PreviewSharp)
	sharpFPS.Store(30)
	if prefs.PreviewFPS == 60 {
		sharpFPS.Store(60)
	}
	nativeCopy_en["native.previewSharpOn"] = "Sharp preview: on (live, full detail; uses more CPU). Click for the light preview"
	nativeCopy_en["native.previewSharpOff"] = "Light preview (updated every 3 s). Click for a sharp, live preview"
	nativeCopy_zh["native.previewSharpOn"] = "清晰预览：开（实时、完整细节，占用更多 CPU）。点击切换为轻量预览"
	nativeCopy_zh["native.previewSharpOff"] = "轻量预览（每 3 秒更新）。点击切换为清晰的实时预览"
	nativeCopy_it["native.previewSharpOn"] = "Anteprima nitida: attiva (in tempo reale, a piena definizione; usa più CPU). Clic per l'anteprima leggera"
	nativeCopy_it["native.previewSharpOff"] = "Anteprima leggera (aggiornata ogni 3 s). Clic per un'anteprima nitida in tempo reale"
}

func sharpPreview() bool { return sharpOn.Load() }

func previewFPS() int { return int(sharpFPS.Load()) }

func setPreviewFPS(fps int) {
	sharpFPS.Store(int32(fps))
	go updatePreferences(func(p *preferences) { p.PreviewFPS = fps })
}

func setSharpPreview(on bool) {
	sharpOn.Store(on)
	go updatePreferences(func(p *preferences) { p.PreviewSharp = on })
}

// PreviewShown tells the host the screen preview is on screen now: the sharp
// capture runs only while somebody looks at it.
// width is the preview's width on screen: the copy is made at that size.
func (h *hostSession) PreviewShown(width int) {
	h.mu.Lock()
	h.previewSeen, h.previewWidth = time.Now(), width
	h.mu.Unlock()
}

// sharpLoop copies the shared monitor 30 or 60 times a second while the
// sharp preview is on and visible.
func (h *hostSession) sharpLoop(share string) {
	next := time.Now()
	for {
		period := time.Second / time.Duration(previewFPS())
		next = next.Add(period)
		if wait := time.Until(next); wait > 0 {
			select {
			case <-h.ctx.Done():
				return
			case <-time.After(wait):
			}
		} else {
			next = time.Now() // fell behind: do not try to catch up
		}
		h.mu.Lock()
		current, target, paused, seen, width := h.share == share, h.source, h.paused, h.previewSeen, h.previewWidth
		h.mu.Unlock()
		if !current {
			return
		}
		if !sharpPreview() || target.Kind != "display" || paused || time.Since(seen) > time.Second {
			continue
		}
		picture, err := captureMonitor(target.SourceID, min(max(width, 640), 2560))
		if err != nil {
			continue
		}
		h.mu.Lock()
		if h.share == share && h.source == target {
			h.preview = picture
		}
		h.mu.Unlock()
		h.changed()
	}
}
