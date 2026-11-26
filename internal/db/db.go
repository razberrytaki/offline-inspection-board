package db

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schema string

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := initialize(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func initialize(ctx context.Context, pool *pgxpool.Pool) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, schema); err != nil {
			return fmt.Errorf("스키마 적용: %w", err)
		}
		if err := seed(ctx, tx); err != nil {
			return fmt.Errorf("데모 데이터 등록: %w", err)
		}
		return nil
	})
}

func seed(ctx context.Context, tx pgx.Tx) error {
	users := [...]struct {
		id, name, role, password string
	}{
		{"worker-1", "직원 1", "worker", "demo-worker-1"},
		{"worker-2", "직원 2", "worker", "demo-worker-2"},
		{"supervisor", "감독자", "supervisor", "demo-supervisor"},
	}
	for _, user := range users {
		hash, err := bcrypt.GenerateFromPassword([]byte(user.password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO users (id, name, role, password_hash)
			VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING
		`, user.id, user.name, user.role, hash)
		if err != nil {
			return err
		}
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO equipment (id, name, retired) VALUES
			('P-12', '냉각수 펌프', false),
			('V-04', '급수 밸브', false),
			('F-08', '배기 팬', false),
			('P-03', '순환 펌프', true)
		ON CONFLICT DO NOTHING
	`)
	return err
}
