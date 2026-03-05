package valkey

import (
	"context"
	"errors"

	"github.com/valkey-io/valkey-go"
)

const defaultRoomIDSequenceKey = "seq:room_id"

type RoomIDSequence struct {
	client valkey.Client
	key    string
}

func NewRoomIDSequence(client valkey.Client, key string) *RoomIDSequence {
	if key == "" {
		key = defaultRoomIDSequenceKey
	}
	return &RoomIDSequence{client: client, key: key}
}

func (s *RoomIDSequence) Next(ctx context.Context) (uint64, error) {
	if s.client == nil {
		return 0, errors.New("valkey client is nil")
	}
	n, err := s.client.Do(ctx, s.client.B().Incr().Key(s.key).Build()).AsInt64()
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, errors.New("room id sequence returned non-positive value")
	}
	return uint64(n), nil
}
