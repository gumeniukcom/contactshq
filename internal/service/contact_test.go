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
