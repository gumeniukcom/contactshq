package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

func newImporterSvc() *service.ImporterService {
	repo := newMockContactRepo()
	ab := &domain.AddressBook{ID: "ab1", UserID: "user1"}
	abRepo := &mockAbRepo{ab: ab}
	return service.NewImporterService(repo, abRepo)
}

func TestImportVCard_ExtractsTitleNote(t *testing.T) {
	svc := newImporterSvc()
	ctx := context.Background()

	vcard := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:test-uid-1\r\nFN:Alice Smith\r\nTITLE:Director\r\nNOTE:Key contact\r\nEND:VCARD\r\n"
	result, err := svc.ImportVCard(ctx, "user1", vcard)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Imported)
	assert.Equal(t, 0, result.Errors)
}

func TestImportVCard_ExtractsTitleNote_MultipleCards(t *testing.T) {
	repo := newMockContactRepo()
	ab := &domain.AddressBook{ID: "ab2", UserID: "user2"}
	abRepo := &mockAbRepo{ab: ab}
	svc := service.NewImporterService(repo, abRepo)
	ctx := context.Background()

	vcard := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:uid-a\r\nFN:Alice\r\nTITLE:CEO\r\nNOTE:VIP\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nUID:uid-b\r\nFN:Bob\r\nEND:VCARD\r\n"

	result, err := svc.ImportVCard(ctx, "user2", vcard)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Imported)

	// Verify the first contact has title/note
	c := repo.byUID["ab2:uid-a"]
	require.NotNil(t, c)
	assert.Equal(t, "CEO", c.Title)
	assert.Equal(t, "VIP", c.Note)

	// Second has empty title/note
	c2 := repo.byUID["ab2:uid-b"]
	require.NotNil(t, c2)
	assert.Equal(t, "", c2.Title)
}

func TestImportCSV_ExtractsTitleNote(t *testing.T) {
	repo := newMockContactRepo()
	ab := &domain.AddressBook{ID: "ab3", UserID: "user3"}
	abRepo := &mockAbRepo{ab: ab}
	svc := service.NewImporterService(repo, abRepo)
	ctx := context.Background()

	csv := "first_name,last_name,email,title,note\nAlice,Smith,alice@example.com,Manager,Important\n"
	result, err := svc.ImportCSV(ctx, "user3", csv)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Imported)

	// Find the created contact
	var found *domain.Contact
	for _, c := range repo.contacts {
		found = c
		break
	}
	require.NotNil(t, found)
	assert.Equal(t, "Manager", found.Title)
	assert.Equal(t, "Important", found.Note)
	assert.Contains(t, found.VCardData, "TITLE:Manager")
	assert.Contains(t, found.VCardData, "NOTE:Important")
}
