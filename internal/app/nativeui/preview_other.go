//go:build !windows

package nativeui

func newLivePreview(uintptr) livePreview { return nil }
