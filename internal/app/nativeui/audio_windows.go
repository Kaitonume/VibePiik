//go:build windows

package nativeui

import (
	"errors"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Microphone capture and speaker playback with the Windows multimedia API
// (winmm waveIn/waveOut): 48 kHz mono PCM16 in 20 ms blocks, default devices.

const (
	audioRate        = 48_000
	audioFrame       = 960 // 20 ms
	audioFrameBytes  = audioFrame * 2
	waveMapper       = 0xFFFFFFFF
	waveFormatPCM    = 1
	whdrDone         = 0x1
	captureBuffers   = 6
	playbackBuffers  = 6
	playbackQueueMax = 4 // 80 ms of queued output
)

var (
	winmm                    = syscall.NewLazyDLL("winmm.dll")
	procWaveInOpen           = winmm.NewProc("waveInOpen")
	procWaveInPrepareHeader  = winmm.NewProc("waveInPrepareHeader")
	procWaveInUnprepare      = winmm.NewProc("waveInUnprepareHeader")
	procWaveInAddBuffer      = winmm.NewProc("waveInAddBuffer")
	procWaveInStart          = winmm.NewProc("waveInStart")
	procWaveInReset          = winmm.NewProc("waveInReset")
	procWaveInClose          = winmm.NewProc("waveInClose")
	procWaveInGetNumDevs     = winmm.NewProc("waveInGetNumDevs")
	procWaveInGetDevCapsW    = winmm.NewProc("waveInGetDevCapsW")
	procWaveOutOpen          = winmm.NewProc("waveOutOpen")
	procWaveOutPrepareHeader = winmm.NewProc("waveOutPrepareHeader")
	procWaveOutUnprepare     = winmm.NewProc("waveOutUnprepareHeader")
	procWaveOutWrite         = winmm.NewProc("waveOutWrite")
	procWaveOutReset         = winmm.NewProc("waveOutReset")
	procWaveOutClose         = winmm.NewProc("waveOutClose")
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procLocalAlloc           = kernel32.NewProc("LocalAlloc")
	procLocalFree            = kernel32.NewProc("LocalFree")
)

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	Size           uint16
}

type waveHdr struct {
	Data          uintptr
	BufferLength  uint32
	BytesRecorded uint32
	User          uintptr
	Flags         uint32
	Loops         uint32
	Next          uintptr
	Reserved      uintptr
}

func pcmFormat() waveFormatEx {
	return waveFormatEx{FormatTag: waveFormatPCM, Channels: 1, SamplesPerSec: audioRate,
		AvgBytesPerSec: audioRate * 2, BlockAlign: 2, BitsPerSample: 16}
}

// osBlock is a WAVEHDR and its data in memory Windows may keep using.
type osBlock struct {
	hdr  *waveHdr
	mem  uintptr
	data []int16
}

func allocBlock() (*osBlock, error) {
	size := unsafe.Sizeof(waveHdr{}) + audioFrameBytes
	mem, _, _ := procLocalAlloc.Call(0x40, size) // LPTR: fixed, zeroed
	if mem == 0 {
		return nil, errors.New("audio buffer allocation failed")
	}
	// LocalAlloc memory is outside the Go heap: convert without vet's heuristic.
	base := *(*unsafe.Pointer)(unsafe.Pointer(&mem))
	hdr := (*waveHdr)(base)
	data := unsafe.Add(base, unsafe.Sizeof(waveHdr{}))
	hdr.Data = uintptr(data)
	hdr.BufferLength = audioFrameBytes
	return &osBlock{hdr: hdr, mem: mem, data: unsafe.Slice((*int16)(data), audioFrame)}, nil
}

func (b *osBlock) free() { procLocalFree.Call(b.mem) }

// microphone captures the default input device and calls onFrame with
// each 20 ms block (the slice is reused after the call).
type microphone struct {
	handle uintptr
	blocks []*osBlock
	stop   chan struct{}
	done   chan struct{}
}

