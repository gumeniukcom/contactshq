package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

const testAddressBookID = "ab1"

func setupRestore(t *testing.T, backupContent string) (*service.BackupService, *mockContactRepo) {
	t.Helper()

	dir := t.TempDir()
	userDir := filepath.Join(dir, "u1")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "backup.vcf"), []byte(backupContent), 0o600); err != nil {
		t.Fatal(err)
	}

	contactRepo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	svc := service.NewBackupService(contactRepo, abRepo, nil, zap.NewNop(), dir, "", 7)

	return svc, contactRepo
}

func seedContacts(repo *mockContactRepo, uids ...string) {
	for _, uid := range uids {
		c := &domain.Contact{ID: "id-" + uid, AddressBookID: testAddressBookID, UID: uid}
		repo.contacts[c.ID] = c
		repo.byUID[testAddressBookID+":"+uid] = c
	}
}

const twoCards = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:new-1\r\nFN:Alice\r\nEND:VCARD\r\n" +
	"BEGIN:VCARD\r\nVERSION:4.0\r\nUID:new-2\r\nFN:Bob\r\nEND:VCARD\r\n"

// An empty backup must not wipe the address book. Before parsing moved ahead of the
// delete, replace-mode ran DeleteAll first and left the user with nothing.
func TestRestore_ReplaceWithEmptyBackupKeepsExistingContacts(t *testing.T) {
	svc, repo := setupRestore(t, "")
	seedContacts(repo, "existing-1", "existing-2")

	_, err := svc.Restore(context.Background(), "u1", "backup.vcf", "replace")

	if !errors.Is(err, service.ErrEmptyBackup) {
		t.Fatalf("Restore() error = %v, want service.ErrEmptyBackup", err)
	}
	if len(repo.contacts) != 2 {
		t.Fatalf("existing contacts destroyed: %d remain, want 2", len(repo.contacts))
	}
}

// A backup whose every card is unparseable is just as dangerous as an empty one.
func TestRestore_ReplaceWithUnparseableBackupKeepsExistingContacts(t *testing.T) {
	svc, repo := setupRestore(t, "this is not a vCard at all\r\nneither is this\r\n")
	seedContacts(repo, "existing-1", "existing-2")

	_, err := svc.Restore(context.Background(), "u1", "backup.vcf", "replace")

	if !errors.Is(err, service.ErrEmptyBackup) {
		t.Fatalf("Restore() error = %v, want service.ErrEmptyBackup", err)
	}
	if len(repo.contacts) != 2 {
		t.Fatalf("existing contacts destroyed: %d remain, want 2", len(repo.contacts))
	}
}

func TestRestore_ReplaceSwapsContents(t *testing.T) {
	svc, repo := setupRestore(t, twoCards)
	seedContacts(repo, "existing-1")

	result, err := svc.Restore(context.Background(), "u1", "backup.vcf", "replace")
	if err != nil {
		t.Fatalf("Restore() = %v", err)
	}

	if result.Imported != 2 {
		t.Errorf("Imported = %d, want 2", result.Imported)
	}
	if _, stillThere := repo.byUID[testAddressBookID+":existing-1"]; stillThere {
		t.Error("replace must remove the previous contacts")
	}
	if _, ok := repo.byUID[testAddressBookID+":new-1"]; !ok {
		t.Error("restored contact new-1 missing")
	}
}

func TestRestore_MergeKeepsExistingAndSkipsDuplicates(t *testing.T) {
	svc, repo := setupRestore(t, twoCards)
	seedContacts(repo, "new-1", "untouched")

	result, err := svc.Restore(context.Background(), "u1", "backup.vcf", "merge")
	if err != nil {
		t.Fatalf("Restore() = %v", err)
	}

	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (new-1 already exists)", result.Skipped)
	}
	if result.Imported != 1 {
		t.Errorf("Imported = %d, want 1 (new-2)", result.Imported)
	}
	if _, ok := repo.byUID[testAddressBookID+":untouched"]; !ok {
		t.Error("merge must not remove existing contacts")
	}
}

