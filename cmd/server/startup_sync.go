package main

import (
	"context"
	"time"

	"github.com/gumeniukcom/contactshq/internal/repository"
	"go.uber.org/zap"
)

// Pipeline run-history pruning belongs to the sync domain (spec 006); the interrupted-run
// reconciliation and missed-backup catch-up beside it belong to backup (spec 005). Splitting
// the seam lets each spec own its half — see constitution Principle VII.

// pruneSyncRuns trims pipeline history. Unlike backup_runs — about one row a day — this table
// gains a row per pipeline execution and grows without bound.
func pruneSyncRuns(ctx context.Context, repo repository.SyncRunRepository, retentionDays int, logger *zap.Logger) {
	if repo == nil || retentionDays <= 0 {
		return
	}
	removed, err := repo.DeleteOlderThan(ctx, time.Now().AddDate(0, 0, -retentionDays))
	if err != nil {
		logger.Warn("failed to prune sync run history", zap.Error(err))
		return
	}
	if removed > 0 {
		logger.Info("pruned sync run history",
			zap.Int("removed", removed), zap.Int("retention_days", retentionDays))
	}
}
