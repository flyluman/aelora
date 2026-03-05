package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (r *RoomRepository) Save(ctx context.Context, msg domain.Message) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
INSERT INTO message_dedup (client_msg_id, room_id, message_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING
`, dedupKey(msg.SenderID, msg.ClientMsgID), msg.RoomID, msg.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrDuplicateClientMessage
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO messages (room_id, created_at, message_id, sender_id, content, message_type, client_msg_id, reply_to_message_id)
VALUES ($1, $2, $3, $4, $5, 'text', $6, $7)
`, msg.RoomID, msg.CreatedAt, msg.ID, msg.SenderID, msg.Content, msg.ClientMsgID, nullable(msg.ReplyToID)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *RoomRepository) ExistsByClientMsgID(ctx context.Context, senderID, clientMsgID string) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM message_dedup WHERE client_msg_id = $1)`, dedupKey(senderID, clientMsgID)).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *RoomRepository) ListByRoom(ctx context.Context, roomID string, since time.Time, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
SELECT room_id, created_at, message_id, sender_id, content, client_msg_id
     , COALESCE(reply_to_message_id, '')
FROM messages
WHERE room_id = $1 AND created_at > $2
ORDER BY created_at ASC, message_id ASC
LIMIT $3
`, roomID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Message, 0, limit)
	for rows.Next() {
		var msg domain.Message
		if err := rows.Scan(&msg.RoomID, &msg.CreatedAt, &msg.ID, &msg.SenderID, &msg.Content, &msg.ClientMsgID, &msg.ReplyToID); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *RoomRepository) GetByIDs(ctx context.Context, roomID string, messageIDs []string) (map[string]domain.Message, error) {
	out := make(map[string]domain.Message, len(messageIDs))
	if len(messageIDs) == 0 {
		return out, nil
	}

	rows, err := r.pool.Query(ctx, `
SELECT room_id, created_at, message_id, sender_id, content, client_msg_id, COALESCE(reply_to_message_id, '')
FROM messages
WHERE room_id = $1 AND message_id = ANY($2)
`, roomID, messageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var msg domain.Message
		if err := rows.Scan(&msg.RoomID, &msg.CreatedAt, &msg.ID, &msg.SenderID, &msg.Content, &msg.ClientMsgID, &msg.ReplyToID); err != nil {
			return nil, err
		}
		out[msg.ID] = msg
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func dedupKey(senderID, clientMsgID string) string {
	sum := sha256.Sum256([]byte(senderID + "\x00" + clientMsgID))
	return hex.EncodeToString(sum[:])
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}
