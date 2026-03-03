package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gumeniukcom/contactshq/internal/service"
	"go.uber.org/zap"
)

type BackupJobPayload struct {
	UserID string `json:"user_id"`
}

type BackupJobHandler struct {
	backupService *service.BackupService
	logger        *zap.Logger
}

func NewBackupJobHandler(backupService *service.BackupService, logger *zap.Logger) *BackupJobHandler {
	return &BackupJobHandler{backupService: backupService, logger: logger}
}

func (h *BackupJobHandler) Handle(ctx context.Context, payload json.RawMessage) error {
	var p BackupJobPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("unmarshal backup job payload: %w", err)
	}

	h.logger.Info("running backup job", zap.String("user_id", p.UserID))
	if _, err := h.backupService.Create(ctx, p.UserID); err != nil {
		return fmt.Errorf("create backup for user %s: %w", p.UserID, err)
	}

	return nil
}
