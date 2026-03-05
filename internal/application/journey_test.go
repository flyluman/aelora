package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/flyluman/aelora/internal/application"
	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type journeyMessageRepo struct {
	byClient map[string]domain.Message
	byRoom   map[string][]domain.Message
}

func (r *journeyMessageRepo) Save(_ context.Context, msg domain.Message) error {
	r.byClient[msg.SenderID+":"+msg.ClientMsgID] = msg
	r.byRoom[msg.RoomID] = append(r.byRoom[msg.RoomID], msg)
	return nil
}

func (r *journeyMessageRepo) ExistsByClientMsgID(_ context.Context, senderID, id string) (bool, error) {
	_, ok := r.byClient[senderID+":"+id]
	return ok, nil
}

func (r *journeyMessageRepo) ListByRoom(_ context.Context, roomID string, since time.Time, limit int) ([]domain.Message, error) {
	out := make([]domain.Message, 0)
	for _, m := range r.byRoom[roomID] {
		if m.CreatedAt.After(since) {
			out = append(out, m)
		}
	}
	if len(out) > limit && limit > 0 {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (r *journeyMessageRepo) GetByIDs(_ context.Context, roomID string, messageIDs []string) (map[string]domain.Message, error) {
	out := make(map[string]domain.Message, len(messageIDs))
	for _, id := range messageIDs {
		for _, m := range r.byRoom[roomID] {
			if m.ID == id {
				out[id] = m
			}
		}
	}
	return out, nil
}

type journeyRoomRepo struct{ room domain.Room }

func (r *journeyRoomRepo) Get(_ context.Context, _ string) (domain.Room, error) { return r.room, nil }
func (r *journeyRoomRepo) Create(_ context.Context, _ domain.Room) error        { return nil }
func (r *journeyRoomRepo) AddMember(_ context.Context, _, _ string) error       { return nil }
func (r *journeyRoomRepo) ListMembers(_ context.Context, _ string) ([]string, error) {
	return []string{"u1", "u2"}, nil
}

type journeyID struct{}

func (journeyID) New() string { return "m-1" }

type journeyClock struct{}

func (journeyClock) Now() time.Time { return time.Unix(1700000000, 0).UTC() }

type journeyStream struct{ last ports.StreamMessage }

func (s *journeyStream) AppendMessage(_ context.Context, roomID string, msg domain.Message) (string, error) {
	s.last = ports.StreamMessage{StreamID: "1700000000000-0", RoomID: roomID, Message: msg}
	return s.last.StreamID, nil
}
func (s *journeyStream) ReadFrom(_ context.Context, _, _ string, _ int) ([]ports.StreamMessage, error) {
	if s.last.StreamID == "" {
		return nil, nil
	}
	return []ports.StreamMessage{s.last}, nil
}
func (s *journeyStream) PublishMessageCreated(_ context.Context, _ domain.Message) error { return nil }
func (s *journeyStream) SubscribeMessageCreated(_ context.Context, _ int) (<-chan domain.Message, func(), error) {
	ch := make(chan domain.Message)
	close(ch)
	return ch, func() {}, nil
}

type journeyOutbox struct{}

func (journeyOutbox) Enqueue(_ context.Context, _ domain.Message, _ bool) error { return nil }
func (journeyOutbox) ListPending(_ context.Context, _ int) ([]ports.OutboxMessage, error) {
	return nil, nil
}
func (journeyOutbox) ClaimPending(_ context.Context, _ string, _ int, _ time.Duration) ([]ports.OutboxMessage, error) {
	return nil, nil
}
func (journeyOutbox) MarkStreamAppended(_ context.Context, _ string) error { return nil }
func (journeyOutbox) MarkDispatched(_ context.Context, _ string) error     { return nil }
func (journeyOutbox) ReleaseClaim(_ context.Context, _, _ string) error    { return nil }

type journeyLimiter struct{}

func (journeyLimiter) Allow(_ string) bool { return true }

type journeyAck struct{ v string }

func (a *journeyAck) SetLastAck(_ context.Context, _, _, _, streamID string) error {
	a.v = streamID
	return nil
}
func (a *journeyAck) GetLastAck(_ context.Context, _, _, _ string) (string, error) { return a.v, nil }

func TestMessageJourneyCoreFlow(t *testing.T) {
	room, _ := domain.NewRoom("room:demo", []string{"u1", "u2"})
	msgRepo := &journeyMessageRepo{byClient: map[string]domain.Message{}, byRoom: map[string][]domain.Message{}}
	roomRepo := &journeyRoomRepo{room: room}
	stream := &journeyStream{}
	ack := &journeyAck{}

	send := application.NewSendMessageUseCase(msgRepo, roomRepo, journeyID{}, journeyClock{}, stream, stream, journeyOutbox{}, journeyLimiter{})
	syncUC := application.NewSyncMessagesUseCase(roomRepo, stream, ack, msgRepo)
	ackUC := application.NewAckMessageUseCase(roomRepo, ack)

	_, err := send.Execute(context.Background(), application.SendMessageCommand{RoomID: "room:demo", SenderID: "u1", Content: "hello", ClientMsgID: "c1"})
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if stream.last.StreamID == "" {
		t.Fatal("expected stream append")
	}

	synced, err := syncUC.Execute(context.Background(), application.SyncMessagesQuery{RoomID: "room:demo", UserID: "u2", DeviceID: "phone2", LastAck: "0", Limit: 50})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if len(synced.Messages) != 1 {
		t.Fatalf("expected 1 synced message, got %d", len(synced.Messages))
	}

	if err := ackUC.Execute(context.Background(), application.AckMessageCommand{RoomID: "room:demo", UserID: "u2", DeviceID: "phone2", StreamID: synced.Messages[0].StreamID}); err != nil {
		t.Fatalf("ack failed: %v", err)
	}
	if ack.v == "" {
		t.Fatal("expected ack value to be saved")
	}
}
