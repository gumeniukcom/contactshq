package repository_test

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/gumeniukcom/contactshq/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// migrateTo applies migrations in order and stops after the named one, so a test can
// seed rows in the schema that existed just before a migration and then run it.
// Applied versions are recorded, letting a test step forward with successive calls.
func migrateTo(ctx context.Context, db *bun.DB, lastVersion string) error {
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		return err
	}

	files, err := fs.Glob(migrations.FS, "*.up.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, file := range files {
		version := strings.TrimSuffix(path.Base(file), ".up.sql")

		var applied int
		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&applied); err != nil {
			return err
		}
		if applied == 0 {
			sqlBytes, err := fs.ReadFile(migrations.FS, file)
			if err != nil {
				return err
			}
			if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
				return fmt.Errorf("execute %s: %w", version, err)
			}
			if _, err := db.ExecContext(ctx,
				"INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
				return err
			}
		}

		if version == lastVersion {
			return nil
		}
	}
	return fmt.Errorf("migration %q not found", lastVersion)
}

// step mirrors the columns migration 019 rewrites.
type step struct {
	source, dest, direction, conflict string
}

func readStep(t *testing.T, db *bun.DB, id string) step {
	t.Helper()

	var s step
	row := db.QueryRow(
		"SELECT source_type, dest_type, direction, conflict_mode FROM pipeline_steps WHERE id = ?", id)
	require.NoError(t, row.Scan(&s.source, &s.dest, &s.direction, &s.conflict))
	return s
}

