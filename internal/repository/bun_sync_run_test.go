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

func TestSyncRun_CreateAndListByUser(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	// Insert a user first (FK constraint)
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?, ?, ?)`, "u1", "u1@test.com", "hash")
	require.NoError(t, err)

	repo := repository.NewBunSyncRunRepository(db)

	run := &domain.SyncRun{
		ID:           uuid.New().String(),
		UserID:       "u1",
		ProviderType: "carddav->internal",
		Status:       "running",
		StartedAt:    time.Now(),
	}
	require.NoError(t, repo.Create(ctx, run))

	runs, err := repo.ListByUser(ctx, "u1", 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, run.ID, runs[0].ID)
	assert.Equal(t, "running", runs[0].Status)
}

func TestSyncRun_ListActiveByUser_FiltersRunning(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?, ?, ?)`, "u2", "u2@test.com", "hash")
	require.NoError(t, err)

	repo := repository.NewBunSyncRunRepository(db)

	runRunning := &domain.SyncRun{
		ID: uuid.New().String(), UserID: "u2", ProviderType: "test", Status: "running", StartedAt: time.Now(),
	}
	runDone := &domain.SyncRun{
		ID: uuid.New().String(), UserID: "u2", ProviderType: "test", Status: "completed", StartedAt: time.Now(),
	}

	require.NoError(t, repo.Create(ctx, runRunning))
	require.NoError(t, repo.Create(ctx, runDone))

	active, err := repo.ListActiveByUser(ctx, "u2")
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "running", active[0].Status)
}

func TestSyncRun_Update_CompletesRun(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?, ?, ?)`, "u3", "u3@test.com", "hash")
	require.NoError(t, err)

	repo := repository.NewBunSyncRunRepository(db)

	run := &domain.SyncRun{
		ID: uuid.New().String(), UserID: "u3", ProviderType: "test", Status: "running", StartedAt: time.Now(),
	}
	require.NoError(t, repo.Create(ctx, run))

	now := time.Now()
	run.Status = "completed"
	run.CreatedCount = 5
	run.UpdatedCount = 2
	run.FinishedAt = &now
	require.NoError(t, repo.Update(ctx, run))

	runs, err := repo.ListByUser(ctx, "u3", 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "completed", runs[0].Status)
	assert.Equal(t, 5, runs[0].CreatedCount)
}
