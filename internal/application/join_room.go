package application

import (
	"context"
	"errors"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type JoinRoomCommand struct {
	RoomID  string
	ActorID string
	UserID  string
}

type JoinRoomUseCase struct {
	rooms ports.RoomRepository
}

type roomRoleReader interface {
	GetMemberRole(ctx context.Context, roomID, userID string) (string, error)
}

func NewJoinRoomUseCase(rooms ports.RoomRepository) *JoinRoomUseCase {
	return &JoinRoomUseCase{rooms: rooms}
}

func (u *JoinRoomUseCase) Execute(ctx context.Context, cmd JoinRoomCommand) error {
	if cmd.RoomID == "" || cmd.ActorID == "" || cmd.UserID == "" {
		return ErrRoomAccessDenied
	}

	if cmd.ActorID == cmd.UserID {
		return ErrUserAlreadyExists
	}

	members, err := u.rooms.ListMembers(ctx, cmd.RoomID)
	if err != nil {
		return err
	}

	actorInRoom := false
	targetInRoom := false
	for _, member := range members {
		if member == cmd.ActorID {
			actorInRoom = true
		}
		if member == cmd.UserID {
			targetInRoom = true
		}
	}
	if !actorInRoom {
		return ErrRoomAccessDenied
	}
	if targetInRoom {
		return ErrUserAlreadyExists
	}

	if cmd.ActorID != cmd.UserID {
		roleRepo, ok := u.rooms.(roomRoleReader)
		if !ok {
			return ErrRoomAccessDenied
		}
		role, err := roleRepo.GetMemberRole(ctx, cmd.RoomID, cmd.ActorID)
		if err != nil {
			if errors.Is(err, domain.ErrRoomNotFound) {
				return ErrRoomAccessDenied
			}
			return err
		}
		if role != "owner" && role != "admin" {
			return ErrRoomAccessDenied
		}
	}

	return u.rooms.AddMember(ctx, cmd.RoomID, cmd.UserID)
}
