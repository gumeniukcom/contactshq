package repository

import (
	"context"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunSyncRunRepository struct {
	db *bun.DB
}

func NewBunSyncRunRepository(db *bun.DB) *BunSyncRunRepository {
	return &BunSyncRunRepository{db: db}
}

func (r *BunSyncRunRepository) Create(ctx context.Context, run *domain.SyncRun) error {
	_, err := r.db.NewInsert().Model(run).Exec(ctx)
	return err
}

func (r *BunSyncRunRepository) Update(ctx context.Context, run *domain.SyncRun) error {
	_, err := r.db.NewUpdate().Model(run).WherePK().Exec(ctx)
	return err
}

func (r *BunSyncRunRepository) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.SyncRun, error) {
	var runs []*domain.SyncRun
	err := r.db.NewSelect().Model(&runs).
		Where("sr.user_id = ?", userID).
		OrderExpr("sr.started_at DESC").
		Limit(limit).
		Scan(ctx)
	return runs, err
}

func (r *BunSyncRunRepository) ListActiveByUser(ctx context.Context, userID string) ([]*domain.SyncRun, error) {
	var runs []*domain.SyncRun
	err := r.db.NewSelect().Model(&runs).
		Where("sr.user_id = ?", userID).
		Where("sr.status = ?", "running").
		OrderExpr("sr.started_at DESC").
		Scan(ctx)
	return runs, err
}

func (r *BunSyncRunRepository) ListByPipeline(ctx context.Context, userID, pipelineID string, limit int) ([]*domain.SyncRun, error) {
	var runs []*domain.SyncRun
	err := r.db.NewSelect().Model(&runs).
		Where("sr.user_id = ?", userID).
		Where("sr.pipeline_id = ?", pipelineID).
		OrderExpr("sr.started_at DESC").
		Limit(limit).
		Scan(ctx)
	return runs, err
}
