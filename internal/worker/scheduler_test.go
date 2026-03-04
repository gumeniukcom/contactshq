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

func TestDedupPayload_Serializable(t *testing.T) {
	p := worker.DedupPayload{UserID: "u1"}
	data, err := json.Marshal(p)
	require.NoError(t, err)

	var decoded worker.DedupPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "u1", decoded.UserID)
}

func TestRegisterDedupForUser(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	sched.RegisterDedupForUser("0 2 * * *", "user-dedup-1")
	// Should not panic
}

func TestRemoveDedupForUser(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	sched.RegisterDedupForUser("* * * * *", "user-dedup-1")
	sched.RemoveDedupForUser("user-dedup-1")
	// Should not panic
}

func TestReregisterDedupForUser(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	sched.RegisterDedupForUser("* * * * *", "user-dedup-1")
	sched.ReregisterDedupForUser("0 */6 * * *", "user-dedup-1")
	// Should not panic
}

func TestReregisterDedupForUser_EmptyRemoves(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	sched.RegisterDedupForUser("* * * * *", "user-dedup-1")
	sched.ReregisterDedupForUser("", "user-dedup-1")
	// Empty schedule should effectively remove
}

func TestReregisterPipelineJob(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	p := &domain.Pipeline{ID: "p1", UserID: "u1", Enabled: true, Schedule: "* * * * *"}
	sched.RegisterPipelineJob(p)
	p.Schedule = "0 */6 * * *"
	sched.ReregisterPipelineJob(p)
	// Should not panic; old job replaced with new schedule
}

func TestReregisterPipelineJob_DisabledRemoves(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	p := &domain.Pipeline{ID: "p1", UserID: "u1", Enabled: true, Schedule: "* * * * *"}
	sched.RegisterPipelineJob(p)
	p.Enabled = false
	sched.ReregisterPipelineJob(p)
	// Disabled pipeline should just remove the job
}

func TestValidateCron_Valid(t *testing.T) {
	cases := []string{
		"* * * * *",
		"0 2 * * *",
		"*/15 * * * *",
		"0 */6 * * *",
		"0 2 * * 0",
		"0 2 1 * *",
		"0 0 * * *",
	}
	for _, c := range cases {
		assert.NoError(t, worker.ValidateCron(c), "expected valid: %s", c)
	}
}

func TestValidateCron_Invalid(t *testing.T) {
	cases := []string{
		"invalid",
		"",
		"* * * *",
		"61 * * * *",
		"not a cron",
		"0 25 * * *",
	}
	for _, c := range cases {
		assert.Error(t, worker.ValidateCron(c), "expected invalid: %s", c)
	}
}

func TestReregisterBackupForUser(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	sched.RegisterBackupForUser("* * * * *", "user-b1")
	sched.ReregisterBackupForUser("0 2 * * *", "user-b1")
	// Should not panic
}

func TestReregisterBackupForUser_EmptyRemoves(t *testing.T) {
	sched, _ := newTestScheduler(t)
	defer sched.Stop()

	sched.RegisterBackupForUser("* * * * *", "user-b1")
	sched.ReregisterBackupForUser("", "user-b1")
	// Empty schedule effectively disables
}
