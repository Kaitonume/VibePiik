//go:build !windows

package nativeui

func newChatOverlay(uintptr) chatOverlay { return nil }

func screenChoices() []screenChoice { return nil }
