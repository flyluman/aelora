package valkey

import (
	"context"
	"sort"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/valkey-io/valkey-go"
)

type RoomRepository struct {
	client valkey.Client
}

func NewRoomRepository(client valkey.Client) *RoomRepository {
	return &RoomRepository{client: client}
}

func (r *RoomRepository) Get(ctx context.Context, roomID string) (domain.Room, error) {
	members, err := r.members(ctx, roomID)
	if err != nil {
		return domain.Room{}, err
	}
	if len(members) == 0 {
		return domain.Room{}, domain.ErrRoomNotFound
	}
	return domain.NewRoom(roomID, members)
}

func (r *RoomRepository) Create(ctx context.Context, room domain.Room) error {
	exists, err := r.client.Do(ctx, r.client.B().Exists().Key(roomMembersKey(room.ID)).Build()).AsInt64()
	if err != nil {
		return err
	}
	if exists > 0 {
		return domain.ErrRoomAlreadyExists
	}

	members := make([]string, 0, len(room.Members))
	for member := range room.Members {
		members = append(members, member)
	}
	if len(members) == 0 {
		return domain.ErrInvalidRoom
	}

	if err := r.client.Do(ctx, r.client.B().Sadd().Key(roomMembersKey(room.ID)).Member(members...).Build()).Error(); err != nil {
		return err
	}
	return r.client.Do(ctx, r.client.B().Set().Key(roomMetaKey(room.ID)).Value(time.Now().UTC().Format(time.RFC3339Nano)).Build()).Error()
}

func (r *RoomRepository) AddMember(ctx context.Context, roomID, userID string) error {
	exists, err := r.client.Do(ctx, r.client.B().Exists().Key(roomMembersKey(roomID)).Build()).AsInt64()
	if err != nil {
		return err
	}
	if exists == 0 {
		return domain.ErrRoomNotFound
	}
	return r.client.Do(ctx, r.client.B().Sadd().Key(roomMembersKey(roomID)).Member(userID).Build()).Error()
}

func (r *RoomRepository) ListMembers(ctx context.Context, roomID string) ([]string, error) {
	members, err := r.members(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, domain.ErrRoomNotFound
	}
	sort.Strings(members)
	return members, nil
}

func (r *RoomRepository) members(ctx context.Context, roomID string) ([]string, error) {
	resp := r.client.Do(ctx, r.client.B().Smembers().Key(roomMembersKey(roomID)).Build())
	if err := resp.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	members, err := resp.AsStrSlice()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	return members, nil
}

func roomMembersKey(roomID string) string { return "room:" + roomID + ":members" }
func roomMetaKey(roomID string) string    { return "room:" + roomID + ":meta" }
