package nativeui

import (
	"math"
	"testing"

	gopus "github.com/thesyncim/gopus"
)

// A voice packet received from a viewer is decoded and mixed for the
// speakers, unless listening is paused or that person is muted.
func TestVoiceDecodeAndMix(t *testing.T) {
	encoder, err := gopus.NewEncoder(gopus.EncoderConfig{SampleRate: audioRate, Channels: 1, Application: gopus.ApplicationVoIP})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := gopus.NewDecoder(gopus.DecoderConfig{SampleRate: audioRate, Channels: 1})
	if err != nil {
		t.Fatal(err)
	}
	v := newVoiceEndpoint(&hostSession{})
	v.state = &voiceState{Publications: []voicePublication{{ID: "publication-1", PeerID: "viewer-1"}}}
	play := &playback{decoder: decoder}
	v.plays["publication-1"] = play
	pcm := make([]int16, audioFrame)
	packet := make([]byte, 1500)
	phase := 0.0
	for frame := 0; frame < 5; frame++ {
		for i := range pcm {
			pcm[i] = int16(8000 * math.Sin(phase))
			phase += 2 * math.Pi * 300 / audioRate
		}
		n, err := encoder.EncodeInt16(pcm, packet)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]int16, audioFrame*6)
		decoded, err := decoder.DecodeInt16(packet[:n], out)
		if err != nil || decoded != audioFrame {
			t.Fatalf("decoded %d %v", decoded, err)
		}
		play.frames = append(play.frames, out[:audioFrame])
	}
	out := make([]int16, audioFrame)
	for i := 0; i < 4; i++ {
		if !v.mix(out) {
			t.Fatal("nothing mixed")
		}
	}
	if rms(out) < 0.05 {
		t.Fatalf("mixed level %.3f", rms(out))
	}
	v.muted["viewer-1"] = true
	if v.mix(out) {
		t.Fatal("a muted person must not be mixed")
	}
	if v.mix(out) {
		t.Fatal("no frames left")
	}
}
