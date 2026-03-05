package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/flyluman/aelora/internal/application"
	"github.com/flyluman/aelora/internal/domain"
	"github.com/labstack/echo/v4"
)

type envelope struct {
	Success bool     `json:"success"`
	Data    any      `json:"data,omitempty"`
	Error   *errBody `json:"error,omitempty"`
}

type errBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type itemResponse struct {
	ID        string `json:"id"`
	RoomID    string `json:"room_id"`
	SenderID  string `json:"sender_id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

func writeSuccess(c echo.Context, status int, data any) error {
	return c.JSON(status, envelope{Success: true, Data: data})
}

func writeError(c echo.Context, status int, code, message string) error {
	return c.JSON(status, envelope{Success: false, Error: &errBody{Code: code, Message: message}})
}

func writeAppError(c echo.Context, err error) error {
	status, code, message := mapAppError(err)
	return writeError(c, status, code, message)
}

func mapAppError(err error) (status int, code, message string) {
	if appErr, ok := errors.AsType[*application.AppError](err); ok {
		return appErr.HTTPStatus, appErr.Code, appErr.Err.Error()
	}

	switch {
	case errors.Is(err, domain.ErrRoomNotFound):
		return http.StatusNotFound, "room_not_found", err.Error()
	case errors.Is(err, domain.ErrRoomAlreadyExists):
		return http.StatusConflict, "room_already_exists", err.Error()
	case errors.Is(err, domain.ErrInvalidMessage), errors.Is(err, domain.ErrInvalidRoom):
		return http.StatusBadRequest, "invalid_request", err.Error()
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}

func toItemResponse(msg domain.Message) itemResponse {
	return itemResponse{
		ID:        msg.ID,
		RoomID:    msg.RoomID,
		SenderID:  msg.SenderID,
		Content:   msg.Content,
		CreatedAt: msg.CreatedAt.Format(time.RFC3339Nano),
	}
}

func roomMemberIDs(room domain.Room) []string {
	ids := make([]string, 0, len(room.Members))
	for id := range room.Members {
		ids = append(ids, id)
	}
	return ids
}

func identityFromContext(c echo.Context) string {
	id, _ := c.Get(identityKey).(string)
	return id
}
