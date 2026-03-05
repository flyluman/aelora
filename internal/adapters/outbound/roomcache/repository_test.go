package roomcache

import (
	"context"
	"errors"
	"testing"

	"github.com/flyluman/aelora/internal/domain"
)

type fakeRoomRepo struct {
	room       domain.Room
	getErr     error
	createErr  error
	addErr     error
	membersErr error
	members    []string
}

func (r *fakeRoomRepo) Get(_ context.Context, _ string) (domain.Room, error) {
	if r.getErr != nil {
		return domain.Room{}, r.getErr
	}
	return r.room, nil
}
func (r *fakeRoomRepo) Create(_ context.Context, room domain.Room) error {
	r.room = room
	return r.createErr
}
func (r *fakeRoomRepo) AddMember(_ context.Context, _, userID string) error {
	if r.addErr != nil {
		return r.addErr
	}
	r.members = append(r.members, userID)
	return nil
}
func (r *fakeRoomRepo) ListMembers(_ context.Context, _ string) ([]string, error) {
	if r.membersErr != nil {
		return nil, r.membersErr
	}
	if len(r.members) > 0 {
		return r.members, nil
	}
	return r.members, nil
}

func (r *fakeRoomRepo) GetMemberRole(_ context.Context, _, userID string) (string, error) {
	if userID == "owner" {
		return "owner", nil
	}
	return "", domain.ErrRoomNotFound
}

func (r *fakeRoomRepo) SetMemberRole(_ context.Context, _, _, _ string) error { return nil }

func TestGetFallsBackToPrimaryAndWarmsCache(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"u1", "u2"})
	primary := &fakeRoomRepo{room: room}
	cache := &fakeRoomRepo{getErr: domain.ErrRoomNotFound}
	repo := NewRepository(primary, cache)

	got, err := repo.Get(context.Background(), "r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "r1" {
		t.Fatalf("expected room r1, got %s", got.ID)
	}
}

func TestAddMemberReturnsPrimaryError(t *testing.T) {
	primary := &fakeRoomRepo{addErr: errors.New("boom")}
	cache := &fakeRoomRepo{}
	repo := NewRepository(primary, cache)
	if err := repo.AddMember(context.Background(), "r1", "u3"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRoleDelegation(t *testing.T) {
	room, _ := domain.NewRoom("r1", []string{"owner"})
	primary := &fakeRoomRepo{room: room}
	cache := &fakeRoomRepo{}
	repo := NewRepository(primary, cache)
	role, err := repo.GetMemberRole(context.Background(), "r1", "owner")
	if err != nil || role != "owner" {
		t.Fatalf("expected owner role, got role=%s err=%v", role, err)
	}
	if err := repo.SetMemberRole(context.Background(), "r1", "owner", "admin"); err != nil {
		t.Fatalf("unexpected set role error: %v", err)
	}
}
