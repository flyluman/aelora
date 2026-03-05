package workers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
	"go.uber.org/zap"
)

type relayOutbox struct {
	mu       sync.Mutex
	items    []ports.OutboxMessage
	marked   []string
	streamed []string
	released []string
}

func (o *relayOutbox) Enqueue(_ context.Context, _ domain.Message, _ bool) error { return nil }
func (o *relayOutbox) ListPending(_ context.Context, _ int) ([]ports.OutboxMessage, error) {
	return nil, nil
}
func (o *relayOutbox) ClaimPending(_ context.Context, _ string, _ int, _ time.Duration) ([]ports.OutboxMessage, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.items
	o.items = nil
	return out, nil
}
func (o *relayOutbox) MarkStreamAppended(_ context.Context, messageID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.streamed = append(o.streamed, messageID)
	return nil
}
func (o *relayOutbox) MarkDispatched(_ context.Context, messageID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.marked = append(o.marked, messageID)
	return nil
}
func (o *relayOutbox) ReleaseClaim(_ context.Context, _, messageID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.released = append(o.released, messageID)
	return nil
}

type relayStream struct{}

func (relayStream) AppendMessage(_ context.Context, _ string, _ domain.Message) (string, error) {
	return "1-0", nil
}
func (relayStream) ReadFrom(_ context.Context, _, _ string, _ int) ([]ports.StreamMessage, error) {
	return nil, nil
}

type relayBus struct{}

func (relayBus) PublishMessageCreated(_ context.Context, _ domain.Message) error { return nil }
func (relayBus) SubscribeMessageCreated(_ context.Context, _ int) (<-chan domain.Message, func(), error) {
	ch := make(chan domain.Message)
	close(ch)
	return ch, func() {}, nil
}

func TestOutboxRelayProcessesPendingItem(t *testing.T) {
	msg := domain.Message{ID: "m1", RoomID: "r1", SenderID: "u1", Content: "x", ClientMsgID: "c1", CreatedAt: time.Now().UTC()}
	outbox := &relayOutbox{items: []ports.OutboxMessage{{Message: msg, StreamAppended: false}}}
	relay := NewMessageOutboxRelay(outbox, relayStream{}, relayBus{}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	relay.Run(ctx, 10*time.Millisecond, 10)

	if len(outbox.streamed) != 1 || outbox.streamed[0] != "m1" {
		t.Fatalf("expected stream appended mark for m1, got %+v", outbox.streamed)
	}
	if len(outbox.marked) != 1 || outbox.marked[0] != "m1" {
		t.Fatalf("expected dispatched mark for m1, got %+v", outbox.marked)
	}
}

type failingRelayBus struct{}

func (failingRelayBus) PublishMessageCreated(_ context.Context, _ domain.Message) error {
	return errors.New("publish failed")
}
func (failingRelayBus) SubscribeMessageCreated(_ context.Context, _ int) (<-chan domain.Message, func(), error) {
	ch := make(chan domain.Message)
	close(ch)
	return ch, func() {}, nil
}

func TestOutboxRelayReleasesClaimOnPublishFailure(t *testing.T) {
	msg := domain.Message{ID: "m2", RoomID: "r1", SenderID: "u1", Content: "x", ClientMsgID: "c2", CreatedAt: time.Now().UTC()}
	outbox := &relayOutbox{items: []ports.OutboxMessage{{Message: msg, StreamAppended: true}}}
	relay := NewMessageOutboxRelay(outbox, relayStream{}, failingRelayBus{}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	relay.Run(ctx, 10*time.Millisecond, 10)

	if len(outbox.marked) != 0 {
		t.Fatalf("expected no dispatched mark when publish fails")
	}
	if len(outbox.released) == 0 || outbox.released[0] != "m2" {
		t.Fatalf("expected claim release for m2, got %+v", outbox.released)
	}
}
