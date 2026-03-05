package application

import (
	"context"
	"errors"
	"testing"

	"github.com/flyluman/aelora/internal/domain"
)

type roleRoomRepo struct {
	room  domain.Room
	roles map[string]string
}

type stubRoomIDGenerator struct {
	id  string
	err error
}

func (g stubRoomIDGenerator) New(context.Context) (string, error) {
	if g.err != nil {
		return "", g.err
	}
	return g.id, nil
}

func (r *roleRoomRepo) Get(_ context.Context, _ string) (domain.Room, error) { return r.room, nil }
func (r *roleRoomRepo) Create(_ context.Context, room domain.Room) error {
	r.room = room
	return nil
}
func (r *roleRoomRepo) AddMember(_ context.Context, _ string, userID string) error {
	r.room.Members[userID] = struct{}{}
	return nil
}
func (r *roleRoomRepo) ListMembers(_ context.Context, _ string) ([]string, error) {
	out := make([]string, 0, len(r.room.Members))
	for id := range r.room.Members {
		out = append(out, id)
	}
	return out, nil
}
func (r *roleRoomRepo) GetMemberRole(_ context.Context, _ string, userID string) (string, error) {
	if role, ok := r.roles[userID]; ok {
		return role, nil
	}
	return "", domain.ErrRoomNotFound
}
func (r *roleRoomRepo) SetMemberRole(_ context.Context, _ string, userID, role string) error {
	if r.roles == nil {
		r.roles = map[string]string{}
	}
	r.roles[userID] = role
	return nil
}

func TestCreateRoomAssignsOwner(t *testing.T) {
	repo := &roleRoomRepo{}
	uc := NewCreateRoomUseCase(repo, stubRoomIDGenerator{id: "ab12CD34"})
	room, err := uc.Execute(context.Background(), CreateRoomCommand{
		Creator:   "u1",
		MemberIDs: []string{"u2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if room.ID != "ab12CD34" || !room.HasMember("u1") {
		t.Fatalf("unexpected room payload")
	}
}

func TestCreateRoomReturnsGeneratorError(t *testing.T) {
	repo := &roleRoomRepo{}
	uc := NewCreateRoomUseCase(repo, stubRoomIDGenerator{err: errors.New("id sequence down")})
	if _, err := uc.Execute(context.Background(), CreateRoomCommand{
		Creator:   "u1",
		MemberIDs: []string{"u2"},
	}); err == nil {
		t.Fatal("expected generator error")
	}
}

func TestJoinRoomRoleGate(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1"})
	repo := &roleRoomRepo{
		room:  room,
		roles: map[string]string{"u1": "member"},
	}
	uc := NewJoinRoomUseCase(repo)
	if err := uc.Execute(context.Background(), JoinRoomCommand{
		RoomID:  "r1",
		ActorID: "u1",
		UserID:  "u2",
	}); err != ErrRoomAccessDenied {
		t.Fatalf("expected access denied, got %v", err)
	}
}
