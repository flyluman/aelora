package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidMessage         = errors.New("invalid message")
	ErrDuplicateClientMessage = errors.New("duplicate client message")
)

type Message struct {
	ID          string    `json:"id"`
	ClientMsgID string    `json:"client_msg_id"`
	RoomID      string    `json:"room_id"`
	SenderID    string    `json:"sender_id"`
	Content     string    `json:"content"`
	ReplyToID   string    `json:"reply_to_message_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func (m Message) Validate() error {
	if m.RoomID == "" || m.SenderID == "" || m.ClientMsgID == "" {
		return ErrInvalidMessage
	}
	if strings.TrimSpace(m.Content) == "" {
		return ErrInvalidMessage
	}
	if len(m.Content) > 4000 {
		return ErrInvalidMessage
	}
	return nil
}
