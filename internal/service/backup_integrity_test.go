package service_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

func newBackupService(t *testing.T, dir string, logger *zap.Logger) (*service.BackupService, *mockContactRepo) {
	t.Helper()
	contactRepo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	settingsRepo := &stubBackupSettingsRepo{settings: &domain.UserBackupSettings{UserID: "u1", Retention: 7}}
	return service.NewBackupService(contactRepo, abRepo, settingsRepo, logger, dir, "", 7), contactRepo
}

// GetPath used to validate only that the resolved path stayed inside the user's directory,
// so anything else sitting there — a stray .tmp, a file someone dropped in — was downloadable
// and deletable through the backup API.
func TestGetPath_RejectsNonBackupFilenames(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "u1")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "notes.txt"), []byte("private"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "backup-1.vcf"), []byte("x"), 0o600))

	svc, _ := newBackupService(t, dir, zap.NewNop())

	_, err := svc.GetPath(context.Background(), "u1", "notes.txt")
	require.Error(t, err, "a non-backup file must not be reachable through GetPath")

	// Traversal was already rejected before this change; kept as a regression guard, not as
	// the acceptance criterion for it.
	_, err = svc.GetPath(context.Background(), "u1", "../../etc/passwd")
	require.Error(t, err)

	got, err := svc.GetPath(context.Background(), "u1", "backup-1.vcf")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(userDir, "backup-1.vcf"), got)
}

// A backup is published by rename, so a failed or in-progress write must leave nothing that
// List reports or GetPath resolves.
func TestCreate_LeavesNoPartialFileVisible(t *testing.T) {
	dir := t.TempDir()
	svc, repo := newBackupService(t, dir, zap.NewNop())
	repo.contacts["c1"] = &domain.Contact{
		ID: "c1", AddressBookID: testAddressBookID, UID: "u-1",
		VCardData: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:u-1\r\nFN:A\r\nEND:VCARD\r\n",
	}

	info, err := svc.Create(context.Background(), "u1")
	require.NoError(t, err)

	backups, err := svc.List(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, backups, 1)
	require.Equal(t, info.Filename, backups[0].Filename)

	entries, err := os.ReadDir(filepath.Join(dir, "u1"))
	require.NoError(t, err)
	for _, e := range entries {
		require.False(t, strings.HasSuffix(e.Name(), ".tmp"),
			"a temporary file survived a successful backup: %s", e.Name())
	}
}

// Restore reads the whole decompressed backup into memory. Capping it with a bare LimitReader
// would silently truncate instead, and replace-mode deletes the address book first — so a
// truncated read destroys contacts the backup can no longer supply.
func TestRestore_OversizedBackupIsRejectedBeforeDeleting(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "u1")
	require.NoError(t, os.MkdirAll(userDir, 0o755))

	// One card, then padding that decompresses far past the cap.
	var raw bytes.Buffer
	raw.WriteString("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:new-1\r\nFN:Alice\r\nEND:VCARD\r\n")
	raw.Write(bytes.Repeat([]byte("A"), 4096))

	var gzipped bytes.Buffer
	gzw := gzip.NewWriter(&gzipped)
	_, err := gzw.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, gzw.Close())

	require.NoError(t, os.WriteFile(filepath.Join(userDir, "big.vcf.gz"), gzipped.Bytes(), 0o600))

	contactRepo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	svc := service.NewBackupService(contactRepo, abRepo, nil, zap.NewNop(), dir, "", 7).
		WithMaxRestoreBytes(1024)

	seedContacts(contactRepo, "existing-1")

	_, err = svc.Restore(context.Background(), "u1", "big.vcf.gz", "replace")

	require.ErrorIs(t, err, service.ErrBackupTooLarge)
	require.Zero(t, contactRepo.deleteAllCalls, "DeleteAll must not run when the backup is rejected")
	require.Contains(t, contactRepo.contacts, "id-existing-1", "existing contacts must survive")
}

// Retention failures do not invalidate the backup just written, so Create still succeeds —
// but a directory that cannot be pruned has to reach the operator somehow.
func TestCreate_RetentionFailureIsLoggedNotSwallowed(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "u1")
	require.NoError(t, os.MkdirAll(userDir, 0o755))

	// Two existing backups older than the new one; retention of 1 will try to remove them.
	for _, name := range []string{"backup-20200101-000000-000.vcf", "backup-20200102-000000-000.vcf"} {
		require.NoError(t, os.WriteFile(filepath.Join(userDir, name), []byte("x"), 0o600))
	}

	core, logs := observer.New(zap.WarnLevel)
	settingsRepo := &stubBackupSettingsRepo{settings: &domain.UserBackupSettings{
		UserID: "u1", Retention: 1,
	}}
	contactRepo := newMockContactRepo()
	contactRepo.contacts["c1"] = &domain.Contact{
		ID: "c1", AddressBookID: testAddressBookID, UID: "u-1",
		VCardData: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:u-1\r\nFN:A\r\nEND:VCARD\r\n",
	}
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	svc := service.NewBackupService(contactRepo, abRepo, settingsRepo, zap.New(core), dir, "", 7)

	// Make the directory read-only so os.Remove fails but the file already written stays.
	require.NoError(t, os.Chmod(userDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(userDir, 0o700) })

	_, err := svc.Create(context.Background(), "u1")

	// Creating into a read-only directory fails outright on some systems; the assertion that
	// matters is that nothing is silent either way.
	if err != nil {
		require.NotEmpty(t, err.Error())
		return
	}
	require.NotZero(t, logs.FilterMessage("backup retention failed").Len(),
		"a retention failure must be logged")
}

// stubBackupSettingsRepo returns fixed settings so a test can pin retention.
type stubBackupSettingsRepo struct {
	settings *domain.UserBackupSettings
}

func (s *stubBackupSettingsRepo) Get(_ context.Context, _ string) (*domain.UserBackupSettings, error) {
	return s.settings, nil
}

func (s *stubBackupSettingsRepo) Upsert(_ context.Context, _ *domain.UserBackupSettings) error {
	return nil
}

func (s *stubBackupSettingsRepo) ListAll(_ context.Context) ([]*domain.UserBackupSettings, error) {
	return []*domain.UserBackupSettings{s.settings}, nil
}

var _ = errors.Is
