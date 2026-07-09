package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	svc := service.NewBackupService(contactRepo, abRepo, nil, dir, "", 7)

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
