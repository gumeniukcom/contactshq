package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/migrations"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// ErrNoMigrations guards against a build that somehow carries no schema: applying
// nothing used to look exactly like being up to date.
var ErrNoMigrations = errors.New("no migrations found")

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

// Migrate applies every embedded migration that is not yet recorded.
func Migrate(ctx context.Context, db *bun.DB) error {
	return MigrateFS(ctx, db, migrations.FS)
}

// MigrateFS applies the migrations found in fsys, in filename order. Each file runs in
// its own transaction together with the row that records it, so a failure part-way
// through a multi-statement migration leaves nothing behind: without that, a second run
// re-applies the statements that already succeeded and dies on "duplicate column",
// wedging the database for good.
func MigrateFS(ctx context.Context, db *bun.DB, fsys fs.FS) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	files, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	if len(files) == 0 {
		return ErrNoMigrations
	}
	sort.Strings(files)

	for _, file := range files {
		version := strings.TrimSuffix(path.Base(file), ".up.sql")

		var count int
		row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version)
		if err := row.Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if count > 0 {
			continue
		}

		migrationSQL, err := fs.ReadFile(fsys, file)
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", file, err)
		}

		if err := applyMigration(ctx, db, version, string(migrationSQL)); err != nil {
			return err
		}
	}

	return nil
}

func applyMigration(ctx context.Context, db *bun.DB, version, migrationSQL string) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx, migrationSQL); err != nil {
			return fmt.Errorf("execute migration %s: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		return nil
	})
}

// SchemaVersion is the latest migration applied, which is the question an operator asks first
// after an upgrade.
//
// A string, because that is what the column holds: the migration's filename stem, such as
// "025_backup_runs". The names are zero-padded, so the lexicographic maximum is also the
// numeric one.
func SchemaVersion(ctx context.Context, db *bun.DB) (string, error) {
	var version string
	err := db.NewSelect().
		TableExpr("schema_migrations").
		ColumnExpr("COALESCE(MAX(version), '')").
		Scan(ctx, &version)
	return version, err
}
