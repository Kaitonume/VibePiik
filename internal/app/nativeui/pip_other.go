//go:build !windows

package nativeui

import "github.com/TNTcraftHIM/Piik/internal/app/nativecapture"

func openPictureInPicture(nativecapture.CaptureTarget, string) (func(), <-chan struct{}, bool) {
	return nil, nil, false
}
