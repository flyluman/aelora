package application

import (
	"context"
	"testing"
	"time"

	"github.com/flyluman/aelora/internal/domain"
)

type listRoomRepo struct{ room domain.Room }

func (r *listRoomRepo) Get(_ context.Context, _ string) (domain.Room, error) { return r.room, nil }
func (r *listRoomRepo) Create(_ context.Context, _ domain.Room) error        { return nil }
func (r *listRoomRepo) AddMember(_ context.Context, _, _ string) error       { return nil }
func (r *listRoomRepo) ListMembers(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

type listMsgRepo struct {
	out []domain.Message
}

func (r *listMsgRepo) Save(_ context.Context, _ domain.Message) error { return nil }
func (r *listMsgRepo) ExistsByClientMsgID(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (r *listMsgRepo) ListByRoom(_ context.Context, _ string, _ time.Time, _ int) ([]domain.Message, error) {
	return r.out, nil
}
func (r *listMsgRepo) GetByIDs(_ context.Context, _ string, _ []string) (map[string]domain.Message, error) {
	return nil, nil
}

func TestListMessagesRequiresMembership(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1"})
	uc := NewListMessagesUseCase(&listRoomRepo{room: room}, &listMsgRepo{})
	_, err := uc.Execute(context.Background(), ListMessagesQuery{RoomID: "r1", UserID: "u2"})
	if err != ErrRoomAccessDenied {
		t.Fatalf("expected access denied, got %v", err)
	}
}
