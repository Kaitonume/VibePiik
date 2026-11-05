package nativeui

import (
	"bytes"
	"embed"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
)

// The web client's comics (Comic.tsx and the hint scenes), captured from its
// tooltip preview page in their still, informative pose (the reduced-motion
// pose) for the dark and light themes: 320×96 each.
//
//go:embed comics/*.png
var comicFiles embed.FS

var (
	comicMu    sync.Mutex
	comicCache = map[string]paint.ImageOp{}
	comicNone  = map[string]bool{}
)

// comicOp returns the picture of a comic kind for the theme.
func comicOp(kind string, dark bool) (paint.ImageOp, bool) {
	name := kind + ".light.png"
	if dark {
		name = kind + ".dark.png"
	}
	comicMu.Lock()
	defer comicMu.Unlock()
	if picture, ok := comicCache[name]; ok {
		return picture, true
	}
	if comicNone[name] {
		return paint.ImageOp{}, false
	}
	data, err := comicFiles.ReadFile("comics/" + name)
	var img image.Image
	if err == nil {
		img, err = png.Decode(bytes.NewReader(data))
	}
	if err != nil {
		comicNone[name] = true
		return paint.ImageOp{}, false
	}
	picture := paint.NewImageOp(img)
	picture.Filter = paint.FilterLinear
	comicCache[name] = picture
	return picture, true
}

// hasComic reports whether kind is a known comic.
func hasComic(kind string) bool {
	_, err := comicFiles.Open("comics/" + kind + ".dark.png")
	return err == nil
}

// comic draws kind widthDp wide (keeping the 320:96 strip shape).
func comic(gtx layout.Context, u *UI, kind string, widthDp float32) layout.Dimensions {
	return comicThemed(gtx, kind, widthDp, u.P.Dark)
}

// comicThemed draws kind in the dark or light look.
func comicThemed(gtx layout.Context, kind string, widthDp float32, dark bool) layout.Dimensions {
	picture, ok := comicOp(kind, dark)
	if !ok {
		return layout.Dimensions{}
	}
	src := picture.Size()
	width := gtx.Dp(unit.Dp(widthDp))
	size := image.Pt(width, width*src.Y/max(1, src.X))
	drawContain(gtx.Ops, picture, size)
	return layout.Dimensions{Size: size}
}

// stateComic is a state comic over the TV picture with its message under it;
// in ✦ mode the comic alone.
func stateComic(gtx layout.Context, u *UI, kind, message string) layout.Dimensions {
	if !hasComic(kind) {
		return tvMessage(gtx, u, message, 16)
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := min(float32(320), float32(gtx.Constraints.Max.X)/gtx.Metric.PxPerDp*0.6)
			// The TV screen is always dark: the stage look of the page.
			// Progress comics (starting, switching, recovering) keep moving.
			loop := kind == "source-starting" || kind == "source-switching" || kind == "recovering" || kind == "signal-connecting" || kind == "signal-recovering"
			if !loop {
				return comicThemed(gtx, kind, width, true)
			}
			return comicPlaying(gtx, kind, width, true, time.Duration(gtx.Now.UnixNano()), true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.Visual() || message == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return tvMessage(gtx, u, message, 15)
			})
		}))
}

// noticeComic is the state comic of the idle TV's notice.
func noticeComic(snap roomSnapshot) string {
	switch {
	case snap.FailureCode != "":
		return "source-failed"
	case snap.Notice == "host.stopNotice" || snap.Notice == "host.shareEnded":
		return "share-ended"
	case snap.Notice == "host.roomInvalid":
		return "room-code-invalid"
	case snap.Notice == "host.accessFailed":
		return "settings-failed"
	}
	return ""
}

// waitingCaptionLabel draws the playful waiting line and wakes up for the next one.
func waitingCaptionLabel(gtx layout.Context, u *UI, ink color.NRGBA, salt int) layout.Dimensions {
	line := u.waitingCaption(gtx.Now, salt)
	if line == "" {
		return layout.Dimensions{}
	}
	if !reducedMotion {
		next := time.Unix((gtx.Now.Unix()/15+1)*15, 0)
		gtx.Execute(op.InvalidateCmd{At: next})
	}
	return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.label(gtx, line, textStyle{size: 12.5, color: ink, align: text.Middle, maxLines: 2})
	})
}

// staticNoiseCells is the .lr-tv-static tile: 90×90 with a dozen grey speckles.
var staticNoiseCells = [][5]int{{4, 9, 5, 0x2c3a52}, {26, 30, 4, 0x1e2a40}, {52, 12, 6, 0x364a68}, {70, 48, 5, 0x232f47},
	{12, 60, 6, 0x304058}, {40, 70, 4, 0x1e2a40}, {64, 74, 5, 0x2c3a52}, {80, 20, 4, 0x304058}, {34, 50, 5, 0x232f47},
	{8, 36, 4, 0x364a68}, {48, 40, 4, 0x2c3a52}, {76, 64, 4, 0x1e2a40}}

// staticNoise is StaticNoise: the tile at .55 opacity, jumping three times
// every 360 ms (steps(3)); still when animations are off.
func staticNoise(gtx layout.Context, size image.Point) {
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	shifts := [][2]float32{{0, 0}, {28, 14}, {-18, 26}}
	step := 0
	if !reducedMotion {
		step = int(gtx.Now.UnixMilli()/120) % 3
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(120 * time.Millisecond)})
	}
	tile := px(gtx, 90)
	dx, dy := px(gtx, shifts[step][0]), px(gtx, shifts[step][1])
	fillRect(gtx.Ops, size, withAlpha(rgb(0x0d1526), .55))
	for y := -tile + float32(math.Mod(float64(dy), float64(tile))); y < float32(size.Y); y += tile {
		for x := -tile + float32(math.Mod(float64(dx), float64(tile))); x < float32(size.X); x += tile {
			for _, c := range staticNoiseCells {
				s := px(gtx, float32(c[2]))
				box := boxOf(image.Pt(int(s), int(s)), 0).offset(x+px(gtx, float32(c[0])), y+px(gtx, float32(c[1])))
				fillBox(gtx.Ops, box, withAlpha(rgb(uint32(c[3])), .55))
			}
		}
	}
}
