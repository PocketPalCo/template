// Package rest_test contains integration tests for the rest package.
package rest_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/core/rtc"
	"github.com/PocketPalCo/shopping-service/internal/infra/rest"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestApp initializes a Fiber app with RTC routes for testing.
func setupTestApp() (*fiber.App, *rtc.RTCService) {
	app := fiber.New()
	rtcService := rtc.NewRTCService()
	// Use a default/test config. Adjust if specific config values are needed for RTC routes.
	cfg := &config.Config{}
	rest.RegisterRTCRoutes(app, rtcService, cfg)
	return app, rtcService
}

// Helper to make requests and return the response
func performRequest(app *fiber.App, method, target string, body io.Reader) (*http.Response, string) {
	req := httptest.NewRequest(method, target, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, _ := app.Test(req, -1) // -1 for no timeout

	respBodyBytes, _ := io.ReadAll(resp.Body)
	return resp, string(respBodyBytes)
}

// TestCreateRoomAPI tests the POST /v1/rtc/room endpoint.
func TestCreateRoomAPI(t *testing.T) {
	app, rtcService := setupTestApp()

	t.Run("create room with specific ID", func(t *testing.T) {
		roomID := "test-room-alpha"
		payload := rest.CreateRoomRequest{RoomID: roomID}
		jsonPayload, _ := json.Marshal(payload)

		resp, body := performRequest(app, "POST", "/v1/rtc/room", bytes.NewBuffer(jsonPayload))

		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Status code should be 201")

		var respData rest.CreateRoomResponse
		err := json.Unmarshal([]byte(body), &respData)
		require.NoError(t, err, "Should unmarshal response")
		assert.Equal(t, roomID, respData.ID, "Response ID should match requested ID")
		assert.Equal(t, 0, respData.Users, "Response users should be 0 for new room")

		// Verify in service
		_, serviceErr := rtcService.GetRoom(roomID)
		assert.NoError(t, serviceErr, "Room should exist in RTCService")
	})

	t.Run("create room with auto-generated ID", func(t *testing.T) {
		resp, body := performRequest(app, "POST", "/v1/rtc/room", nil) // No body

		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Status code should be 201")

		var respData rest.CreateRoomResponse
		err := json.Unmarshal([]byte(body), &respData)
		require.NoError(t, err, "Should unmarshal response")
		assert.NotEmpty(t, respData.ID, "Response ID should be auto-generated and not empty")
		_, err = uuid.Parse(respData.ID) // Check if it's a valid UUID
		assert.NoError(t, err, "Auto-generated ID should be a valid UUID")
		assert.Equal(t, 0, respData.Users, "Response users should be 0 for new room")

		// Verify in service
		_, serviceErr := rtcService.GetRoom(respData.ID)
		assert.NoError(t, serviceErr, "Auto-generated room should exist in RTCService")
	})

	t.Run("create room that already exists", func(t *testing.T) {
		roomID := "test-room-beta"
		// Create it once
		_, _ = rtcService.CreateRoom(roomID)

		payload := rest.CreateRoomRequest{RoomID: roomID}
		jsonPayload, _ := json.Marshal(payload)
		resp, body := performRequest(app, "POST", "/v1/rtc/room", bytes.NewBuffer(jsonPayload))

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "Status code should be 500 for existing room")

		var errResp rest.ErrorResponse
		err := json.Unmarshal([]byte(body), &errResp)
		require.NoError(t, err, "Should unmarshal error response")
		assert.Contains(t, errResp.Error, "already exists", "Error message should indicate room already exists")
	})

	t.Run("create room with invalid JSON payload", func(t *testing.T) {
		invalidJSON := `{"room_id": "test-room-gamma",}` // Trailing comma makes it invalid
		resp, body := performRequest(app, "POST", "/v1/rtc/room", bytes.NewBufferString(invalidJSON))

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Status code should be 400 for invalid JSON")
		var errResp rest.ErrorResponse
		err := json.Unmarshal([]byte(body), &errResp)
		require.NoError(t, err, "Should unmarshal error response")
		assert.Equal(t, "Cannot parse JSON", errResp.Error)
	})
}

