package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"github.com/gumeniukcom/contactshq/internal/domain"
)

type BunBackupRunRepository struct {
	db *bun.DB
}

func NewBunBackupRunRepository(db *bun.DB) *BunBackupRunRepository {
	return &BunBackupRunRepository{db: db}
}

func (r *BunBackupRunRepository) Create(ctx context.Context, run *domain.BackupRun) error {
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now()
	}
	_, err := r.db.NewInsert().Model(run).Exec(ctx)
	return err
}

func (r *BunBackupRunRepository) Update(ctx context.Context, run *domain.BackupRun) error {
	_, err := r.db.NewUpdate().Model(run).WherePK().Exec(ctx)
	return err
}

// ListByUser returns the newest runs first.
func (r *BunBackupRunRepository) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.BackupRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var runs []*domain.BackupRun
	err := r.db.NewSelect().
		Model(&runs).
		Where("user_id = ?", userID).
		Order("started_at DESC").
		Limit(limit).
		Scan(ctx)
	return runs, err
}

// LastSuccess returns the most recent run that actually produced a backup.
//
// Only 'completed' counts: a failed or still-running row says nothing about whether the data
// is safe, and treating either as a success is precisely the lie this table exists to end.
func (r *BunBackupRunRepository) LastSuccess(ctx context.Context, userID string) (*domain.BackupRun, error) {
	run := new(domain.BackupRun)
	err := r.db.NewSelect().
		Model(run).
		Where("user_id = ?", userID).
		Where("status = ?", domain.BackupRunOK).
		Order("started_at DESC").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

// LastRun returns the most recent run whatever its outcome.
func (r *BunBackupRunRepository) LastRun(ctx context.Context, userID string) (*domain.BackupRun, error) {
	run := new(domain.BackupRun)
	err := r.db.NewSelect().
		Model(run).
		Where("user_id = ?", userID).
		Order("started_at DESC").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

// MarkStaleInterrupted closes rows left 'running' by a process that died.
//
// startedBefore is the moment this process started: a row created after it belongs to a run
// happening right now. Without that bound, a second instance — a rolling deploy, a second
// replica on PostgreSQL — would mark its neighbour's live runs as interrupted, and the history
// would start lying in exactly the direction it was introduced to stop.
func (r *BunBackupRunRepository) MarkStaleInterrupted(ctx context.Context, startedBefore time.Time) (int, error) {
	now := time.Now()
	res, err := r.db.NewUpdate().
		Model((*domain.BackupRun)(nil)).
		Set("status = ?", domain.BackupRunInterrupted).
		Set("finished_at = ?", now).
		Set("error_message = ?", "server restarted").
		Where("status = ?", domain.BackupRunRunning).
		Where("started_at < ?", startedBefore).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	return int(affected), err
}
