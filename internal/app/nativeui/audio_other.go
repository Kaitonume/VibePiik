//go:build !windows

package nativeui

import "errors"

const (
	audioRate  = 48_000
	audioFrame = 960
)

type microphone struct{}
type speaker struct{}

func openMicrophone(uint32, func([]int16)) (*microphone, error) {
	return nil, errors.New("unsupported")
}
func waveInDevice(string) uint32                       { return 0 }
func (*microphone) Close()                             {}
func openSpeaker(func([]int16) bool) (*speaker, error) { return nil, errors.New("unsupported") }
func (*speaker) Close()                                {}

type echoMicrophone struct{}

func (*echoMicrophone) Close() {}

func openEchoCancelledMicrophone(int, func([]int16)) (*echoMicrophone, error) {
	return nil, errors.New("unsupported")
}

type audioDevice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func listCaptureDevices() ([]audioDevice, error) { return nil, errors.New("unsupported") }
func captureIndex(string) int                    { return -1 }
