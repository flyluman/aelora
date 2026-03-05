package valkey

import (
	"context"
	"time"

	"github.com/valkey-io/valkey-go"
)

type AckRepository struct {
	client valkey.Client
	ttl    time.Duration
}

func NewAckRepository(client valkey.Client) *AckRepository {
	return &AckRepository{client: client, ttl: 30 * 24 * time.Hour}
}

func (r *AckRepository) SetLastAck(ctx context.Context, userID, deviceID, roomID, streamID string) error {
	return r.client.Do(ctx, r.client.B().Set().Key(ackKey(userID, deviceID, roomID)).Value(streamID).ExSeconds(int64(r.ttl.Seconds())).Build()).Error()
}

func (r *AckRepository) GetLastAck(ctx context.Context, userID, deviceID, roomID string) (string, error) {
	resp := r.client.Do(ctx, r.client.B().Get().Key(ackKey(userID, deviceID, roomID)).Build())
	if err := resp.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return "", nil
		}
		return "", err
	}
	return resp.ToString()
}

func ackKey(userID, deviceID, roomID string) string {
	return "last_ack:" + userID + ":" + deviceID + ":" + roomID
}
