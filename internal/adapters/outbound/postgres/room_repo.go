package postgres

import (
	"context"
	"errors"
	"sort"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RoomRepository struct {
	pool *pgxpool.Pool
}

func NewRoomRepositoryFromPool(pool *Pool) (*RoomRepository, error) {
	repo := &RoomRepository{pool: pool.Pool}
	return repo, nil
}

func NewRoomRepository(ctx context.Context, dsn string) (*RoomRepository, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	repo := &RoomRepository{pool: pool}
	if err := repo.ensureOutboxLeaseColumns(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return repo, nil
}

func (r *RoomRepository) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

func (r *RoomRepository) Ping(ctx context.Context) error {
	if r.pool == nil {
		return errors.New("postgres pool is nil")
	}
	return r.pool.Ping(ctx)
}

func (r *RoomRepository) Get(ctx context.Context, roomID string) (domain.Room, error) {
	members, err := r.ListMembers(ctx, roomID)
	if err != nil {
		return domain.Room{}, err
	}
	return domain.NewRoom(roomID, members)
}

func (r *RoomRepository) Create(ctx context.Context, room domain.Room) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `INSERT INTO rooms (id) VALUES ($1)`, room.ID); err != nil {
		if isPGCode(err, "23505") {
			return domain.ErrRoomAlreadyExists
		}
		return err
	}

	for member := range room.Members {
		if _, err := tx.Exec(ctx, `INSERT INTO room_members (room_id, user_id, role) VALUES ($1, $2, 'member')`, room.ID, member); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (r *RoomRepository) AddMember(ctx context.Context, roomID, userID string) error {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM rooms WHERE id = $1)`, roomID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return domain.ErrRoomNotFound
	}

	_, err := r.pool.Exec(ctx, `
INSERT INTO room_members (room_id, user_id)
VALUES ($1, $2)
ON CONFLICT (room_id, user_id) DO NOTHING
`, roomID, userID)
	return err
}

func (r *RoomRepository) GetMemberRole(ctx context.Context, roomID, userID string) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, `SELECT role FROM room_members WHERE room_id = $1 AND user_id = $2`, roomID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrRoomNotFound
	}
	if err != nil {
		return "", err
	}
	return role, nil
}

func (r *RoomRepository) SetMemberRole(ctx context.Context, roomID, userID, role string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE room_members SET role = $1 WHERE room_id = $2 AND user_id = $3`, role, roomID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrRoomNotFound
	}
	return nil
}

func (r *RoomRepository) ListMembers(ctx context.Context, roomID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id FROM room_members WHERE room_id = $1`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]string, 0, 8)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		members = append(members, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, domain.ErrRoomNotFound
	}
	sort.Strings(members)
	return members, nil
}

func isPGCode(err error, code string) bool {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == code
	}
	return false
}
