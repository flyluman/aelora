package application

import (
	"context"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type RegisterPushTokenCommand struct {
	UserID   string
	DeviceID string
	Platform string
	Token    string
}

type RegisterPushTokenUseCase struct {
	tokens ports.PushTokenRepository
}

func NewRegisterPushTokenUseCase(tokens ports.PushTokenRepository) *RegisterPushTokenUseCase {
	return &RegisterPushTokenUseCase{tokens: tokens}
}

func (u *RegisterPushTokenUseCase) Execute(ctx context.Context, cmd RegisterPushTokenCommand) error {
	if cmd.UserID == "" || cmd.DeviceID == "" || cmd.Token == "" {
		return ErrInvalidPushToken
	}
	if cmd.Platform != ports.PushPlatformAndroid && cmd.Platform != ports.PushPlatformIOS {
		return ErrInvalidPushToken
	}
	return u.tokens.Upsert(ctx, ports.PushToken{
		UserID:   cmd.UserID,
		DeviceID: cmd.DeviceID,
		Platform: cmd.Platform,
		Token:    cmd.Token,
	})
}

type DeletePushTokenCommand struct {
	UserID   string
	DeviceID string
}

type DeletePushTokenUseCase struct {
	tokens ports.PushTokenRepository
}

func NewDeletePushTokenUseCase(tokens ports.PushTokenRepository) *DeletePushTokenUseCase {
	return &DeletePushTokenUseCase{tokens: tokens}
}

func (u *DeletePushTokenUseCase) Execute(ctx context.Context, cmd DeletePushTokenCommand) error {
	if cmd.UserID == "" || cmd.DeviceID == "" {
		return ErrInvalidPushToken
	}
	return u.tokens.Delete(ctx, cmd.UserID, cmd.DeviceID)
}

type NotifyOfflineMembersUseCase struct {
	presence ports.PresenceRepository
	tokens   ports.PushTokenRepository
	notifier ports.PushNotifier
}

func NewNotifyOfflineMembersUseCase(
	presence ports.PresenceRepository,
	tokens ports.PushTokenRepository,
	notifier ports.PushNotifier,
) *NotifyOfflineMembersUseCase {
	return &NotifyOfflineMembersUseCase{
		presence: presence,
		tokens:   tokens,
		notifier: notifier,
	}
}

func (u *NotifyOfflineMembersUseCase) Execute(ctx context.Context, msg domain.Message, userID string) error {
	if userID == "" || userID == msg.SenderID {
		return nil
	}
	online, err := u.presence.IsOnline(ctx, userID)
	if err != nil {
		return err
	}
	if online {
		return nil
	}
	targets, err := u.tokens.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, target := range targets {
		_ = u.notifier.NotifyMessage(ctx, target, msg)
	}
	return nil
}
