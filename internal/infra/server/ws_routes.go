package server

import (
	"fmt"
	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/core/rtc" // Import RTC package
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"log/slog"
	// "sync" // Commented out as clientsMu will be removed
)

// Commenting out old client management, RTCService will handle this
// var (
// 	clients   = make(map[string]map[*websocket.Conn]struct{})
// 	clientsMu sync.RWMutex
// )

// func onConnect(c *websocket.Conn, userID string) {
// 	slog.Info("WS connected", slog.String("id", userID))
// 	registerConn(userID, c)
// }

// func onDisconnect(c *websocket.Conn, userID string) {
// 	slog.Info("WS disconnected", slog.String("id", userID))
// 	unregisterConn(userID, c)
// }

// func onClose(c *websocket.Conn, roomID string, userID string, rtcService *rtc.RTCService) { // Modified signature
// 	slog.Info("WS closed", slog.String("roomID", roomID), slog.String("userID", userID))
// 	if err := rtcService.LeaveRoom(roomID, userID); err != nil {
// 		slog.Error("Error leaving room on WS close", slog.String("roomID", roomID), slog.String("userID", userID), slog.String("error", err.Error()))
// 	}
// }

func onError(c *websocket.Conn, roomID string, userID string, err error) { // Modified signature
	slog.Error("WS error", slog.String("roomID", roomID), slog.String("userID", userID), slog.String("error", err.Error()))
}

// func onMessage(c *websocket.Conn, mt int, msg []byte) {
// 	slog.Info("WS message", slog.String("id", c.Params("id")), slog.String("msg", string(msg)))
// 	if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
// 		slog.Error("failed to write message", slog.String("error", err.Error()))
// 	}
// }

func setupWs(app *fiber.App, config *config.Config, db postgres.DB, rtcService *rtc.RTCService) { // Added rtcService parameter
	app.Use("/ws", upgradeMiddleware)

	log := slog.With("ws routes", "initWsRoutes")

	cfg := websocket.Config{
		RecoverHandler: func(conn *websocket.Conn) {
			if err := recover(); err != nil {
				// Extract roomID and userID from params for logging, if possible.
				// This might be tricky as conn.Params might not be populated if error is early.
				roomID := conn.Params("roomID")
				userID := conn.Params("userID")
				slog.Error("WS panic recovered", slog.String("roomID", roomID), slog.String("userID", userID), "error", err)
				// Attempt to send a generic error message.
				// This might fail if the connection is already broken.
				_ = conn.WriteJSON(fiber.Map{"error": "internal server error"})
			}
		},
	}

	ws := websocket.New(defaultHandler(rtcService), cfg) // Pass rtcService

	app.Get("/ws/:roomID/:userID", ws) // Updated route
}

func upgradeMiddleware(c *fiber.Ctx) error {
	if websocket.IsWebSocketUpgrade(c) {
		c.Locals("allowed", true)
	}

	return c.Next()
}

func defaultHandler(rtcService *rtc.RTCService) func(c *websocket.Conn) { // Accept rtcService
	return func(c *websocket.Conn) {
		roomID := c.Params("roomID")
		userID := c.Params("userID")

		if roomID == "" || userID == "" {
			slog.Error("WS connection failed: roomID or userID is empty", slog.String("roomID", roomID), slog.String("userID", userID))
			_ = c.WriteMessage(websocket.TextMessage, []byte("RoomID and UserID are required."))
			_ = c.Close()
			return
		}

		slog.Info("WS connected, attempting to join room", slog.String("roomID", roomID), slog.String("userID", userID))

		// Join Room
		_, err := rtcService.JoinRoom(roomID, userID, c)
		if err != nil {
			slog.Error("Failed to join RTC room", slog.String("roomID", roomID), slog.String("userID", userID), slog.String("error", err.Error()))
			_ = c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error joining room: %s", err.Error())))
			_ = c.Close()
			return
		}
		slog.Info("User successfully joined RTC room", slog.String("roomID", roomID), slog.String("userID", userID))

		// Defer LeaveRoom
		defer func() {
			slog.Info("WS disconnecting, leaving room", slog.String("roomID", roomID), slog.String("userID", userID))
			if err := rtcService.LeaveRoom(roomID, userID); err != nil {
				slog.Error("Error leaving RTC room on disconnect", slog.String("roomID", roomID), slog.String("userID", userID), slog.String("error", err.Error()))
			}
			// onClose(c, roomID, userID, rtcService) // Using direct LeaveRoom call
		}()

		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					slog.Error("WS read error (unexpected close)", slog.String("roomID", roomID), slog.String("userID", userID), slog.String("err", err.Error()))
				} else if err.Error() == "websocket: close sent" || err.Error() == "websocket: close 1005 (no status)" || err.Error() == "websocket: close 1001 (going away)" {
					slog.Info("WS read: connection closed by client", slog.String("roomID", roomID), slog.String("userID", userID), slog.String("err", err.Error()))
				} else {
					slog.Info("WS read error", slog.String("roomID", roomID), slog.String("userID", userID), slog.String("err", err.Error()))
				}
				onError(c, roomID, userID, err) // Log original error
				break // Exit loop on error
			}

			slog.Info("WS message received", slog.String("roomID", roomID), slog.String("userID", userID), slog.String("msg", string(msg)), slog.Int("type", mt))

			// Signal Message
			if err := rtcService.SignalMessage(roomID, userID, msg); err != nil {
				slog.Error("Error signaling message in RTC room", slog.String("roomID", roomID), slog.String("userID", userID), slog.String("error", err.Error()))
				// Optionally, inform the sender about the failure
				// _ = c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error sending signal: %s", err.Error())))
			}
		}
	}
}

// Commenting out old client management functions as RTCService will handle this logic
// func registerConn(userID string, conn *websocket.Conn) {
// 	clientsMu.Lock()
// 	defer clientsMu.Unlock()
// 	set, ok := clients[userID]
// 	if !ok {
// 		set = make(map[*websocket.Conn]struct{})
// 		clients[userID] = set
// 	}
// 	set[conn] = struct{}{}

// 	slog.Info("WS registered", slog.String("id", userID), slog.Int("count", len(set)))
// 	go Broadcast([]byte(fmt.Sprintf("User %s joined: %s", userID, conn)))
// }

// func unregisterConn(userID string, conn *websocket.Conn) {
// 	clientsMu.Lock()
// 	defer clientsMu.Unlock()
// 	if set, ok := clients[userID]; ok {
// 		delete(set, conn)
// 		if len(set) == 0 {
// 			delete(clients, userID)
// 		}
// 	}
// }

// func Broadcast(message []byte) {
// 	clientsMu.RLock()
// 	defer clientsMu.RUnlock()
// 	for uid, set := range clients {
// 		for conn := range set {
// 			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
// 				slog.Warn("broadcast failed", slog.String("to", uid), slog.String("err", err.Error()))
// 			}
// 		}
// 	}
// }

// func SendToUser(userID string, message []byte) error {
// 	clientsMu.RLock()
// 	defer clientsMu.RUnlock()
// 	set, ok := clients[userID]
// 	if !ok || len(set) == 0 {
// 		return fmt.Errorf("user %s not connected", userID)
// 	}
// 	for conn := range set {
// 		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
// 			slog.Warn("send failed", slog.String("to", userID), slog.String("err", err.Error()))
// 		}
// 	}
// 	return nil
// }
