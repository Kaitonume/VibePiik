package nativeui

import (
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// TestEchoCancellerProbe (NATIVEUI_AEC_PROBE=1) checks that the Windows Voice
// Capture DSP starts and yields 20 ms frames. Audio is only counted, never kept.
func TestEchoCancellerProbe(t *testing.T) {
	if os.Getenv("NATIVEUI_AEC_PROBE") != "1" {
		t.Skip("NATIVEUI_AEC_PROBE not set")
	}
	var frames atomic.Int64
	var size atomic.Int64
	mic, err := openEchoCancelledMicrophone(-1, func(frame []int16) {
		frames.Add(1)
		size.Store(int64(len(frame)))
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	mic.Close()
	t.Logf("frames in 3 s: %d (frame of %d samples)", frames.Load(), size.Load())
	if frames.Load() < 100 {
		t.Fatalf("too few frames: %d", frames.Load())
	}
}

// TestCaptureDevicesProbe (NATIVEUI_DEVICES_PROBE=1) lists microphones without
// opening any; only the count and whether IDs and names were read are logged.
func TestCaptureDevicesProbe(t *testing.T) {
	if os.Getenv("NATIVEUI_DEVICES_PROBE") != "1" {
		t.Skip("NATIVEUI_DEVICES_PROBE not set")
	}
	devices, err := listCaptureDevices()
	if err != nil {
		t.Fatal(err)
	}
	named := 0
	for i, device := range devices {
		if device.ID != "" && device.Label != "" {
			named++
		}
		if captureIndex(device.ID) != i {
			t.Fatalf("index of device %d is %d", i, captureIndex(device.ID))
		}
		if waveInDevice(device.Label) == waveMapper {
			t.Logf("device %d has no waveIn match", i)
		}
	}
	t.Logf("microphones: %d, with id and name: %d", len(devices), named)
}
