package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
)

type BackupJobPayload struct {
	UserID string `json:"user_id"`
	// Trigger distinguishes the nightly run from a catch-up started at boot. Empty means
	// scheduled, so payloads written before this field existed still read correctly.
	Trigger string `json:"trigger,omitempty"`
}

type BackupJobHandler struct {
	backupService BackupCreator
	logger        *zap.Logger
}

func NewBackupJobHandler(backupService BackupCreator, logger *zap.Logger) *BackupJobHandler {
	return &BackupJobHandler{backupService: backupService, logger: logger}
}

func (h *BackupJobHandler) Handle(ctx context.Context, payload json.RawMessage) error {
	var p BackupJobPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("unmarshal backup job payload: %w", err)
	}

	h.logger.Info("running backup job", zap.String("user_id", p.UserID))
	trigger := p.Trigger
	if trigger == "" {
		trigger = domain.BackupTriggerScheduled
	}
	if _, err := h.backupService.CreateWithTrigger(ctx, p.UserID, trigger); err != nil {
		return fmt.Errorf("create backup for user %s: %w", p.UserID, err)
	}

	return nil
}
