package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/repository"
)

func setupCursor(t *testing.T) (context.Context, *repository.BunSyncCursorRepository, string) {
	t.Helper()

	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	userID := uuid.New().String()
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, userID, "c@x.com", "h")
	require.NoError(t, err)

	return ctx, repository.NewBunSyncCursorRepository(db), userID
}

func TestSyncCursor_AbsentIsEmptyNotError(t *testing.T) {
	ctx, repo, userID := setupCursor(t)

	got, err := repo.Get(ctx, userID, "google->internal")
	require.NoError(t, err, "a missing cursor is a full sync, not a failure")
	assert.Empty(t, got)
}

func TestSyncCursor_SetThenGet(t *testing.T) {
	ctx, repo, userID := setupCursor(t)

	require.NoError(t, repo.Set(ctx, userID, "google->internal", "token-1"))

	got, err := repo.Get(ctx, userID, "google->internal")
	require.NoError(t, err)
	assert.Equal(t, "token-1", got)
}

// Set must upsert: a second Set for the same pipeline replaces the cursor rather than
// inserting a duplicate.
func TestSyncCursor_SetUpserts(t *testing.T) {
	ctx, repo, userID := setupCursor(t)

	require.NoError(t, repo.Set(ctx, userID, "google->internal", "token-1"))
	require.NoError(t, repo.Set(ctx, userID, "google->internal", "token-2"))

	got, err := repo.Get(ctx, userID, "google->internal")
	require.NoError(t, err)
	assert.Equal(t, "token-2", got)
}

func TestSyncCursor_SeparatePerProvider(t *testing.T) {
	ctx, repo, userID := setupCursor(t)

	require.NoError(t, repo.Set(ctx, userID, "google->internal", "g"))
	require.NoError(t, repo.Set(ctx, userID, "carddav->internal", "c"))

	g, _ := repo.Get(ctx, userID, "google->internal")
	c, _ := repo.Get(ctx, userID, "carddav->internal")
	assert.Equal(t, "g", g)
	assert.Equal(t, "c", c)
}

func TestSyncCursor_Delete(t *testing.T) {
	ctx, repo, userID := setupCursor(t)
	require.NoError(t, repo.Set(ctx, userID, "google->internal", "token-1"))

	require.NoError(t, repo.Delete(ctx, userID, "google->internal"))

	got, err := repo.Get(ctx, userID, "google->internal")
	require.NoError(t, err)
	assert.Empty(t, got, "a deleted cursor forces the next run to resync fully")
}
