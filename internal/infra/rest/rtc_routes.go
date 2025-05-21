// Package rest handles the REST API endpoints.
package rest

import (
	"fmt"
	"log" // Added for logging errors

	"github.com/PocketPalCo/shopping-service/config" // Import main config
	"github.com/PocketPalCo/shopping-service/internal/core/rtc"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid" // For generating room IDs
)

// CreateRoomRequest defines the expected request body for creating a room.
type CreateRoomRequest struct {
	RoomID string `json:"room_id"`
}

// CreateRoomResponse defines the response body for creating a room.
type CreateRoomResponse struct {
	ID    string `json:"id"`
	Users int    `json:"users"` // Number of users in the room
}

// JoinLeaveRoomRequest defines the expected request body for joining or leaving a room.
type JoinLeaveRoomRequest struct {
	UserID string `json:"user_id"`
}

// ErrorResponse defines a standard error response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse defines a standard success response.
type SuccessResponse struct {
	Message string `json:"message"`
}

// GetRoomResponse defines the response for getting room details.
type GetRoomResponse struct {
	ID    string   `json:"id"`
	Users []string `json:"users"` // List of user IDs in the room
}

// CreateRoomHandler handles the creation of a new room.
// @Summary Create a new RTC room
// @Description Creates a new RTC room. A room ID can be provided, or one will be generated.
// @Tags rtc
// @Accept json
// @Produce json
// @Param body body CreateRoomRequest false "Room creation details"
// @Success 201 {object} CreateRoomResponse "Room created successfully"
// @Failure 400 {object} ErrorResponse "Bad Request - Invalid input"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /v1/rtc/room [post]
func CreateRoomHandler(rtcService *rtc.RTCService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		req := new(CreateRoomRequest)
		if err := c.BodyParser(req); err != nil && err != fiber.ErrUnprocessableEntity { // Allow empty body for auto-generation
			log.Printf("CreateRoomHandler: Error parsing request body: %v\n", err)
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Cannot parse JSON"})
		}

		roomID := req.RoomID
		if roomID == "" {
			roomID = uuid.New().String()
		}

		room, err := rtcService.CreateRoom(roomID)
		if err != nil {
			log.Printf("CreateRoomHandler: Error creating room %s: %v\n", roomID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
		}

		return c.Status(fiber.StatusCreated).JSON(CreateRoomResponse{
			ID:    room.ID,
			Users: len(room.Users),
		})
	}
}

// JoinRoomHandler handles a user joining a room.
// @Summary Join an RTC room
// @Description Allows a user to join an existing RTC room.
// @Tags rtc
// @Accept json
// @Produce json
// @Param roomId path string true "Room ID"
// @Param body body JoinLeaveRoomRequest true "User ID to join the room"
// @Success 200 {object} SuccessResponse "User joined successfully"
// @Failure 400 {object} ErrorResponse "Bad Request - Invalid input or user ID missing"
// @Failure 404 {object} ErrorResponse "Not Found - Room not found"
// @Failure 500 {object} ErrorResponse "Internal Server Error - Could not join room"
// @Router /v1/rtc/room/{roomId}/join [post]
func JoinRoomHandler(rtcService *rtc.RTCService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		roomID := c.Params("roomId")
		if roomID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Room ID is required"})
		}

		req := new(JoinLeaveRoomRequest)
		if err := c.BodyParser(req); err != nil {
			log.Printf("JoinRoomHandler: Error parsing request body for room %s: %v\n", roomID, err)
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Cannot parse JSON"})
		}

		if req.UserID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "User ID is required"})
		}

		// For now, we pass nil as the websocket.Conn.
		// This will be handled by the actual WebSocket connection upgrade later.
		_, err := rtcService.JoinRoom(roomID, req.UserID, nil)
		if err != nil {
			log.Printf("JoinRoomHandler: Error joining room %s for user %s: %v\n", roomID, req.UserID, err)
			if err.Error() == fmt.Sprintf("room %s not found", roomID) {
				return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: err.Error()})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
		}

		return c.Status(fiber.StatusOK).JSON(SuccessResponse{Message: fmt.Sprintf("User %s joined room %s", req.UserID, roomID)})
	}
}

