package repository

import (
	"context"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunSyncConflictRepository struct {
	db *bun.DB
}

func NewBunSyncConflictRepository(db *bun.DB) *BunSyncConflictRepository {
	return &BunSyncConflictRepository{db: db}
}

func (r *BunSyncConflictRepository) Create(ctx context.Context, c *domain.SyncConflict) error {
	_, err := r.db.NewInsert().Model(c).Exec(ctx)
	return err
}

func (r *BunSyncConflictRepository) GetByID(ctx context.Context, id string) (*domain.SyncConflict, error) {
	c := new(domain.SyncConflict)
	err := r.db.NewSelect().Model(c).Where("sc.id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *BunSyncConflictRepository) ListByUser(ctx context.Context, userID, status string, limit, offset int) ([]*domain.SyncConflict, int, error) {
	var conflicts []*domain.SyncConflict
	q := r.db.NewSelect().Model(&conflicts).Where("sc.user_id = ?", userID)
	if status != "" {
		q = q.Where("sc.status = ?", status)
	}
	total, err := q.OrderExpr("sc.created_at DESC").
		Limit(limit).Offset(offset).
		ScanAndCount(ctx)
	return conflicts, total, err
}

func (r *BunSyncConflictRepository) ListPendingByProvider(ctx context.Context, userID, providerType string) ([]*domain.SyncConflict, error) {
	var conflicts []*domain.SyncConflict
	err := r.db.NewSelect().Model(&conflicts).
		Where("sc.user_id = ?", userID).
		Where("sc.provider_type = ?", providerType).
		Where("sc.status = ?", "pending").
		OrderExpr("sc.created_at DESC").
		Scan(ctx)
	return conflicts, err
}

func (r *BunSyncConflictRepository) Update(ctx context.Context, c *domain.SyncConflict) error {
	_, err := r.db.NewUpdate().Model(c).WherePK().Exec(ctx)
	return err
}

func (r *BunSyncConflictRepository) DeleteByProvider(ctx context.Context, userID, providerType string) error {
	_, err := r.db.NewDelete().Model((*domain.SyncConflict)(nil)).
		Where("user_id = ?", userID).
		Where("provider_type = ?", providerType).
		Exec(ctx)
	return err
}

func (r *BunSyncConflictRepository) CountPending(ctx context.Context, userID string) (int, error) {
	count, err := r.db.NewSelect().Model((*domain.SyncConflict)(nil)).
		Where("user_id = ?", userID).
		Where("status = ?", "pending").
		Count(ctx)
	return count, err
}
