package roomcache

import (
	"context"
	"errors"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type Repository struct {
	primary ports.RoomRepository
	cache   ports.RoomRepository
}

type roleReader interface {
	GetMemberRole(ctx context.Context, roomID, userID string) (string, error)
}

type roleWriter interface {
	SetMemberRole(ctx context.Context, roomID, userID, role string) error
}

func NewRepository(primary, cache ports.RoomRepository) *Repository {
	return &Repository{primary: primary, cache: cache}
}

func (r *Repository) Get(ctx context.Context, roomID string) (domain.Room, error) {
	room, err := r.cache.Get(ctx, roomID)
	if err == nil {
		return room, nil
	}

	room, err = r.primary.Get(ctx, roomID)
	if err != nil {
		return domain.Room{}, err
	}
	r.warmCache(ctx, room)
	return room, nil
}

func (r *Repository) Create(ctx context.Context, room domain.Room) error {
	if err := r.primary.Create(ctx, room); err != nil {
		return err
	}
	r.warmCache(ctx, room)
	return nil
}

func (r *Repository) AddMember(ctx context.Context, roomID, userID string) error {
	if err := r.primary.AddMember(ctx, roomID, userID); err != nil {
		return err
	}
	if err := r.cache.AddMember(ctx, roomID, userID); err != nil {
		if errors.Is(err, domain.ErrRoomNotFound) {
			room, primaryErr := r.primary.Get(ctx, roomID)
			if primaryErr == nil {
				r.warmCache(ctx, room)
			}
		}
	}
	return nil
}

func (r *Repository) ListMembers(ctx context.Context, roomID string) ([]string, error) {
	members, err := r.cache.ListMembers(ctx, roomID)
	if err == nil {
		return members, nil
	}

	members, err = r.primary.ListMembers(ctx, roomID)
	if err != nil {
		return nil, err
	}
	room, roomErr := domain.NewRoom(roomID, members)
	if roomErr == nil {
		r.warmCache(ctx, room)
	}
	return members, nil
}

func (r *Repository) warmCache(ctx context.Context, room domain.Room) {
	if err := r.cache.Create(ctx, room); err != nil && !errors.Is(err, domain.ErrRoomAlreadyExists) {
		return
	}
	for member := range room.Members {
		_ = r.cache.AddMember(ctx, room.ID, member)
	}
}

func (r *Repository) GetMemberRole(ctx context.Context, roomID, userID string) (string, error) {
	roleRepo, ok := r.primary.(roleReader)
	if !ok {
		return "", domain.ErrRoomNotFound
	}
	return roleRepo.GetMemberRole(ctx, roomID, userID)
}

func (r *Repository) SetMemberRole(ctx context.Context, roomID, userID, role string) error {
	roleRepo, ok := r.primary.(roleWriter)
	if !ok {
		return domain.ErrRoomNotFound
	}
	return roleRepo.SetMemberRole(ctx, roomID, userID, role)
}
