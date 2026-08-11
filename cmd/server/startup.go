package main

import (
	"context"
	"time"

	"github.com/adhocore/gronx"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	"github.com/gumeniukcom/contactshq/internal/worker"
)

// reconcileInterruptedRuns closes history rows left open by a process that died.
//
// Without it a `docker kill` leaves backup and sync runs sitting at "running" forever, and
// the history — introduced precisely to answer "did last night's backup work?" — starts
// answering "it is still going", indefinitely.
//
// Bounded to runs that began before this process did. That makes it safe on the supported
// single-instance deployment and harmless rather than destructive if someone runs two: a
// blind UPDATE would mark the neighbour's live runs as interrupted. Proper leases with a
// heartbeat are the answer for a multi-instance setup, which this project does not support;
// the condition for revisiting is a second replica.
func reconcileInterruptedRuns(
	ctx context.Context,
	backupRuns repository.BackupRunRepository,
	syncRuns repository.SyncRunRepository,
	startedAt time.Time,
	logger *zap.Logger,
) {
	if backupRuns != nil {
		if n, err := backupRuns.MarkStaleInterrupted(ctx, startedAt); err != nil {
			logger.Warn("failed to reconcile interrupted backup runs", zap.Error(err))
		} else if n > 0 {
			logger.Info("closed interrupted backup runs", zap.Int("count", n))
		}
	}

	if syncRuns != nil {
		if n, err := syncRuns.MarkStaleInterrupted(ctx, startedAt); err != nil {
			logger.Warn("failed to reconcile interrupted sync runs", zap.Error(err))
		} else if n > 0 {
			logger.Info("closed interrupted sync runs", zap.Int("count", n))
		}
	}
}

// catchUpMissedBackups queues one backup per user whose last success is older than their
// schedule allows.
//
// This is what replaces a durable job queue for the one case that actually costs something:
// the container is killed overnight, the scheduled backup never runs, and nobody finds out
// until they need the backup. A cron schedule alone does not cover it — the next fire is
// tomorrow, and the day's backup is simply gone.
//
// At most one catch-up per user per start, so a server restarted repeatedly does not queue a
// backlog of identical work.
func catchUpMissedBackups(
	ctx context.Context,
	users []string,
	backupSvc *service.BackupService,
	runs repository.BackupRunRepository,
	w worker.TaskWorker,
	logger *zap.Logger,
) {
	if runs == nil || backupSvc == nil || w == nil {
		return
	}

	now := time.Now()
	for _, userID := range users {
		schedule, err := backupSvc.GetUserSchedule(ctx, userID)
		if err != nil {
			logger.Warn("failed to read backup schedule for catch-up",
				zap.String("user_id", userID), zap.Error(err))
			continue
		}
		if schedule == "" {
			continue // backups are off for this user; nothing was missed
		}

		period, ok := schedulePeriod(schedule, now)
		if !ok {
			continue
		}

		last, err := runs.LastSuccess(ctx, userID)
		if err != nil {
			logger.Warn("failed to read last successful backup",
				zap.String("user_id", userID), zap.Error(err))
			continue
		}

		// Never backed up counts as overdue: the schedule is on, so the user asked for one.
		age := period + time.Hour
		if last != nil {
			age = now.Sub(last.StartedAt)
		}
		if age <= period {
			continue
		}

		if err := enqueueCatchupBackup(ctx, w, userID); err != nil {
			logger.Warn("failed to queue a catch-up backup",
				zap.String("user_id", userID), zap.Error(err))
			continue
		}

		logger.Info("queued a catch-up backup",
			zap.String("user_id", userID),
			zap.String("schedule", schedule),
			zap.Duration("since_last_success", age.Round(time.Minute)),
			zap.Duration("schedule_period", period))
	}
}

// schedulePeriod estimates the gap between two firings of a cron expression by asking for the
// next two and measuring. Cheaper than special-casing every schedule shape, and it stays
// correct if the expressions grow more elaborate.
func schedulePeriod(schedule string, from time.Time) (time.Duration, bool) {
	first, err := gronx.NextTickAfter(schedule, from, false)
	if err != nil {
		return 0, false
	}
	second, err := gronx.NextTickAfter(schedule, first, false)
	if err != nil {
		return 0, false
	}
	period := second.Sub(first)
	if period <= 0 {
		return 0, false
	}
	return period, true
}

func enqueueCatchupBackup(ctx context.Context, w worker.TaskWorker, userID string) error {
	return w.Enqueue(ctx, "backup", jobsBackupPayload{
		UserID:  userID,
		Trigger: domain.BackupTriggerCatchup,
	})
}

// jobsBackupPayload mirrors jobs.BackupJobPayload. Declared here to keep cmd from depending
// on the jobs package for a two-field struct it only ever marshals.
type jobsBackupPayload struct {
	UserID  string `json:"user_id"`
	Trigger string `json:"trigger,omitempty"`
}
