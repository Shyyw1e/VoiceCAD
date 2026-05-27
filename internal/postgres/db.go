package postgres

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/Shyyw1e/VoiceCAD/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(ctx context.Context, cfg config.PostgresConfig) (*sql.DB, error) {
	if err := EnsureDatabase(ctx, cfg); err != nil {
		return nil, err
	}

	db, err := sql.Open("pgx", cfg.DSN())
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(int(cfg.MaxOpenConns))
	db.SetMaxIdleConns(int(cfg.MinIdleConns))
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func EnsureDatabase(ctx context.Context, cfg config.PostgresConfig) error {
	adminDB, err := sql.Open("pgx", cfg.DSNForDatabase("postgres"))
	if err != nil {
		return err
	}
	defer adminDB.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := adminDB.PingContext(pingCtx); err != nil {
		return err
	}

	var exists bool
	if err := adminDB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, cfg.Database).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err = adminDB.ExecContext(ctx, `CREATE DATABASE `+quoteIdent(cfg.Database))
	return err
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
