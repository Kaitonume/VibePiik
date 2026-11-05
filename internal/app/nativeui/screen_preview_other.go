//go:build !windows

package nativeui

import (
	"errors"
	"image"
)

func captureMonitor(string, int) (*image.RGBA, error) { return nil, errors.New("unsupported") }
