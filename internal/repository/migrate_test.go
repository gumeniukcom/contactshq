package repository_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"

	"github.com/gumeniukcom/contactshq/internal/repository"
)

// TestMain navigates to the project root so that "migrations/" can be resolved.
func TestMain(m *testing.M) {
	// Walk up from the current package directory until we find go.mod
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			_ = os.Chdir(dir)
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	os.Exit(m.Run())
}

// setupTestDB opens an in-memory SQLite DB and returns it ready to have
// migrations applied.
func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { db.Close() })

	return db
}

func TestMigrate_CreatesSchemaMigrationsTable(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := repository.Migrate(ctx, db)
	require.NoError(t, err)

	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations")
	require.NoError(t, row.Scan(&count))
	assert.GreaterOrEqual(t, count, 1, "at least one migration should be recorded")
}

func TestMigrate_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	require.NoError(t, repository.Migrate(ctx, db))
	// Running again must not error (CREATE TABLE IF NOT EXISTS everywhere)
	require.NoError(t, repository.Migrate(ctx, db))

	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations")
	require.NoError(t, row.Scan(&count))
	assert.GreaterOrEqual(t, count, 1)
}

func TestMigrate_ExpectedTablesExist(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	require.NoError(t, repository.Migrate(ctx, db))

	tables := []string{"users", "address_books", "contacts", "sync_states", "pipelines", "pipeline_steps", "jobs", "sync_runs", "schema_migrations"}
	for _, table := range tables {
		var n int
		row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table)
		require.NoError(t, row.Scan(&n), "checking table %s", table)
		assert.Equal(t, 1, n, "table %s should exist", table)
	}
}
