package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type unitMessageRepo struct {
	msgs    map[string]domain.Message
	exists  bool
	saveErr error
}

func (r *unitMessageRepo) Save(_ context.Context, msg domain.Message) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.msgs[msg.ID] = msg
	return nil
}

func (r *unitMessageRepo) ExistsByClientMsgID(_ context.Context, _, _ string) (bool, error) {
	return r.exists, nil
}

func (r *unitMessageRepo) ListByRoom(_ context.Context, _ string, _ time.Time, _ int) ([]domain.Message, error) {
	return nil, nil
}

func (r *unitMessageRepo) GetByIDs(_ context.Context, _ string, ids []string) (map[string]domain.Message, error) {
	out := map[string]domain.Message{}
	for _, id := range ids {
		if msg, ok := r.msgs[id]; ok {
			out[id] = msg
		}
	}
	return out, nil
}

type unitRoomRepo struct {
	room domain.Room
}

func (r *unitRoomRepo) Get(_ context.Context, _ string) (domain.Room, error) { return r.room, nil }
func (r *unitRoomRepo) Create(_ context.Context, _ domain.Room) error        { return nil }
func (r *unitRoomRepo) AddMember(_ context.Context, _, _ string) error       { return nil }
func (r *unitRoomRepo) ListMembers(_ context.Context, _ string) ([]string, error) {
	return []string{"u1", "u2"}, nil
}

type unitID struct{}

func (unitID) New() string { return "m-2" }

type unitClock struct{}

func (unitClock) Now() time.Time { return time.Unix(1700000010, 0).UTC() }

type unitStream struct{}

func (unitStream) AppendMessage(_ context.Context, _ string, _ domain.Message) (string, error) {
	return "1-0", nil
}
func (unitStream) ReadFrom(_ context.Context, _, _ string, _ int) ([]ports.StreamMessage, error) {
	return nil, nil
}
func (unitStream) PublishMessageCreated(_ context.Context, _ domain.Message) error { return nil }
func (unitStream) SubscribeMessageCreated(_ context.Context, _ int) (<-chan domain.Message, func(), error) {
	ch := make(chan domain.Message)
	close(ch)
	return ch, func() {}, nil
}

type allowLimiter struct{}

func (allowLimiter) Allow(_ string) bool { return true }

type denyLimiter struct{}

func (denyLimiter) Allow(_ string) bool { return false }

type noopOutbox struct{}

func (noopOutbox) Enqueue(_ context.Context, _ domain.Message, _ bool) error { return nil }
func (noopOutbox) ListPending(_ context.Context, _ int) ([]ports.OutboxMessage, error) {
	return nil, nil
}
func (noopOutbox) ClaimPending(_ context.Context, _ string, _ int, _ time.Duration) ([]ports.OutboxMessage, error) {
	return nil, nil
}
func (noopOutbox) MarkStreamAppended(_ context.Context, _ string) error { return nil }
func (noopOutbox) MarkDispatched(_ context.Context, _ string) error     { return nil }
func (noopOutbox) ReleaseClaim(_ context.Context, _, _ string) error    { return nil }

type streamFailingStream struct{}

func (streamFailingStream) AppendMessage(_ context.Context, _ string, _ domain.Message) (string, error) {
	return "", errors.New("append failed")
}
func (streamFailingStream) ReadFrom(_ context.Context, _, _ string, _ int) ([]ports.StreamMessage, error) {
	return nil, nil
}
func (streamFailingStream) PublishMessageCreated(_ context.Context, _ domain.Message) error {
	return nil
}
func (streamFailingStream) SubscribeMessageCreated(_ context.Context, _ int) (<-chan domain.Message, func(), error) {
	ch := make(chan domain.Message)
	close(ch)
	return ch, func() {}, nil
}

type recordingOutbox struct {
	enqueued []ports.OutboxMessage
	err      error
}

func (o *recordingOutbox) Enqueue(_ context.Context, msg domain.Message, streamAppended bool) error {
	if o.err != nil {
		return o.err
	}
	o.enqueued = append(o.enqueued, ports.OutboxMessage{Message: msg, StreamAppended: streamAppended})
	return nil
}
func (o *recordingOutbox) ListPending(_ context.Context, _ int) ([]ports.OutboxMessage, error) {
	return nil, nil
}
func (o *recordingOutbox) ClaimPending(_ context.Context, _ string, _ int, _ time.Duration) ([]ports.OutboxMessage, error) {
	return nil, nil
}
func (o *recordingOutbox) MarkStreamAppended(_ context.Context, _ string) error { return nil }
func (o *recordingOutbox) MarkDispatched(_ context.Context, _ string) error     { return nil }
func (o *recordingOutbox) ReleaseClaim(_ context.Context, _, _ string) error    { return nil }

type publishFailStream struct{ unitStream }

func (publishFailStream) PublishMessageCreated(_ context.Context, _ domain.Message) error {
	return errors.New("publish fail")
}

