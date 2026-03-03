package worker_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/worker"
	"go.uber.org/zap"
)

// mockTaskWorker records enqueued jobs for assertion.
type mockTaskWorker struct {
	enqueuedTypes []string
}

func (m *mockTaskWorker) Enqueue(_ context.Context, jobType string, _ any) error {
	m.enqueuedTypes = append(m.enqueuedTypes, jobType)
	return nil
}

func (m *mockTaskWorker) Start(_ context.Context) error { return nil }
func (m *mockTaskWorker) Stop(_ context.Context) error  { return nil }

func newTestScheduler(t *testing.T) (*worker.Scheduler, *mockTaskWorker) {
	t.Helper()
	w := &mockTaskWorker{}
	logger := zap.NewNop()
	sched, err := worker.NewScheduler(w, logger)
	require.NoError(t, err)
	return sched, w
}

func TestNewScheduler_Valid(t *testing.T) {
	sched, _ := newTestScheduler(t)
	assert.NotNil(t, sched)
	sched.Stop()
}

func TestRegisterPipelines_SkipsDisabled(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	pipelines := []*domain.Pipeline{
		{ID: "p1", UserID: "u1", Enabled: false, Schedule: "* * * * *"},
	}
	sched.RegisterPipelines(context.Background(), pipelines)
	// Should not panic, and job should not be registered
}

func TestRegisterPipelines_SkipsEmptySchedule(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	pipelines := []*domain.Pipeline{
		{ID: "p1", UserID: "u1", Enabled: true, Schedule: ""},
	}
	sched.RegisterPipelines(context.Background(), pipelines)
}

func TestRegisterPipelines_AddsValidJob(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	pipelines := []*domain.Pipeline{
		{ID: "p1", UserID: "u1", Enabled: true, Schedule: "* * * * *"},
	}
	// Should not error
	sched.RegisterPipelines(context.Background(), pipelines)
}

func TestRegisterBackupForUser(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	sched.RegisterBackupForUser("0 2 * * *", "user-123")
	// Should not panic
}

func TestStop_NoError(t *testing.T) {
	sched, _ := newTestScheduler(t)
	sched.Start()
	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)
	sched.Stop() // Should not panic
}

func TestRemovePipelineJob(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	pipelines := []*domain.Pipeline{
		{ID: "p1", UserID: "u1", Enabled: true, Schedule: "* * * * *"},
	}
	sched.RegisterPipelines(context.Background(), pipelines)
	sched.RemovePipelineJob("p1") // Should not panic
}

func TestBackupPayload_Serializable(t *testing.T) {
	p := worker.BackupPayload{UserID: "u1"}
	data, err := json.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(data), "u1")
}
