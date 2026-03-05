package application

import (
	"context"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type ListMessagesQuery struct {
	RoomID string
	UserID string
	Since  time.Time
	Limit  int
}

type ListMessagesUseCase struct {
	rooms    ports.RoomRepository
	messages ports.MessageRepository
}

func NewListMessagesUseCase(rooms ports.RoomRepository, messages ports.MessageRepository) *ListMessagesUseCase {
	return &ListMessagesUseCase{rooms: rooms, messages: messages}
}

func (u *ListMessagesUseCase) Execute(ctx context.Context, q ListMessagesQuery) ([]domain.Message, error) {
	room, err := u.rooms.Get(ctx, q.RoomID)
	if err != nil {
		return nil, err
	}
	if !room.HasMember(q.UserID) {
		return nil, ErrRoomAccessDenied
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > 500 {
		q.Limit = 500
	}
	return u.messages.ListByRoom(ctx, q.RoomID, q.Since, q.Limit)
}
