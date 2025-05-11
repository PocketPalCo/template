package server

import (
	"fmt"
	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"log/slog"
	"sync"
)

var (
	clients   = make(map[string]map[*websocket.Conn]struct{})
	clientsMu sync.RWMutex
)

func onConnect(c *websocket.Conn, userID string) {
	slog.Info("WS connected", slog.String("id", userID))
	registerConn(userID, c)
}

func onDisconnect(c *websocket.Conn, userID string) {
	slog.Info("WS disconnected", slog.String("id", userID))
	unregisterConn(userID, c)
}

func onClose(c *websocket.Conn, userID string) {
	slog.Info("WS closed", slog.String("id", c.Params("id")))
	onDisconnect(c, userID)
}

func onError(c *websocket.Conn, err error) {
	slog.Error("WS error", slog.String("id", c.Params("id")), slog.String("error", err.Error()))
}

func onMessage(c *websocket.Conn, mt int, msg []byte) {
	slog.Info("WS message", slog.String("id", c.Params("id")), slog.String("msg", string(msg)))
	if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
		slog.Error("failed to write message", slog.String("error", err.Error()))
	}
}

func setupWs(app *fiber.App, config *config.Config, db postgres.DB) {
	app.Use("/ws", upgradeMiddleware)

	log := slog.With("ws routes", "initWsRoutes")

	cfg := websocket.Config{
		RecoverHandler: func(conn *websocket.Conn) {
			if err := recover(); err != nil {
				err := conn.WriteJSON(fiber.Map{"customError": "error occurred"})
				if err != nil {
					log.Error("failed to write error message", slog.String("error", err.Error()))
				}
			}
		},
	}

	ws := websocket.New(defaultHandler(), cfg)

	app.Get("/ws/:id/*", ws)
}

func upgradeMiddleware(c *fiber.Ctx) error {
	if websocket.IsWebSocketUpgrade(c) {
		c.Locals("allowed", true)
	}

	return c.Next()
}

func defaultHandler() func(c *websocket.Conn) {
	return func(c *websocket.Conn) {
		userID := c.Params("id")

		if userID == "" {
			slog.Error("userID is empty")
			err := c.Close()
			if err != nil {
				slog.Error("failed to close connection", slog.String("error", err.Error()))
				onError(c, err)
				onClose(c, userID)
			}
			return
		}
		slog.Info("WS connected", slog.String("id", userID))

		onConnect(c, userID)
		defer onDisconnect(c, userID)

		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				slog.Info("read error", slog.String("id", userID), slog.String("err", err.Error()))
				onError(c, err)
				break
			}
			onMessage(c, mt, msg)
			slog.Info("recv", slog.String("id", userID), slog.String("msg", string(msg)))
		}

	}
}

func registerConn(userID string, conn *websocket.Conn) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	set, ok := clients[userID]
	if !ok {
		set = make(map[*websocket.Conn]struct{})
		clients[userID] = set
	}
	set[conn] = struct{}{}

	slog.Info("WS registered", slog.String("id", userID), slog.Int("count", len(set)))
	go Broadcast([]byte(fmt.Sprintf("User %s joined: %s", userID, conn)))
}

func unregisterConn(userID string, conn *websocket.Conn) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	if set, ok := clients[userID]; ok {
		delete(set, conn)
		if len(set) == 0 {
			delete(clients, userID)
		}
	}
}

func Broadcast(message []byte) {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	for uid, set := range clients {
		for conn := range set {
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				slog.Warn("broadcast failed", slog.String("to", uid), slog.String("err", err.Error()))
			}
		}
	}
}

func SendToUser(userID string, message []byte) error {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	set, ok := clients[userID]
	if !ok || len(set) == 0 {
		return fmt.Errorf("user %s not connected", userID)
	}
	for conn := range set {
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			slog.Warn("send failed", slog.String("to", userID), slog.String("err", err.Error()))
		}
	}
	return nil
}
