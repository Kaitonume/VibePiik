package encoded

import (
	"encoding/hex"

	"github.com/pion/rtp/codecs"
)

// H264Payloader packetizes AVC access units without their SVC NAL units.
//
// Some hardware encoders (AMD AMF) put an SVC prefix NAL unit (type 14) before
// every slice. AVC decoders ignore it, but Pion sends it as its own tiny RTP
// packet at the start of each frame. When Wi-Fi reorders that packet behind the
// slice that carries the marker bit, libwebrtc's H.264 packet buffer completes
// the frame without it and leaves the late packet as a frame that never ends:
// every following delta frame then waits for a keyframe (about 3 s on PIIK TV).
type H264Payloader struct {
	codecs.H264Payloader
}

func (payloader *H264Payloader) Payload(mtu uint16, payload []byte) [][]byte {
	return payloader.H264Payloader.Payload(mtu, StripSVCNALUnits(payload))
}

func isSVCNALUnit(header byte) bool {
	switch header & 0x1f {
	case 14, 15, 20: // prefix NAL, subset SPS, coded slice extension
		return true
	}
	return false
}

// StripSVCNALUnits returns the Annex-B access unit without SVC NAL units. The
// input is returned unchanged when it contains none.
func StripSVCNALUnits(data []byte) []byte {
	type nal struct{ start, end int }
	var units []nal
	strip := false
	previous := -1
	for index := 0; index+2 < len(data); index++ {
		if data[index] != 0 || data[index+1] != 0 || data[index+2] != 1 {
			continue
		}
		begin := index
		if begin > 0 && data[begin-1] == 0 {
			begin--
		}
		if previous >= 0 {
			units = append(units, nal{previous, begin})
		}
		previous = index + 3
		if previous < len(data) && isSVCNALUnit(data[previous]) {
			strip = true
		}
		index += 2
	}
	if !strip {
		return data
	}
	units = append(units, nal{previous, len(data)})
	out := make([]byte, 0, len(data))
	for _, unit := range units {
		if unit.start >= unit.end || isSVCNALUnit(data[unit.start]) {
			continue
		}
		out = append(out, 0, 0, 0, 1)
		out = append(out, data[unit.start:unit.end]...)
	}
	return out
}

// H264ConstrainedBaselineLevel recognizes the equivalent coding-tool subsets
// in RFC 6184 Table 5. Callers own their permitted level range.
func H264ConstrainedBaselineLevel(value string) (byte, bool) {
	if len(value) != 6 {
		return 0, false
	}
	profile, err := hex.DecodeString(value)
	if err != nil {
		return 0, false
	}
	constrainedBaseline := (profile[0] == 0x42 && profile[1]&0x4f == 0x40) ||
		(profile[0] == 0x4d && profile[1]&0x8f == 0x80) ||
		(profile[0] == 0x58 && profile[1]&0xcf == 0xc0)
	return profile[2], constrainedBaseline
}
