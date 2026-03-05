package ports

import (
	"context"
	"time"

	"github.com/flyluman/aelora/internal/domain"
)

type MessageRepository interface {
	Save(ctx context.Context, msg domain.Message) error
	ExistsByClientMsgID(ctx context.Context, senderID, clientMsgID string) (bool, error)
	ListByRoom(ctx context.Context, roomID string, since time.Time, limit int) ([]domain.Message, error)
	GetByIDs(ctx context.Context, roomID string, messageIDs []string) (map[string]domain.Message, error)
}

type MessageOutboxRepository interface {
	Enqueue(ctx context.Context, msg domain.Message, streamAppended bool) error
	ListPending(ctx context.Context, limit int) ([]OutboxMessage, error)
	ClaimPending(ctx context.Context, consumerID string, limit int, lease time.Duration) ([]OutboxMessage, error)
	MarkStreamAppended(ctx context.Context, messageID string) error
	MarkDispatched(ctx context.Context, messageID string) error
	ReleaseClaim(ctx context.Context, consumerID, messageID string) error
}

type OutboxMessage struct {
	Message        domain.Message
	StreamAppended bool
}

type RoomRepository interface {
	Get(ctx context.Context, roomID string) (domain.Room, error)
	Create(ctx context.Context, room domain.Room) error
	AddMember(ctx context.Context, roomID, userID string) error
	ListMembers(ctx context.Context, roomID string) ([]string, error)
}

type RoomIDGenerator interface {
	New(ctx context.Context) (string, error)
}

type EventBus interface {
	PublishMessageCreated(ctx context.Context, msg domain.Message) error
	SubscribeMessageCreated(ctx context.Context, buffer int) (<-chan domain.Message, func(), error)
}
