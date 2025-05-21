// Package rtc_test contains unit tests for the rtc package.
package rtc_test

import (
	"fmt"
	"testing"

	"github.com/PocketPalCo/shopping-service/internal/core/rtc"
	"github.com/gofiber/contrib/websocket" // Import for User.Conn, though direct testing is hard
)

// Helper function to create a mock websocket.Conn for testing.
// Note: This is a very basic mock. In a real scenario, you might need a more sophisticated mock
// or to refactor RTCService to use an interface for connections to make testing easier.
func newMockConn() *websocket.Conn {
	// For the purpose of these unit tests, we don't need a fully functional connection.
	// We only need to check if the pointer is stored and retrieved correctly.
	// In a real application, you might not be able to instantiate websocket.Conn directly
	// or might need to use a library that provides mock WebSocket connections.
	// However, since User.Conn is a direct struct pointer, we pass nil for now,
	// as the service logic doesn't dereference it in a way that would panic in these tests.
	// If it did, we'd need a more complex setup or a refactor.
	return nil
}

// TestNewRTCService tests the NewRTCService function.
func TestNewRTCService(t *testing.T) {
	service := rtc.NewRTCService()
	if service == nil {
		t.Fatal("NewRTCService() returned nil")
	}
	if service.Rooms == nil {
		t.Error("NewRTCService() Rooms map is nil")
	}
	if len(service.Rooms) != 0 {
		t.Errorf("NewRTCService() Rooms map is not empty, got %d, want %d", len(service.Rooms), 0)
	}
}

// TestCreateRoom tests the CreateRoom method.
func TestCreateRoom(t *testing.T) {
	service := rtc.NewRTCService()
	roomID := "test-room-1"

	// Test successful room creation
	room, err := service.CreateRoom(roomID)
	if err != nil {
		t.Fatalf("CreateRoom() with new ID failed: %v", err)
	}
	if room == nil {
		t.Fatal("CreateRoom() returned nil room on success")
	}
	if room.ID != roomID {
		t.Errorf("CreateRoom() room ID got %s, want %s", room.ID, roomID)
	}
	if len(room.Users) != 0 {
		t.Errorf("CreateRoom() new room Users map is not empty, got %d, want %d", len(room.Users), 0)
	}

	// Verify room is in service.Rooms
	_, ok := service.Rooms[roomID]
	if !ok {
		t.Errorf("CreateRoom() room %s not found in service.Rooms after creation", roomID)
	}

	// Test creating a room that already exists
	_, err = service.CreateRoom(roomID)
	if err == nil {
		t.Errorf("CreateRoom() with existing ID expected error, got nil")
	}
	expectedErr := fmt.Sprintf("room %s already exists", roomID)
	if err != nil && err.Error() != expectedErr {
		t.Errorf("CreateRoom() with existing ID error got '%v', want '%s'", err, expectedErr)
	}
}

// TestGetRoom tests the GetRoom method.
func TestGetRoom(t *testing.T) {
	service := rtc.NewRTCService()
	roomID := "test-room-get"

	// Test getting a non-existent room
	_, err := service.GetRoom(roomID)
	if err == nil {
		t.Errorf("GetRoom() with non-existent ID expected error, got nil")
	}
	expectedErr := fmt.Sprintf("room %s not found", roomID)
	if err != nil && err.Error() != expectedErr {
		t.Errorf("GetRoom() with non-existent ID error got '%v', want '%s'", err, expectedErr)
	}

	// Create a room
	createdRoom, _ := service.CreateRoom(roomID)

	// Test getting an existing room
	retrievedRoom, err := service.GetRoom(roomID)
	if err != nil {
		t.Fatalf("GetRoom() with existing ID failed: %v", err)
	}
	if retrievedRoom == nil {
		t.Fatal("GetRoom() returned nil for existing room")
	}
	if retrievedRoom.ID != roomID {
		t.Errorf("GetRoom() room ID got %s, want %s", retrievedRoom.ID, roomID)
	}
	if retrievedRoom != createdRoom { // Check if it's the same instance
		t.Error("GetRoom() returned a different room instance than expected")
	}
}

