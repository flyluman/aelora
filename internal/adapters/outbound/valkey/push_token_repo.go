package valkey

import (
	"context"
	"errors"
	"strings"

	"github.com/flyluman/aelora/internal/ports"
	"github.com/valkey-io/valkey-go"
)

type PushTokenRepository struct {
	client valkey.Client
}

func NewPushTokenRepository(client valkey.Client) *PushTokenRepository {
	return &PushTokenRepository{client: client}
}

func (r *PushTokenRepository) Upsert(ctx context.Context, token ports.PushToken) error {
	value := token.Platform + "|" + token.Token
	if err := r.client.Do(ctx, r.client.B().Hset().Key(pushTokenKey(token.UserID)).FieldValue().FieldValue(token.DeviceID, value).Build()).Error(); err != nil {
		return err
	}
	return r.client.Do(ctx, r.client.B().Expire().Key(pushTokenKey(token.UserID)).Seconds(90*24*60*60).Build()).Error()
}

func (r *PushTokenRepository) Delete(ctx context.Context, userID, deviceID string) error {
	return r.client.Do(ctx, r.client.B().Hdel().Key(pushTokenKey(userID)).Field(deviceID).Build()).Error()
}

func (r *PushTokenRepository) ListByUser(ctx context.Context, userID string) ([]ports.PushToken, error) {
	resp := r.client.Do(ctx, r.client.B().Hgetall().Key(pushTokenKey(userID)).Build())
	if err := resp.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	m, err := resp.AsStrMap()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]ports.PushToken, 0, len(m))
	for deviceID, raw := range m {
		platform, token, err := splitToken(raw)
		if err != nil {
			continue
		}
		out = append(out, ports.PushToken{
			UserID:   userID,
			DeviceID: deviceID,
			Platform: platform,
			Token:    token,
		})
	}
	return out, nil
}

func splitToken(raw string) (string, string, error) {
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid push token value")
	}
	return parts[0], parts[1], nil
}

func pushTokenKey(userID string) string {
	return "push_tokens:" + userID
}
