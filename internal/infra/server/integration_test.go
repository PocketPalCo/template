//go:build integration

package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"

	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/websocket"
)

func startTestServer(t *testing.T) (*fiber.App, net.Listener) {
	t.Helper()
	cfg, err := config.ConfigFromEnvironment()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	conn, err := postgres.Init(&cfg)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	// ensure test table exists
	_, err = conn.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS test_table (
        id UUID PRIMARY KEY,
        created_at TIMESTAMPTZ NOT NULL,
        updated_at TIMESTAMPTZ NOT NULL,
        text TEXT NOT NULL
    )`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	app := fiber.New()
	initGlobalMiddlewares(app, &cfg)
	registerHttpRoutes(app, &cfg, conn)
	setupWs(app, &cfg, conn)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go app.Listener(ln)
	t.Cleanup(func() {
		_ = app.Shutdown()
		conn.Close()
	})
	return app, ln
}

func TestRESTEndpoint(t *testing.T) {
	_, ln := startTestServer(t)
	url := "http://" + ln.Addr().String() + "/v1/test"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var data []Resp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected rows, got 0")
	}
}

func TestWebSocketEcho(t *testing.T) {
	_, ln := startTestServer(t)
	url := "ws://" + ln.Addr().String() + "/ws/123"
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// ignore join broadcast
	if _, _, err := c.ReadMessage(); err != nil {
		t.Fatalf("read join: %v", err)
	}
	msg := "hello"
	if err := c.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != msg {
		t.Fatalf("expected %s got %s", msg, string(data))
	}
}
