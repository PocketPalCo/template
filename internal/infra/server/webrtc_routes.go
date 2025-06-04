package server

import (
	"encoding/json"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/pion/webrtc/v3"
)

// offerRequest represents a WebRTC SDP offer.
type offerRequest struct {
	SDP string `json:"sdp"`
}

// answerResponse represents a WebRTC SDP answer.
type answerResponse struct {
	SDP string `json:"sdp"`
}

func setupWebRTC(app *fiber.App) {
	api := webrtc.NewAPI()

	app.Post("/webrtc/offer", func(c *fiber.Ctx) error {
		var req offerRequest
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		peer, err := api.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			slog.Error("webrtc: create peer", slog.String("err", err.Error()))
			return fiber.ErrInternalServerError
		}

		peer.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
			go handleRemoteTrack(track)
		})

		// Apply the offer from the client.
		if err = peer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: req.SDP}); err != nil {
			slog.Error("webrtc: remote description", slog.String("err", err.Error()))
			return fiber.ErrInternalServerError
		}

		// Create and set the local answer.
		answer, err := peer.CreateAnswer(nil)
		if err != nil {
			slog.Error("webrtc: create answer", slog.String("err", err.Error()))
			return fiber.ErrInternalServerError
		}
		if err = peer.SetLocalDescription(answer); err != nil {
			slog.Error("webrtc: local description", slog.String("err", err.Error()))
			return fiber.ErrInternalServerError
		}

		<-webrtc.GatheringCompletePromise(peer)

		local := peer.LocalDescription()
		resp, _ := json.Marshal(answerResponse{SDP: local.SDP})
		return c.Send(resp)
	})
}

// handleRemoteTrack forwards PCM payloads from the received track to other services.
func handleRemoteTrack(track *webrtc.TrackRemote) {
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			slog.Error("webrtc: read rtp", slog.String("err", err.Error()))
			return
		}

		forwardPCMToMicroservice(pkt.Payload)
	}
}

// forwardPCMToMicroservice is a placeholder for delivering PCM data to a downstream service.
func forwardPCMToMicroservice(data []byte) {
	slog.Debug("pcm chunk received", slog.Int("bytes", len(data)))
}
