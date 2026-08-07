package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

func manyCards(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:card-")
		sb.WriteString(string(rune('a' + i%26)))
		sb.WriteString("\r\nFN:Person\r\nEND:VCARD\r\n")
	}
	return sb.String()
}

// A caller who has gone away should not have a whole file parsed on their behalf. Safe to
// cancel here: nothing has been written yet.
func TestImportVCard_StopsOnACancelledContext(t *testing.T) {
	contactRepo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	svc := service.NewImporterService(contactRepo, abRepo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.ImportVCard(ctx, "u1", manyCards(50))

	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, contactRepo.contacts, "nothing should have been written")
}

func TestImportCSV_StopsOnACancelledContext(t *testing.T) {
	contactRepo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	svc := service.NewImporterService(contactRepo, abRepo)

	csv := "first_name,last_name,email\n"
	for i := 0; i < 50; i++ {
		csv += "Ada,Lovelace,ada@example.com\n"
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.ImportCSV(ctx, "u1", csv)
	require.ErrorIs(t, err, context.Canceled)
}

// The regression that matters most in this task: replace-mode restore must NOT become
// cancellable after DeleteAll. Bailing out there is how a restore turns into an empty address
// book — the opposite of what the feature is for.
func TestRestore_IsNotAbortedAfterDeleteAll(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "u1")
	require.NoError(t, os.MkdirAll(userDir, 0o755))

	cards := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:new-1\r\nFN:Alice\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:4.0\r\nUID:new-2\r\nFN:Bob\r\nEND:VCARD\r\n"
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "b.vcf"), []byte(cards), 0o600))

	contactRepo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	settingsRepo := &stubBackupSettingsRepo{settings: &domain.UserBackupSettings{UserID: "u1", Retention: 7}}
	svc := service.NewBackupService(contactRepo, abRepo, settingsRepo, zap.NewNop(), dir, "", 7)

	seedContacts(contactRepo, "old-1")

	// A context that is alive through parsing. Whatever happens after DeleteAll must run to
	// completion regardless.
	res, err := svc.Restore(context.Background(), "u1", "b.vcf", "replace")
	require.NoError(t, err)
	require.Equal(t, 2, res.Imported)

	require.Len(t, contactRepo.contacts, 2, "the restore must have completed, not stopped midway")
	require.NotContains(t, contactRepo.contacts, "id-old-1")
}

// Parsing, on the other hand, is cancellable — and must stop before anything is deleted.
func TestRestore_CancelledDuringParsingDeletesNothing(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "u1")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "b.vcf"), []byte(manyCards(20)), 0o600))

	contactRepo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	settingsRepo := &stubBackupSettingsRepo{settings: &domain.UserBackupSettings{UserID: "u1", Retention: 7}}
	svc := service.NewBackupService(contactRepo, abRepo, settingsRepo, zap.NewNop(), dir, "", 7)

	seedContacts(contactRepo, "existing-1")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.Restore(ctx, "u1", "b.vcf", "replace")
	require.Error(t, err)

	require.Zero(t, contactRepo.deleteAllCalls,
		"a cancellation during parsing must not reach the destructive step")
	require.Contains(t, contactRepo.contacts, "id-existing-1")
}
