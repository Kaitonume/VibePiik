package piiktv

import "syscall"

// Language picks the language of the Piik TV texts on the Piik pages: "it" or
// "en". Like the App console, it follows the Windows display language, not
// regional formats or the browser.
func Language() string {
	language, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	return languageFromWindowsUI(language)
}

func languageFromWindowsUI(language uintptr) string {
	// PRIMARYLANGID: every Italian region has LANG_ITALIAN (0x10).
	// https://learn.microsoft.com/en-us/windows/win32/api/winnt/nf-winnt-primarylangid
	if language&0x3ff == 0x10 {
		return "it"
	}
	return "en"
}
