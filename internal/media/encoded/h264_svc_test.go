package encoded

import (
	"bytes"
	"testing"
)

func TestStripSVCNALUnitsRemovesPrefixNAL(t *testing.T) {
	// AMD AMF access unit: AUD, prefix NAL (type 14), slice, filler.
	au := []byte{
		0, 0, 0, 1, 0x09, 0x30,
		0, 0, 0, 1, 0x4e, 0x80, 0x80, 0x00, 0x20,
		0, 0, 0, 1, 0x41, 0x9a, 0x02, 0x03,
		0, 0, 1, 0x0c, 0xff, 0xff,
	}
	want := []byte{
		0, 0, 0, 1, 0x09, 0x30,
		0, 0, 0, 1, 0x41, 0x9a, 0x02, 0x03,
		0, 0, 0, 1, 0x0c, 0xff, 0xff,
	}
	if got := StripSVCNALUnits(au); !bytes.Equal(got, want) {
		t.Fatalf("got % x\nwant % x", got, want)
	}
}

func TestStripSVCNALUnitsKeepsPlainAVC(t *testing.T) {
	au := []byte{0, 0, 0, 1, 0x67, 0x42, 0xc0, 0x1f, 0, 0, 0, 1, 0x68, 0xce, 0, 0, 1, 0x65, 0x88, 0x84}
	got := StripSVCNALUnits(au)
	if &got[0] != &au[0] || len(got) != len(au) {
		t.Fatal("an access unit without SVC NAL units must be returned unchanged")
	}
}

func TestH264PayloaderSendsNoPrefixPacket(t *testing.T) {
	au := []byte{
		0, 0, 0, 1, 0x09, 0x30,
		0, 0, 0, 1, 0x4e, 0x80, 0x80, 0x00, 0x20,
		0, 0, 0, 1, 0x41, 0x9a, 0x02, 0x03,
	}
	payloads := (&H264Payloader{}).Payload(1200, au)
	if len(payloads) != 1 || payloads[0][0]&0x1f != 1 {
		t.Fatalf("want one packet carrying the slice, got %d packets", len(payloads))
	}
}
