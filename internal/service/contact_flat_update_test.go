package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

// A flat update used to rebuild the card from scratch with BuildVCard, which renders only the
// properties this application models — so PUT {"first_name":"X"} silently deleted PHOTO, KEY,
// X-ABLabel and anything else the card arrived with. It merges into the stored bytes now.
func TestUpdate_FlatEditKeepsUnmodelledProperties(t *testing.T) {
	repo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}

	contact := &domain.Contact{
		ID:            "c-flat",
		AddressBookID: testAddressBookID,
		UID:           "uid-flat",
		FirstName:     "Ada",
		LastName:      "Lovelace",
		VCardData: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:uid-flat\r\nFN:Ada Lovelace\r\n" +
			"N:Lovelace;Ada;;;\r\nPHOTO:data:image/jpeg;base64,/9j/4AAQ\r\n" +
			"X-ABLabel:Analytical Engine\r\nEND:VCARD\r\n",
	}
	repo.contacts[contact.ID] = contact

	svc := service.NewContactService(repo, abRepo)

	newFirst := "Augusta"
	updated, err := svc.Update(t.Context(), "u1", contact.ID, service.UpdateContactInput{
		FirstName: &newFirst,
	})
	require.NoError(t, err)

	require.Contains(t, updated.VCardData, "Augusta", "the change must land")
	require.Contains(t, updated.VCardData, "PHOTO:", "a photo must survive a flat name edit")
	require.Contains(t, updated.VCardData, "Analytical Engine",
		"an unmodelled property must survive a flat name edit (go-vcard upper-cases the name)")
	require.Contains(t, updated.VCardData, "FN:Augusta Lovelace",
		"a renamed contact must not keep the old display name")
}
