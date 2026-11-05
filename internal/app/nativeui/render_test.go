package nativeui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// renderPNG draws widget at size (pixels, scale 1) into a PNG under
// NATIVEUI_SHOTS. Skipped when the variable is unset.
func renderPNG(t *testing.T, name string, size image.Point, widget func(gtx layout.Context)) {
	t.Helper()
	directory := os.Getenv("NATIVEUI_SHOTS")
	if directory == "" {
		t.Skip("NATIVEUI_SHOTS not set")
	}
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skipf("headless GPU unavailable: %v", err)
	}
	defer window.Release()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size), Source: input.Source{}}
	widget(gtx)
	if err := window.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
	if err := window.Screenshot(img); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(directory, 0o755)
	file, err := os.Create(filepath.Join(directory, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}

func TestRenderGlyphSheet(t *testing.T) {
	names := make([]string, 0, len(glyphSources))
	for name := range glyphSources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, palette := range []Palette{LightPalette(), DarkPalette()} {
		theme := "light"
		if palette.Dark {
			theme = "dark"
		}
		renderPNG(t, "glyphs-"+theme, image.Pt(15*40+20, 7*40+80), func(gtx layout.Context) {
			fillRect(gtx.Ops, gtx.Constraints.Max, palette.Wall)
			for i, name := range names {
				offset := op.Offset(image.Pt(10+(i%15)*40, 10+(i/15)*40)).Push(gtx.Ops)
				glyph(gtx, name, 24, palette.Ink)
				offset.Pop()
			}
			offset := op.Offset(image.Pt(10, 7*40+20)).Push(gtx.Ops)
			brandMark(gtx, 52, palette.Ink)
			offset.Pop()
			_ = f32.Point{}
		})
	}
}

func TestHoverLiftShot(t *testing.T) {
	directory := os.Getenv("NATIVEUI_SHOTS")
	if directory == "" {
		t.Skip("NATIVEUI_SHOTS not set")
	}
	ui := &UI{P: DarkPalette(), Shaper: newShaper(false), Lang: "en"}
	var buttons [3]Button
	var cards [2]RadioCard
	var router input.Router
	start := time.Now()
	draw := func(gtx layout.Context) {
		fillRect(gtx.Ops, gtx.Constraints.Max, ui.P.Wall)
		for i := range buttons {
			o := op.Offset(image.Pt(20+i*150, 20)).Push(gtx.Ops)
			g := gtx
			g.Constraints.Min = image.Point{}
			buttons[i].Layout(g, ui, ButtonStyle{Icon: "sliders", Caption: "Button", Size: sizeCompact})
			o.Pop()
		}
		for i := range cards {
			o := op.Offset(image.Pt(20+i*230, 90)).Push(gtx.Ops)
			g := gtx
			g.Constraints = layout.Constraints{Max: image.Pt(210, 200)}
			cards[i].Layout(g, ui, "globe", "Card", "hint", false)
			o.Pop()
		}
	}
	frame := func(now time.Time) *op.Ops {
		ops := new(op.Ops)
		gtx := layout.Context{Ops: ops, Now: now, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(520, 260))}
		draw(gtx)
		router.Frame(ops)
		return ops
	}
	frame(start)
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(200, 40)})
	frame(start.Add(10 * time.Millisecond))
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(330, 150)})
	var ops *op.Ops
	for i := 1; i <= 40; i++ {
		ops = frame(start.Add(time.Duration(i) * 20 * time.Millisecond))
	}
	window, err := headless.NewWindow(520, 260)
	if err != nil {
		t.Skipf("headless GPU unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(ops); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 520, 260))
	if err := window.Screenshot(img); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, "hover-lift.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	_ = png.Encode(file, img)
}
