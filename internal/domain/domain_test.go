package domain

import (
	"testing"
	"time"
)

func TestNewRoomValidation(t *testing.T) {
	if _, err := NewRoom("", []string{"u1"}); err != ErrInvalidRoom {
		t.Fatalf("expected ErrInvalidRoom for empty room id, got %v", err)
	}
	if _, err := NewRoom("r1", []string{""}); err != ErrInvalidRoom {
		t.Fatalf("expected ErrInvalidRoom for empty member id, got %v", err)
	}
	room, err := NewRoom("r1", []string{"u1", "u1", "u2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !room.HasMember("u1") || !room.HasMember("u2") {
		t.Fatalf("expected room members to be set")
	}
}

func TestMessageValidate(t *testing.T) {
	msg := Message{
		ID:          "m1",
		ClientMsgID: "c1",
		RoomID:      "r1",
		SenderID:    "u1",
		Content:     "hello",
		CreatedAt:   time.Now(),
	}
	if err := msg.Validate(); err != nil {
		t.Fatalf("expected valid message, got %v", err)
	}
	msg.Content = "   "
	if err := msg.Validate(); err != ErrInvalidMessage {
		t.Fatalf("expected ErrInvalidMessage for blank content, got %v", err)
	}
}
