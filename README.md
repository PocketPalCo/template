# template

This project provides a simple REST API and WebSocket server. It now also exposes a WebRTC endpoint for receiving raw PCM audio from a browser.

## WebRTC usage

1. Start the Go server (`go run ./cmd`).
2. Open `public/webrtc.html` in a browser.
3. Click **Start WebRTC** and grant microphone permission. The page sends microphone audio to the server using WebRTC.

The WebRTC endpoint is available at `/webrtc/offer` and accepts a JSON payload containing the client's SDP offer. The server responds with an SDP answer after ICE gathering completes. Incoming audio is forwarded to a placeholder function for integration with other services.
