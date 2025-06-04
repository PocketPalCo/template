//go:build integration

package rtc

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// signalPeers performs a minimal SDP+ICE exchange between two peer connections.
func signalPeers(t *testing.T, a, b *webrtc.PeerConnection) {
	t.Helper()
	a.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			b.AddICECandidate(c.ToJSON())
		}
	})
	b.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			a.AddICECandidate(c.ToJSON())
		}
	})
	offer, err := a.CreateOffer(nil)
	if err != nil {
		t.Fatalf("offer: %v", err)
	}
	if err := a.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local: %v", err)
	}
	if err := b.SetRemoteDescription(offer); err != nil {
		t.Fatalf("set remote: %v", err)
	}
	answer, err := b.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if err := b.SetLocalDescription(answer); err != nil {
		t.Fatalf("set local: %v", err)
	}
	if err := a.SetRemoteDescription(answer); err != nil {
		t.Fatalf("set remote: %v", err)
	}
}

func waitConnected(t *testing.T, pc *webrtc.PeerConnection) {
	t.Helper()
	done := make(chan struct{})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			close(done)
		}
	})
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("peer connection not established")
	}
}

// TestRTCAudio verifies a simple audio track exchange between peers.
func TestRTCAudio(t *testing.T) {
	offerer, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("offer pc: %v", err)
	}
	answerer, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("answer pc: %v", err)
	}
	defer offerer.Close()
	defer answerer.Close()

	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "audio", "pion")
	if err != nil {
		t.Fatalf("create track: %v", err)
	}
	if _, err := offerer.AddTrack(track); err != nil {
		t.Fatalf("add track: %v", err)
	}

	received := make(chan struct{})
	answerer.OnTrack(func(*webrtc.TrackRemote, *webrtc.RTPReceiver) {
		close(received)
	})

	signalPeers(t, offerer, answerer)
	waitConnected(t, offerer)
	waitConnected(t, answerer)

	if err := track.WriteSample(media.Sample{Data: []byte{0x00}, Duration: time.Second}); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatalf("did not receive audio")
	}
}
