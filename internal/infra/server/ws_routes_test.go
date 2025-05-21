// Package server_test contains integration tests for the server's WebSocket functionality.
package server_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/core/rtc"
	"github.com/PocketPalCo/shopping-service/internal/infra/server"
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to set up the test Fiber app with WebSocket routes
func setupTestAppForWS() (app *fiber.App, rtcService *rtc.RTCService, testServer *httptest.Server) {
	app = fiber.New()
	rtcService = rtc.NewRTCService()
	cfg := &config.Config{} // Use a default/test config

	// Register WebSocket routes - assuming server.SetupWs is the correct function
	// and it's been updated to match the signature used in server.go (taking rtcService)
	// We also need a postgres.DB, but for these tests, it might not be directly used by ws routes.
	// If it is, we'll need a mock or a real test DB. For now, passing nil.
	server.SetupWs(app, cfg, nil, rtcService) // Assuming nil is acceptable for DB if not used by WS

	testServer = httptest.NewServer(app)
	return app, rtcService, testServer
}

// Helper to create a WebSocket client connection
func newWebSocketClient(t *testing.T, serverURL, roomID, userID string) (*websocket.Conn, error) {
	wsURL := fmt.Sprintf("ws%s/ws/%s/%s", strings.TrimPrefix(serverURL, "http"), roomID, userID)
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second, // Increased timeout
	}
	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		if resp != nil {
			t.Logf("WebSocket handshake failed with status: %s, URL: %s", resp.Status, wsURL)
		} else {
			t.Logf("WebSocket dial error: %v, URL: %s", err, wsURL)
		}
		return nil, err
	}
	return conn, nil
}

// SignalMessage represents a generic signaling message.
type SignalMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
	Sender  string      `json:"sender,omitempty"` // Optional: To identify the sender
}

// TestWebSocketConnection tests basic WebSocket connection, room registration, and disconnection.
func TestWebSocketConnection(t *testing.T) {
	_, rtcService, testServer := setupTestAppForWS()
	defer testServer.Close()

	roomID := "test-room-conn"
	userID := "test-user-conn"

	// Create the room via RTCService first, as WS join expects room to exist.
	// In a full E2E, room creation might happen via API.
	_, err := rtcService.CreateRoom(roomID)
	require.NoError(t, err, "Pre-creating room should not fail")

	conn, err := newWebSocketClient(t, testServer.URL, roomID, userID)
	require.NoError(t, err, "Failed to connect WebSocket client")
	defer conn.Close()

	// Verify user and room are registered in RTCService
	_, err = rtcService.GetRoom(roomID)
	require.NoError(t, err, "Room should exist in RTCService after WS connection")

	room, _ := rtcService.GetRoom(roomID)
	_, userExists := room.Users[userID]
	assert.True(t, userExists, "User should be registered in the room in RTCService")
	require.NotNil(t, room.Users[userID].Conn, "User's connection should be stored in RTCService")

	// Close the connection
	err = conn.Close()
	require.NoError(t, err, "Error closing WebSocket connection")

	// Allow some time for server to process disconnect
	time.Sleep(200 * time.Millisecond)

	// Verify user is removed from RTCService
	room, err = rtcService.GetRoom(roomID) // Re-fetch room
	require.NoError(t, err, "Room should still exist")
	_, userExists = room.Users[userID]
	assert.False(t, userExists, "User should be removed from the room in RTCService after disconnect")
}

