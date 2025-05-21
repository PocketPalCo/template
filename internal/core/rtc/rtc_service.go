// Package rtc handles the core Real-Time Communication logic.
package rtc

import (
	"fmt"
	"sync"

	"github.com/gofiber/contrib/websocket"
)

// Room represents a communication room.
type Room struct {
	ID    string
	Users map[string]*User
}

// User represents a user in a room.
type User struct {
	ID   string
	Conn *websocket.Conn
}

// RTCService manages rooms and users.
type RTCService struct {
	Rooms map[string]*Room
	mu    sync.RWMutex
}

// NewRTCService creates a new RTCService.
func NewRTCService() *RTCService {
	return &RTCService{
		Rooms: make(map[string]*Room),
	}
}

// CreateRoom creates a new room with the given ID.
func (s *RTCService) CreateRoom(roomID string) (*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.Rooms[roomID]; exists {
		return nil, fmt.Errorf("room %s already exists", roomID)
	}

	room := &Room{
		ID:    roomID,
		Users: make(map[string]*User),
	}
	s.Rooms[roomID] = room
	return room, nil
}

// JoinRoom adds a user to a room.
func (s *RTCService) JoinRoom(roomID string, userID string, conn *websocket.Conn) (*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.Rooms[roomID]
	if !exists {
		return nil, fmt.Errorf("room %s not found", roomID)
	}

	if _, userExists := room.Users[userID]; userExists {
		return nil, fmt.Errorf("user %s already in room %s", userID, roomID)
	}

	user := &User{
		ID:   userID,
		Conn: conn,
	}
	room.Users[userID] = user
	return room, nil
}

// LeaveRoom removes a user from a room.
func (s *RTCService) LeaveRoom(roomID string, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.Rooms[roomID]
	if !exists {
		return fmt.Errorf("room %s not found", roomID)
	}

	if _, userExists := room.Users[userID]; !userExists {
		return fmt.Errorf("user %s not in room %s", userID, roomID)
	}

	delete(room.Users, userID)
	return nil
}

// GetRoom retrieves a room by its ID.
func (s *RTCService) GetRoom(roomID string) (*Room, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, exists := s.Rooms[roomID]
	if !exists {
		return nil, fmt.Errorf("room %s not found", roomID)
	}
	return room, nil
}

// SignalMessage sends a signal message to users in a room (excluding the sender).
// For now, it just logs the action.
func (s *RTCService) SignalMessage(roomID string, senderID string, message []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, exists := s.Rooms[roomID]
	if !exists {
		return fmt.Errorf("room %s not found", roomID)
	}

	fmt.Printf("SignalMessage: Room=%s, Sender=%s, MessageLength=%d\n", roomID, senderID, len(message))

	var sendErrors []error
	for userID, user := range room.Users {
		if userID != senderID { // Do not send the message back to the sender
			if user.Conn != nil {
				// It's important to handle errors here, e.g., by logging or removing dead connections.
				// For simplicity in this example, we'll collect errors.
				// Note: WriteMessage is not concurrency-safe for multiple goroutines writing to the same Conn.
				// However, each user has their own Conn, and this loop is synchronous for writes to different Conns.
				// If SignalMessage itself could be called concurrently for the *same user*, User.Conn access would need a mutex.
				// But here, s.mu.RLock() protects Rooms map, and each user.Conn is distinct.
				if err := user.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
					sendErrors = append(sendErrors, fmt.Errorf("failed to send message to user %s in room %s: %w", userID, roomID, err))
					// TODO: Consider removing user or marking connection as stale if WriteMessage fails.
					// For example:
					// go s.LeaveRoom(roomID, userID) // This would need its own error handling and careful locking.
				}
			}
		}
	}

	if len(sendErrors) > 0 {
		// For now, just return the first error, or a summary.
		// In a real app, you might log all errors.
		return fmt.Errorf("errors during SignalMessage broadcast: %v", sendErrors)
	}

	return nil
}
