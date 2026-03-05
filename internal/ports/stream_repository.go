package ports

import (
	"context"

	"github.com/flyluman/aelora/internal/domain"
)

type StreamMessage struct {
	StreamID string         `json:"stream_id"`
	RoomID   string         `json:"room_id"`
	Message  domain.Message `json:"message"`
}

type StreamRepository interface {
	AppendMessage(ctx context.Context, roomID string, msg domain.Message) (string, error)
	ReadFrom(ctx context.Context, roomID, lastAck string, limit int) ([]StreamMessage, error)
}
