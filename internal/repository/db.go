package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func NewDB(cfg config.DatabaseConfig) (*bun.DB, error) {
	var sqldb *sql.DB
	var db *bun.DB

	switch cfg.Driver {
	case "sqlite":
		var err error
		sqldb, err = sql.Open(sqliteshim.ShimName, cfg.DSN+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
		if err != nil {
			return nil, fmt.Errorf("open sqlite: %w", err)
		}
		sqldb.SetMaxOpenConns(1)
		db = bun.NewDB(sqldb, sqlitedialect.New())

	case "postgres":
		sqldb = sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(cfg.DSN)))
		db = bun.NewDB(sqldb, pgdialect.New())

	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

func Migrate(ctx context.Context, db *bun.DB) error {
	// Ensure schema_migrations table exists
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// Find all migration files
	files, err := filepath.Glob("migrations/*.up.sql")
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(files)

	for _, file := range files {
		version := strings.TrimSuffix(filepath.Base(file), ".up.sql")

		// Check if already applied
		var count int
		row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version)
		if err := row.Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if count > 0 {
			continue
		}

		// Read and execute migration
		migrationSQL, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", file, err)
		}

		if _, err = db.ExecContext(ctx, string(migrationSQL)); err != nil {
			return fmt.Errorf("execute migration %s: %w", version, err)
		}

		// Record as applied
		if _, err = db.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
	}

	return nil
}
