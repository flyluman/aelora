package valkey

import (
	"testing"
	"time"
)

func TestNewStreamRepositoryDefaults(t *testing.T) {
	r := NewStreamRepository(nil, "", 0, 0)
	if r.eventStreamKey != "stream:events:message_created" {
		t.Fatalf("unexpected event stream key: %s", r.eventStreamKey)
	}
	if r.roomMaxLen != 20000 {
		t.Fatalf("unexpected room max len: %d", r.roomMaxLen)
	}
	if r.eventMaxLen != 50000 {
		t.Fatalf("unexpected event max len: %d", r.eventMaxLen)
	}
}

func TestNewPresenceRepositoryDefaults(t *testing.T) {
	r := NewPresenceRepository(nil, 0)
	if r.ttl != 30*time.Second {
		t.Fatalf("unexpected ttl: %v", r.ttl)
	}
}

func TestNewAckRepositoryDefaults(t *testing.T) {
	r := NewAckRepository(nil)
	if r.ttl != 30*24*time.Hour {
		t.Fatalf("unexpected ttl: %v", r.ttl)
	}
}