// TestJoinRoom tests the JoinRoom method.
func TestJoinRoom(t *testing.T) {
	service := rtc.NewRTCService()
	roomID := "test-room-join"
	userID1 := "user1"
	userID2 := "user2"
	mockConn1 := newMockConn()
	mockConn2 := newMockConn()

	// Test joining a non-existent room
	_, err := service.JoinRoom(roomID, userID1, mockConn1)
	if err == nil {
		t.Errorf("JoinRoom() to non-existent room expected error, got nil")
	}
	expectedErrNonExistent := fmt.Sprintf("room %s not found", roomID)
	if err != nil && err.Error() != expectedErrNonExistent {
		t.Errorf("JoinRoom() to non-existent room error got '%v', want '%s'", err, expectedErrNonExistent)
	}

	// Create a room first
	_, _ = service.CreateRoom(roomID)

	// Test successfully joining an existing room
	room, err := service.JoinRoom(roomID, userID1, mockConn1)
	if err != nil {
		t.Fatalf("JoinRoom() failed for user1: %v", err)
	}
	if room == nil {
		t.Fatal("JoinRoom() returned nil room on successful join for user1")
	}
	if len(room.Users) != 1 {
		t.Errorf("JoinRoom() user1, room user count got %d, want %d", len(room.Users), 1)
	}
	user1InRoom, ok := room.Users[userID1]
	if !ok {
		t.Fatalf("JoinRoom() user1 not found in room.Users map")
	}
	if user1InRoom.ID != userID1 {
		t.Errorf("JoinRoom() user1 ID in room got %s, want %s", user1InRoom.ID, userID1)
	}
	if user1InRoom.Conn != mockConn1 {
		t.Errorf("JoinRoom() user1 Conn in room not stored correctly")
	}

	// Test another user joining the same room
	room, err = service.JoinRoom(roomID, userID2, mockConn2)
	if err != nil {
		t.Fatalf("JoinRoom() failed for user2: %v", err)
	}
	if len(room.Users) != 2 {
		t.Errorf("JoinRoom() user2, room user count got %d, want %d", len(room.Users), 2)
	}
	user2InRoom, ok := room.Users[userID2]
	if !ok {
		t.Fatalf("JoinRoom() user2 not found in room.Users map")
	}
	if user2InRoom.ID != userID2 {
		t.Errorf("JoinRoom() user2 ID in room got %s, want %s", user2InRoom.ID, userID2)
	}
	if user2InRoom.Conn != mockConn2 {
		t.Errorf("JoinRoom() user2 Conn in room not stored correctly")
	}

	// Test a user joining a room they are already in
	_, err = service.JoinRoom(roomID, userID1, mockConn1) // User1 tries to join again
	if err == nil {
		t.Errorf("JoinRoom() with already joined user expected error, got nil")
	}
	expectedErrExistingUser := fmt.Sprintf("user %s already in room %s", userID1, roomID)
	if err != nil && err.Error() != expectedErrExistingUser {
		t.Errorf("JoinRoom() with already joined user error got '%v', want '%s'", err, expectedErrExistingUser)
	}
	if len(room.Users) != 2 { // Ensure user count hasn't changed
		t.Errorf("JoinRoom() user count changed after attempt to re-join, got %d, want %d", len(room.Users), 2)
	}
}

// TestLeaveRoom tests the LeaveRoom method.
func TestLeaveRoom(t *testing.T) {
	service := rtc.NewRTCService()
	roomID := "test-room-leave"
	userID1 := "user1"
	userID2 := "user2" // Another user to ensure room is not deleted prematurely
	mockConn1 := newMockConn()
	mockConn2 := newMockConn()

	// Test leaving a non-existent room
	err := service.LeaveRoom(roomID, userID1)
	if err == nil {
		t.Errorf("LeaveRoom() from non-existent room expected error, got nil")
	}
	expectedErrNonExistent := fmt.Sprintf("room %s not found", roomID)
	if err != nil && err.Error() != expectedErrNonExistent {
		t.Errorf("LeaveRoom() from non-existent room error got '%v', want '%s'", err, expectedErrNonExistent)
	}

	// Create room and add users
	_, _ = service.CreateRoom(roomID)
	_, _ = service.JoinRoom(roomID, userID1, mockConn1)
	_, _ = service.JoinRoom(roomID, userID2, mockConn2)

	// Test a user leaving a room they are not in (but room exists)
	nonExistentUserID := "ghost-user"
	err = service.LeaveRoom(roomID, nonExistentUserID)
	if err == nil {
		t.Errorf("LeaveRoom() for user not in room expected error, got nil")
	}
	expectedErrUserNotFound := fmt.Sprintf("user %s not in room %s", nonExistentUserID, roomID)
	if err != nil && err.Error() != expectedErrUserNotFound {
		t.Errorf("LeaveRoom() for user not in room error got '%v', want '%s'", err, expectedErrUserNotFound)
	}

	// Test a user successfully leaving a room
	err = service.LeaveRoom(roomID, userID1)
	if err != nil {
		t.Fatalf("LeaveRoom() for user1 failed: %v", err)
	}

	// Verify user1 is removed
	room, _ := service.GetRoom(roomID)
	if _, ok := room.Users[userID1]; ok {
		t.Errorf("LeaveRoom() user1 still found in room.Users after leaving")
	}
	if len(room.Users) != 1 {
		t.Errorf("LeaveRoom() room user count got %d, want %d after user1 left", len(room.Users), 1)
	}
	// Verify user2 is still there
	if _, ok := room.Users[userID2]; !ok {
		t.Errorf("LeaveRoom() user2 was unexpectedly removed after user1 left")
	}

	// Test the same user trying to leave again (now they are not in the room)
	err = service.LeaveRoom(roomID, userID1)
	if err == nil {
		t.Errorf("LeaveRoom() for user1 again (should not be in room) expected error, got nil")
	}
	expectedErrUser1NotInRoom := fmt.Sprintf("user %s not in room %s", userID1, roomID)
	if err != nil && err.Error() != expectedErrUser1NotInRoom {
		t.Errorf("LeaveRoom() for user1 again error got '%v', want '%s'", err, expectedErrUser1NotInRoom)
	}
}

