package application

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type SyncMessagesQuery struct {
	RoomID   string
	UserID   string
	DeviceID string
	LastAck  string
	Limit    int
}

type SyncMessagesUseCase struct {
	rooms  ports.RoomRepository
	stream ports.StreamRepository
	ack    ports.AckRepository
	msgs   ports.MessageRepository
}

func NewSyncMessagesUseCase(rooms ports.RoomRepository, stream ports.StreamRepository, ack ports.AckRepository, msgs ports.MessageRepository) *SyncMessagesUseCase {
	return &SyncMessagesUseCase{rooms: rooms, stream: stream, ack: ack, msgs: msgs}
}

type SyncMessagesResult struct {
	Messages []ports.StreamMessage
	Replies  map[string]domain.Message
}

func (u *SyncMessagesUseCase) Execute(ctx context.Context, q SyncMessagesQuery) (SyncMessagesResult, error) {
	room, err := u.rooms.Get(ctx, q.RoomID)
	if err != nil {
		return SyncMessagesResult{}, err
	}
	if !room.HasMember(q.UserID) {
		return SyncMessagesResult{}, ErrRoomAccessDenied
	}
	if q.LastAck == "" {
		q.LastAck, err = u.ack.GetLastAck(ctx, q.UserID, q.DeviceID, q.RoomID)
		if err != nil {
			return SyncMessagesResult{}, err
		}
		if q.LastAck == "" {
			q.LastAck = "0"
		}
	}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	messages, err := u.stream.ReadFrom(ctx, q.RoomID, q.LastAck, q.Limit)
	if err != nil {
		return SyncMessagesResult{}, err
	}
	if len(messages) == 0 && u.msgs != nil {
		recovered, recoverErr := u.recoverAndBackfill(ctx, q.RoomID, q.LastAck, q.Limit)
		if recoverErr != nil {
			return SyncMessagesResult{}, recoverErr
		}
		messages = recovered
	}

	replyIDs := make([]string, 0, len(messages))
	seen := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		if message.Message.ReplyToID == "" {
			continue
		}
		if _, ok := seen[message.Message.ReplyToID]; ok {
			continue
		}
		seen[message.Message.ReplyToID] = struct{}{}
		replyIDs = append(replyIDs, message.Message.ReplyToID)
	}
	if len(replyIDs) == 0 || u.msgs == nil {
		return SyncMessagesResult{Messages: messages, Replies: map[string]domain.Message{}}, nil
	}

	replies, err := u.msgs.GetByIDs(ctx, q.RoomID, replyIDs)
	if err != nil {
		return SyncMessagesResult{}, err
	}
	return SyncMessagesResult{Messages: messages, Replies: replies}, nil
}

func (u *SyncMessagesUseCase) recoverAndBackfill(ctx context.Context, roomID, lastAck string, limit int) ([]ports.StreamMessage, error) {
	since := time.Time{}
	if lastAck != "" && lastAck != "0" {
		if ts, ok := streamIDToTime(lastAck); ok {
			since = ts
		}
	}

	history, err := u.msgs.ListByRoom(ctx, roomID, since, limit)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, nil
	}

	out := make([]ports.StreamMessage, 0, len(history))
	for _, msg := range history {
		streamID, err := u.stream.AppendMessage(ctx, roomID, msg)
		if err != nil {
			return nil, err
		}
		out = append(out, ports.StreamMessage{
			StreamID: streamID,
			RoomID:   roomID,
			Message:  msg,
		})
	}
	return out, nil
}

func streamIDToTime(streamID string) (time.Time, bool) {
	parts := strings.SplitN(streamID, "-", 2)
	if len(parts) != 2 {
		return time.Time{}, false
	}
	ms, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(ms).UTC(), true
}

type AckMessageCommand struct {
	RoomID   string
	UserID   string
	DeviceID string
	StreamID string
}

type AckMessageUseCase struct {
	rooms ports.RoomRepository
	ack   ports.AckRepository
}

func NewAckMessageUseCase(rooms ports.RoomRepository, ack ports.AckRepository) *AckMessageUseCase {
	return &AckMessageUseCase{rooms: rooms, ack: ack}
}

func (u *AckMessageUseCase) Execute(ctx context.Context, cmd AckMessageCommand) error {
	if strings.TrimSpace(cmd.RoomID) == "" || strings.TrimSpace(cmd.UserID) == "" || strings.TrimSpace(cmd.DeviceID) == "" || strings.TrimSpace(cmd.StreamID) == "" {
		return ErrInvalidAck
	}
	room, err := u.rooms.Get(ctx, cmd.RoomID)
	if err != nil {
		return err
	}
	if !room.HasMember(cmd.UserID) {
		return ErrRoomAccessDenied
	}
	return u.ack.SetLastAck(ctx, cmd.UserID, cmd.DeviceID, cmd.RoomID, cmd.StreamID)
}

func StreamMessagesToDomainMessages(messages []ports.StreamMessage) []domain.Message {
	out := make([]domain.Message, 0, len(messages))
	for _, message := range messages {
		out = append(out, message.Message)
	}
	return out
}
