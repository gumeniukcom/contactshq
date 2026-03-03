package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
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
		conflictMode := stepInput.ConflictMode
		if conflictMode == "" {
			conflictMode = "source_wins"
		}
		step := &domain.PipelineStep{
			ID:           uuid.New().String(),
			PipelineID:   pipeline.ID,
			Order:        i + 1,
			SourceType:   stepInput.SourceType,
			SourceConfig: stepInput.SourceConfig,
			DestType:     stepInput.DestType,
			DestConfig:   stepInput.DestConfig,
			ConflictMode: conflictMode,
		}
		if err := s.pipelineRepo.CreateStep(ctx, step); err != nil {
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
		conflictMode := stepInput.ConflictMode
		if conflictMode == "" {
			conflictMode = "source_wins"
		}
		step := &domain.PipelineStep{
			ID:           uuid.New().String(),
			PipelineID:   pipelineID,
			Order:        i + 1,
			SourceType:   stepInput.SourceType,
			SourceConfig: stepInput.SourceConfig,
			DestType:     stepInput.DestType,
			DestConfig:   stepInput.DestConfig,
			ConflictMode: conflictMode,
		}
		if err := s.pipelineRepo.CreateStep(ctx, step); err != nil {
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
