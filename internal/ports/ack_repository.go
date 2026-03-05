package ports

import "context"

type AckRepository interface {
	SetLastAck(ctx context.Context, userID, deviceID, roomID, streamID string) error
	GetLastAck(ctx context.Context, userID, deviceID, roomID string) (string, error)
}
