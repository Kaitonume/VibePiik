package nativeui

import "testing"

func TestEveryGlyphParses(t *testing.T) {
	if len(glyphSources) < 80 {
		t.Fatalf("only %d glyphs", len(glyphSources))
	}
	for name, source := range glyphSources {
		shapes, err := parseFragment(source.svg, source.solid)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(shapes) == 0 {
			t.Errorf("%s: no shapes", name)
		}
		for _, shape := range shapes {
			if len(shape.segs) == 0 && len(shape.dots) == 0 {
				t.Errorf("%s: empty shape", name)
			}
		}
	}
}

func TestTokenizeCompactPathData(t *testing.T) {
	got := tokenizePath("M9.5 8.5a2.5 2.5 0 0 1 5 0c0 2-2.5 2-2.5 4M12 16.5h.01l-.5.5a1 1 0 011 1")
	want := []string{"M", "9.5", "8.5", "a", "2.5", "2.5", "0", "0", "1", "5", "0", "c", "0", "2", "-2.5", "2", "-2.5", "4",
		"M", "12", "16.5", "h", ".01", "l", "-.5", ".5", "a", "1", "1", "0", "0", "1", "1", "1"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d: got %q want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestDotSubpaths(t *testing.T) {
	shapes, err := parseFragment(`<path d="M9 5h.01M15 5h.01M3 3l18 18"/>`, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes[0].dots) != 2 || len(shapes[0].segs) != 2 {
		t.Fatalf("dots %v segs %d", shapes[0].dots, len(shapes[0].segs))
	}
}
