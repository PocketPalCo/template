//go:build integration

package server_test

import (
    "context"
    "encoding/json"
    "net"
    "net/http"
    "testing"
    "time"

    "github.com/PocketPalCo/shopping-service/config"
    "github.com/PocketPalCo/shopping-service/internal/infra/postgres"
    srv "github.com/PocketPalCo/shopping-service/internal/infra/server"
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    fiberws "github.com/gofiber/contrib/websocket"
    "github.com/gorilla/websocket"
    "github.com/google/uuid"
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
    app.Use(cors.New())
    api := app.Group("/v1")
    api.Get("/test", func(c *fiber.Ctx) error {
        id := uuid.New()
        if _, err := conn.Exec(c.Context(), "INSERT INTO test_table (id, created_at, updated_at, text) VALUES ($1, $2, $3, $4)", id, time.Now(), time.Now(), "sample"); err != nil {
            return fiber.NewError(fiber.StatusInternalServerError, err.Error())
        }
        rows, err := conn.Query(c.Context(), "SELECT id, text FROM test_table")
        if err != nil {
            return fiber.NewError(fiber.StatusInternalServerError, err.Error())
        }
        defer rows.Close()
        resp := make([]srv.Resp, 0)
        for rows.Next() {
            var r srv.Resp
            if err := rows.Scan(&r.ID, &r.Text); err != nil {
                return fiber.NewError(fiber.StatusInternalServerError, err.Error())
            }
            resp = append(resp, r)
        }
        return c.JSON(resp)
    })
    app.Use("/ws", func(c *fiber.Ctx) error {
        if fiberws.IsWebSocketUpgrade(c) {
            c.Locals("allowed", true)
        }
        return c.Next()
    })
    app.Get("/ws/:id", fiberws.New(func(conn *fiberws.Conn) {
        // Echo messages back to the client
        for {
            mt, msg, err := conn.ReadMessage()
            if err != nil {
                return
            }
            if err := conn.WriteMessage(mt, msg); err != nil {
                return
            }
        }
    }))

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
    var data []srv.Resp
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
