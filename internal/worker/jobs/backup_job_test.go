package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
	"github.com/gumeniukcom/contactshq/internal/worker/jobs"
)

type stubBackupCreator struct {
	calls []struct{ userID, trigger string }
	err   error
}

func (s *stubBackupCreator) CreateWithTrigger(_ context.Context, userID, trigger string) (*service.BackupInfo, error) {
	s.calls = append(s.calls, struct{ userID, trigger string }{userID, trigger})
	if s.err != nil {
		return nil, s.err
	}
	return &service.BackupInfo{Filename: "backup.vcf", Size: 42}, nil
}

func payload(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

// These tests are the reason the handlers take interfaces at all: before, asserting on the
// wrapped error meant standing up a real service, a real database and a real filesystem.
func TestBackupJob_RunsTheBackupAsScheduledByDefault(t *testing.T) {
	creator := &stubBackupCreator{}
	handler := jobs.NewBackupJobHandler(creator, zap.NewNop())

	err := handler.Handle(context.Background(), payload(t, jobs.BackupJobPayload{UserID: "u1"}))
	require.NoError(t, err)

	require.Len(t, creator.calls, 1)
	require.Equal(t, "u1", creator.calls[0].userID)
	require.Equal(t, domain.BackupTriggerScheduled, creator.calls[0].trigger,
		"a payload written before the trigger field existed must still read as scheduled")
}

func TestBackupJob_PassesAnExplicitTriggerThrough(t *testing.T) {
	creator := &stubBackupCreator{}
	handler := jobs.NewBackupJobHandler(creator, zap.NewNop())

	err := handler.Handle(context.Background(), payload(t, jobs.BackupJobPayload{
		UserID: "u1", Trigger: domain.BackupTriggerCatchup,
	}))
	require.NoError(t, err)

	require.Equal(t, domain.BackupTriggerCatchup, creator.calls[0].trigger)
}

// A failure has to come back naming the user, or the log line is unusable on a multi-user
// instance.
func TestBackupJob_WrapsAFailureWithTheUserID(t *testing.T) {
	creator := &stubBackupCreator{err: errors.New("disk full")}
	handler := jobs.NewBackupJobHandler(creator, zap.NewNop())

	err := handler.Handle(context.Background(), payload(t, jobs.BackupJobPayload{UserID: "u1"}))

	require.Error(t, err)
	require.Contains(t, err.Error(), "u1")
	require.Contains(t, err.Error(), "disk full")
}

func TestBackupJob_RejectsAnUnreadablePayload(t *testing.T) {
	creator := &stubBackupCreator{}
	handler := jobs.NewBackupJobHandler(creator, zap.NewNop())

	err := handler.Handle(context.Background(), json.RawMessage("not json"))

	require.Error(t, err)
	require.Empty(t, creator.calls, "a payload that cannot be read must not reach the service")
}
