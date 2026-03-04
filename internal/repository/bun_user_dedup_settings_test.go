package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

func setupDedupSettingsTest(t *testing.T) (context.Context, *repository.BunUserDedupSettingsRepository, string) {
	t.Helper()
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	repo := repository.NewBunUserDedupSettingsRepository(db)
	userID := uuid.New().String()

	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, userID, "dedup@test.com", "h")
	require.NoError(t, err)

	return ctx, repo, userID
}

func TestDedupSettings_GetReturnsNilWhenNotSet(t *testing.T) {
	ctx, repo, userID := setupDedupSettingsTest(t)

	s, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.Nil(t, s)
}

func TestDedupSettings_UpsertInsertAndGet(t *testing.T) {
	ctx, repo, userID := setupDedupSettingsTest(t)

	settings := &domain.UserDedupSettings{
		UserID:    userID,
		Schedule:  "0 2 * * *",
		Enabled:   true,
		UpdatedAt: time.Now(),
	}
	require.NoError(t, repo.Upsert(ctx, settings))

	got, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, userID, got.UserID)
	assert.Equal(t, "0 2 * * *", got.Schedule)
	assert.True(t, got.Enabled)
}

func TestDedupSettings_UpsertUpdatesExisting(t *testing.T) {
	ctx, repo, userID := setupDedupSettingsTest(t)

	// Insert
	settings := &domain.UserDedupSettings{
		UserID:    userID,
		Schedule:  "0 2 * * *",
		Enabled:   true,
		UpdatedAt: time.Now(),
	}
	require.NoError(t, repo.Upsert(ctx, settings))

	// Update
	settings.Schedule = "0 */6 * * *"
	settings.Enabled = false
	settings.UpdatedAt = time.Now()
	require.NoError(t, repo.Upsert(ctx, settings))

	got, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "0 */6 * * *", got.Schedule)
	assert.False(t, got.Enabled)
}

func TestDedupSettings_ListAllEmpty(t *testing.T) {
	ctx, repo, _ := setupDedupSettingsTest(t)

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestDedupSettings_ListAllReturnsAll(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	repo := repository.NewBunUserDedupSettingsRepository(db)

	// Create two users with settings
	for i, email := range []string{"a@test.com", "b@test.com"} {
		uid := uuid.New().String()
		_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, uid, email, "h")
		require.NoError(t, err)

		s := &domain.UserDedupSettings{
			UserID:    uid,
			Schedule:  "0 2 * * *",
			Enabled:   i == 0, // first enabled, second disabled
			UpdatedAt: time.Now(),
		}
		require.NoError(t, repo.Upsert(ctx, s))
	}

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}
