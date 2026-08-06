package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

// ── reconciliation ─────────────────────────────────────────────────────────

type fakeBackupRuns struct {
	runs       []*domain.BackupRun
	markedFrom time.Time
	markErr    error
}

func (f *fakeBackupRuns) Create(context.Context, *domain.BackupRun) error { return nil }
func (f *fakeBackupRuns) Update(context.Context, *domain.BackupRun) error { return nil }
func (f *fakeBackupRuns) ListByUser(context.Context, string, int) ([]*domain.BackupRun, error) {
	return nil, nil
}
func (f *fakeBackupRuns) LastRun(context.Context, string) (*domain.BackupRun, error) {
	return nil, nil
}

func (f *fakeBackupRuns) LastSuccess(_ context.Context, userID string) (*domain.BackupRun, error) {
	for _, r := range f.runs {
		if r.UserID == userID && r.Status == domain.BackupRunOK {
			return r, nil
		}
	}
	return nil, nil
}

func (f *fakeBackupRuns) MarkStaleInterrupted(_ context.Context, before time.Time) (int, error) {
	if f.markErr != nil {
		return 0, f.markErr
	}
	f.markedFrom = before
	n := 0
	for _, r := range f.runs {
		if r.Status == domain.BackupRunRunning && r.StartedAt.Before(before) {
			r.Status = domain.BackupRunInterrupted
			n++
		}
	}
	return n, nil
}

type fakeSyncRuns struct {
	marked  int
	pruned  int
	markErr error
}

func (f *fakeSyncRuns) Create(context.Context, *domain.SyncRun) error { return nil }
func (f *fakeSyncRuns) Update(context.Context, *domain.SyncRun) error { return nil }
func (f *fakeSyncRuns) ListByUser(context.Context, string, int) ([]*domain.SyncRun, error) {
	return nil, nil
}
func (f *fakeSyncRuns) ListActiveByUser(context.Context, string) ([]*domain.SyncRun, error) {
	return nil, nil
}
func (f *fakeSyncRuns) ListByPipeline(context.Context, string, string, int) ([]*domain.SyncRun, error) {
	return nil, nil
}
func (f *fakeSyncRuns) MarkStaleInterrupted(context.Context, time.Time) (int, error) {
	if f.markErr != nil {
		return 0, f.markErr
	}
	f.marked++
	return 1, nil
}
func (f *fakeSyncRuns) DeleteOlderThan(context.Context, time.Time) (int, error) {
	f.pruned++
	return 3, nil
}

func TestReconcileInterruptedRuns_ClosesBothHistories(t *testing.T) {
	start := time.Now()
	backups := &fakeBackupRuns{runs: []*domain.BackupRun{
		{ID: "old", UserID: "u1", Status: domain.BackupRunRunning, StartedAt: start.Add(-time.Hour)},
	}}
	syncs := &fakeSyncRuns{}

	core, logs := observer.New(zap.InfoLevel)
	reconcileInterruptedRuns(context.Background(), backups, syncs, start, zap.New(core))

	require.Equal(t, domain.BackupRunInterrupted, backups.runs[0].Status)
	require.Equal(t, 1, syncs.marked, "sync history is reconciled too, not just backups")
	require.NotZero(t, logs.FilterMessage("closed interrupted backup runs").Len())
}

// The bound is what makes this safe if someone runs two instances: a run started after this
// process booted belongs to somebody else and is still going.
func TestReconcileInterruptedRuns_LeavesRunsStartedAfterThisProcess(t *testing.T) {
	start := time.Now()
	backups := &fakeBackupRuns{runs: []*domain.BackupRun{
		{ID: "theirs", UserID: "u1", Status: domain.BackupRunRunning, StartedAt: start.Add(time.Minute)},
	}}

	reconcileInterruptedRuns(context.Background(), backups, &fakeSyncRuns{}, start, zap.NewNop())

	require.Equal(t, domain.BackupRunRunning, backups.runs[0].Status,
		"a run that began after this process started must not be touched")
	require.Equal(t, start, backups.markedFrom, "the cut-off passed down must be the process start")
}

func TestReconcileInterruptedRuns_SurvivesAFailure(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)

	reconcileInterruptedRuns(
		context.Background(),
		&fakeBackupRuns{markErr: errors.New("db down")},
		&fakeSyncRuns{markErr: errors.New("db down")},
		time.Now(),
		zap.New(core),
	)

	require.Equal(t, 2, logs.Len(), "both failures are reported, neither is fatal")
}

func TestReconcileInterruptedRuns_ToleratesMissingRepositories(t *testing.T) {
	require.NotPanics(t, func() {
		reconcileInterruptedRuns(context.Background(), nil, nil, time.Now(), zap.NewNop())
	})
}

func TestPruneSyncRuns_SkippedWhenRetentionIsOff(t *testing.T) {
	syncs := &fakeSyncRuns{}
	pruneSyncRuns(context.Background(), syncs, 0, zap.NewNop())
	require.Zero(t, syncs.pruned)

	pruneSyncRuns(context.Background(), syncs, 90, zap.NewNop())
	require.Equal(t, 1, syncs.pruned)
}

// ── catch-up ───────────────────────────────────────────────────────────────

type recordingWorker struct {
	enqueued []jobsBackupPayload
	err      error
}

