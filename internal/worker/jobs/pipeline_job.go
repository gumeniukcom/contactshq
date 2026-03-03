package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gumeniukcom/contactshq/internal/repository"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
	"go.uber.org/zap"
)

type PipelineJobPayload struct {
	PipelineID string `json:"pipeline_id"`
	UserID     string `json:"user_id"`
}

type PipelineJobHandler struct {
	orchestrator *chqsync.PipelineOrchestrator
	pipelineRepo repository.PipelineRepository
	logger       *zap.Logger
}

func NewPipelineJobHandler(orchestrator *chqsync.PipelineOrchestrator, pipelineRepo repository.PipelineRepository, logger *zap.Logger) *PipelineJobHandler {
	return &PipelineJobHandler{
		orchestrator: orchestrator,
		pipelineRepo: pipelineRepo,
		logger:       logger,
	}
}

func (h *PipelineJobHandler) Handle(ctx context.Context, payload json.RawMessage) error {
	var p PipelineJobPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}

	pipeline, err := h.pipelineRepo.GetByID(ctx, p.PipelineID)
	if err != nil {
		return fmt.Errorf("get pipeline: %w", err)
	}
	if pipeline == nil {
		return fmt.Errorf("pipeline not found: %s", p.PipelineID)
	}

	results, err := h.orchestrator.Execute(ctx, p.UserID, pipeline)
	if err != nil {
		return fmt.Errorf("execute pipeline: %w", err)
	}

	for _, r := range results {
		if r.Error != "" {
			h.logger.Error("pipeline step error",
				zap.String("pipeline_id", p.PipelineID),
				zap.Int("step", r.StepOrder),
				zap.String("error", r.Error),
			)
		} else if r.Result != nil {
			h.logger.Info("pipeline step completed",
				zap.String("pipeline_id", p.PipelineID),
				zap.Int("step", r.StepOrder),
				zap.Int("created", r.Result.Created),
				zap.Int("updated", r.Result.Updated),
				zap.Int("deleted", r.Result.Deleted),
			)
		}
	}

	return nil
}
