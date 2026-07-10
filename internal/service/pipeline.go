package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
)

var ErrPipelineNotFound = errors.New("pipeline not found")

type PipelineService struct {
	pipelineRepo repository.PipelineRepository
}

func NewPipelineService(pipelineRepo repository.PipelineRepository) *PipelineService {
	return &PipelineService{pipelineRepo: pipelineRepo}
}

type CreatePipelineInput struct {
	Name     string               `json:"name"`
	Enabled  bool                 `json:"enabled"`
	Schedule string               `json:"schedule"`
	Steps    []CreatePipelineStep `json:"steps"`
}

type CreatePipelineStep struct {
	SourceType   string `json:"source_type"`
	SourceConfig string `json:"source_config"`
	DestType     string `json:"dest_type"`
	DestConfig   string `json:"dest_config"`
	ConflictMode string `json:"conflict_mode"`
	Direction    string `json:"direction"`
}

// buildStep normalises the two fields a client may leave blank. Direction was previously
// absent from this struct entirely, so every step silently fell back to the column
// default no matter what the user chose in the form.
func buildStep(pipelineID string, order int, in CreatePipelineStep) *domain.PipelineStep {
	conflictMode := in.ConflictMode
	if conflictMode == "" {
		conflictMode = "source_wins"
	}
	direction := in.Direction
	if direction == "" {
		direction = string(chqsync.SyncModeImport)
	}
	return &domain.PipelineStep{
		ID:           uuid.New().String(),
		PipelineID:   pipelineID,
		Order:        order,
		SourceType:   in.SourceType,
		SourceConfig: in.SourceConfig,
		DestType:     in.DestType,
		DestConfig:   in.DestConfig,
		ConflictMode: conflictMode,
		Direction:    direction,
	}
}

func (s *PipelineService) Create(ctx context.Context, userID string, input CreatePipelineInput) (*domain.Pipeline, error) {
	now := time.Now()
	pipeline := &domain.Pipeline{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      input.Name,
		Enabled:   input.Enabled,
		Schedule:  input.Schedule,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.pipelineRepo.Create(ctx, pipeline); err != nil {
		return nil, err
	}

	for i, stepInput := range input.Steps {
		if err := s.pipelineRepo.CreateStep(ctx, buildStep(pipeline.ID, i+1, stepInput)); err != nil {
			return nil, err
		}
	}

	return s.pipelineRepo.GetByID(ctx, pipeline.ID)
}

func (s *PipelineService) GetByID(ctx context.Context, userID, pipelineID string) (*domain.Pipeline, error) {
	pipeline, err := s.pipelineRepo.GetByID(ctx, pipelineID)
	if err != nil {
		return nil, err
	}
	if pipeline == nil || pipeline.UserID != userID {
		return nil, ErrPipelineNotFound
	}
	return pipeline, nil
}

func (s *PipelineService) List(ctx context.Context, userID string) ([]*domain.Pipeline, error) {
	return s.pipelineRepo.ListByUser(ctx, userID)
}

func (s *PipelineService) Update(ctx context.Context, userID, pipelineID string, input CreatePipelineInput) (*domain.Pipeline, error) {
	pipeline, err := s.GetByID(ctx, userID, pipelineID)
	if err != nil {
		return nil, err
	}

	pipeline.Name = input.Name
	pipeline.Enabled = input.Enabled
	pipeline.Schedule = input.Schedule
	pipeline.UpdatedAt = time.Now()

	if err := s.pipelineRepo.Update(ctx, pipeline); err != nil {
		return nil, err
	}

	// Replace steps
	if err := s.pipelineRepo.DeleteSteps(ctx, pipelineID); err != nil {
		return nil, err
	}

	for i, stepInput := range input.Steps {
		if err := s.pipelineRepo.CreateStep(ctx, buildStep(pipelineID, i+1, stepInput)); err != nil {
			return nil, err
		}
	}

	return s.pipelineRepo.GetByID(ctx, pipelineID)
}

func (s *PipelineService) Delete(ctx context.Context, userID, pipelineID string) error {
	_, err := s.GetByID(ctx, userID, pipelineID)
	if err != nil {
		return err
	}
	return s.pipelineRepo.Delete(ctx, pipelineID)
}