// TestSignalMessage tests the SignalMessage method (basic functionality).
// This test primarily checks that the method doesn't panic and attempts to access users.
// It does not verify actual message sending over WebSockets.
func TestSignalMessage(t *testing.T) {
	service := rtc.NewRTCService()
	roomID := "test-room-signal"
	senderID := "senderUser"
	receiverID := "receiverUser"
	mockConnSender := newMockConn()
	mockConnReceiver := newMockConn() // In a real scenario, this would receive the message

	// Test signaling in a non-existent room
	err := service.SignalMessage(roomID, senderID, []byte("hello"))
	if err == nil {
		t.Errorf("SignalMessage() in non-existent room expected error, got nil")
	}
	expectedErrNonExistent := fmt.Sprintf("room %s not found", roomID)
	if err != nil && err.Error() != expectedErrNonExistent {
		t.Errorf("SignalMessage() in non-existent room error got '%v', want '%s'", err, expectedErrNonExistent)
	}

	// Create room and add users
	_, _ = service.CreateRoom(roomID)
	_, _ = service.JoinRoom(roomID, senderID, mockConnSender)
	_, _ = service.JoinRoom(roomID, receiverID, mockConnReceiver)

	// Test successful signal (no panic, no error returned by current implementation)
	// The current SignalMessage just logs and doesn't send, so no error is expected.
	// This test ensures it runs without issues.
	err = service.SignalMessage(roomID, senderID, []byte("hello"))
	if err != nil {
		t.Fatalf("SignalMessage() failed: %v", err)
	}

	// Test signaling from a user not in the room (though room exists)
	// RTCService.SignalMessage doesn't currently check if senderID is in the room's user list.
	// It only checks if the room exists. This behavior could be a point of discussion.
	// For now, this should not return an error as long as the room exists.
	err = service.SignalMessage(roomID, "nonExistentSender", []byte("test"))
	if err != nil {
		t.Fatalf("SignalMessage() from non-existent sender failed: %v", err)
	}

	// Test signaling with an empty message
	err = service.SignalMessage(roomID, senderID, []byte(""))
	if err != nil {
		t.Fatalf("SignalMessage() with empty message failed: %v", err)
	}

	// Test signaling in a room with only one user (the sender)
	singleUserRoomID := "single-user-room"
	singleUserID := "singleUser"
	_, _ = service.CreateRoom(singleUserRoomID)
	_, _ = service.JoinRoom(singleUserRoomID, singleUserID, newMockConn())
	err = service.SignalMessage(singleUserRoomID, singleUserID, []byte("lonely signal"))
	if err != nil {
		t.Fatalf("SignalMessage() in single-user room failed: %v", err)
	}
	// If SignalMessage had a way to count recipients, we'd check it's 0 here.
	// For now, just ensure no error/panic.
}

