package valkey

import (
	"context"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
	"github.com/sony/gobreaker"
)

type circuitBreakerRoomRepository struct {
	inner *RoomRepository
	cb    *gobreaker.CircuitBreaker
}

func newCircuitBreakerRoomRepository(inner *RoomRepository) *circuitBreakerRoomRepository {
	return &circuitBreakerRoomRepository{
		inner: inner,
		cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "valkey.room_repository",
		}),
	}
}

func (r *circuitBreakerRoomRepository) Get(ctx context.Context, roomID string) (domain.Room, error) {
	res, err := r.cb.Execute(func() (any, error) {
		return r.inner.Get(ctx, roomID)
	})
	if err != nil {
		return domain.Room{}, err
	}
	return res.(domain.Room), nil
}

func (r *circuitBreakerRoomRepository) Create(ctx context.Context, room domain.Room) error {
	_, err := r.cb.Execute(func() (any, error) {
		return nil, r.inner.Create(ctx, room)
	})
	return err
}

func (r *circuitBreakerRoomRepository) AddMember(ctx context.Context, roomID, userID string) error {
	_, err := r.cb.Execute(func() (any, error) {
		return nil, r.inner.AddMember(ctx, roomID, userID)
	})
	return err
}

func (r *circuitBreakerRoomRepository) ListMembers(ctx context.Context, roomID string) ([]string, error) {
	res, err := r.cb.Execute(func() (any, error) {
		return r.inner.ListMembers(ctx, roomID)
	})
	if err != nil {
		return nil, err
	}
	return res.([]string), nil
}

type circuitBreakerAckRepository struct {
	inner *AckRepository
	cb    *gobreaker.CircuitBreaker
}

func newCircuitBreakerAckRepository(inner *AckRepository) *circuitBreakerAckRepository {
	return &circuitBreakerAckRepository{
		inner: inner,
		cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "valkey.ack_repository",
		}),
	}
}

func (r *circuitBreakerAckRepository) SetLastAck(ctx context.Context, userID, deviceID, roomID, streamID string) error {
	_, err := r.cb.Execute(func() (any, error) {
		return nil, r.inner.SetLastAck(ctx, userID, deviceID, roomID, streamID)
	})
	return err
}

func (r *circuitBreakerAckRepository) GetLastAck(ctx context.Context, userID, deviceID, roomID string) (string, error) {
	res, err := r.cb.Execute(func() (any, error) {
		return r.inner.GetLastAck(ctx, userID, deviceID, roomID)
	})
	if err != nil {
		return "", err
	}
	return res.(string), nil
}

type circuitBreakerPresenceRepository struct {
	inner *PresenceRepository
	cb    *gobreaker.CircuitBreaker
}

func newCircuitBreakerPresenceRepository(inner *PresenceRepository) *circuitBreakerPresenceRepository {
	return &circuitBreakerPresenceRepository{
		inner: inner,
		cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "valkey.presence_repository",
		}),
	}
}

func (r *circuitBreakerPresenceRepository) SetOnline(ctx context.Context, userID, deviceID string) error {
	_, err := r.cb.Execute(func() (any, error) {
		return nil, r.inner.SetOnline(ctx, userID, deviceID)
	})
	return err
}

func (r *circuitBreakerPresenceRepository) SetOffline(ctx context.Context, userID, deviceID string) error {
	_, err := r.cb.Execute(func() (any, error) {
		return nil, r.inner.SetOffline(ctx, userID, deviceID)
	})
	return err
}

func (r *circuitBreakerPresenceRepository) IsOnline(ctx context.Context, userID string) (bool, error) {
	res, err := r.cb.Execute(func() (any, error) {
		return r.inner.IsOnline(ctx, userID)
	})
	if err != nil {
		return false, err
	}
	return res.(bool), nil
}

type circuitBreakerPushTokenRepository struct {
	inner *PushTokenRepository
	cb    *gobreaker.CircuitBreaker
}

func newCircuitBreakerPushTokenRepository(inner *PushTokenRepository) *circuitBreakerPushTokenRepository {
	return &circuitBreakerPushTokenRepository{
		inner: inner,
		cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "valkey.push_token_repository",
		}),
	}
}

func (r *circuitBreakerPushTokenRepository) Upsert(ctx context.Context, token ports.PushToken) error {
	_, err := r.cb.Execute(func() (any, error) {
		return nil, r.inner.Upsert(ctx, token)
	})
	return err
}

func (r *circuitBreakerPushTokenRepository) Delete(ctx context.Context, userID, deviceID string) error {
	_, err := r.cb.Execute(func() (any, error) {
		return nil, r.inner.Delete(ctx, userID, deviceID)
	})
	return err
}

func (r *circuitBreakerPushTokenRepository) ListByUser(ctx context.Context, userID string) ([]ports.PushToken, error) {
	res, err := r.cb.Execute(func() (any, error) {
		return r.inner.ListByUser(ctx, userID)
	})
	if err != nil {
		return nil, err
	}
	return res.([]ports.PushToken), nil
}

type circuitBreakerStreamRepository struct {
	inner *StreamRepository
	cb    *gobreaker.CircuitBreaker
}

func newCircuitBreakerStreamRepository(inner *StreamRepository) *circuitBreakerStreamRepository {
	return &circuitBreakerStreamRepository{
		inner: inner,
		cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "valkey.stream_repository",
		}),
	}
}

func (r *circuitBreakerStreamRepository) AppendMessage(ctx context.Context, roomID string, msg domain.Message) (string, error) {
	res, err := r.cb.Execute(func() (any, error) {
		return r.inner.AppendMessage(ctx, roomID, msg)
	})
	if err != nil {
		return "", err
	}
	return res.(string), nil
}

func (r *circuitBreakerStreamRepository) ReadFrom(ctx context.Context, roomID, lastAck string, limit int) ([]ports.StreamMessage, error) {
	res, err := r.cb.Execute(func() (any, error) {
		return r.inner.ReadFrom(ctx, roomID, lastAck, limit)
	})
	if err != nil {
		return nil, err
	}
	return res.([]ports.StreamMessage), nil
}

func (r *circuitBreakerStreamRepository) PublishMessageCreated(ctx context.Context, msg domain.Message) error {
	_, err := r.cb.Execute(func() (any, error) {
		return nil, r.inner.PublishMessageCreated(ctx, msg)
	})
	return err
}

func (r *circuitBreakerStreamRepository) SubscribeMessageCreated(ctx context.Context, buffer int) (<-chan domain.Message, func(), error) {
	return r.inner.SubscribeMessageCreated(ctx, buffer)
}

type circuitBreakerRoomIDSequence struct {
	inner *RoomIDSequence
	cb    *gobreaker.CircuitBreaker
}

func newCircuitBreakerRoomIDSequence(inner *RoomIDSequence) *circuitBreakerRoomIDSequence {
	return &circuitBreakerRoomIDSequence{
		inner: inner,
		cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "valkey.room_id_sequence",
		}),
	}
}

func (s *circuitBreakerRoomIDSequence) Next(ctx context.Context) (uint64, error) {
	res, err := s.cb.Execute(func() (any, error) {
		return s.inner.Next(ctx)
	})
	if err != nil {
		return 0, err
	}
	return res.(uint64), nil
}
