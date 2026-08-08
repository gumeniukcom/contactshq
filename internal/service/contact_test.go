package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

func newContactSvc() (*service.ContactService, *mockContactRepo) {
	repo := newMockContactRepo()
	ab := &domain.AddressBook{ID: "ab1", UserID: "user1"}
	abRepo := &mockAbRepo{ab: ab}
	return service.NewContactService(repo, abRepo), repo
}

// setupContactWithGeo stores a contact whose card carries a GEO property, as a card
// imported from another CardDAV client or from Google would.
func setupContactWithGeo(t *testing.T) (*service.ContactService, *domain.Contact) {
	t.Helper()

	repo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}

	contact := &domain.Contact{
		ID:            "c1",
		AddressBookID: testAddressBookID,
		UID:           "uid-geo",
		FirstName:     "Jane",
		LastName:      "Doe",
		Geo:           "geo:52.5,13.4",
		VCardData: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:uid-geo\r\nFN:Jane Doe\r\nN:Doe;Jane;;;\r\n" +
			"GEO:geo:52.5,13.4\r\nEND:VCARD\r\n",
	}
	repo.contacts[contact.ID] = contact
	repo.byUID[testAddressBookID+":uid-geo"] = contact

	return service.NewContactService(repo, abRepo), contact
}

// GEO is a managed property, so MergeIntoVCard replaces it on every fields edit. The form
// payload must therefore be able to carry it, or an ordinary edit deletes a value the user
// was never shown.
func TestUpdate_FieldsEditKeepsGeo(t *testing.T) {
	svc, _ := setupContactWithGeo(t)

	got, err := svc.Update(context.Background(), "u1", "c1", service.UpdateContactInput{
		Fields: &service.ContactFields{
			FirstName: "Janet",
			LastName:  "Doe",
			Geo:       "geo:52.5,13.4",
		},
	})
	require.NoError(t, err)

	assert.Contains(t, got.VCardData, "Janet", "the edit must be applied")
	assert.Contains(t, got.VCardData, "GEO:geo:52.5,13.4", "GEO must survive a form edit")
	assert.Equal(t, "geo:52.5,13.4", got.Geo)
}

// The `fields` payload is a full replacement of the managed set: an empty value clears the
// property, exactly as an empty `note` deletes a note. Preserving GEO when it is absent
// would make it the one managed property no form edit can clear.
func TestUpdate_FieldsEditWithEmptyGeoClearsIt(t *testing.T) {
	svc, _ := setupContactWithGeo(t)

	got, err := svc.Update(context.Background(), "u1", "c1", service.UpdateContactInput{
		Fields: &service.ContactFields{FirstName: "Janet", LastName: "Doe"},
	})
	require.NoError(t, err)

	assert.NotContains(t, got.VCardData, "GEO")
	assert.Empty(t, got.Geo)
}

func TestCreate_SetsAllFields(t *testing.T) {
	svc, _ := newContactSvc()
	ctx := context.Background()

	c, err := svc.Create(ctx, "user1", service.CreateContactInput{
		FirstName: "Alice",
		LastName:  "Smith",
		Email:     "alice@example.com",
		Title:     "CEO",
		Note:      "VIP customer",
	})
	require.NoError(t, err)
	assert.Equal(t, "CEO", c.Title)
	assert.Equal(t, "VIP customer", c.Note)
	assert.Contains(t, c.VCardData, "TITLE:CEO")
	assert.Contains(t, c.VCardData, "NOTE:VIP customer")
}

func TestCreate_EmptyTitleNote_NotInVCard(t *testing.T) {
	svc, _ := newContactSvc()
	ctx := context.Background()

	c, err := svc.Create(ctx, "user1", service.CreateContactInput{
		FirstName: "Bob",
	})
	require.NoError(t, err)
	assert.NotContains(t, c.VCardData, "TITLE:")
	assert.NotContains(t, c.VCardData, "NOTE:")
}

func TestUpdate_ModifiesTitleNote(t *testing.T) {
	svc, repo := newContactSvc()
	ctx := context.Background()

	// Create a contact first
	c, err := svc.Create(ctx, "user1", service.CreateContactInput{
		FirstName: "Carol",
	})
	require.NoError(t, err)
	// Verify it's in the repo by ID
	_ = repo

	newTitle := "CTO"
	newNote := "Interesting person"
	updated, err := svc.Update(ctx, "user1", c.ID, service.UpdateContactInput{
		Title: &newTitle,
		Note:  &newNote,
	})
	require.NoError(t, err)
	assert.Equal(t, "CTO", updated.Title)
	assert.Equal(t, "Interesting person", updated.Note)
	assert.Contains(t, updated.VCardData, "TITLE:CTO")
	assert.Contains(t, updated.VCardData, "NOTE:Interesting person")
}

func TestGenerateVCard_IncludesTitleNote(t *testing.T) {
	svc, _ := newContactSvc()
	ctx := context.Background()

	c, err := svc.Create(ctx, "user1", service.CreateContactInput{
		FirstName: "Dave",
		Title:     "Engineer",
		Note:      "Likes Go",
	})
	require.NoError(t, err)
	assert.True(t, strings.Contains(c.VCardData, "TITLE:Engineer"), "vcard should contain TITLE")
	assert.True(t, strings.Contains(c.VCardData, "NOTE:Likes Go"), "vcard should contain NOTE")
}

func TestGenerateVCard_OmitsEmptyFields(t *testing.T) {
	svc, _ := newContactSvc()
	ctx := context.Background()

	c, err := svc.Create(ctx, "user1", service.CreateContactInput{
		FirstName: "Eve",
	})
	require.NoError(t, err)
	assert.False(t, strings.Contains(c.VCardData, "TITLE:"), "vcard should not contain empty TITLE")
	assert.False(t, strings.Contains(c.VCardData, "NOTE:"), "vcard should not contain empty NOTE")
}
