package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/platform/migrations"
)

func TestMessageRepositoryReplyRoundTrip_Integration(t *testing.T) {
	dsn := os.Getenv("AELORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set AELORA_TEST_POSTGRES_DSN to run integration tests")
	}
	ctx := context.Background()
	if err := migrations.Run(ctx, migrations.Config{
		Enabled:               true,
		PostgresDSN:           dsn,
		PostgresMigrationsDir: "../../../../migrations/postgres",
	}); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	repo, err := NewRoomRepository(ctx, dsn)
	if err != nil {
		t.Fatalf("new repository failed: %v", err)
	}
	defer repo.Close()

	roomID := "it-room-" + time.Now().UTC().Format("20060102150405.000000000")
	room, _ := domain.NewRoom(roomID, []string{"u1", "u2"})
	if err := repo.Create(ctx, room); err != nil {
		t.Fatalf("create room failed: %v", err)
	}

	orig := domain.Message{
		ID:          "it-m-1-" + time.Now().UTC().Format("150405"),
		ClientMsgID: "it-c-1-" + time.Now().UTC().Format("150405"),
		RoomID:      roomID,
		SenderID:    "u1",
		Content:     "original",
		CreatedAt:   time.Now().UTC().Add(-1 * time.Second),
	}
	reply := domain.Message{
		ID:          "it-m-2-" + time.Now().UTC().Format("150405"),
		ClientMsgID: "it-c-2-" + time.Now().UTC().Format("150405"),
		RoomID:      roomID,
		SenderID:    "u2",
		Content:     "reply",
		ReplyToID:   orig.ID,
		CreatedAt:   time.Now().UTC(),
	}

	if err := repo.Save(ctx, orig); err != nil {
		t.Fatalf("save original failed: %v", err)
	}
	if err := repo.Save(ctx, reply); err != nil {
		t.Fatalf("save reply failed: %v", err)
	}

	messages, err := repo.ListByRoom(ctx, roomID, time.Unix(0, 0), 10)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[1].ReplyToID != orig.ID {
		t.Fatalf("expected reply to %s, got %s", orig.ID, messages[1].ReplyToID)
	}

	found, err := repo.GetByIDs(ctx, roomID, []string{orig.ID, reply.ID})
	if err != nil {
		t.Fatalf("get by ids failed: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("expected 2 messages in map, got %d", len(found))
	}
}