// Add more advanced tests for SignalMessage if User struct or RTCService is refactored for testability,
// e.g., by adding a mockable sender interface or a way to inspect outgoing messages.
// For example, if User had a field like `LastMessageReceived []byte` (for testing only):
/*
func TestSignalMessage_VerifyMessageDelivery(t *testing.T) {
	service := rtc.NewRTCService()
	roomID := "test-room-signal-delivery"
	senderID := "sender"
	receiverID1 := "receiver1"
	receiverID2 := "receiver2"

	// Mock connections - in a real test with interfaces, these would be mocks
	// that allow inspecting sent data.
	mockConnSender := newMockConn()
	mockConnReceiver1 := newMockConn() // This would be a mock that can "receive" a message
	mockConnReceiver2 := newMockConn() // Same here

	_, _ = service.CreateRoom(roomID)
	room, _ := service.GetRoom(roomID)

	// For this hypothetical test, assume User struct is modified for testing:
	// type User struct {
	// 	ID   string
	// 	Conn *websocket.Conn
	// 	LastMessageReceived []byte // TESTING ONLY
	// }
	// And RTCService.SignalMessage is modified to populate this field (again, for testing).

	userSender := &rtc.User{ID: senderID, Conn: mockConnSender}
	userReceiver1 := &rtc.User{ID: receiverID1, Conn: mockConnReceiver1}
	userReceiver2 := &rtc.User{ID: receiverID2, Conn: mockConnReceiver2}

	room.Users[senderID] = userSender
	room.Users[receiverID1] = userReceiver1
	room.Users[receiverID2] = userReceiver2

	message := []byte("super secret signal")
	err := service.SignalMessage(roomID, senderID, message)
	if err != nil {
		t.Fatalf("SignalMessage() failed: %v", err)
	}

	// Assert that receiver1 got the message and sender/receiver2 (if sender was also a receiver) did not
	// This requires RTCService.SignalMessage to be modified to use user.Conn.WriteMessage
	// and for the mockConn to record what was written.
	// if !bytes.Equal(userReceiver1.LastMessageReceived, message) {
	// 	t.Errorf("Receiver1 did not receive the correct message. Got %s, want %s", userReceiver1.LastMessageReceived, message)
	// }
	// if userSender.LastMessageReceived != nil {
	// 	t.Errorf("Sender should not have received their own message. Got %s", userSender.LastMessageReceived)
	// }

	// This part is highly dependent on how message sending is implemented and mocked.
	// The current rtc_service.go does not actually send, so this test cannot be fully realized yet.
	t.Log("TestSignalMessage_VerifyMessageDelivery is a placeholder for more advanced testing if service is refactored.")
}
*/

// Note on websocket.Conn:
// The `websocket.Conn` from `github.com/gofiber/contrib/websocket` is a concrete struct.
// True unit testing of message sending would require either:
// 1. An interface for the connection that can be mocked (e.g., `type MessageSender interface { WriteMessage(int, []byte) error }`).
//    RTCService and User would use this interface.
// 2. Running a real WebSocket server and client within the test, which leans towards integration testing.
// 3. Modifying User struct for tests to include a channel or callback that SignalMessage uses.
// For these unit tests, we focus on the state management logic of RTCService.
// The actual `SignalMessage` implementation in `rtc_service.go` currently only logs and doesn't send,
// so these tests verify it runs without error.
// The placeholder `TestSignalMessage_VerifyMessageDelivery` illustrates how one might test further.

func ExampleRTCService_CreateRoom() {
	service := rtc.NewRTCService()
	room, err := service.CreateRoom("example-room")
	if err != nil {
		fmt.Printf("Error creating room: %v\n", err)
		return
	}
	fmt.Printf("Room created: %s, Users: %d\n", room.ID, len(room.Users))

	// Try to create it again
	_, err = service.CreateRoom("example-room")
	if err != nil {
		fmt.Printf("Error creating room again: %v\n", err)
	}
	// Output:
	// Room created: example-room, Users: 0
	// Error creating room again: room example-room already exists
}

func ExampleRTCService_JoinRoom() {
	service := rtc.NewRTCService()
	roomID := "chat-room-101"
	userID := "alice"

	// Attempt to join before room exists
	_, err := service.JoinRoom(roomID, userID, nil) // Using nil for mock conn
	if err != nil {
		fmt.Printf("Error joining non-existent room: %v\n", err)
	}

	_, _ = service.CreateRoom(roomID)
	room, err := service.JoinRoom(roomID, userID, nil)
	if err != nil {
		fmt.Printf("Error joining room: %v\n", err)
		return
	}
	fmt.Printf("User %s joined room %s. Total users: %d\n", userID, room.ID, len(room.Users))

	// Attempt to join again
	_, err = service.JoinRoom(roomID, userID, nil)
	if err != nil {
		fmt.Printf("Error joining room again: %v\n", err)
	}
	// Output:
	// Error joining non-existent room: room chat-room-101 not found
	// User alice joined room chat-room-101. Total users: 1
	// Error joining room again: user alice already in room chat-room-101
}
