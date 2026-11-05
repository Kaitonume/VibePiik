//go:build !windows

package nativeui

import "image"

func systemDark() bool       { return false }
func systemLanguage() string { return "en" }

var reducedMotion = false

func cursorInWindow(uintptr) (image.Point, bool) { return image.Point{}, false }
func cursorOnScreen() (image.Point, bool)        { return image.Point{}, false }
