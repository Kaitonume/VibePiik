//go:build !windows

package piiktv

import (
	"os"
	"strings"
)

// Language picks the language of the Piik TV texts on the Piik pages: "it" or
// "en", from the same locale variables as the App console.
func Language() string {
	var locale string
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale = os.Getenv(name); locale != "" {
			break
		}
	}
	if locale == "" || locale == "C" || locale == "POSIX" || strings.HasPrefix(locale, "C.") {
		return "en"
	}
	if preferred, _, _ := strings.Cut(os.Getenv("LANGUAGE"), ":"); preferred != "" {
		locale = preferred
	}
	if end := strings.IndexAny(locale, "_-.@"); end >= 0 {
		locale = locale[:end]
	}
	if strings.EqualFold(locale, "it") {
		return "it"
	}
	return "en"
}
