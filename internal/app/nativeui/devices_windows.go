//go:build windows

package nativeui

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

// Active capture endpoints (microphones) through the Core Audio
// IMMDeviceEnumerator. Their order is the one the Voice Capture DSP uses for
// MFPKEY_WMAAECMA_DEVICE_INDEXES.

var (
	clsidMMDeviceEnumerator = comGUID{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = comGUID{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	keyDeviceFriendlyName   = propertyKey{comGUID{0xA45C254E, 0xDF1C, 0x4EFD, [8]byte{0x80, 0x20, 0x67, 0xD1, 0x46, 0xA8, 0x50, 0xE0}}, 14}
	// MFPKEY_WMAAECMA_DEVICE_INDEXES.
	keyAECDeviceIndexes = propertyKey{comGUID{0x6F52C567, 0x0360, 0x4BD2, [8]byte{0x96, 0x17, 0xCC, 0xBF, 0x14, 0x21, 0xC9, 0x39}}, 4}

	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	procPropVariantClear = ole32.NewProc("PropVariantClear")
)

const (
	eCapture          = 1
	deviceStateActive = 1
	vtLPWSTR          = 31
)

// audioDevice is one microphone: Windows endpoint ID and friendly name.
type audioDevice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func wideString(address uintptr) string {
	if address == 0 {
		return ""
	}
	var chars []uint16
	for p := address; ; p += 2 {
		c := *(*uint16)(osPointer(p))
		if c == 0 || len(chars) > 4096 {
			break
		}
		chars = append(chars, c)
	}
	return syscall.UTF16ToString(chars)
}

// listCaptureDevices lists active microphones in endpoint order.
func listCaptureDevices() ([]audioDevice, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procCoInitializeEx.Call(0, coinitMultithreaded)
	defer procCoUninitialize.Call()
	var enumerator uintptr
	if hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, 0x17, // CLSCTX_ALL
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enumerator))); hr != 0 || enumerator == 0 {
		return nil, errors.New("audio devices are unavailable")
	}
	defer comCall(enumerator, 2)
	var collection uintptr
	if hr := comCall(enumerator, 3, eCapture, deviceStateActive, uintptr(unsafe.Pointer(&collection))); hr != 0 || collection == 0 {
		return nil, errors.New("microphones could not be listed")
	}
	defer comCall(collection, 2)
	var count uint32
	if hr := comCall(collection, 3, uintptr(unsafe.Pointer(&count))); hr != 0 {
		return nil, errors.New("microphones could not be counted")
	}
	devices := make([]audioDevice, 0, count)
	for i := uint32(0); i < count; i++ {
		var device uintptr
		if hr := comCall(collection, 4, uintptr(i), uintptr(unsafe.Pointer(&device))); hr != 0 || device == 0 {
			continue
		}
		entry := audioDevice{}
		var id uintptr
		if comCall(device, 5, uintptr(unsafe.Pointer(&id))) == 0 { // GetId
			entry.ID = wideString(id)
			procCoTaskMemFree.Call(id)
		}
		var store uintptr
		if comCall(device, 4, 0, uintptr(unsafe.Pointer(&store))) == 0 && store != 0 { // OpenPropertyStore(STGM_READ)
			var value propVariant
			if comCall(store, 5, uintptr(unsafe.Pointer(&keyDeviceFriendlyName)), uintptr(unsafe.Pointer(&value))) == 0 && value.VT == vtLPWSTR {
				entry.Label = wideString(uintptr(value.Value))
			}
			procPropVariantClear.Call(uintptr(unsafe.Pointer(&value)))
			comCall(store, 2)
		}
		comCall(device, 2)
		if entry.ID != "" {
			devices = append(devices, entry)
		}
	}
	return devices, nil
}

// captureIndex is the endpoint index of a microphone ID, -1 for the default.
func captureIndex(id string) int {
	if id == "" {
		return -1
	}
	devices, err := listCaptureDevices()
	if err != nil {
		return -1
	}
	for i, device := range devices {
		if device.ID == id {
			return i
		}
	}
	return -1
}

// aecDeviceIndexes is the DEVICE_INDEXES value: render in the high word,
// capture in the low word, -1 for the default device.
func aecDeviceIndexes(capture int) int64 {
	return int64(int32(uint32(0xFFFF)<<16 | uint32(capture)&0xFFFF))
}
