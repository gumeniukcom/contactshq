package repository

import (
	"context"
	"time"

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

// MarkStaleInterrupted closes sync runs left open by a process that died.
//
// Bounded to runs started before this process did: without that, a second instance would mark
// its neighbour's live runs as interrupted. See BunBackupRunRepository.MarkStaleInterrupted —
// this is the same reasoning, applied to the other history table.
func (r *BunSyncRunRepository) MarkStaleInterrupted(ctx context.Context, startedBefore time.Time) (int, error) {
	res, err := r.db.NewUpdate().
		Model((*domain.SyncRun)(nil)).
		Set("status = ?", "interrupted").
		Set("finished_at = ?", time.Now()).
		Set("error_message = ?", "server restarted").
		Where("status = ?", "running").
		Where("started_at < ?", startedBefore).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	return int(affected), err
}

// DeleteOlderThan prunes finished runs. Unlike backup_runs — roughly one row a day — this
// table gains a row per pipeline execution and grows without bound.
func (r *BunSyncRunRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	res, err := r.db.NewDelete().
		Model((*domain.SyncRun)(nil)).
		Where("started_at < ?", cutoff).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	return int(affected), err
}
