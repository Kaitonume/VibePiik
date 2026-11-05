package nativeui

import (
	"fmt"
	"image"
	"strings"
	"time"
)

// chatOverlay shows the chat outside the Gio window (overlay_windows.go).
type chatOverlay interface {
	Update(overlayState)
	Close()
}

// desktopOverlay is the setting of the chat over other apps.
type desktopOverlay struct {
	On      bool
	Monitor int    // index in listMonitors (0 = primary)
	Corner  string // one of desktopCorners
	Seconds int    // how long a line stays
}

// overlayState is everything the overlay thread draws.
type overlayState struct {
	Lang    string
	Spoken  string // language kept under ✦
	Look    overlayLook
	Preview image.Rectangle // TV picture, client pixels of the owner (empty: none)
	Clip    image.Rectangle // visible part of Preview
	Flights []overlayFlight
	Desktop desktopOverlay
	Lines   []overlayLine
}

// screenChoice is one screen offered for the chat over other apps.
type screenChoice struct {
	Width, Height int
	Primary       bool
}

func activeFlights(flights []overlayFlight, now time.Time) []overlayFlight {
	var out []overlayFlight
	for _, f := range flights {
		if !f.done(now) {
			out = append(out, f)
		}
	}
	return out
}

func activeLines(lines []overlayLine, now time.Time) []overlayLine {
	var out []overlayLine
	for _, line := range lines {
		if now.Before(line.Until) {
			out = append(out, line)
		}
	}
	return out
}

func nextExpiry(lines []overlayLine, now time.Time) time.Duration {
	next := time.Duration(0)
	for _, line := range lines {
		if wait := line.Until.Sub(now); wait > 0 && (next == 0 || wait < next) {
			next = wait
		}
	}
	return next
}

func desktopKey(lines []overlayLine, state overlayState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%v|%+v|%s|", state.Look, state.Desktop, state.Lang)
	for _, line := range lines {
		b.WriteString(line.ID + ";")
	}
	return b.String()
}
