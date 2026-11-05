package nativeui

import (
	"os"
	"path/filepath"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/text"
)

const (
	sansFace = "Segoe UI"
	monoFace = "Cascadia Mono"
)

var (
	// Comma-separated typefaces are fallbacks: symbols (✦) and Chinese text come
	// from the faces Windows itself uses for them.
	sans = font.Font{Typeface: sansFace + ", Segoe UI Symbol, Microsoft YaHei"}
	mono = font.Font{Typeface: monoFace + ", Segoe UI Symbol"}
)

// windowsFonts are the faces the web client gets from its font stacks on
// Windows: --font resolves to Segoe UI (Microsoft YaHei for CJK) and --mono to
// Cascadia Mono. Only installed system files are read; none are bundled.
var windowsFonts = []struct {
	file     string
	typeface font.Typeface
	weight   font.Weight
}{
	{"segoeui.ttf", sansFace, font.Normal},
	{"seguisb.ttf", sansFace, font.SemiBold},
	{"segoeuib.ttf", sansFace, font.Bold},
	{"seguibl.ttf", sansFace, font.Black},
	{"CascadiaMono.ttf", monoFace, font.Normal},
	{"consolab.ttf", monoFace, font.Bold},
	{"seguisym.ttf", "Segoe UI Symbol", font.Normal},
}

// chineseFonts are large (35 MB of collections, ~150 MB parsed): loaded only
// while the interface is in Chinese.
var chineseFonts = []struct {
	file     string
	typeface font.Typeface
	weight   font.Weight
}{
	{"msyh.ttc", "Microsoft YaHei", font.Normal},
	{"msyhbd.ttc", "Microsoft YaHei", font.Bold},
}

// loadFonts returns the system faces above, falling back to the Go fonts when
// a file is missing (non-Windows builds, trimmed installs).
func loadFonts(chinese bool) []font.FontFace {
	directory := filepath.Join(os.Getenv("WINDIR"), "Fonts")
	var faces []font.FontFace
	entries := windowsFonts
	if chinese {
		entries = append(append(entries[:0:0], windowsFonts...), chineseFonts...)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(directory, entry.file))
		if err != nil {
			continue
		}
		collection, err := opentype.ParseCollection(data)
		if err != nil {
			continue
		}
		for _, face := range collection {
			face.Font.Typeface = entry.typeface
			face.Font.Weight = entry.weight
			face.Font.Style = font.Regular
			faces = append(faces, face)
		}
	}
	for _, face := range gofont.Collection() {
		// Last-resort fallback only: keep the Go fonts under their own names so
		// they never replace an installed Segoe UI face.
		faces = append(faces, face)
	}
	return faces
}

func newShaper(chinese bool) *text.Shaper {
	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(loadFonts(chinese)))
}