// A restore must not lose the contacts that follow an embedded photo.
func TestRestore_LongPhotoLineDoesNotTruncateBackup(t *testing.T) {
	photo := strings.Repeat("A", 200_000)
	content := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:photo\r\nFN:Photo\r\nPHOTO:" + photo + "\r\nEND:VCARD\r\n" + twoCards

	svc, repo := setupRestore(t, content)

	result, err := svc.Restore(context.Background(), "u1", "backup.vcf", "replace")
	if err != nil {
		t.Fatalf("Restore() = %v", err)
	}

	if result.Imported != 3 {
		t.Fatalf("Imported = %d, want 3 — cards after the photo were dropped", result.Imported)
	}
	for _, uid := range []string{"photo", "new-1", "new-2"} {
		if _, ok := repo.byUID[testAddressBookID+":"+uid]; !ok {
			t.Errorf("contact %s missing after restore", uid)
		}
	}
}

// setupRestoreWithSync wires a sync-state repository into the restore path.
func setupRestoreWithSync(t *testing.T, backupContent string) (*service.BackupService, *mockContactRepo, *mockStateRepo) {
	t.Helper()

	svc, repo := setupRestore(t, backupContent)
	stateRepo := &mockStateRepo{states: map[string]*domain.SyncState{}}
	return svc.WithSyncStateRepo(stateRepo), repo, stateRepo
}

func seedState(stateRepo *mockStateRepo, id, localUID string) {
	stateRepo.states[localUID] = &domain.SyncState{
		ID:           id,
		UserID:       "u1",
		ProviderType: "google->internal",
		RemoteID:     "people/" + id,
		LocalID:      localUID,
		LocalETag:    "stale",
	}
}

// A restore must never delete contacts on the remote. The sync state of a contact the
// backup did not bring back still maps a remote contact to a local one that is gone, and
// the next export reads that as "deleted locally".
func TestRestore_DropsSyncStateOfContactsThatDidNotComeBack(t *testing.T) {
	svc, repo, stateRepo := setupRestoreWithSync(t, twoCards)
	seedContacts(repo, "new-1", "vanishes")
	seedState(stateRepo, "st-kept", "new-1")
	seedState(stateRepo, "st-orphan", "vanishes")

	_, err := svc.Restore(context.Background(), "u1", "backup.vcf", "replace")
	require.NoError(t, err)

	_, kept := stateRepo.states["new-1"]
	assert.True(t, kept, "the state of a restored contact must survive")

	_, orphan := stateRepo.states["vanishes"]
	assert.False(t, orphan, "the state of a contact the restore dropped must be removed")
}

// Merge-mode restore leaves everything in place, so no state may be dropped.
func TestRestore_MergeKeepsSyncStateOfUntouchedContacts(t *testing.T) {
	svc, repo, stateRepo := setupRestoreWithSync(t, twoCards)
	seedContacts(repo, "untouched")
	seedState(stateRepo, "st-1", "untouched")

	_, err := svc.Restore(context.Background(), "u1", "backup.vcf", "merge")
	require.NoError(t, err)

	_, kept := stateRepo.states["untouched"]
	assert.True(t, kept)
}

// A contact restored under its old UID keeps its mapping: the restored content then
// travels outward as an ordinary edit rather than as a deletion.
func TestRestore_KeepsSyncStateOfRestoredContacts(t *testing.T) {
	svc, repo, stateRepo := setupRestoreWithSync(t, twoCards)
	seedContacts(repo, "new-1")
	seedState(stateRepo, "st-1", "new-1")
	seedState(stateRepo, "st-2", "new-2")

	_, err := svc.Restore(context.Background(), "u1", "backup.vcf", "replace")
	require.NoError(t, err)

	assert.Len(t, stateRepo.states, 2, "both restored contacts keep their sync state")
}

// Without a sync-state repository the restore still works; the wiring is optional.
func TestRestore_WithoutSyncStateRepoStillRestores(t *testing.T) {
	svc, repo := setupRestore(t, twoCards)
	seedContacts(repo, "existing")

	result, err := svc.Restore(context.Background(), "u1", "backup.vcf", "replace")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Imported)
}
