package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/repository"
)

type DedupJobPayload struct {
	UserID string `json:"user_id"`
}

type DedupJobHandler struct {
	detector DuplicateScanner
	logger   *zap.Logger

	// mergeLogRepo and retentionDays prune the merge history. It rides along with this job
	// because merge_log belongs to the duplicates feature and this is the feature's only
	// recurring work; PruneMergeLog is also called once at startup so retention still
	// happens on an instance with no dedup schedule.
	mergeLogRepo  repository.MergeLogRepository
	retentionDays int
}

func NewDedupJobHandler(detector DuplicateScanner, logger *zap.Logger) *DedupJobHandler {
	return &DedupJobHandler{detector: detector, logger: logger}
}

// WithMergeLogRetention enables pruning of merge history older than retentionDays.
func (h *DedupJobHandler) WithMergeLogRetention(repo repository.MergeLogRepository, retentionDays int) *DedupJobHandler {
	h.mergeLogRepo = repo
	h.retentionDays = retentionDays
	return h
}

// PruneMergeLog removes merge records past the retention window.
//
// Every record carries a snapshot of the discarded card, so an unpruned table grows by a
// contact per merge and never shrinks.
func PruneMergeLog(ctx context.Context, repo repository.MergeLogRepository, retentionDays int, logger *zap.Logger) {
	if repo == nil || retentionDays <= 0 {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	removed, err := repo.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		logger.Warn("failed to prune the merge log", zap.Error(err))
		return
	}
	if removed > 0 {
		logger.Info("pruned merge log",
			zap.Int("removed", removed), zap.Int("retention_days", retentionDays))
	}
}

func (h *DedupJobHandler) Handle(ctx context.Context, payload json.RawMessage) error {
	var p DedupJobPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("unmarshal dedup job payload: %w", err)
	}

	h.logger.Info("running dedup job", zap.String("user_id", p.UserID))
	result, err := h.detector.Detect(ctx, p.UserID)
	if err != nil {
		return fmt.Errorf("dedup for user %s: %w", p.UserID, err)
	}

	h.logger.Info("dedup job completed",
		zap.String("user_id", p.UserID),
		zap.Int("found", result.Found),
		zap.Int("checked", result.Checked),
	)

	PruneMergeLog(ctx, h.mergeLogRepo, h.retentionDays, h.logger)
	return nil
}