func TestSendMessageRejectsUnknownReplyTarget(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1", "u2"})
	msgRepo := &unitMessageRepo{msgs: map[string]domain.Message{}}
	send := NewSendMessageUseCase(msgRepo, &unitRoomRepo{room: room}, unitID{}, unitClock{}, unitStream{}, unitStream{}, noopOutbox{}, allowLimiter{})

	_, err := send.Execute(context.Background(), SendMessageCommand{
		RoomID:      "r1",
		SenderID:    "u1",
		Content:     "reply",
		ClientMsgID: "c-1",
		ReplyToID:   "missing",
	})
	if err != ErrReplyTarget {
		t.Fatalf("expected ErrReplyTarget, got %v", err)
	}
}

func TestSendMessageRateLimited(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1"})
	msgRepo := &unitMessageRepo{msgs: map[string]domain.Message{}}
	send := NewSendMessageUseCase(msgRepo, &unitRoomRepo{room: room}, unitID{}, unitClock{}, unitStream{}, unitStream{}, noopOutbox{}, denyLimiter{})
	_, err := send.Execute(context.Background(), SendMessageCommand{RoomID: "r1", SenderID: "u1", Content: "x", ClientMsgID: "c"})
	if err != ErrRateLimited {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestSendMessageDuplicateExistsCheck(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1"})
	msgRepo := &unitMessageRepo{msgs: map[string]domain.Message{}, exists: true}
	send := NewSendMessageUseCase(msgRepo, &unitRoomRepo{room: room}, unitID{}, unitClock{}, unitStream{}, unitStream{}, noopOutbox{}, allowLimiter{})
	_, err := send.Execute(context.Background(), SendMessageCommand{RoomID: "r1", SenderID: "u1", Content: "x", ClientMsgID: "c"})
	if err != ErrDuplicateMessage {
		t.Fatalf("expected ErrDuplicateMessage, got %v", err)
	}
}

func TestSendMessageRoomAccessDenied(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u2"})
	msgRepo := &unitMessageRepo{msgs: map[string]domain.Message{}}
	send := NewSendMessageUseCase(msgRepo, &unitRoomRepo{room: room}, unitID{}, unitClock{}, unitStream{}, unitStream{}, noopOutbox{}, allowLimiter{})
	_, err := send.Execute(context.Background(), SendMessageCommand{RoomID: "r1", SenderID: "u1", Content: "x", ClientMsgID: "c"})
	if err != ErrRoomAccessDenied {
		t.Fatalf("expected ErrRoomAccessDenied, got %v", err)
	}
}

func TestSendMessagePublishFailEnqueuesOutbox(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1"})
	msgRepo := &unitMessageRepo{msgs: map[string]domain.Message{}}
	outbox := &recordingOutbox{}
	stream := publishFailStream{}
	send := NewSendMessageUseCase(msgRepo, &unitRoomRepo{room: room}, unitID{}, unitClock{}, stream, stream, outbox, allowLimiter{})
	_, err := send.Execute(context.Background(), SendMessageCommand{RoomID: "r1", SenderID: "u1", Content: "x", ClientMsgID: "c"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(outbox.enqueued) != 1 || !outbox.enqueued[0].StreamAppended {
		t.Fatalf("expected streamAppended=true enqueue, got %+v", outbox.enqueued)
	}
}

func TestSendMessagePublishFailOutboxErrorReturned(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1"})
	msgRepo := &unitMessageRepo{msgs: map[string]domain.Message{}}
	outbox := &recordingOutbox{err: errors.New("outbox fail")}
	stream := publishFailStream{}
	send := NewSendMessageUseCase(msgRepo, &unitRoomRepo{room: room}, unitID{}, unitClock{}, stream, stream, outbox, allowLimiter{})
	_, err := send.Execute(context.Background(), SendMessageCommand{RoomID: "r1", SenderID: "u1", Content: "x", ClientMsgID: "c"})
	if err == nil {
		t.Fatalf("expected error when outbox enqueue fails")
	}
}

func TestSendMessageQueuesOutboxWhenStreamAppendFails(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1", "u2"})
	msgRepo := &unitMessageRepo{msgs: map[string]domain.Message{}}
	outbox := &recordingOutbox{}
	send := NewSendMessageUseCase(msgRepo, &unitRoomRepo{room: room}, unitID{}, unitClock{}, streamFailingStream{}, unitStream{}, outbox, allowLimiter{})

	msg, err := send.Execute(context.Background(), SendMessageCommand{
		RoomID:      "r1",
		SenderID:    "u1",
		Content:     "hello",
		ClientMsgID: "c-append-fail",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if msg.ID == "" {
		t.Fatal("expected message to be returned")
	}
	if len(outbox.enqueued) != 1 {
		t.Fatalf("expected 1 outbox enqueue, got %d", len(outbox.enqueued))
	}
	if outbox.enqueued[0].StreamAppended {
		t.Fatal("expected stream_appended=false when stream append fails")
	}
}
