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

const syncedCard = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:uid-1\r\nFN:Jane Doe\r\nN:Doe;Jane;;;\r\n" +
	"EMAIL:jane@example.com\r\nPHOTO:data:image/jpeg;base64,/9j/4AAQSk\r\n" +
	"X-ABLabel:_$!<Work>!$_\r\nEND:VCARD\r\n"

func setupContactService(t *testing.T) (*service.ContactService, *mockContactRepo, *domain.Contact) {
	t.Helper()

	repo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}

	contact := &domain.Contact{
		ID:            "c1",
		AddressBookID: testAddressBookID,
		UID:           "uid-1",
		FirstName:     "Jane",
		LastName:      "Doe",
		VCardData:     syncedCard,
	}
	repo.contacts[contact.ID] = contact
	repo.byUID[testAddressBookID+":uid-1"] = contact

	return service.NewContactService(repo, abRepo), repo, contact
}

// Renaming a contact through the form must not destroy the photo and custom properties
// that arrived with it from Google or a phone.
func TestUpdate_WithFieldsPreservesUnmodelledProperties(t *testing.T) {
	svc, _, _ := setupContactService(t)

	got, err := svc.Update(context.Background(), "u1", "c1", service.UpdateContactInput{
		Fields: &service.ContactFields{
			FirstName: "Janet",
			LastName:  "Doe",
			Emails:    []service.ContactFieldValue{{Value: "janet@example.com"}},
		},
	})
	require.NoError(t, err)

	assert.Contains(t, got.VCardData, "Janet", "the edit must be applied")
	assert.Contains(t, got.VCardData, "janet@example.com")
	assert.NotContains(t, got.VCardData, "jane@example.com", "the old email must be gone")

	assert.Contains(t, got.VCardData, "PHOTO:", "the photo must survive an edit")
	assert.Contains(t, got.VCardData, "/9j/4AAQSk")
	assert.Contains(t, got.VCardData, "X-ABLABEL:", "custom properties must survive an edit")
}

// The flat columns must reflect what actually landed in the card.
func TestUpdate_WithFieldsRefreshesFlatColumns(t *testing.T) {
	svc, _, _ := setupContactService(t)

	got, err := svc.Update(context.Background(), "u1", "c1", service.UpdateContactInput{
		Fields: &service.ContactFields{
			FirstName:  "Janet",
			LastName:   "Smith",
			Org:        "Acme",
			Title:      "Engineer",
			Emails:     []service.ContactFieldValue{{Value: "janet@example.com", Type: "work"}},
			Phones:     []service.ContactFieldValue{{Value: "+15551234567", Type: "cell"}},
			Categories: []string{"vip"},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "Janet", got.FirstName)
	assert.Equal(t, "Smith", got.LastName)
	assert.Equal(t, "Acme", got.Org)
	assert.Equal(t, "Engineer", got.Title)
	assert.Equal(t, "janet@example.com", got.Email)
	assert.Equal(t, "+15551234567", got.Phone)
}

func TestUpdate_WithFieldsKeepsUID(t *testing.T) {
	svc, _, _ := setupContactService(t)

	got, err := svc.Update(context.Background(), "u1", "c1", service.UpdateContactInput{
		Fields: &service.ContactFields{FirstName: "Janet"},
	})
	require.NoError(t, err)

	assert.Equal(t, "uid-1", got.UID)
	assert.Contains(t, got.VCardData, "UID:uid-1")
}

// Clearing a repeated field in the form must remove it from the stored card.
func TestUpdate_WithFieldsClearsRemovedValues(t *testing.T) {
	svc, _, _ := setupContactService(t)

	got, err := svc.Update(context.Background(), "u1", "c1", service.UpdateContactInput{
		Fields: &service.ContactFields{FirstName: "Janet", Emails: nil},
	})
	require.NoError(t, err)

	assert.NotContains(t, got.VCardData, "EMAIL")
	assert.Contains(t, got.VCardData, "PHOTO:", "clearing an email must not touch the photo")
}

// vcard_data remains an explicit, whole-card replacement for clients that own the card.
func TestUpdate_WithVCardDataStillReplacesWholeCard(t *testing.T) {
	svc, _, _ := setupContactService(t)
	replacement := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:uid-1\r\nFN:Replaced\r\nEND:VCARD\r\n"

	got, err := svc.Update(context.Background(), "u1", "c1", service.UpdateContactInput{
		VCardData: &replacement,
	})
	require.NoError(t, err)

	assert.NotContains(t, got.VCardData, "PHOTO:")
	assert.Contains(t, got.VCardData, "Replaced")
}

func TestCreate_WithFieldsBuildsFullCard(t *testing.T) {
	repo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	svc := service.NewContactService(repo, abRepo)

	got, err := svc.Create(context.Background(), "u1", service.CreateContactInput{
		Fields: &service.ContactFields{
			FirstName:  "New",
			LastName:   "Person",
			Nickname:   "Newbie",
			Emails:     []service.ContactFieldValue{{Value: "a@example.com", Type: "home"}},
			Addresses:  []service.ContactAddressValue{{Street: "1 Main St", City: "Springfield"}},
			Categories: []string{"friends"},
			Birthday:   "1990-01-01",
		},
	})
	require.NoError(t, err)

	for _, want := range []string{"New", "Person", "Newbie", "a@example.com", "Springfield", "friends", "BDAY"} {
		assert.True(t, strings.Contains(got.VCardData, want), "card is missing %q:\n%s", want, got.VCardData)
	}
	assert.NotEmpty(t, got.UID)
}

func TestContactFields_ToParsedSkipsEmptyValues(t *testing.T) {
	f := service.ContactFields{
		Emails:     []service.ContactFieldValue{{Value: ""}, {Value: "keep@example.com"}},
		Phones:     []service.ContactFieldValue{{Value: ""}},
		Categories: []string{"", "vip"},
		Addresses:  []service.ContactAddressValue{{}, {City: "Springfield"}},
	}

	p := f.ToParsed("uid")

	require.Len(t, p.Emails, 1)
	assert.Equal(t, "keep@example.com", p.Emails[0].Value)
	assert.Empty(t, p.Phones)
	assert.Equal(t, []string{"vip"}, p.Categories)
	require.Len(t, p.Addresses, 1)
	assert.Equal(t, "Springfield", p.Addresses[0].City)
}
