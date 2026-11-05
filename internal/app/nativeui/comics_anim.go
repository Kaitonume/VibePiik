package nativeui

import (
	"bytes"
	"embed"
	"image"
	"image/png"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// Animated comics: 16 frames over the web's 3.2 s comic loop, captured from
// the same preview page with motion on, packed as one 256-colour strip per
// comic and theme (comics/anim/<kind>.<theme>.png, frames stacked top to
// bottom). Comics without motion have no strip and stay still.
//
//go:embed comics/anim/*.png
var comicAnimFiles embed.FS

const (
	comicFrames   = 16
	comicDuration = 3200 * time.Millisecond
)

type comicStrip struct {
	frames []paint.ImageOp
}

var comicStrips = map[string]*comicStrip{} // guarded by comicMu

func comicStripFor(kind string, dark bool) *comicStrip {
	name := kind + ".light.png"
	if dark {
		name = kind + ".dark.png"
	}
	comicMu.Lock()
	defer comicMu.Unlock()
	if strip, ok := comicStrips[name]; ok {
		return strip
	}
	var strip *comicStrip
	if data, err := comicAnimFiles.ReadFile("comics/anim/" + name); err == nil {
		if img, err := png.Decode(bytes.NewReader(data)); err == nil {
			b := img.Bounds()
			h := b.Dy() / comicFrames
			sub, ok := img.(interface {
				SubImage(image.Rectangle) image.Image
			})
			if ok && h > 0 {
				strip = &comicStrip{}
				for i := 0; i < comicFrames; i++ {
					frame := paint.NewImageOp(sub.SubImage(image.Rect(b.Min.X, b.Min.Y+i*h, b.Max.X, b.Min.Y+(i+1)*h)))
					frame.Filter = paint.FilterLinear
					strip.frames = append(strip.frames, frame)
				}
			}
		}
	}
	comicStrips[name] = strip
	return strip
}

// comicPlaying draws kind widthDp wide at the moment elapsed since it
// appeared: once through the loop then the last pose, or looping.
func comicPlaying(gtx layout.Context, kind string, widthDp float32, dark bool, elapsed time.Duration, loop bool) layout.Dimensions {
	strip := comicStripFor(kind, dark)
	if strip == nil || reducedMotion {
		return comicThemed(gtx, kind, widthDp, dark)
	}
	if loop {
		elapsed %= comicDuration
	}
	index := int(elapsed * comicFrames / comicDuration)
	if index >= comicFrames {
		index = comicFrames - 1
	} else {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(comicDuration / comicFrames)})
	}
	picture := strip.frames[max(index, 0)]
	src := picture.Size()
	width := gtx.Dp(unit.Dp(widthDp))
	size := image.Pt(width, width*src.Y/max(1, src.X))
	drawContain(gtx.Ops, picture, size)
	return layout.Dimensions{Size: size}
}
