package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/flyluman/aelora/internal/application"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type createRoomRequest struct {
	MemberIDs []string `json:"member_ids"`
}

type roomMembershipResponse struct {
	RoomID      string   `json:"room_id"`
	Members     []string `json:"members"`
	Actor       string   `json:"actor"`
	AddedUserID string   `json:"added_user_id,omitempty"`
}

type sendMessageRequest struct {
	Content     string `json:"content"`
	ClientMsgID string `json:"client_msg_id"`
	ReplyToID   string `json:"reply_to_message_id"`
}

type messageResponse struct {
	MessageID      string    `json:"message_id"`
	ClientMsgID    string    `json:"client_msg_id"`
	RoomID         string    `json:"room_id"`
	SenderID       string    `json:"sender_id"`
	Content        string    `json:"content"`
	ReplyToMessage string    `json:"reply_to_message_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type registerPushTokenRequest struct {
	Platform string `json:"platform"`
	Token    string `json:"token"`
}

func (h *Handler) createRoom(c echo.Context) error {
	var req createRoomRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
	}

	userID := identityFromContext(c)
	if userID == "" {
		userID = c.Request().Header.Get("X-User-ID")
	}
	if userID == "" {
		return writeError(c, http.StatusBadRequest, "missing_user_identity", "missing user identity")
	}

	room, err := h.app.CreateRoom.Execute(c.Request().Context(), application.CreateRoomCommand{
		Creator:   userID,
		MemberIDs: req.MemberIDs,
	})
	if err != nil {
		return writeAppError(c, err)
	}

	members := roomMemberIDs(room)
	h.logger().Info("audit.room_created",
		zap.String("room_id", room.ID),
		zap.String("actor_id", userID),
		zap.Strings("members", members),
	)

	return writeSuccess(c, http.StatusCreated, roomMembershipResponse{
		RoomID:  room.ID,
		Members: members,
		Actor:   userID,
	})
}

func (h *Handler) joinRoom(c echo.Context) error {
	userID := identityFromContext(c)
	if userID == "" {
		userID = c.Request().Header.Get("X-User-ID")
	}
	if userID == "" {
		return writeError(c, http.StatusBadRequest, "missing_user_identity", "missing user identity")
	}

	targetUserID := c.Param("user_id")
	roomID := c.Param("room_id")
	if roomID == "" || targetUserID == "" {
		return writeError(c, http.StatusBadRequest, "missing_path_param", "missing room_id or user_id in path")
	}

	if err := h.app.JoinRoom.Execute(c.Request().Context(), application.JoinRoomCommand{
		RoomID:  roomID,
		ActorID: userID,
		UserID:  targetUserID,
	}); err != nil {
		return writeAppError(c, err)
	}

	members, err := h.hub.rooms.ListMembers(c.Request().Context(), roomID)
	if err != nil {
		return writeAppError(c, err)
	}

	h.logger().Info("audit.room_member_added",
		zap.String("room_id", roomID),
		zap.String("actor_id", userID),
		zap.String("added_user_id", targetUserID),
	)

	return writeSuccess(c, http.StatusOK, roomMembershipResponse{
		RoomID:      roomID,
		Members:     members,
		Actor:       userID,
		AddedUserID: targetUserID,
	})
}

func (h *Handler) sendMessage(c echo.Context) error {
	var req sendMessageRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
	}

	roomID := c.Param("room_id")
	userID := identityFromContext(c)
	if userID == "" {
		userID = c.Request().Header.Get("X-User-ID")
	}
	if userID == "" || roomID == "" {
		return writeError(c, http.StatusBadRequest, "missing_field", "missing user identity or room_id")
	}

	msg, err := h.app.SendMessage.Execute(c.Request().Context(), application.SendMessageCommand{
		RoomID:      roomID,
		SenderID:    userID,
		Content:     req.Content,
		ClientMsgID: req.ClientMsgID,
		ReplyToID:   req.ReplyToID,
	})
	if err != nil {
		return writeAppError(c, err)
	}

	return writeSuccess(c, http.StatusCreated, toItemResponse(msg))
}

func (h *Handler) listMessages(c echo.Context) error {
	roomID := c.Param("room_id")
	userID := identityFromContext(c)
	if userID == "" {
		userID = c.Request().Header.Get("X-User-ID")
	}
	if userID == "" {
		return writeError(c, http.StatusBadRequest, "missing_user_identity", "missing user identity")
	}

	limit := 50
	if raw := c.QueryParam("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "invalid_limit", "invalid limit")
		}
		limit = parsed
	}

	var since time.Time
	if raw := c.QueryParam("since"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "invalid_since", "invalid since timestamp")
		}
		since = parsed
	}

	messages, err := h.app.ListMessages.Execute(c.Request().Context(), application.ListMessagesQuery{
		RoomID: roomID,
		UserID: userID,
		Limit:  limit,
		Since:  since,
	})
	if err != nil {
		return writeAppError(c, err)
	}

	out := make([]messageResponse, 0, len(messages))
	for _, msg := range messages {
		out = append(out, messageResponse{
			MessageID:      msg.ID,
			ClientMsgID:    msg.ClientMsgID,
			RoomID:         msg.RoomID,
			SenderID:       msg.SenderID,
			Content:        msg.Content,
			ReplyToMessage: msg.ReplyToID,
			CreatedAt:      msg.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, envelope{
		Success: true,
		Data: map[string]any{
			"room_id":  roomID,
			"messages": out,
			"count":    len(out),
			"limit":    limit,
		},
	})
}

func (h *Handler) registerPushTokenHandler(c echo.Context) error {
	userID := identityFromContext(c)
	if userID == "" {
		userID = c.Request().Header.Get("X-User-ID")
	}
	if userID == "" {
		return writeError(c, http.StatusBadRequest, "missing_user_identity", "missing user identity")
	}

	var req registerPushTokenRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
	}

	deviceID := c.Param("device_id")
	if deviceID == "" {
		return writeError(c, http.StatusBadRequest, "missing_device_id", "missing device_id in path")
	}

	if err := h.app.RegisterPush.Execute(c.Request().Context(), application.RegisterPushTokenCommand{
		UserID:   userID,
		DeviceID: deviceID,
		Platform: req.Platform,
		Token:    req.Token,
	}); err != nil {
		return writeAppError(c, err)
	}

	h.logger().Info("audit.push_token_registered",
		zap.String("user_id", userID),
		zap.String("device_id", deviceID),
		zap.String("platform", req.Platform),
		zap.Int("token_length", len(req.Token)),
	)

	return writeSuccess(c, http.StatusOK, map[string]string{
		"user_id":   userID,
		"device_id": deviceID,
		"platform":  req.Platform,
		"action":    "registered",
	})
}

func (h *Handler) deletePushTokenHandler(c echo.Context) error {
	userID := identityFromContext(c)
	if userID == "" {
		userID = c.Request().Header.Get("X-User-ID")
	}
	if userID == "" {
		return writeError(c, http.StatusBadRequest, "missing_user_identity", "missing user identity")
	}

	deviceID := c.Param("device_id")
	if deviceID == "" {
		return writeError(c, http.StatusBadRequest, "missing_device_id", "missing device_id in path")
	}

	if err := h.app.DeletePush.Execute(c.Request().Context(), application.DeletePushTokenCommand{
		UserID:   userID,
		DeviceID: deviceID,
	}); err != nil {
		return writeAppError(c, err)
	}

	h.logger().Info("audit.push_token_deleted",
		zap.String("user_id", userID),
		zap.String("device_id", deviceID),
	)

	return writeSuccess(c, http.StatusOK, map[string]string{
		"user_id":   userID,
		"device_id": deviceID,
		"action":    "deleted",
	})
}

func (h *Handler) live(c echo.Context) error {
	return writeSuccess(c, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ready(c echo.Context) error {
	if h.health == nil {
		return writeSuccess(c, http.StatusOK, map[string]string{"status": "ok"})
	}

	report := h.health.Check(c.Request().Context())
	if report.Status != "ok" {
		h.logger().Warn("service_unavailable",
			zap.Any("health_report", report),
		)
		return c.JSON(http.StatusServiceUnavailable, envelope{
			Success: false,
			Error: &errBody{
				Code:    "service_unavailable",
				Message: "service unavailable",
			},
		})
	}

	return writeSuccess(c, http.StatusOK, report)
}
