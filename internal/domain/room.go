package domain

import "errors"

var ErrInvalidRoom = errors.New("invalid room")
var ErrRoomNotFound = errors.New("room not found")
var ErrRoomAlreadyExists = errors.New("room already exists")

type Room struct {
	ID      string              `json:"id"`
	Members map[string]struct{} `json:"members"`
}

func NewRoom(id string, members []string) (Room, error) {
	if id == "" {
		return Room{}, ErrInvalidRoom
	}
	set := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member == "" {
			return Room{}, ErrInvalidRoom
		}
		set[member] = struct{}{}
	}
	if len(set) == 0 {
		return Room{}, ErrInvalidRoom
	}
	return Room{ID: id, Members: set}, nil
}

func (r Room) HasMember(userID string) bool {
	_, ok := r.Members[userID]
	return ok
}