func openMicrophone(device uint32, onFrame func([]int16)) (*microphone, error) {
	format := pcmFormat()
	var handle uintptr
	if result, _, _ := procWaveInOpen.Call(uintptr(unsafe.Pointer(&handle)), uintptr(device), uintptr(unsafe.Pointer(&format)), 0, 0, 0); result != 0 {
		return nil, errors.New("no microphone is available")
	}
	m := &microphone{handle: handle, stop: make(chan struct{}), done: make(chan struct{})}
	for i := 0; i < captureBuffers; i++ {
		block, err := allocBlock()
		if err != nil {
			m.release()
			return nil, err
		}
		m.blocks = append(m.blocks, block)
		procWaveInPrepareHeader.Call(handle, uintptr(unsafe.Pointer(block.hdr)), unsafe.Sizeof(waveHdr{}))
		procWaveInAddBuffer.Call(handle, uintptr(unsafe.Pointer(block.hdr)), unsafe.Sizeof(waveHdr{}))
	}
	if result, _, _ := procWaveInStart.Call(handle); result != 0 {
		m.release()
		return nil, errors.New("microphone could not start")
	}
	frame := make([]int16, audioFrame)
	go func() {
		defer close(m.done)
		next := 0
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-ticker.C:
			}
			for {
				block := m.blocks[next]
				if block.hdr.Flags&whdrDone == 0 {
					break
				}
				count := int(block.hdr.BytesRecorded / 2)
				copy(frame, block.data[:count])
				for i := count; i < audioFrame; i++ {
					frame[i] = 0
				}
				if count > 0 {
					onFrame(frame)
				}
				block.hdr.Flags &^= whdrDone
				block.hdr.BytesRecorded = 0
				procWaveInAddBuffer.Call(m.handle, uintptr(unsafe.Pointer(block.hdr)), unsafe.Sizeof(waveHdr{}))
				next = (next + 1) % len(m.blocks)
			}
		}
	}()
	return m, nil
}

func (m *microphone) release() {
	procWaveInReset.Call(m.handle)
	for _, block := range m.blocks {
		procWaveInUnprepare.Call(m.handle, uintptr(unsafe.Pointer(block.hdr)), unsafe.Sizeof(waveHdr{}))
		block.free()
	}
	m.blocks = nil
	procWaveInClose.Call(m.handle)
}

func (m *microphone) Close() {
	close(m.stop)
	<-m.done
	m.release()
}

// speaker plays 20 ms blocks pulled from next (which fills the frame and
// returns false when there is nothing to play).
type speaker struct {
	handle uintptr
	blocks []*osBlock
	stop   chan struct{}
	done   chan struct{}
	mu     sync.Mutex
}

func openSpeaker(next func([]int16) bool) (*speaker, error) {
	format := pcmFormat()
	var handle uintptr
	if result, _, _ := procWaveOutOpen.Call(uintptr(unsafe.Pointer(&handle)), waveMapper, uintptr(unsafe.Pointer(&format)), 0, 0, 0); result != 0 {
		return nil, errors.New("no audio output is available")
	}
	s := &speaker{handle: handle, stop: make(chan struct{}), done: make(chan struct{})}
	for i := 0; i < playbackBuffers; i++ {
		block, err := allocBlock()
		if err != nil {
			s.release()
			return nil, err
		}
		block.hdr.Flags = whdrDone // free
		s.blocks = append(s.blocks, block)
		procWaveOutPrepareHeader.Call(handle, uintptr(unsafe.Pointer(block.hdr)), unsafe.Sizeof(waveHdr{}))
		block.hdr.Flags |= whdrDone
	}
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
			}
			queued := 0
			for _, block := range s.blocks {
				if block.hdr.Flags&whdrDone == 0 {
					queued++
				}
			}
			// Keep up to playbackQueueMax blocks in flight; refill after stalls.
			for queued < playbackQueueMax {
				var free *osBlock
				for _, block := range s.blocks {
					if block.hdr.Flags&whdrDone != 0 {
						free = block
						break
					}
				}
				if free == nil || !next(free.data) {
					break
				}
				free.hdr.Flags &^= whdrDone
				procWaveOutWrite.Call(s.handle, uintptr(unsafe.Pointer(free.hdr)), unsafe.Sizeof(waveHdr{}))
				queued++
			}
		}
	}()
	return s, nil
}

func (s *speaker) release() {
	procWaveOutReset.Call(s.handle)
	for _, block := range s.blocks {
		procWaveOutUnprepare.Call(s.handle, uintptr(unsafe.Pointer(block.hdr)), unsafe.Sizeof(waveHdr{}))
		block.free()
	}
	s.blocks = nil
	procWaveOutClose.Call(s.handle)
}

func (s *speaker) Close() {
	close(s.stop)
	<-s.done
	s.release()
}

// waveInCaps is WAVEINCAPSW.
type waveInCaps struct {
	Mid, Pid      uint16
	DriverVersion uint32
	Name          [32]uint16
	Formats       uint32
	Channels      uint16
	Reserved      uint16
}

// waveInDevice finds the waveIn device of a microphone by its name (winmm
// names are the endpoint names cut to 31 characters); WAVE_MAPPER otherwise.
func waveInDevice(label string) uint32 {
	if label == "" {
		return waveMapper
	}
	count, _, _ := procWaveInGetNumDevs.Call()
	for i := uintptr(0); i < count; i++ {
		var caps waveInCaps
		if result, _, _ := procWaveInGetDevCapsW.Call(i, uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps)); result != 0 {
			continue
		}
		name := syscall.UTF16ToString(caps.Name[:])
		if name != "" && strings.HasPrefix(label, name) {
			return uint32(i)
		}
	}
	return waveMapper
}
