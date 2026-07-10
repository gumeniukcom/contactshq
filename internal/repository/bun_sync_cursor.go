package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunSyncCursorRepository struct {
	db *bun.DB
}

func NewBunSyncCursorRepository(db *bun.DB) *BunSyncCursorRepository {
	return &BunSyncCursorRepository{db: db}
}

// Get returns the stored cursor for a pipeline, or "" when there is none. An absent cursor
// is not an error: it means the next run is a full sync.
func (r *BunSyncCursorRepository) Get(ctx context.Context, userID, providerType string) (string, error) {
	var cursor string
	err := r.db.NewSelect().
		Model((*domain.SyncCursor)(nil)).
		Column("cursor").
		Where("user_id = ? AND provider_type = ?", userID, providerType).
		Scan(ctx, &cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return cursor, err
}

// Set upserts the cursor for a pipeline.
func (r *BunSyncCursorRepository) Set(ctx context.Context, userID, providerType, cursor string) error {
	row := &domain.SyncCursor{
		ID:           uuid.New().String(),
		UserID:       userID,
		ProviderType: providerType,
		Cursor:       cursor,
		UpdatedAt:    time.Now(),
	}
	_, err := r.db.NewInsert().
		Model(row).
		On("CONFLICT (user_id, provider_type) DO UPDATE").
		Set("cursor = EXCLUDED.cursor").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// Delete removes a cursor, forcing the next run to resynchronise fully. Called when a
// provider reports the cursor is no longer valid.
func (r *BunSyncCursorRepository) Delete(ctx context.Context, userID, providerType string) error {
	_, err := r.db.NewDelete().
		Model((*domain.SyncCursor)(nil)).
		Where("user_id = ? AND provider_type = ?", userID, providerType).
		Exec(ctx)
	return err
}
