package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gumeniukcom/contactshq/internal/service"
	"go.uber.org/zap"
)

type DedupJobPayload struct {
	UserID string `json:"user_id"`
}

type DedupJobHandler struct {
	detector *service.DuplicateDetector
	logger   *zap.Logger
}

func NewDedupJobHandler(detector *service.DuplicateDetector, logger *zap.Logger) *DedupJobHandler {
	return &DedupJobHandler{detector: detector, logger: logger}
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
	return nil
}
