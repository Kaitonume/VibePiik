package piiktv

import "testing"

func TestLanguageFromWindowsUI(t *testing.T) {
	for language, want := range map[uintptr]string{
		0x0410: "it", // it-IT
		0x0810: "it", // it-CH
		0x0409: "en", // en-US
		0x0804: "en", // zh-CN: the Piik TV texts have no Chinese
		0x0407: "en", // de-DE
	} {
		if got := languageFromWindowsUI(language); got != want {
			t.Errorf("languageFromWindowsUI(%#04x) = %q, want %q", language, got, want)
		}
	}
	if got := Language(); got != "it" && got != "en" {
		t.Fatalf("Language() = %q", got)
	}
}