// LeaveRoomHandler handles a user leaving a room.
// @Summary Leave an RTC room
// @Description Allows a user to leave an RTC room.
// @Tags rtc
// @Accept json
// @Produce json
// @Param roomId path string true "Room ID"
// @Param body body JoinLeaveRoomRequest true "User ID to leave the room"
// @Success 200 {object} SuccessResponse "User left successfully"
// @Failure 400 {object} ErrorResponse "Bad Request - Invalid input or user ID missing"
// @Failure 404 {object} ErrorResponse "Not Found - Room or user not found"
// @Failure 500 {object} ErrorResponse "Internal Server Error - Could not leave room"
// @Router /v1/rtc/room/{roomId}/leave [post]
func LeaveRoomHandler(rtcService *rtc.RTCService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		roomID := c.Params("roomId")
		if roomID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Room ID is required"})
		}

		req := new(JoinLeaveRoomRequest)
		if err := c.BodyParser(req); err != nil {
			log.Printf("LeaveRoomHandler: Error parsing request body for room %s: %v\n", roomID, err)
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Cannot parse JSON"})
		}

		if req.UserID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "User ID is required"})
		}

		err := rtcService.LeaveRoom(roomID, req.UserID)
		if err != nil {
			log.Printf("LeaveRoomHandler: Error leaving room %s for user %s: %v\n", roomID, req.UserID, err)
			if err.Error() == fmt.Sprintf("room %s not found", roomID) || err.Error() == fmt.Sprintf("user %s not in room %s", req.UserID, roomID) {
				return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: err.Error()})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
		}

		return c.Status(fiber.StatusOK).JSON(SuccessResponse{Message: fmt.Sprintf("User %s left room %s", req.UserID, roomID)})
	}
}

// GetRoomHandler handles retrieving details of a room.
// @Summary Get RTC room details
// @Description Retrieves the details of a specific RTC room, including the list of users.
// @Tags rtc
// @Produce json
// @Param roomId path string true "Room ID"
// @Success 200 {object} GetRoomResponse "Room details retrieved successfully"
// @Failure 404 {object} ErrorResponse "Not Found - Room not found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /v1/rtc/room/{roomId} [get]
func GetRoomHandler(rtcService *rtc.RTCService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		roomID := c.Params("roomId")
		if roomID == "" {
			// This case should ideally be caught by Fiber's routing if the param is defined as required
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Room ID is required"})
		}

		room, err := rtcService.GetRoom(roomID)
		if err != nil {
			log.Printf("GetRoomHandler: Error getting room %s: %v\n", roomID, err)
			if err.Error() == fmt.Sprintf("room %s not found", roomID) {
				return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: err.Error()})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: err.Error()})
		}

		var userIDs []string
		for userID := range room.Users {
			userIDs = append(userIDs, userID)
		}

		return c.Status(fiber.StatusOK).JSON(GetRoomResponse{
			ID:    room.ID,
			Users: userIDs,
		})
	}
}

// RegisterRTCRoutes registers the RTC service routes with the Fiber app.
func RegisterRTCRoutes(app *fiber.App, rtcService *rtc.RTCService, cfg *config.Config) { // Changed to use config.Config
	// Group routes for RTC
	rtcGroup := app.Group("/v1/rtc")

	// Room management endpoints
	rtcGroup.Post("/room", CreateRoomHandler(rtcService))
	rtcGroup.Get("/room/:roomId", GetRoomHandler(rtcService))
	rtcGroup.Post("/room/:roomId/join", JoinRoomHandler(rtcService))
	rtcGroup.Post("/room/:roomId/leave", LeaveRoomHandler(rtcService))

	log.Println("RTC routes registered.")
}
