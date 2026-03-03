package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-co-op/gocron/v2"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"go.uber.org/zap"
)

// PipelinePayload is the job payload for pipeline execution.
type PipelinePayload struct {
	PipelineID string `json:"pipeline_id"`
	UserID     string `json:"user_id"`
}

// BackupPayload is the job payload for backup creation.
type BackupPayload struct {
	UserID string `json:"user_id"`
}

// Scheduler wraps gocron and enqueues jobs via a TaskWorker.
type Scheduler struct {
	s      gocron.Scheduler
	worker TaskWorker
	logger *zap.Logger
}

// NewScheduler creates and returns a new Scheduler backed by gocron.
func NewScheduler(worker TaskWorker, logger *zap.Logger) (*Scheduler, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("create gocron scheduler: %w", err)
	}
	return &Scheduler{s: s, worker: worker, logger: logger}, nil
}

// RegisterPipelines registers a cron job for each enabled pipeline that has a schedule.
func (s *Scheduler) RegisterPipelines(_ context.Context, pipelines []*domain.Pipeline) {
	for _, p := range pipelines {
		if !p.Enabled || p.Schedule == "" {
			continue
		}
		pID := p.ID
		uID := p.UserID
		tag := "pipeline:" + pID
		_, err := s.s.NewJob(
			gocron.CronJob(p.Schedule, false),
			gocron.NewTask(func(id, userID string) {
				payload, _ := json.Marshal(PipelinePayload{PipelineID: id, UserID: userID})
				if err := s.worker.Enqueue(context.Background(), "pipeline", json.RawMessage(payload)); err != nil {
					s.logger.Error("failed to enqueue pipeline job", zap.String("pipeline_id", id), zap.Error(err))
				}
			}, pID, uID),
			gocron.WithName(tag),
			gocron.WithTags(tag),
		)
		if err != nil {
			s.logger.Error("failed to register pipeline job", zap.String("pipeline_id", pID), zap.Error(err))
			continue
		}
		s.logger.Info("registered pipeline job", zap.String("pipeline_id", pID), zap.String("schedule", p.Schedule))
	}
}

// RegisterBackupForUser registers a cron job that creates a backup for the given user.
func (s *Scheduler) RegisterBackupForUser(schedule, userID string) {
	tag := "backup:" + userID
	_, err := s.s.NewJob(
		gocron.CronJob(schedule, false),
		gocron.NewTask(func(uid string) {
			payload, _ := json.Marshal(BackupPayload{UserID: uid})
			if err := s.worker.Enqueue(context.Background(), "backup", json.RawMessage(payload)); err != nil {
				s.logger.Error("failed to enqueue backup job", zap.String("user_id", uid), zap.Error(err))
			}
		}, userID),
		gocron.WithName(tag),
		gocron.WithTags(tag),
	)
	if err != nil {
		s.logger.Error("failed to register backup job", zap.String("user_id", userID), zap.Error(err))
		return
	}
	s.logger.Info("registered backup job", zap.String("user_id", userID), zap.String("schedule", schedule))
}

// RemovePipelineJob removes a scheduled pipeline job by pipeline ID.
func (s *Scheduler) RemovePipelineJob(pipelineID string) {
	s.s.RemoveByTags("pipeline:" + pipelineID)
}

// RemoveBackupForUser removes any scheduled backup job for the given user.
func (s *Scheduler) RemoveBackupForUser(userID string) {
	s.s.RemoveByTags("backup:" + userID)
}

// ReregisterBackupForUser removes the existing backup job for the user and,
// if schedule is non-empty, registers a new one. Passing an empty schedule
// effectively disables the scheduled backup for that user.
func (s *Scheduler) ReregisterBackupForUser(schedule, userID string) {
	s.RemoveBackupForUser(userID)
	if schedule != "" {
		s.RegisterBackupForUser(schedule, userID)
	}
}

// Start begins executing scheduled jobs.
func (s *Scheduler) Start() {
	s.s.Start()
}

// Stop shuts down the scheduler gracefully.
func (s *Scheduler) Stop() {
	_ = s.s.Shutdown()
}