// seedPipeline writes the rows a pre-019 database would hold, bypassing the domain
// structs so the fixture stays pinned to the old schema's semantics.
func seedPipeline(t *testing.T, db *bun.DB, ctx context.Context) {
	t.Helper()

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, role)
		 VALUES ('u1', 'a@example.com', 'x', 'A', 'user')`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx,
		`INSERT INTO pipelines (id, user_id, name, enabled) VALUES ('p1', 'u1', 'P', 1)`)
	require.NoError(t, err)
}

func TestMigrate019_InvertedStepIsSwapped(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, migrateTo(ctx, db, "018_sync_conflict_remote_etag"))
	seedPipeline(t, db, ctx)

	// The shape the pipeline form produced by default: internal as the source.
	_, err := db.ExecContext(ctx,
		`INSERT INTO pipeline_steps (id, pipeline_id, step_order, source_type, source_config,
		    dest_type, dest_config, conflict_mode, direction)
		 VALUES ('s1', 'p1', 1, 'internal', '{}', 'carddav', '{"endpoint":"x"}', 'source_wins', 'pull')`)
	require.NoError(t, err)

	require.NoError(t, migrateTo(ctx, db, "019_normalize_pipeline_direction"))

	got := readStep(t, db, "s1")
	assert.Equal(t, "carddav", got.source, "external provider becomes the source")
	assert.Equal(t, "internal", got.dest, "internal book becomes the destination")
	// It used to copy internal -> carddav; that is an export once the sides are named right.
	assert.Equal(t, "export", got.direction)
	// "source wins" meant "internal wins", which is "dest wins" after the swap.
	assert.Equal(t, "dest_wins", got.conflict)

	var cfg string
	require.NoError(t, db.QueryRow("SELECT source_config FROM pipeline_steps WHERE id='s1'").Scan(&cfg))
	assert.JSONEq(t, `{"endpoint":"x"}`, cfg, "the provider's config must follow it to the source column")
}

func TestMigrate019_InvertedPushBecomesImport(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, migrateTo(ctx, db, "018_sync_conflict_remote_etag"))
	seedPipeline(t, db, ctx)

	_, err := db.ExecContext(ctx,
		`INSERT INTO pipeline_steps (id, pipeline_id, step_order, source_type, source_config,
		    dest_type, dest_config, conflict_mode, direction)
		 VALUES ('s1', 'p1', 1, 'internal', '{}', 'google', '{}', 'dest_wins', 'push')`)
	require.NoError(t, err)

	require.NoError(t, migrateTo(ctx, db, "019_normalize_pipeline_direction"))

	got := readStep(t, db, "s1")
	assert.Equal(t, "google", got.source)
	assert.Equal(t, "internal", got.dest)
	assert.Equal(t, "import", got.direction, "push meant dest->source, i.e. google->internal")
	assert.Equal(t, "source_wins", got.conflict)
}

func TestMigrate019_AlreadyNormalStepOnlyRenamesDirection(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, migrateTo(ctx, db, "018_sync_conflict_remote_etag"))
	seedPipeline(t, db, ctx)

	_, err := db.ExecContext(ctx,
		`INSERT INTO pipeline_steps (id, pipeline_id, step_order, source_type, source_config,
		    dest_type, dest_config, conflict_mode, direction)
		 VALUES ('s1', 'p1', 1, 'carddav', '{"endpoint":"y"}', 'internal', '{}', 'source_wins', 'bidirectional')`)
	require.NoError(t, err)

	require.NoError(t, migrateTo(ctx, db, "019_normalize_pipeline_direction"))

	got := readStep(t, db, "s1")
	assert.Equal(t, "carddav", got.source)
	assert.Equal(t, "internal", got.dest)
	assert.Equal(t, "two_way", got.direction)
	assert.Equal(t, "source_wins", got.conflict, "conflict mode must not flip for an already-normal step")
}

// Sync state is not derived data. Dropping it would make the next run import every
// remote contact as new and duplicate the address book, so the two sides are swapped.
func TestMigrate019_SwapsSyncStateSides(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, migrateTo(ctx, db, "018_sync_conflict_remote_etag"))
	seedPipeline(t, db, ctx)

	_, err := db.ExecContext(ctx,
		`INSERT INTO sync_states (id, user_id, provider_type, remote_id, local_id, remote_etag, local_etag, content_hash)
		 VALUES ('st1', 'u1', 'internal->google', 'local-uid', 'people/c1', 'etag-internal', 'etag-google', 'h')`)
	require.NoError(t, err)

	// A state that was already the right way round must be left alone.
	_, err = db.ExecContext(ctx,
		`INSERT INTO sync_states (id, user_id, provider_type, remote_id, local_id, remote_etag, local_etag, content_hash)
		 VALUES ('st2', 'u1', 'carddav->internal', 'remote-uid', 'local-uid-2', 'etag-remote', 'etag-local', 'h')`)
	require.NoError(t, err)

	require.NoError(t, migrateTo(ctx, db, "019_normalize_pipeline_direction"))

	var pt, remoteID, localID, remoteETag, localETag string
	require.NoError(t, db.QueryRow(
		`SELECT provider_type, remote_id, local_id, remote_etag, local_etag FROM sync_states WHERE id='st1'`).
		Scan(&pt, &remoteID, &localID, &remoteETag, &localETag))

	assert.Equal(t, "google->internal", pt)
	assert.Equal(t, "people/c1", remoteID, "the Google id must end up on the remote side")
	assert.Equal(t, "local-uid", localID, "the internal UID must end up on the local side")
	assert.Equal(t, "etag-google", remoteETag)
	assert.Equal(t, "etag-internal", localETag)

	require.NoError(t, db.QueryRow(
		`SELECT provider_type, remote_id, local_id FROM sync_states WHERE id='st2'`).
		Scan(&pt, &remoteID, &localID))
	assert.Equal(t, "carddav->internal", pt, "an already-normal state is untouched")
	assert.Equal(t, "remote-uid", remoteID)
	assert.Equal(t, "local-uid-2", localID)
}

// Conflicts on inverted pipelines were unresolvable: resolution loads local_contact_id as
// an internal contact UID, and there it held the remote provider's id. They are dropped
// and re-detected, unlike sync state.
func TestMigrate019_DropsInvertedConflictsKeepsOthers(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, migrateTo(ctx, db, "018_sync_conflict_remote_etag"))
	seedPipeline(t, db, ctx)

	_, err := db.ExecContext(ctx,
		`INSERT INTO sync_conflicts (id, user_id, provider_type, remote_id, local_contact_id, status)
		 VALUES ('c1', 'u1', 'internal->google', 'local-uid', 'people/c1', 'pending')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO sync_conflicts (id, user_id, provider_type, remote_id, local_contact_id, status)
		 VALUES ('c2', 'u1', 'carddav->internal', 'remote-uid', 'local-uid', 'pending')`)
	require.NoError(t, err)

	require.NoError(t, migrateTo(ctx, db, "019_normalize_pipeline_direction"))

	var ids []string
	rows, err := db.QueryContext(ctx, "SELECT id FROM sync_conflicts ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []string{"c2"}, ids, "only the inverted pipeline's conflicts are dropped")
}
