package application

import (
	"context"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type CreateRoomCommand struct {
	Creator   string
	MemberIDs []string
}

type CreateRoomUseCase struct {
	rooms ports.RoomRepository
	ids   ports.RoomIDGenerator
}

type roomRoleWriter interface {
	SetMemberRole(ctx context.Context, roomID, userID, role string) error
}

func NewCreateRoomUseCase(rooms ports.RoomRepository, ids ports.RoomIDGenerator) *CreateRoomUseCase {
	if ids == nil {
		ids = NewSequentialRoomIDGenerator(nil, 0)
	}
	return &CreateRoomUseCase{rooms: rooms, ids: ids}
}

func (u *CreateRoomUseCase) Execute(ctx context.Context, cmd CreateRoomCommand) (domain.Room, error) {
	roomID, err := u.ids.New(ctx)
	if err != nil {
		return domain.Room{}, err
	}

	members := make([]string, 0, len(cmd.MemberIDs)+1)
	members = append(members, cmd.Creator)
	members = append(members, cmd.MemberIDs...)

	room, err := domain.NewRoom(roomID, members)
	if err != nil {
		return domain.Room{}, err
	}
	if err := u.rooms.Create(ctx, room); err != nil {
		return domain.Room{}, err
	}
	if roleRepo, ok := u.rooms.(roomRoleWriter); ok {
		if err := roleRepo.SetMemberRole(ctx, room.ID, cmd.Creator, "owner"); err != nil {
			return domain.Room{}, err
		}
	}
	return room, nil
}
