package application

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type syncRoomRepo struct {
	room domain.Room
}

func (r *syncRoomRepo) Get(_ context.Context, _ string) (domain.Room, error) { return r.room, nil }
func (r *syncRoomRepo) Create(_ context.Context, _ domain.Room) error        { return nil }
func (r *syncRoomRepo) AddMember(_ context.Context, _, _ string) error       { return nil }
func (r *syncRoomRepo) ListMembers(_ context.Context, _ string) ([]string, error) {
	return []string{"u1", "u2"}, nil
}

type syncStreamRepo struct {
	messages []ports.StreamMessage
	nextID   int
}

func (s *syncStreamRepo) AppendMessage(_ context.Context, _ string, _ domain.Message) (string, error) {
	s.nextID++
	return "1-" + strconv.Itoa(s.nextID), nil
}
func (s *syncStreamRepo) ReadFrom(_ context.Context, _, _ string, _ int) ([]ports.StreamMessage, error) {
	return s.messages, nil
}

type syncAckRepo struct{}

func (syncAckRepo) SetLastAck(_ context.Context, _, _, _, _ string) error        { return nil }
func (syncAckRepo) GetLastAck(_ context.Context, _, _, _ string) (string, error) { return "", nil }

type syncMessageRepo struct {
	replies map[string]domain.Message
	history []domain.Message
}

func (r *syncMessageRepo) Save(_ context.Context, _ domain.Message) error { return nil }
func (r *syncMessageRepo) ExistsByClientMsgID(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (r *syncMessageRepo) ListByRoom(_ context.Context, _ string, since time.Time, _ int) ([]domain.Message, error) {
	if len(r.history) == 0 {
		return nil, nil
	}
	out := make([]domain.Message, 0, len(r.history))
	for _, msg := range r.history {
		if msg.CreatedAt.After(since) {
			out = append(out, msg)
		}
	}
	return out, nil
}
func (r *syncMessageRepo) GetByIDs(_ context.Context, _ string, ids []string) (map[string]domain.Message, error) {
	out := map[string]domain.Message{}
	for _, id := range ids {
		if msg, ok := r.replies[id]; ok {
			out[id] = msg
		}
	}
	return out, nil
}

func TestSyncMessagesReturnsReplyPayloads(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1", "u2"})
	original := domain.Message{ID: "m-1", RoomID: "r1", SenderID: "u1", Content: "hello", ClientMsgID: "c1"}
	reply := domain.Message{ID: "m-2", RoomID: "r1", SenderID: "u2", Content: "reply", ClientMsgID: "c2", ReplyToID: "m-1"}

	stream := &syncStreamRepo{
		messages: []ports.StreamMessage{{StreamID: "10-0", RoomID: "r1", Message: reply}},
	}
	msgRepo := &syncMessageRepo{replies: map[string]domain.Message{"m-1": original}}
	uc := NewSyncMessagesUseCase(&syncRoomRepo{room: room}, stream, syncAckRepo{}, msgRepo)

	result, err := uc.Execute(context.Background(), SyncMessagesQuery{RoomID: "r1", UserID: "u2", DeviceID: "d1", LastAck: "0", Limit: 10})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if got := result.Replies["m-1"].ID; got != "m-1" {
		t.Fatalf("expected hydrated reply m-1, got %q", got)
	}
}

func TestAckMessageRejectsInvalidPayload(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1", "u2"})
	uc := NewAckMessageUseCase(&syncRoomRepo{room: room}, syncAckRepo{})

	err := uc.Execute(context.Background(), AckMessageCommand{
		RoomID:   "r1",
		UserID:   "u2",
		DeviceID: "d1",
		StreamID: "",
	})
	if err != ErrInvalidAck {
		t.Fatalf("expected ErrInvalidAck, got %v", err)
	}
}

func TestSyncMessagesFallsBackToPostgresWhenRedisStreamIsEmpty(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1", "u2"})
	msg := domain.Message{
		ID:          "m-10",
		ClientMsgID: "c-10",
		RoomID:      "r1",
		SenderID:    "u1",
		Content:     "fallback",
		CreatedAt:   time.Unix(1700001000, 0).UTC(),
	}
	stream := &syncStreamRepo{messages: nil}
	msgRepo := &syncMessageRepo{history: []domain.Message{msg}}
	uc := NewSyncMessagesUseCase(&syncRoomRepo{room: room}, stream, syncAckRepo{}, msgRepo)

	result, err := uc.Execute(context.Background(), SyncMessagesQuery{
		RoomID:   "r1",
		UserID:   "u2",
		DeviceID: "d1",
		LastAck:  "0",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 recovered message, got %d", len(result.Messages))
	}
	if result.Messages[0].Message.ID != "m-10" {
		t.Fatalf("expected recovered message m-10, got %q", result.Messages[0].Message.ID)
	}
}

func TestStreamIDToTime(t *testing.T) {
	if _, ok := streamIDToTime("bad"); ok {
		t.Fatal("expected invalid stream id parse")
	}
	if _, ok := streamIDToTime("abc-1"); ok {
		t.Fatal("expected invalid millis parse")
	}
	ts, ok := streamIDToTime("1700000000000-1")
	if !ok {
		t.Fatal("expected valid stream id parse")
	}
	if ts.UnixMilli() != 1700000000000 {
		t.Fatalf("unexpected millis: %d", ts.UnixMilli())
	}
}
