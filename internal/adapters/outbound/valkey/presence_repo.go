package valkey

import (
	"context"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
)

type PresenceRepository struct {
	client valkey.Client
	ttl    time.Duration
}

func NewPresenceRepository(client valkey.Client, ttl time.Duration) *PresenceRepository {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &PresenceRepository{client: client, ttl: ttl}
}

func (r *PresenceRepository) SetOnline(ctx context.Context, userID, deviceID string) error {
	if err := r.client.Do(ctx, r.client.B().Set().Key(presenceKey(userID, deviceID)).Value(strconv.FormatInt(time.Now().Unix(), 10)).ExSeconds(int64(r.ttl.Seconds())).Build()).Error(); err != nil {
		return err
	}
	if err := r.client.Do(ctx, r.client.B().Sadd().Key(presenceDevicesKey(userID)).Member(deviceID).Build()).Error(); err != nil {
		return err
	}
	return r.client.Do(ctx, r.client.B().Expire().Key(presenceDevicesKey(userID)).Seconds(int64((2 * r.ttl).Seconds())).Build()).Error()
}

func (r *PresenceRepository) SetOffline(ctx context.Context, userID, deviceID string) error {
	if err := r.client.Do(ctx, r.client.B().Del().Key(presenceKey(userID, deviceID)).Build()).Error(); err != nil {
		return err
	}
	_ = r.client.Do(ctx, r.client.B().Srem().Key(presenceDevicesKey(userID)).Member(deviceID).Build()).Error()
	return nil
}

func (r *PresenceRepository) IsOnline(ctx context.Context, userID string) (bool, error) {
	resp := r.client.Do(ctx, r.client.B().Smembers().Key(presenceDevicesKey(userID)).Build())
	if err := resp.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return false, nil
		}
		return false, err
	}
	devices, err := resp.AsStrSlice()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return false, nil
		}
		return false, err
	}
	if len(devices) == 0 {
		return false, nil
	}

	online := false
	for _, deviceID := range devices {
		exists, err := r.client.Do(ctx, r.client.B().Exists().Key(presenceKey(userID, deviceID)).Build()).AsInt64()
		if err != nil {
			return false, err
		}
		if exists > 0 {
			online = true
			continue
		}
		_ = r.client.Do(ctx, r.client.B().Srem().Key(presenceDevicesKey(userID)).Member(deviceID).Build()).Error()
	}
	return online, nil
}

func presenceKey(userID, deviceID string) string {
	return "presence:" + userID + ":" + deviceID
}

func presenceDevicesKey(userID string) string {
	return "presence_devices:" + userID
}
