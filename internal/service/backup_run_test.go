package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type memBackupRunRepo struct {
	runs []*domain.BackupRun
}

func (m *memBackupRunRepo) Create(_ context.Context, run *domain.BackupRun) error {
	m.runs = append(m.runs, run)
	return nil
}

func (m *memBackupRunRepo) Update(_ context.Context, run *domain.BackupRun) error {
	for i, r := range m.runs {
		if r.ID == run.ID {
			m.runs[i] = run
			return nil
		}
	}
	return errors.New("no such run")
}

func (m *memBackupRunRepo) ListByUser(_ context.Context, userID string, _ int) ([]*domain.BackupRun, error) {
	var out []*domain.BackupRun
	for _, r := range m.runs {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memBackupRunRepo) LastSuccess(_ context.Context, userID string) (*domain.BackupRun, error) {
	var best *domain.BackupRun
	for _, r := range m.runs {
		if r.UserID == userID && r.Status == domain.BackupRunOK &&
			(best == nil || r.StartedAt.After(best.StartedAt)) {
			best = r
		}
	}
	return best, nil
}

func (m *memBackupRunRepo) LastRun(_ context.Context, userID string) (*domain.BackupRun, error) {
	var best *domain.BackupRun
	for _, r := range m.runs {
		if r.UserID == userID && (best == nil || r.StartedAt.After(best.StartedAt)) {
			best = r
		}
	}
	return best, nil
}

func (m *memBackupRunRepo) MarkStaleInterrupted(_ context.Context, before time.Time) (int, error) {
	n := 0
	for _, r := range m.runs {
		if r.Status == domain.BackupRunRunning && r.StartedAt.Before(before) {
			r.Status = domain.BackupRunInterrupted
			n++
		}
	}
	return n, nil
}

// failingContactRepo makes the backup itself fail.
type failingContactRepo struct {
	*mockContactRepo
}

func (f *failingContactRepo) ListAll(context.Context, string) ([]*domain.Contact, error) {
	return nil, errors.New("database unavailable")
}

func newRecordedBackupService(t *testing.T, dir string) (*service.BackupService, *memBackupRunRepo, *mockContactRepo) {
	t.Helper()
	svc, contactRepo := newBackupService(t, dir, zap.NewNop())
	runs := &memBackupRunRepo{}
	return svc.WithRunRepo(runs), runs, contactRepo
}

func seedOneContact(repo *mockContactRepo) {
	repo.contacts["c1"] = &domain.Contact{
		ID: "c1", AddressBookID: testAddressBookID, UID: "u-1",
		VCardData: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:u-1\r\nFN:A\r\nEND:VCARD\r\n",
	}
}

func TestBackupRun_SuccessIsRecordedWithTheFileDetails(t *testing.T) {
	svc, runs, contactRepo := newRecordedBackupService(t, t.TempDir())
	seedOneContact(contactRepo)

	info, err := svc.Create(context.Background(), "u1")
	require.NoError(t, err)

	require.Len(t, runs.runs, 1)
	run := runs.runs[0]
	require.Equal(t, domain.BackupRunOK, run.Status)
	require.Equal(t, domain.BackupTriggerManual, run.Trigger, "Create is the manual path")
	require.Equal(t, info.Filename, run.Filename)
	require.Equal(t, info.Size, run.SizeBytes)
	require.Equal(t, 1, run.ContactCount)
	require.NotNil(t, run.FinishedAt)
	require.Empty(t, run.ErrorMessage)
}

func TestBackupRun_ScheduledTriggerIsRecordedSeparately(t *testing.T) {
	svc, runs, contactRepo := newRecordedBackupService(t, t.TempDir())
	seedOneContact(contactRepo)

	_, err := svc.CreateWithTrigger(context.Background(), "u1", domain.BackupTriggerScheduled)
	require.NoError(t, err)

	require.Len(t, runs.runs, 1)
	require.Equal(t, domain.BackupTriggerScheduled, runs.runs[0].Trigger)
}

// A failed backup must leave a row saying so. Silence is what this table exists to end.
func TestBackupRun_FailureIsRecordedWithTheCause(t *testing.T) {
	dir := t.TempDir()
	contactRepo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	settingsRepo := &stubBackupSettingsRepo{settings: &domain.UserBackupSettings{UserID: "u1", Retention: 7}}
	runs := &memBackupRunRepo{}

	svc := service.NewBackupService(
		&failingContactRepo{mockContactRepo: contactRepo}, abRepo, settingsRepo,
		zap.NewNop(), dir, "", 7,
	).WithRunRepo(runs)

	_, err := svc.Create(context.Background(), "u1")
	require.Error(t, err)

	require.Len(t, runs.runs, 1)
	require.Equal(t, domain.BackupRunFailed, runs.runs[0].Status)
	require.Contains(t, runs.runs[0].ErrorMessage, "database unavailable")
	require.NotNil(t, runs.runs[0].FinishedAt)
}

// The acceptance criterion for 6.1: finalisation runs on a context of its own, so a backup
// interrupted by a shutdown is recorded as failed rather than left "running" forever.
func TestBackupRun_IsFinalisedEvenWhenTheCallersContextIsCancelled(t *testing.T) {
	svc, runs, contactRepo := newRecordedBackupService(t, t.TempDir())
	seedOneContact(contactRepo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = svc.Create(ctx, "u1")

	require.Len(t, runs.runs, 1)
	require.NotEqual(t, domain.BackupRunRunning, runs.runs[0].Status,
		"a cancelled caller must not leave the run open forever")
	require.NotNil(t, runs.runs[0].FinishedAt)
}

// Without a run repository the service must behave exactly as it did before.
func TestBackupRun_WithoutARepositoryNothingChanges(t *testing.T) {
	svc, contactRepo := newBackupService(t, t.TempDir(), zap.NewNop())
	seedOneContact(contactRepo)

	info, err := svc.Create(context.Background(), "u1")
	require.NoError(t, err)
	require.NotEmpty(t, info.Filename)
}