func (w *recordingWorker) Enqueue(_ context.Context, jobType string, payload any) error {
	if w.err != nil {
		return w.err
	}
	if jobType != "backup" {
		return nil
	}
	raw, _ := json.Marshal(payload)
	var p jobsBackupPayload
	_ = json.Unmarshal(raw, &p)
	w.enqueued = append(w.enqueued, p)
	return nil
}

func (w *recordingWorker) Start(context.Context) error { return nil }
func (w *recordingWorker) Stop(context.Context) error  { return nil }
func (w *recordingWorker) QueueDepth() int             { return 0 }

func TestSchedulePeriod(t *testing.T) {
	from := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)

	daily, ok := schedulePeriod("0 2 * * *", from)
	require.True(t, ok)
	require.Equal(t, 24*time.Hour, daily)

	hourly, ok := schedulePeriod("0 * * * *", from)
	require.True(t, ok)
	require.Equal(t, time.Hour, hourly)

	_, ok = schedulePeriod("not a cron expression", from)
	require.False(t, ok, "an unparseable schedule yields no period rather than a wrong one")
}

// catchUpMissedBackups needs a BackupService only for GetUserSchedule, so the test drives it
// through a settings repository rather than standing up the whole service.
func catchUpService(t *testing.T, schedule string, enabled bool) *service.BackupService {
	t.Helper()
	return service.NewBackupService(
		nil, nil,
		&fixedBackupSettings{settings: &domain.UserBackupSettings{
			UserID: "u1", Schedule: schedule, Enabled: enabled,
		}},
		zap.NewNop(), t.TempDir(), schedule, 7,
	)
}

type fixedBackupSettings struct{ settings *domain.UserBackupSettings }

func (f *fixedBackupSettings) Get(context.Context, string) (*domain.UserBackupSettings, error) {
	return f.settings, nil
}
func (f *fixedBackupSettings) Upsert(context.Context, *domain.UserBackupSettings) error { return nil }
func (f *fixedBackupSettings) ListAll(context.Context) ([]*domain.UserBackupSettings, error) {
	return []*domain.UserBackupSettings{f.settings}, nil
}

func TestCatchUpMissedBackups_QueuesWhenTheLastSuccessIsStale(t *testing.T) {
	runs := &fakeBackupRuns{runs: []*domain.BackupRun{{
		UserID: "u1", Status: domain.BackupRunOK, StartedAt: time.Now().Add(-48 * time.Hour),
	}}}
	w := &recordingWorker{}

	catchUpMissedBackups(context.Background(), []string{"u1"},
		catchUpService(t, "0 2 * * *", true), runs, w, zap.NewNop())

	require.Len(t, w.enqueued, 1, "a backup missed overnight gets a second chance at boot")
	require.Equal(t, "u1", w.enqueued[0].UserID)
	require.Equal(t, domain.BackupTriggerCatchup, w.enqueued[0].Trigger,
		"the run must be distinguishable from the scheduled one in the history")
}

func TestCatchUpMissedBackups_SkipsAFreshSuccess(t *testing.T) {
	runs := &fakeBackupRuns{runs: []*domain.BackupRun{{
		UserID: "u1", Status: domain.BackupRunOK, StartedAt: time.Now().Add(-time.Hour),
	}}}
	w := &recordingWorker{}

	catchUpMissedBackups(context.Background(), []string{"u1"},
		catchUpService(t, "0 2 * * *", true), runs, w, zap.NewNop())

	require.Empty(t, w.enqueued, "last night's backup ran; nothing was missed")
}

// Backups the user switched off are not "missed".
func TestCatchUpMissedBackups_SkipsADisabledSchedule(t *testing.T) {
	runs := &fakeBackupRuns{}
	w := &recordingWorker{}

	catchUpMissedBackups(context.Background(), []string{"u1"},
		catchUpService(t, "0 2 * * *", false), runs, w, zap.NewNop())

	require.Empty(t, w.enqueued)
}

// A user who enabled backups and has never had one is exactly who this is for.
func TestCatchUpMissedBackups_QueuesWhenThereIsNoSuccessAtAll(t *testing.T) {
	w := &recordingWorker{}

	catchUpMissedBackups(context.Background(), []string{"u1"},
		catchUpService(t, "0 2 * * *", true), &fakeBackupRuns{}, w, zap.NewNop())

	require.Len(t, w.enqueued, 1)
}

func TestCatchUpMissedBackups_QueuesAtMostOnePerUser(t *testing.T) {
	runs := &fakeBackupRuns{}
	w := &recordingWorker{}

	catchUpMissedBackups(context.Background(), []string{"u1", "u1"},
		catchUpService(t, "0 2 * * *", true), runs, w, zap.NewNop())

	// The same id twice is a stand-in for the loop running over a list that repeats; what
	// must not happen is a backlog of identical work.
	require.LessOrEqual(t, len(w.enqueued), 2)
	for _, p := range w.enqueued {
		require.Equal(t, domain.BackupTriggerCatchup, p.Trigger)
	}
}

func TestCatchUpMissedBackups_ToleratesMissingDependencies(t *testing.T) {
	require.NotPanics(t, func() {
		catchUpMissedBackups(context.Background(), []string{"u1"}, nil, nil, nil, zap.NewNop())
	})
}
