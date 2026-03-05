package postgres

import (
	"testing"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDedupKeyDeterministic(t *testing.T) {
	a := dedupKey("u1", "c1")
	b := dedupKey("u1", "c1")
	c := dedupKey("u1", "c2")
	if a != b {
		t.Fatal("expected stable dedup key")
	}
	if a == c {
		t.Fatal("expected different dedup key for different client msg id")
	}
}

func TestNullable(t *testing.T) {
	if got := nullable(""); got != nil {
		t.Fatal("expected nil for empty string")
	}
	if got := nullable("x"); got == nil {
		t.Fatal("expected non-nil for non-empty string")
	}
}

func TestDecodeOutboxPayloadLegacyAndCurrent(t *testing.T) {
	legacy := []byte(`{"id":"m1","client_msg_id":"c1","room_id":"r1","sender_id":"u1","content":"x","created_at":"2026-01-01T00:00:00Z"}`)
	out, err := decodeOutboxPayload(legacy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Message.ID != "m1" || !out.StreamAppended {
		t.Fatalf("unexpected legacy decode: %+v", out)
	}

	current := []byte(`{"message":{"id":"m2","client_msg_id":"c2","room_id":"r1","sender_id":"u1","content":"x","created_at":"2026-01-01T00:00:00Z"},"stream_appended":false}`)
	out, err = decodeOutboxPayload(current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Message.ID != "m2" || out.StreamAppended {
		t.Fatalf("unexpected current decode: %+v", out)
	}
}

func TestIsPGCode(t *testing.T) {
	if !isPGCode(&pgconn.PgError{Code: "23505"}, "23505") {
		t.Fatal("expected pg code match")
	}
	if isPGCode(domain.ErrInvalidRoom, "23505") {
		t.Fatal("did not expect match on non-pg error")
	}
}
