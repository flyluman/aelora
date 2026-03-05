package application

import (
	"context"
	"sync"
	"testing"
)

func TestRoomIDGeneratorFormat(t *testing.T) {
	gen := NewSequentialRoomIDGenerator(NewLocalRoomIDSequence(42), 0x13579)
	id, err := gen.New(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id) != 8 {
		t.Fatalf("expected 8-char id, got %d", len(id))
	}
	for _, ch := range id {
		if !((ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')) {
			t.Fatalf("id is not base62: %s", id)
		}
	}
}

type sharedSequence struct {
	mu sync.Mutex
	n  uint64
}

func (s *sharedSequence) Next(_ context.Context) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	return s.n, nil
}

func TestRoomIDGeneratorSharedSequenceAcrossInstances(t *testing.T) {
	seq := &sharedSequence{}
	genA := NewSequentialRoomIDGenerator(seq, 0x2468)
	genB := NewSequentialRoomIDGenerator(seq, 0x2468)

	seen := map[string]struct{}{}
	for range 1000 {
		idA, err := genA.New(context.Background())
		if err != nil {
			t.Fatalf("generator A failed: %v", err)
		}
		idB, err := genB.New(context.Background())
		if err != nil {
			t.Fatalf("generator B failed: %v", err)
		}
		if _, ok := seen[idA]; ok {
			t.Fatalf("duplicate id from generator A: %s", idA)
		}
		seen[idA] = struct{}{}
		if _, ok := seen[idB]; ok {
			t.Fatalf("duplicate id from generator B: %s", idB)
		}
		seen[idB] = struct{}{}
	}
}
