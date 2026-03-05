package valkey

import (
	"context"
	"testing"
)

func TestNewRoomIDSequenceDefaults(t *testing.T) {
	seq := NewRoomIDSequence(nil, "")
	if seq.key != defaultRoomIDSequenceKey {
		t.Fatalf("expected key %q, got %q", defaultRoomIDSequenceKey, seq.key)
	}
}

func TestRoomIDSequenceNilClient(t *testing.T) {
	seq := NewRoomIDSequence(nil, "")
	_, err := seq.Next(context.Background())
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}
