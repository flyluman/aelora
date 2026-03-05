package ports

import (
	"context"

	"github.com/flyluman/aelora/internal/domain"
)

const (
	PushPlatformAndroid = "android"
	PushPlatformIOS     = "ios"
)

type PushToken struct {
	UserID   string
	DeviceID string
	Platform string
	Token    string
}

type PushTokenRepository interface {
	Upsert(ctx context.Context, token PushToken) error
	Delete(ctx context.Context, userID, deviceID string) error
	ListByUser(ctx context.Context, userID string) ([]PushToken, error)
}

type PushNotifier interface {
	NotifyMessage(ctx context.Context, token PushToken, message domain.Message) error
}
