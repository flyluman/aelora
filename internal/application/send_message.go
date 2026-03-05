package application

import (
	"context"
	"errors"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/platform/telemetry"
	"github.com/flyluman/aelora/internal/ports"
)

type MessageRateLimiter interface {
	Allow(key string) bool
}

type SendMessageCommand struct {
	RoomID      string
	SenderID    string
	Content     string
	ClientMsgID string
	ReplyToID   string
}

type SendMessageUseCase struct {
	messages ports.MessageRepository
	rooms    ports.RoomRepository
	ids      ports.IDGenerator
	clock    ports.Clock
	stream   ports.StreamRepository
	bus      ports.EventBus
	outbox   ports.MessageOutboxRepository
	limiter  MessageRateLimiter
}

func NewSendMessageUseCase(
	messages ports.MessageRepository,
	rooms ports.RoomRepository,
	ids ports.IDGenerator,
	clock ports.Clock,
	stream ports.StreamRepository,
	bus ports.EventBus,
	outbox ports.MessageOutboxRepository,
	limiter MessageRateLimiter,
) *SendMessageUseCase {
	return &SendMessageUseCase{
		messages: messages,
		rooms:    rooms,
		ids:      ids,
		clock:    clock,
		stream:   stream,
		bus:      bus,
		outbox:   outbox,
		limiter:  limiter,
	}
}

func (u *SendMessageUseCase) Execute(ctx context.Context, cmd SendMessageCommand) (domain.Message, error) {
	if u.limiter != nil {
		if !u.limiter.Allow(cmd.SenderID + ":" + cmd.RoomID) {
			return domain.Message{}, ErrRateLimited
		}
	}

	exists, err := u.messages.ExistsByClientMsgID(ctx, cmd.SenderID, cmd.ClientMsgID)
	if err != nil {
		return domain.Message{}, err
	}
	if exists {
		return domain.Message{}, ErrDuplicateMessage
	}

	room, err := u.rooms.Get(ctx, cmd.RoomID)
	if err != nil {
		return domain.Message{}, err
	}
	if !room.HasMember(cmd.SenderID) {
		return domain.Message{}, ErrRoomAccessDenied
	}

	msg := domain.Message{
		ID:          u.ids.New(),
		ClientMsgID: cmd.ClientMsgID,
		RoomID:      cmd.RoomID,
		SenderID:    cmd.SenderID,
		Content:     cmd.Content,
		ReplyToID:   cmd.ReplyToID,
		CreatedAt:   u.clock.Now().UTC(),
	}
	if cmd.ReplyToID != "" {
		replies, err := u.messages.GetByIDs(ctx, cmd.RoomID, []string{cmd.ReplyToID})
		if err != nil {
			return domain.Message{}, err
		}
		if _, ok := replies[cmd.ReplyToID]; !ok {
			return domain.Message{}, ErrReplyTarget
		}
	}
	if err := msg.Validate(); err != nil {
		return domain.Message{}, err
	}
	if err := u.messages.Save(ctx, msg); err != nil {
		if errors.Is(err, domain.ErrDuplicateClientMessage) {
			return domain.Message{}, ErrDuplicateMessage
		}
		return domain.Message{}, err
	}
	if _, err := u.stream.AppendMessage(ctx, cmd.RoomID, msg); err != nil {
		telemetry.IncStreamAppendFailure()
		if u.outbox == nil {
			return domain.Message{}, err
		}
		if enqueueErr := u.outbox.Enqueue(ctx, msg, false); enqueueErr != nil {
			telemetry.IncOutboxEnqueueFailure()
			return domain.Message{}, enqueueErr
		}
		telemetry.IncOutboxEnqueued()
		return msg, nil
	}
	if err := u.bus.PublishMessageCreated(ctx, msg); err != nil {
		telemetry.IncEventPublishFailure()
		if u.outbox != nil {
			if enqueueErr := u.outbox.Enqueue(ctx, msg, true); enqueueErr != nil {
				telemetry.IncOutboxEnqueueFailure()
				return domain.Message{}, enqueueErr
			}
			telemetry.IncOutboxEnqueued()
			return msg, nil
		}
		return domain.Message{}, err
	}
	return msg, nil
}
