package repository_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

// setupPostgres connects to the database named by TEST_POSTGRES_DSN and hands back an
// empty schema. Every migration is written in SQLite-flavoured SQL, yet docker-compose —
// the documented way to install this — runs PostgreSQL. Nothing verified that the two
// agreed until these tests existed.
//
// The test skips when the variable is unset, so `go test ./...` still works offline.
func setupPostgres(t *testing.T) *bun.DB {
	t.Helper()

	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set; skipping PostgreSQL migration tests")
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.Ping(), "cannot reach the PostgreSQL named by TEST_POSTGRES_DSN")

	// Each test gets a clean public schema.
	ctx := context.Background()
	_, err := db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	require.NoError(t, err)

	return db
}

func TestPostgres_MigrateAppliesEverySchemaObject(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()

	require.NoError(t, repository.Migrate(ctx, db))

	for _, table := range []string{
		"users", "address_books", "contacts", "contact_emails", "contact_phones",
		"contact_addresses", "contact_urls", "contact_ims", "contact_categories",
		"contact_dates", "sync_states", "sync_runs", "sync_conflicts", "pipelines",
		"pipeline_steps", "provider_connections", "app_passwords",
	} {
		var exists bool
		require.NoError(t, db.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=?)",
			table).Scan(&exists))
		assert.True(t, exists, "table %s is missing on PostgreSQL", table)
	}
}

func TestPostgres_MigrateIsIdempotent(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()

	require.NoError(t, repository.Migrate(ctx, db))
	require.NoError(t, repository.Migrate(ctx, db), "a second run must apply nothing")

	var applied int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&applied))
	assert.Greater(t, applied, 15)
}

// Migration 019 rewrites rows with substr() and || string concatenation. Those behave
// differently across dialects, and it had only ever run on SQLite.
func TestPostgres_Migration019SwapsInvertedRows(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()

	require.NoError(t, migrateTo(ctx, db, "018_sync_conflict_remote_etag"))

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, role)
		 VALUES ('u1', 'a@example.com', 'x', 'A', 'user')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO pipelines (id, user_id, name, enabled) VALUES ('p1', 'u1', 'P', true)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO pipeline_steps (id, pipeline_id, step_order, source_type, source_config,
		    dest_type, dest_config, conflict_mode, direction)
		 VALUES ('s1', 'p1', 1, 'internal', '{}', 'carddav', '{"endpoint":"x"}', 'source_wins', 'pull')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO sync_states (id, user_id, provider_type, remote_id, local_id, remote_etag, local_etag, content_hash)
		 VALUES ('st1', 'u1', 'internal->google', 'local-uid', 'people/c1', 'etag-internal', 'etag-google', 'h')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO sync_conflicts (id, user_id, provider_type, remote_id, local_contact_id, status)
		 VALUES ('c1', 'u1', 'internal->google', 'local-uid', 'people/c1', 'pending')`)
	require.NoError(t, err)

	require.NoError(t, migrateTo(ctx, db, "019_normalize_pipeline_direction"))

	var source, dest, direction, conflict string
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT source_type, dest_type, direction, conflict_mode FROM pipeline_steps WHERE id='s1'").
		Scan(&source, &dest, &direction, &conflict))
	assert.Equal(t, "carddav", source)
	assert.Equal(t, "internal", dest)
	assert.Equal(t, "export", direction)
	assert.Equal(t, "dest_wins", conflict)

	var providerType, remoteID, localID string
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT provider_type, remote_id, local_id FROM sync_states WHERE id='st1'").
		Scan(&providerType, &remoteID, &localID))
	assert.Equal(t, "google->internal", providerType, "substr() and || must behave as on SQLite")
	assert.Equal(t, "people/c1", remoteID)
	assert.Equal(t, "local-uid", localID)

	var conflicts int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_conflicts").Scan(&conflicts))
	assert.Equal(t, 0, conflicts, "unresolvable conflicts on inverted pipelines are dropped")
}

// The contact write path fans out across eight tables in one transaction; PostgreSQL
// enforces foreign keys strictly, so a mistake there shows up here and not on SQLite.
func TestPostgres_SaveContactWithChildren(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ('u1', 'a@example.com', 'x')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO address_books (id, user_id, name) VALUES ('ab1', 'u1', 'Contacts')`)
	require.NoError(t, err)

	repo := repository.NewBunContactRepository(db)
	c := newContact("ab1")

	require.NoError(t, repo.Save(ctx, c, childRecordsFixture()))

	got, err := repo.GetByIDWithRelations(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Len(t, got.Emails, 1)
	assert.Len(t, got.Phones, 1)
	assert.Len(t, got.Categories, 1)

	// Deleting the contact must cascade to its children.
	require.NoError(t, repo.Delete(ctx, c.ID))

	var orphans int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM contact_emails WHERE contact_id = ?", c.ID).Scan(&orphans))
	assert.Equal(t, 0, orphans, "child rows must cascade on delete")
}

func childRecordsFixture() domain.ChildRecords {
	return domain.ChildRecords{
		Emails:     []*domain.ContactEmail{{Value: "jane@example.com", Type: "work"}},
		Phones:     []*domain.ContactPhone{{Value: "+15551234567", Type: "cell"}},
		Categories: []*domain.ContactCategory{{Value: "vip"}},
	}
}