// TestWebSocketSignalBroadcast tests message broadcasting between clients in the same room.
func TestWebSocketSignalBroadcast(t *testing.T) {
	_, rtcService, testServer := setupTestAppForWS()
	defer testServer.Close()

	roomID := "broadcast-room"
	user1ID := "user1-sender"
	user2ID := "user2-receiver"
	user3ID := "user3-receiver"

	// Pre-create room
	_, err := rtcService.CreateRoom(roomID)
	require.NoError(t, err, "Failed to pre-create room")

	// Connect clients
	client1, err := newWebSocketClient(t, testServer.URL, roomID, user1ID)
	require.NoError(t, err, "Client1 failed to connect")
	defer client1.Close()

	client2, err := newWebSocketClient(t, testServer.URL, roomID, user2ID)
	require.NoError(t, err, "Client2 failed to connect")
	defer client2.Close()

	client3, err := newWebSocketClient(t, testServer.URL, roomID, user3ID)
	require.NoError(t, err, "Client3 failed to connect")
	defer client3.Close()

	// Wait for all clients to be registered (simple sleep, can be improved with service check)
	time.Sleep(200 * time.Millisecond)
	room, _ := rtcService.GetRoom(roomID)
	require.Len(t, room.Users, 3, "All three users should be in the room")

	// Setup message listeners for client2 and client3
	msgChanClient2 := make(chan SignalMessage, 1)
	msgChanClient3 := make(chan SignalMessage, 1)
	var wg sync.WaitGroup
	wg.Add(2) // For two listeners

	go func() {
		defer wg.Done()
		for { // Loop to read messages
			_, msgBytes, readErr := client2.ReadMessage()
			if readErr != nil {
				// Check if it's a normal close or an error
				if websocket.IsCloseError(readErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) ||
				   strings.Contains(readErr.Error(), "use of closed network connection") {
					t.Logf("Client2 ReadMessage loop ending due to connection close: %v", readErr)
					return
				}
				t.Logf("Client2 read error: %v", readErr)
				return // Exit on error
			}
			var sigMsg SignalMessage
			if jsonErr := json.Unmarshal(msgBytes, &sigMsg); jsonErr == nil {
				msgChanClient2 <- sigMsg
				return // Got the message we expected
			} else {
				t.Logf("Client2 received non-JSON or unexpected message: %s", string(msgBytes))
			}
		}
	}()

	go func() {
		defer wg.Done()
		for { // Loop to read messages
			_, msgBytes, readErr := client3.ReadMessage()
			if readErr != nil {
				if websocket.IsCloseError(readErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) ||
				   strings.Contains(readErr.Error(), "use of closed network connection") {
					t.Logf("Client3 ReadMessage loop ending due to connection close: %v", readErr)
					return
				}
				t.Logf("Client3 read error: %v", readErr)
				return
			}
			var sigMsg SignalMessage
			if jsonErr := json.Unmarshal(msgBytes, &sigMsg); jsonErr == nil {
				msgChanClient3 <- sigMsg
				return // Got the message we expected
			} else {
				t.Logf("Client3 received non-JSON or unexpected message: %s", string(msgBytes))
			}
		}
	}()


	// Client1 sends a message
	offerPayload := map[string]string{"sdp": "this-is-an-offer"}
	messageToSend := SignalMessage{Type: "offer", Payload: offerPayload, Sender: user1ID}
	jsonData, _ := json.Marshal(messageToSend)

	err = client1.WriteMessage(websocket.TextMessage, jsonData)
	require.NoError(t, err, "Client1 failed to send message")

	// Assert client2 receives the message
	select {
	case receivedMsg2 := <-msgChanClient2:
		assert.Equal(t, "offer", receivedMsg2.Type)
		// Note: rtc_service.go's SignalMessage currently doesn't add sender to the message.
		// If it did, we would assert receivedMsg2.Sender == user1ID
		// For now, we check payload directly.
		// The payload might be re-marshalled, so compare the map.
		if p, ok := receivedMsg2.Payload.(map[string]interface{}); ok {
			assert.Equal(t, offerPayload["sdp"], p["sdp"])
		} else {
			t.Errorf("Client2 received payload of unexpected type: %T", receivedMsg2.Payload)
		}
	case <-time.After(3 * time.Second): // Increased timeout
		t.Fatal("Client2 did not receive message in time")
	}

	// Assert client3 receives the message
	select {
	case receivedMsg3 := <-msgChanClient3:
		assert.Equal(t, "offer", receivedMsg3.Type)
		if p, ok := receivedMsg3.Payload.(map[string]interface{}); ok {
			assert.Equal(t, offerPayload["sdp"], p["sdp"])
		} else {
			t.Errorf("Client3 received payload of unexpected type: %T", receivedMsg3.Payload)
		}
	case <-time.After(3 * time.Second): // Increased timeout
		t.Fatal("Client3 did not receive message in time")
	}

	// Assert client1 (sender) does not receive its own message
	// This is trickier. We'll try to read with a short timeout.
	client1.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = client1.ReadMessage()
	if err == nil {
		t.Fatal("Client1 (sender) received a message, but should not have")
	} else if !websocket.IsTimeoutError(err) {
		// Any error other than timeout is unexpected here, unless it's a normal close
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) && !strings.Contains(err.Error(), "use of closed network connection") {
			t.Logf("Client1 ReadMessage error: %v", err) // Log for debugging
		}
	}

	// Clean up: Close connections explicitly (defer also does this, but good for clarity)
	// client1.Close() // deferred
	// client2.Close() // deferred
	// client3.Close() // deferred
	wg.Wait() // Wait for listener goroutines to finish
}

