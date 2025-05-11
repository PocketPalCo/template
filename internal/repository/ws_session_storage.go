package repository

import (
	"context"
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"time"
)

type SessionStorageRepo struct {
	conn postgres.DB
}

type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func NewSessionStorageRepo(conn postgres.DB) *SessionStorageRepo {
	return &SessionStorageRepo{
		conn: conn,
	}
}

func (s *SessionStorageRepo) Get(ctx context.Context, key string) (string, error) {
	var value string
	// language=sql
	err := s.conn.QueryRow(ctx, "SELECT value FROM ws_sessions WHERE key = $1 AND deleted_at IS NULL LIMIT 1", key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *SessionStorageRepo) Set(ctx context.Context, key, value string) error {
	id := uuid.New()
	// language=sql
	_, err := s.conn.Exec(
		ctx,
		"INSERT INTO ws_sessions (id, key, value, created_at, updated_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (key) DO NOTHING",
		id, key, value, time.Now(), time.Now())
	if err != nil {
		return err
	}
	return nil
}

func (s *SessionStorageRepo) Delete(ctx context.Context, key string) error {
	// language=sql
	_, err := s.conn.Exec(ctx, "UPDATE ws_sessions SET deleted_at = $1 WHERE key = $2", time.Now(), key)
	if err != nil {
		return err
	}
	return nil
}

func (s *SessionStorageRepo) GetAll(ctx context.Context) ([]KeyValue, error) {
	// language=sql
	rows, err := s.conn.Query(ctx, "SELECT key, value FROM ws_sessions WHERE deleted_at IS NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result, err := pgx.CollectRows(rows, pgx.RowToStructByName[KeyValue])
	if err != nil {
		return nil, err
	}

	return result, nil

}
