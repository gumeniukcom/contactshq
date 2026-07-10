package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

func seedBulk(repo *mockContactRepo, n int) []string {
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		c := &domain.Contact{
			ID:            fmt.Sprintf("c%d", i),
			AddressBookID: testAddressBookID,
			UID:           fmt.Sprintf("uid%d", i),
			LastName:      fmt.Sprintf("Person%02d", i),
			VCardData:     fmt.Sprintf("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:uid%d\r\nFN:Person %d\r\nEND:VCARD\r\n", i, i),
		}
		repo.contacts[c.ID] = c
		repo.byUID[testAddressBookID+":"+c.UID] = c
		ids = append(ids, c.ID)
	}
	return ids
}

func newBulkFixture(t *testing.T) (*service.ContactService, *service.ExporterService, *mockContactRepo, []string) {
	t.Helper()

	repo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	ids := seedBulk(repo, 3)

	return service.NewContactService(repo, abRepo), service.NewExporterService(repo, abRepo), repo, ids
}

func TestDeleteMany_DeletesInOneCallAndReportsCount(t *testing.T) {
	svc, _, repo, ids := newBulkFixture(t)

	deleted, err := svc.DeleteMany(context.Background(), "u1", ids[:2])
	require.NoError(t, err)

	assert.Equal(t, 2, deleted)
	assert.Len(t, repo.contacts, 1)
}

// Repeated ids must not inflate the count the user is shown.
func TestDeleteMany_DeduplicatesIDs(t *testing.T) {
	svc, _, _, ids := newBulkFixture(t)

	deleted, err := svc.DeleteMany(context.Background(), "u1", []string{ids[0], ids[0], ids[0]})
	require.NoError(t, err)

	assert.Equal(t, 1, deleted)
}

func TestDeleteMany_RejectsAnOversizedRequest(t *testing.T) {
	svc, _, repo, _ := newBulkFixture(t)

	tooMany := make([]string, service.MaxBulkIDs+1)
	_, err := svc.DeleteMany(context.Background(), "u1", tooMany)

	require.ErrorIs(t, err, service.ErrTooManyIDs)
	assert.Len(t, repo.contacts, 3, "nothing may be deleted when the request is rejected")
}

func TestDeleteMany_EmptyListDeletesNothing(t *testing.T) {
	svc, _, repo, _ := newBulkFixture(t)

	deleted, err := svc.DeleteMany(context.Background(), "u1", nil)
	require.NoError(t, err)

	assert.Equal(t, 0, deleted)
	assert.Len(t, repo.contacts, 3)
}

func TestExportVCardByIDs_ExportsOnlyTheSelectedContacts(t *testing.T) {
	_, exporter, _, ids := newBulkFixture(t)

	data, err := exporter.ExportVCardByIDs(context.Background(), "u1", ids[:2])
	require.NoError(t, err)

	assert.Equal(t, 2, strings.Count(data, "BEGIN:VCARD"), "exactly the selected contacts")
	assert.Contains(t, data, "UID:uid0")
	assert.Contains(t, data, "UID:uid1")
	assert.NotContains(t, data, "UID:uid2")
}

// An empty selection means "the whole address book", which is what the plain export does.
func TestExportVCardByIDs_EmptyIDsExportsEverything(t *testing.T) {
	_, exporter, _, _ := newBulkFixture(t)

	data, err := exporter.ExportVCardByIDs(context.Background(), "u1", nil)
	require.NoError(t, err)

	assert.Equal(t, 3, strings.Count(data, "BEGIN:VCARD"))
}

func TestExportVCard_MatchesExportVCardByIDsWithNoIDs(t *testing.T) {
	_, exporter, _, _ := newBulkFixture(t)
	ctx := context.Background()

	all, err := exporter.ExportVCard(ctx, "u1")
	require.NoError(t, err)
	byIDs, err := exporter.ExportVCardByIDs(ctx, "u1", nil)
	require.NoError(t, err)

	assert.Equal(t, all, byIDs)
}

func TestExportVCardByIDs_RejectsAnOversizedRequest(t *testing.T) {
	_, exporter, _, _ := newBulkFixture(t)

	_, err := exporter.ExportVCardByIDs(context.Background(), "u1", make([]string, service.MaxBulkIDs+1))

	require.True(t, errors.Is(err, service.ErrTooManyIDs))
}