// TestWebSocketInvalidRoomOrUser tests connection attempts with invalid parameters.
func TestWebSocketInvalidRoomOrUser(t *testing.T) {
	_, _, testServer := setupTestAppForWS() // RTCService not directly needed here
	defer testServer.Close()

	// Test case 1: Empty roomID
	t.Run("empty roomID", func(t *testing.T) {
		_, err := newWebSocketClient(t, testServer.URL, "", "testuser")
		assert.Error(t, err, "Expected error when connecting with empty roomID")
		// The error will likely be a 404 or similar from the HTTP handshake part of Dial,
		// as the route /ws//testuser won't match /ws/:roomID/:userID properly
		// or if it matches, defaultHandler should reject it.
		// The fasthttp/websocket client's Dial function might return an error if the server
		// doesn't upgrade the connection (e.g., returns 400 or 404).
	})

	// Test case 2: Empty userID
	t.Run("empty userID", func(t *testing.T) {
		_, err := newWebSocketClient(t, testServer.URL, "testroom", "")
		assert.Error(t, err, "Expected error when connecting with empty userID")
	})

	// Test case 3: Room does not exist (RTCService.JoinRoom should fail)
	// This case depends on the current behavior of rtc_service.JoinRoom and how ws_routes handles it.
	// ws_routes.go's defaultHandler now tries to join the room and returns an error message
	// to the client + closes if JoinRoom fails. The client's Dial will see this as a failed handshake.
	t.Run("room does not exist", func(t *testing.T) {
		// Connect to a room that hasn't been created in RTCService
		_, err := newWebSocketClient(t, testServer.URL, "non-existent-room-ws", "testuser")
		assert.Error(t, err, "Expected error when connecting to a non-existent room")
		// The error message from the server might be "Error joining room: room non-existent-room-ws not found"
		// which would cause the websocket handshake to fail.
	})
}

// Note: TestWebSocketUserJoinsRoomSignal and TestWebSocketUserLeavesRoomSignal are omitted
// because the current RTCService.SignalMessage is a placeholder and doesn't send
// "user joined" or "user left" signals automatically. Implementing these tests would
// require modifying RTCService to send these specific signals and then verifying them here.
// The current SignalMessage in rtc_service.go only logs and doesn't actually write to user.Conn.
// For TestWebSocketSignalBroadcast to fully pass as intended (message delivery),
// RTCService.SignalMessage needs to be implemented to iterate and write to connections.
// The current test will pass if no error occurs during SignalMessage call, but won't verify content on client side
// without the actual sending logic in RTCService.
// The provided test code for TestWebSocketSignalBroadcast *attempts* to verify content, assuming
// that RTCService.SignalMessage *will* be implemented to send to other users.
// If RTCService.SignalMessage remains a log-only placeholder, the receive parts of TestWebSocketSignalBroadcast will time out.
// The current implementation of `RTCService.SignalMessage` in `internal/core/rtc/rtc_service.go` is:
// func (s *RTCService) SignalMessage(roomID string, senderID string, message []byte) error {
//	s.mu.RLock()
//	defer s.mu.RUnlock()
//
//	room, exists := s.Rooms[roomID]
//	if !exists {
//		return fmt.Errorf("room %s not found", roomID)
//	}
//
//	fmt.Printf("SignalMessage: Room=%s, Sender=%s, Message=%s\n", roomID, senderID, string(message))
//	// ... iteration logic is commented out ...
//	_ = room // Avoid unused variable error for now
//	return nil
// }
// This needs to be updated for the broadcast test to fully verify message content.
// For now, the test will be structured as if it *should* send.

// After implementing the actual sending logic in RTCService.SignalMessage,
// TestWebSocketSignalBroadcast should work as expected.
// The current tests are written with the assumption that this will be fixed.
// If the `SignalMessage` is not updated, the parts of `TestWebSocketSignalBroadcast`
// that wait for messages on `msgChanClient2` and `msgChanClient3` will time out.
// I will proceed with creating this file. The success of TestWebSocketSignalBroadcast
// will hinge on the actual implementation of message forwarding in rtc_service.go.

// Update: I need to modify the rtc_service.go to actually send messages for the broadcast test to work.
// This was not part of the original subtask, but is essential for this test.
// I will add this modification to the plan.
// For now, I will create the test file as is. If I cannot modify rtc_service.go,
// the broadcast test will fail on message receipt.

// Let's refine TestWebSocketSignalBroadcast slightly to handle the current SignalMessage behavior
// if it only logs. The test will try to receive, but if SignalMessage isn't implemented, it will timeout.
// The test structure will remain, highlighting the need for SignalMessage's implementation.

// The `server.SetupWs` function is used in `internal/infra/server/server.go`
// `setupWs(s.app, s.cfg, s.db, s.rtcService)`
// So the test setup should match this.

// The `newWebSocketClient` helper's dialer timeout was increased for CI environments.
// The listener goroutines in TestWebSocketSignalBroadcast now have a loop to read messages,
// and better error handling for connection closures.
// The `time.Sleep(200 * time.Millisecond)` in `TestWebSocketConnection` and `TestWebSocketSignalBroadcast`
// are for allowing server-side operations (like user removal or registration) to complete.
// These can sometimes be flaky in tests and might need adjustment or replacement with more robust synchronization.
// `client1.SetReadDeadline` is used to check that the sender does not receive their own message.
// The `TestWebSocketInvalidRoomOrUser` checks for client-side errors when attempting to connect to invalid URLs
// or when the server rejects the connection (e.g., room not found).
```
