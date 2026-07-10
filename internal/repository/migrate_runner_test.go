package repository_test

import (
	"context"
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/repository"
)

// Migrations are embedded, so a binary started from any directory carries its schema.
// Reading them from a `migrations/` folder relative to the working directory meant a
// server launched elsewhere applied nothing and served every request against an empty
// database, while /health cheerfully answered 200.
func TestMigrate_WorksFromAnyWorkingDirectory(t *testing.T) {
	original, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(original) })

	db := setupTestDB(t)
	ctx := context.Background()

	require.NoError(t, repository.Migrate(ctx, db))

	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='contacts'").Scan(&count))
	assert.Equal(t, 1, count, "the contacts table must exist regardless of the working directory")
}

// A build carrying no migrations must fail loudly. Applying nothing looked exactly like
// being up to date.
func TestMigrateFS_EmptyFilesystemIsAnError(t *testing.T) {
	db := setupTestDB(t)

	err := repository.MigrateFS(context.Background(), db, fstest.MapFS{})

	assert.ErrorIs(t, err, repository.ErrNoMigrations)
}

// Each migration runs in its own transaction. A failure part-way through a multi-statement
// file used to leave the successful statements applied and the version unrecorded, so the
// next run replayed them and died on "duplicate column" — permanently.
func TestMigrateFS_FailedMigrationRollsBackEntirely(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	fsys := fstest.MapFS{
		"001_first.up.sql": &fstest.MapFile{Data: []byte(
			`CREATE TABLE widgets (id TEXT PRIMARY KEY);`)},
		// The second statement fails: the column already exists.
		"002_broken.up.sql": &fstest.MapFile{Data: []byte(
			"ALTER TABLE widgets ADD COLUMN color TEXT;\nALTER TABLE widgets ADD COLUMN color TEXT;\n")},
	}

	err := repository.MigrateFS(ctx, db, fsys)
	require.Error(t, err)

	var colCount int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('widgets') WHERE name='color'").Scan(&colCount))
	assert.Equal(t, 0, colCount, "the half-applied statement must be rolled back")

	var recorded int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations WHERE version='002_broken'").Scan(&recorded))
	assert.Equal(t, 0, recorded, "a failed migration must not be recorded")

	// Because nothing was left behind, fixing the file and re-running works.
	fsys["002_broken.up.sql"] = &fstest.MapFile{Data: []byte("ALTER TABLE widgets ADD COLUMN color TEXT;\n")}
	require.NoError(t, repository.MigrateFS(ctx, db, fsys))

	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('widgets') WHERE name='color'").Scan(&colCount))
	assert.Equal(t, 1, colCount)
}

func TestMigrateFS_AppliesInFilenameOrderAndRecordsEach(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	fsys := fstest.MapFS{
		"002_second.up.sql": &fstest.MapFile{Data: []byte(`ALTER TABLE widgets ADD COLUMN color TEXT;`)},
		"001_first.up.sql":  &fstest.MapFile{Data: []byte(`CREATE TABLE widgets (id TEXT PRIMARY KEY);`)},
	}

	require.NoError(t, repository.MigrateFS(ctx, db, fsys))

	var versions []string
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var v string
		require.NoError(t, rows.Scan(&v))
		versions = append(versions, v)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []string{"001_first", "002_second"}, versions)
}

func TestMigrateFS_IsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	require.NoError(t, repository.Migrate(ctx, db))
	require.NoError(t, repository.Migrate(ctx, db), "a second run must be a no-op")
}
