package repository

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/gumeniukcom/contactshq/internal/domain"
)

type BunMergeLogRepository struct {
	db *bun.DB
}

func NewBunMergeLogRepository(db *bun.DB) *BunMergeLogRepository {
	return &BunMergeLogRepository{db: db}
}

func (r *BunMergeLogRepository) Create(ctx context.Context, entry *domain.MergeLogEntry) error {
	if entry.MergedAt.IsZero() {
		entry.MergedAt = time.Now()
	}
	_, err := r.db.NewInsert().Model(entry).Exec(ctx)
	return err
}

// ListByUser returns the newest entries first.
func (r *BunMergeLogRepository) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.MergeLogEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var entries []*domain.MergeLogEntry
	err := r.db.NewSelect().
		Model(&entries).
		Where("user_id = ?", userID).
		Order("merged_at DESC").
		Limit(limit).
		Scan(ctx)
	return entries, err
}

// DeleteOlderThan prunes entries merged before the cutoff, returning how many went.
//
// The snapshots in loser_vcard are the reason this exists: without pruning the table grows
// with a copy of a contact per merge, forever.
func (r *BunMergeLogRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	res, err := r.db.NewDelete().
		Model((*domain.MergeLogEntry)(nil)).
		Where("merged_at < ?", cutoff).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	return int(affected), err
}
