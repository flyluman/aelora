package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

func (r *RoomRepository) Enqueue(ctx context.Context, msg domain.Message, streamAppended bool) error {
	payload, err := json.Marshal(ports.OutboxMessage{Message: msg, StreamAppended: streamAppended})
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
INSERT INTO message_outbox (message_id, created_at, payload, dispatched, claimed_by, claimed_until)
VALUES ($1, $2, $3, false)
ON CONFLICT (message_id) DO NOTHING
`, msg.ID, msg.CreatedAt, payload)
	return err
}

func (r *RoomRepository) ListPending(ctx context.Context, limit int) ([]ports.OutboxMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
SELECT payload
FROM message_outbox
WHERE dispatched = false
ORDER BY created_at ASC
LIMIT $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ports.OutboxMessage, 0, limit)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		msg, err := decodeOutboxPayload(payload)
		if err != nil {
			continue
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *RoomRepository) MarkDispatched(ctx context.Context, messageID string) error {
	_, err := r.pool.Exec(ctx, `
UPDATE message_outbox
SET dispatched = true, dispatched_at = NOW(), claimed_by = NULL, claimed_until = NULL
WHERE message_id = $1
`, messageID)
	return err
}

func (r *RoomRepository) ClaimPending(ctx context.Context, consumerID string, limit int, lease time.Duration) ([]ports.OutboxMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	leaseUntil := time.Now().UTC().Add(lease)

	rows, err := r.pool.Query(ctx, `
WITH candidates AS (
	SELECT message_id
	FROM message_outbox
	WHERE dispatched = false AND (claimed_until IS NULL OR claimed_until < NOW())
	ORDER BY created_at ASC
	LIMIT $1
	FOR UPDATE SKIP LOCKED
), claimed AS (
	UPDATE message_outbox AS m
	SET claimed_by = $2, claimed_until = $3
	FROM candidates AS c
	WHERE m.message_id = c.message_id
	RETURNING m.payload
)
SELECT payload FROM claimed
`, limit, consumerID, leaseUntil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ports.OutboxMessage, 0, limit)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		msg, err := decodeOutboxPayload(payload)
		if err != nil {
			continue
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *RoomRepository) MarkStreamAppended(ctx context.Context, messageID string) error {
	var payload []byte
	if err := r.pool.QueryRow(ctx, `SELECT payload FROM message_outbox WHERE message_id = $1`, messageID).Scan(&payload); err != nil {
		return err
	}
	decoded, err := decodeOutboxPayload(payload)
	if err != nil {
		return err
	}
	decoded.StreamAppended = true
	nextPayload, err := json.Marshal(decoded)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `UPDATE message_outbox SET payload = $2 WHERE message_id = $1`, messageID, nextPayload)
	return err
}

func (r *RoomRepository) ReleaseClaim(ctx context.Context, consumerID, messageID string) error {
	_, err := r.pool.Exec(ctx, `
UPDATE message_outbox
SET claimed_by = NULL, claimed_until = NULL
WHERE message_id = $1 AND claimed_by = $2
`, messageID, consumerID)
	return err
}

func (r *RoomRepository) ensureOutboxLeaseColumns(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, `ALTER TABLE IF EXISTS message_outbox ADD COLUMN IF NOT EXISTS claimed_by TEXT`); err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, `ALTER TABLE IF EXISTS message_outbox ADD COLUMN IF NOT EXISTS claimed_until TIMESTAMPTZ`); err != nil {
		return err
	}
	return nil
}

func decodeOutboxPayload(payload []byte) (ports.OutboxMessage, error) {
	var outboxMsg ports.OutboxMessage
	if err := json.Unmarshal(payload, &outboxMsg); err == nil {
		if outboxMsg.Message.ID != "" {
			return outboxMsg, nil
		}
	}

	// Backward compatibility for legacy payload rows that were plain domain.Message.
	var legacy domain.Message
	if err := json.Unmarshal(payload, &legacy); err != nil {
		return ports.OutboxMessage{}, err
	}
	return ports.OutboxMessage{Message: legacy, StreamAppended: true}, nil
}
