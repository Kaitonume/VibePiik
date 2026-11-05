//go:build windows

package nativeui

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Echo-cancelled microphone: the Windows Voice Capture DSP (CWMAudioAEC, the
// DMO behind Windows communications apps). In source mode it opens the
// default microphone itself and takes the default speakers' output as the echo
// reference, then returns clean 16 kHz mono speech with echo cancellation,
// noise suppression and automatic gain applied.

type comGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidVoiceCaptureDSP = comGUID{0x745057C7, 0xF353, 0x4F2D, [8]byte{0xA7, 0xEE, 0x58, 0x43, 0x44, 0x77, 0x73, 0x0E}}
	iidIMediaObject      = comGUID{0xD8AD0F58, 0x5494, 0x4102, [8]byte{0x97, 0xC5, 0xEC, 0x79, 0x8E, 0x59, 0xBC, 0xF4}}
	iidIPropertyStore    = comGUID{0x886D8EEB, 0x8CF2, 0x4446, [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99}}
	iidIUnknown          = comGUID{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIMediaBuffer      = comGUID{0x59EFF8B9, 0x938C, 0x4A26, [8]byte{0x82, 0xF2, 0x95, 0xCB, 0x84, 0xCD, 0xC8, 0x37}}
	mediaTypeAudio       = comGUID{0x73647561, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
	mediaSubtypePCM      = comGUID{0x00000001, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
	formatWaveFormatEx   = comGUID{0x05589F81, 0xC356, 0x11CE, [8]byte{0xBF, 0x01, 0x00, 0xAA, 0x00, 0x55, 0x59, 0x5A}}
	// MFPKEY_WMAAECMA_SYSTEM_MODE and MFPKEY_WMAAECMA_DMO_SOURCE_MODE.
	keyAECSystemMode = propertyKey{comGUID{0x6F52C567, 0x0360, 0x4BD2, [8]byte{0x96, 0x17, 0xCC, 0xBF, 0x14, 0x21, 0xC9, 0x39}}, 2}
	keyAECSourceMode = propertyKey{comGUID{0x6F52C567, 0x0360, 0x4BD2, [8]byte{0x96, 0x17, 0xCC, 0xBF, 0x14, 0x21, 0xC9, 0x39}}, 3}
)

var (
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
)

type propertyKey struct {
	FmtID comGUID
	PID   uint32
}

// propVariant is PROPVARIANT on 64-bit Windows (24 bytes).
type propVariant struct {
	VT        uint16
	reserved1 uint16
	reserved2 uint16
	reserved3 uint16
	Value     int64
	pad       int64
}

type dmoMediaType struct {
	MajorType           comGUID
	SubType             comGUID
	FixedSizeSamples    int32
	TemporalCompression int32
	SampleSize          uint32
	FormatType          comGUID
	Unk                 uintptr
	FormatSize          uint32
	Format              uintptr
}

type dmoOutputDataBuffer struct {
	Buffer     uintptr
	Status     uint32
	_          uint32
	Timestamp  int64
	Timelength int64
}

const (
	aecRate                 = 16_000
	aecFrame                = aecRate / 50 // 20 ms
	aecBufferBytes          = aecRate * 2  // 1 s of mono PCM16
	dmoOutputIncomplete     = 0x01000000
	coinitMultithreaded     = 0x0
	clsctxInprocServer      = 0x1
	vtI4                    = 3
	vtBool                  = 11
	singleChannelAEC        = 0
	eNoInterface            = 0x80004002
	sOK                     = 0
	sFalse                  = 1
	imediabufferMethodCount = 6
)

// comCall calls method index of the COM object obj.
func comCall(obj uintptr, index int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(osPointer(obj))
	method := *(*uintptr)(osPointer(vtbl + uintptr(index)*unsafe.Sizeof(uintptr(0))))
	result, _, _ := syscall.SyscallN(method, append([]uintptr{obj}, args...)...)
	return result
}

// mediaBuffer is a minimal IMediaBuffer for ProcessOutput, in OS memory:
// [vtbl pointer][length][max][data...].
type mediaBuffer struct {
	obj  uintptr
	vtbl uintptr
	data uintptr
}

var (
	bufferCallbacks     [imediabufferMethodCount]uintptr
	bufferCallbacksOnce sync.Once
)

func mediaBufferFields(this uintptr) (length *uint32, maxLength *uint32, data uintptr) {
	base := *(*unsafe.Pointer)(unsafe.Pointer(&this))
	length = (*uint32)(unsafe.Add(base, 8))
	maxLength = (*uint32)(unsafe.Add(base, 12))
	return length, maxLength, this + 16
}

func newMediaBuffer() (*mediaBuffer, error) {
	bufferCallbacksOnce.Do(func() {
		bufferCallbacks[0] = syscall.NewCallback(func(this, riid, out uintptr) uintptr {
			id := *(*comGUID)(*(*unsafe.Pointer)(unsafe.Pointer(&riid)))
			if id == iidIUnknown || id == iidIMediaBuffer {
				*(*uintptr)(*(*unsafe.Pointer)(unsafe.Pointer(&out))) = this
				return sOK
			}
			*(*uintptr)(*(*unsafe.Pointer)(unsafe.Pointer(&out))) = 0
			return eNoInterface
		})
		bufferCallbacks[1] = syscall.NewCallback(func(this uintptr) uintptr { return 1 }) // AddRef
		bufferCallbacks[2] = syscall.NewCallback(func(this uintptr) uintptr { return 1 }) // Release
		bufferCallbacks[3] = syscall.NewCallback(func(this, length uintptr) uintptr {     // SetLength
			current, maxLength, _ := mediaBufferFields(this)
			if uint32(length) > *maxLength {
				return 0x80070057 // E_INVALIDARG
			}
			*current = uint32(length)
			return sOK
		})
		bufferCallbacks[4] = syscall.NewCallback(func(this, out uintptr) uintptr { // GetMaxLength
			_, maxLength, _ := mediaBufferFields(this)
			*(*uint32)(*(*unsafe.Pointer)(unsafe.Pointer(&out))) = *maxLength
			return sOK
		})
		bufferCallbacks[5] = syscall.NewCallback(func(this, buffer, length uintptr) uintptr { // GetBufferAndLength
			current, _, data := mediaBufferFields(this)
			if buffer != 0 {
				*(*uintptr)(*(*unsafe.Pointer)(unsafe.Pointer(&buffer))) = data
			}
			if length != 0 {
				*(*uint32)(*(*unsafe.Pointer)(unsafe.Pointer(&length))) = *current
			}
			return sOK
		})
	})
	vtbl, _, _ := procLocalAlloc.Call(0x40, uintptr(imediabufferMethodCount)*unsafe.Sizeof(uintptr(0)))
	obj, _, _ := procLocalAlloc.Call(0x40, 16+aecBufferBytes)
	if vtbl == 0 || obj == 0 {
		return nil, errors.New("audio buffer allocation failed")
	}
	table := unsafe.Slice((*uintptr)(*(*unsafe.Pointer)(unsafe.Pointer(&vtbl))), imediabufferMethodCount)
	copy(table, bufferCallbacks[:])
	*(*uintptr)(*(*unsafe.Pointer)(unsafe.Pointer(&obj))) = vtbl
	_, maxLength, _ := mediaBufferFields(obj)
	*maxLength = aecBufferBytes
	return &mediaBuffer{obj: obj, vtbl: vtbl, data: obj + 16}, nil
}

func (b *mediaBuffer) free() {
	procLocalFree.Call(b.obj)
	procLocalFree.Call(b.vtbl)
}

// echoMicrophone runs the Voice Capture DSP on its own COM thread.
type echoMicrophone struct {
	stop chan struct{}
	done chan struct{}
}

func (m *echoMicrophone) Close() {
	close(m.stop)
	<-m.done
}

// openEchoCancelledMicrophone starts the DSP on microphone index device (-1:
// default) and calls onFrame with 20 ms
// blocks of 16 kHz mono PCM16 (the slice is reused after the call).
func openEchoCancelledMicrophone(device int, onFrame func([]int16)) (*echoMicrophone, error) {
	m := &echoMicrophone{stop: make(chan struct{}), done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		defer close(m.done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		procCoInitializeEx.Call(0, coinitMultithreaded)
		defer procCoUninitialize.Call()
		var dmo uintptr
		if hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidVoiceCaptureDSP)), 0, clsctxInprocServer,
			uintptr(unsafe.Pointer(&iidIMediaObject)), uintptr(unsafe.Pointer(&dmo))); hr != 0 || dmo == 0 {
			ready <- errors.New("echo cancellation is unavailable")
			return
		}
		defer comCall(dmo, 2) // Release
		var store uintptr
		if hr := comCall(dmo, 0, uintptr(unsafe.Pointer(&iidIPropertyStore)), uintptr(unsafe.Pointer(&store))); hr != 0 || store == 0 {
			ready <- errors.New("echo cancellation settings are unavailable")
			return
		}
		defer comCall(store, 2)
		mode := propVariant{VT: vtI4, Value: singleChannelAEC}
		source := propVariant{VT: vtBool, Value: -1}
		comCall(store, 6, uintptr(unsafe.Pointer(&keyAECSourceMode)), uintptr(unsafe.Pointer(&source)))
		if device >= 0 {
			// A chosen microphone; the echo reference stays the default speakers.
			indexes := propVariant{VT: vtI4, Value: aecDeviceIndexes(device)}
			comCall(store, 6, uintptr(unsafe.Pointer(&keyAECDeviceIndexes)), uintptr(unsafe.Pointer(&indexes)))
		}
		if hr := comCall(store, 6, uintptr(unsafe.Pointer(&keyAECSystemMode)), uintptr(unsafe.Pointer(&mode))); hr != 0 {
			ready <- errors.New("echo cancellation mode was refused")
			return
		}
		format := waveFormatEx{FormatTag: waveFormatPCM, Channels: 1, SamplesPerSec: aecRate, AvgBytesPerSec: aecRate * 2,
			BlockAlign: 2, BitsPerSample: 16}
		mediaType := dmoMediaType{MajorType: mediaTypeAudio, SubType: mediaSubtypePCM, FixedSizeSamples: 1, SampleSize: 2,
			FormatType: formatWaveFormatEx, FormatSize: uint32(unsafe.Sizeof(format)), Format: uintptr(unsafe.Pointer(&format))}
		if hr := comCall(dmo, 9, 0, uintptr(unsafe.Pointer(&mediaType)), 0); hr != 0 { // SetOutputType
			ready <- errors.New("echo cancellation format was refused")
			return
		}
		runtime.KeepAlive(&format)
		if hr := comCall(dmo, 18); hr != 0 { // AllocateStreamingResources
			ready <- errors.New("echo cancellation could not start (no microphone?)")
			return
		}
		defer comCall(dmo, 19)
		buffer, err := newMediaBuffer()
		if err != nil {
			ready <- err
			return
		}
		defer buffer.free()
		ready <- nil
		frame := make([]int16, aecFrame)
		filled := 0
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-ticker.C:
			}
			for {
				length, _, data := mediaBufferFields(buffer.obj)
				*length = 0
				output := dmoOutputDataBuffer{Buffer: buffer.obj}
				var status uint32
				hr := comCall(dmo, 22, 0, 1, uintptr(unsafe.Pointer(&output)), uintptr(unsafe.Pointer(&status))) // ProcessOutput
				if hr != sOK && hr != sFalse {
					break
				}
				samples := unsafe.Slice((*int16)(*(*unsafe.Pointer)(unsafe.Pointer(&data))), *length/2)
				for _, sample := range samples {
					frame[filled] = sample
					filled++
					if filled == aecFrame {
						onFrame(frame)
						filled = 0
					}
				}
				if output.Status&dmoOutputIncomplete == 0 {
					break
				}
			}
		}
	}()
	if err := <-ready; err != nil {
		<-m.done
		return nil, err
	}
	return m, nil
}

// osPointer turns an address of memory outside the Go heap (COM objects,
// LocalAlloc) into a pointer.
func osPointer(address uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&address)) }
