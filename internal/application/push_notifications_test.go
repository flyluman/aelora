package application

import (
	"context"
	"testing"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type pushPresenceRepo struct {
	online map[string]bool
}

func (p *pushPresenceRepo) SetOnline(_ context.Context, _, _ string) error  { return nil }
func (p *pushPresenceRepo) SetOffline(_ context.Context, _, _ string) error { return nil }
func (p *pushPresenceRepo) IsOnline(_ context.Context, userID string) (bool, error) {
	return p.online[userID], nil
}

type pushTokenRepo struct {
	tokens map[string][]ports.PushToken
}

func (r *pushTokenRepo) Upsert(_ context.Context, token ports.PushToken) error {
	r.tokens[token.UserID] = append(r.tokens[token.UserID], token)
	return nil
}
func (r *pushTokenRepo) Delete(_ context.Context, userID, deviceID string) error {
	existing := r.tokens[userID]
	filtered := existing[:0]
	for _, token := range existing {
		if token.DeviceID != deviceID {
			filtered = append(filtered, token)
		}
	}
	r.tokens[userID] = filtered
	return nil
}
func (r *pushTokenRepo) ListByUser(_ context.Context, userID string) ([]ports.PushToken, error) {
	return r.tokens[userID], nil
}

type captureNotifier struct {
	count int
}

func (n *captureNotifier) NotifyMessage(_ context.Context, _ ports.PushToken, _ domain.Message) error {
	n.count++
	return nil
}

func TestRegisterPushTokenValidatesPlatform(t *testing.T) {
	uc := NewRegisterPushTokenUseCase(&pushTokenRepo{tokens: map[string][]ports.PushToken{}})
	err := uc.Execute(context.Background(), RegisterPushTokenCommand{
		UserID: "u1", DeviceID: "d1", Platform: "web", Token: "abc",
	})
	if err != ErrInvalidPushToken {
		t.Fatalf("expected ErrInvalidPushToken, got %v", err)
	}
}

func TestNotifyOfflineMembersSendsPushOnlyWhenOffline(t *testing.T) {
	tokens := &pushTokenRepo{
		tokens: map[string][]ports.PushToken{
			"u2": {
				{UserID: "u2", DeviceID: "android-1", Platform: ports.PushPlatformAndroid, Token: "tok-a"},
				{UserID: "u2", DeviceID: "ios-1", Platform: ports.PushPlatformIOS, Token: "tok-i"},
			},
		},
	}
	notifier := &captureNotifier{}
	uc := NewNotifyOfflineMembersUseCase(
		&pushPresenceRepo{online: map[string]bool{"u2": false}},
		tokens,
		notifier,
	)

	err := uc.Execute(context.Background(), domain.Message{ID: "m1", RoomID: "r1", SenderID: "u1", Content: "hello"}, "u2")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if notifier.count != 2 {
		t.Fatalf("expected 2 push sends, got %d", notifier.count)
	}
}