// TestGetRoomAPI tests the GET /v1/rtc/room/{roomId} endpoint.
func TestGetRoomAPI(t *testing.T) {
	app, rtcService := setupTestApp()

	t.Run("get existing room", func(t *testing.T) {
		roomID := "test-room-gamma"
		createdRoom, _ := rtcService.CreateRoom(roomID)
		_, _ = rtcService.JoinRoom(roomID, "userA", nil) // Add a user

		resp, body := performRequest(app, "GET", fmt.Sprintf("/v1/rtc/room/%s", roomID), nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var respData rest.GetRoomResponse
		err := json.Unmarshal([]byte(body), &respData)
		require.NoError(t, err)
		assert.Equal(t, createdRoom.ID, respData.ID)
		assert.Len(t, respData.Users, 1)
		assert.Contains(t, respData.Users, "userA")
	})

	t.Run("get non-existent room", func(t *testing.T) {
		roomID := "non-existent-room"
		resp, body := performRequest(app, "GET", fmt.Sprintf("/v1/rtc/room/%s", roomID), nil)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		var errResp rest.ErrorResponse
		err := json.Unmarshal([]byte(body), &errResp)
		require.NoError(t, err)
		assert.Contains(t, errResp.Error, "not found")
	})
}

// TestJoinRoomAPI tests the POST /v1/rtc/room/{roomId}/join endpoint.
func TestJoinRoomAPI(t *testing.T) {
	app, rtcService := setupTestApp()

	roomID := "test-room-delta"
	_, _ = rtcService.CreateRoom(roomID) // Pre-create the room

	t.Run("successfully join room", func(t *testing.T) {
		userID := "user-charlie"
		payload := rest.JoinLeaveRoomRequest{UserID: userID}
		jsonPayload, _ := json.Marshal(payload)

		resp, body := performRequest(app, "POST", fmt.Sprintf("/v1/rtc/room/%s/join", roomID), bytes.NewBuffer(jsonPayload))

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var successResp rest.SuccessResponse
		err := json.Unmarshal([]byte(body), &successResp)
		require.NoError(t, err)
		assert.Contains(t, successResp.Message, "joined room")

		// Verify in service
		room, _ := rtcService.GetRoom(roomID)
		_, userExists := room.Users[userID]
		assert.True(t, userExists, "User should be in the room in RTCService")
	})

	t.Run("join non-existent room", func(t *testing.T) {
		userID := "user-delta"
		payload := rest.JoinLeaveRoomRequest{UserID: userID}
		jsonPayload, _ := json.Marshal(payload)

		resp, body := performRequest(app, "POST", "/v1/rtc/room/non-existent-room-id/join", bytes.NewBuffer(jsonPayload))

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		var errResp rest.ErrorResponse
		err := json.Unmarshal([]byte(body), &errResp)
		require.NoError(t, err)
		assert.Contains(t, errResp.Error, "not found")
	})

	t.Run("join room with missing user_id", func(t *testing.T) {
		payload := rest.JoinLeaveRoomRequest{} // Empty UserID
		jsonPayload, _ := json.Marshal(payload)

		resp, body := performRequest(app, "POST", fmt.Sprintf("/v1/rtc/room/%s/join", roomID), bytes.NewBuffer(jsonPayload))

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var errResp rest.ErrorResponse
		err := json.Unmarshal([]byte(body), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "User ID is required", errResp.Error)
	})

	t.Run("join room user already in", func(t *testing.T) {
		existingUserID := "user-echo"
		_, _ = rtcService.JoinRoom(roomID, existingUserID, nil) // Add user directly

		payload := rest.JoinLeaveRoomRequest{UserID: existingUserID}
		jsonPayload, _ := json.Marshal(payload)
		resp, body := performRequest(app, "POST", fmt.Sprintf("/v1/rtc/room/%s/join", roomID), bytes.NewBuffer(jsonPayload))

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode) // As per current handler logic
		var errResp rest.ErrorResponse
		err := json.Unmarshal([]byte(body), &errResp)
		require.NoError(t, err)
		assert.Contains(t, errResp.Error, "already in room")
	})
}

// TestLeaveRoomAPI tests the POST /v1/rtc/room/{roomId}/leave endpoint.
func TestLeaveRoomAPI(t *testing.T) {
	app, rtcService := setupTestApp()

	roomID := "test-room-foxtrot"
	userID := "user-golf"

	// Pre-create room and add user
	_, _ = rtcService.CreateRoom(roomID)
	_, _ = rtcService.JoinRoom(roomID, userID, nil)

	t.Run("successfully leave room", func(t *testing.T) {
		payload := rest.JoinLeaveRoomRequest{UserID: userID}
		jsonPayload, _ := json.Marshal(payload)

		resp, body := performRequest(app, "POST", fmt.Sprintf("/v1/rtc/room/%s/leave", roomID), bytes.NewBuffer(jsonPayload))

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var successResp rest.SuccessResponse
		err := json.Unmarshal([]byte(body), &successResp)
		require.NoError(t, err)
		assert.Contains(t, successResp.Message, "left room")

		// Verify in service
		room, _ := rtcService.GetRoom(roomID)
		_, userExists := room.Users[userID]
		assert.False(t, userExists, "User should not be in the room in RTCService after leaving")
	})

	t.Run("leave non-existent room", func(t *testing.T) {
		payload := rest.JoinLeaveRoomRequest{UserID: userID}
		jsonPayload, _ := json.Marshal(payload)

		resp, body := performRequest(app, "POST", "/v1/rtc/room/non-existent-room-id/leave", bytes.NewBuffer(jsonPayload))

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		var errResp rest.ErrorResponse
		err := json.Unmarshal([]byte(body), &errResp)
		require.NoError(t, err)
		assert.Contains(t, errResp.Error, "not found")
	})

	t.Run("leave room user not in", func(t *testing.T) {
		otherUserID := "user-hotel" // This user was never in the room
		payload := rest.JoinLeaveRoomRequest{UserID: otherUserID}
		jsonPayload, _ := json.Marshal(payload)

		resp, body := performRequest(app, "POST", fmt.Sprintf("/v1/rtc/room/%s/leave", roomID), bytes.NewBuffer(jsonPayload))

		assert.Equal(t, http.StatusNotFound, resp.StatusCode) // As per current handler logic
		var errResp rest.ErrorResponse
		err := json.Unmarshal([]byte(body), &errResp)
		require.NoError(t, err)
		assert.Contains(t, errResp.Error, "not in room")
	})

	t.Run("leave room with missing user_id", func(t *testing.T) {
		payload := rest.JoinLeaveRoomRequest{} // Empty UserID
		jsonPayload, _ := json.Marshal(payload)

		resp, body := performRequest(app, "POST", fmt.Sprintf("/v1/rtc/room/%s/leave", roomID), bytes.NewBuffer(jsonPayload))

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var errResp rest.ErrorResponse
		err := json.Unmarshal([]byte(body), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "User ID is required", errResp.Error)
	})
}
