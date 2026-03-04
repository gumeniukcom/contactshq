package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/go-co-op/gocron/v2"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/robfig/cron/v3"
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

// DedupPayload is the job payload for duplicate detection.
type DedupPayload struct {
	UserID string `json:"user_id"`
}

// ValidateCron checks whether expr is a valid 5-field cron expression.
func ValidateCron(expr string) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	if err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return nil
}

// Scheduler wraps gocron and enqueues jobs via a TaskWorker.
type Scheduler struct {
	mu     sync.Mutex
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
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range pipelines {
		s.registerPipelineJobLocked(p)
	}
}

// RegisterPipelineJob registers a cron job for a single pipeline (if enabled and has schedule).
func (s *Scheduler) RegisterPipelineJob(p *domain.Pipeline) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registerPipelineJobLocked(p)
}

func (s *Scheduler) registerPipelineJobLocked(p *domain.Pipeline) {
	if !p.Enabled || p.Schedule == "" {
		return
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
		return
	}
	s.logger.Info("registered pipeline job", zap.String("pipeline_id", pID), zap.String("schedule", p.Schedule))
}

// ReregisterPipelineJob removes and re-registers the scheduled job for a pipeline.
func (s *Scheduler) ReregisterPipelineJob(p *domain.Pipeline) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s.RemoveByTags("pipeline:" + p.ID)
	s.registerPipelineJobLocked(p)
}

// RegisterBackupForUser registers a cron job that creates a backup for the given user.
func (s *Scheduler) RegisterBackupForUser(schedule, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registerBackupForUserLocked(schedule, userID)
}

func (s *Scheduler) registerBackupForUserLocked(schedule, userID string) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s.RemoveByTags("pipeline:" + pipelineID)
}

// RemoveBackupForUser removes any scheduled backup job for the given user.
func (s *Scheduler) RemoveBackupForUser(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s.RemoveByTags("backup:" + userID)
}

// ReregisterBackupForUser removes the existing backup job for the user and,
// if schedule is non-empty, registers a new one. Passing an empty schedule
// effectively disables the scheduled backup for that user.
func (s *Scheduler) ReregisterBackupForUser(schedule, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s.RemoveByTags("backup:" + userID)
	if schedule != "" {
		s.registerBackupForUserLocked(schedule, userID)
	}
}

// RegisterDedupForUser registers a cron job that runs duplicate detection for the given user.
func (s *Scheduler) RegisterDedupForUser(schedule, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registerDedupForUserLocked(schedule, userID)
}

func (s *Scheduler) registerDedupForUserLocked(schedule, userID string) {
	tag := "dedup:" + userID
	_, err := s.s.NewJob(
		gocron.CronJob(schedule, false),
		gocron.NewTask(func(uid string) {
			payload, _ := json.Marshal(DedupPayload{UserID: uid})
			if err := s.worker.Enqueue(context.Background(), "dedup", json.RawMessage(payload)); err != nil {
				s.logger.Error("failed to enqueue dedup job", zap.String("user_id", uid), zap.Error(err))
			}
		}, userID),
		gocron.WithName(tag),
		gocron.WithTags(tag),
	)
	if err != nil {
		s.logger.Error("failed to register dedup job", zap.String("user_id", userID), zap.Error(err))
		return
	}
	s.logger.Info("registered dedup job", zap.String("user_id", userID), zap.String("schedule", schedule))
}

// RemoveDedupForUser removes any scheduled dedup job for the given user.
func (s *Scheduler) RemoveDedupForUser(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s.RemoveByTags("dedup:" + userID)
}

// ReregisterDedupForUser removes the existing dedup job and, if schedule is non-empty, registers a new one.
func (s *Scheduler) ReregisterDedupForUser(schedule, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s.RemoveByTags("dedup:" + userID)
	if schedule != "" {
		s.registerDedupForUserLocked(schedule, userID)
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
