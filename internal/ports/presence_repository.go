package ports

import "context"

type PresenceRepository interface {
	SetOnline(ctx context.Context, userID, deviceID string) error
	SetOffline(ctx context.Context, userID, deviceID string) error
	IsOnline(ctx context.Context, userID string) (bool, error)
}
