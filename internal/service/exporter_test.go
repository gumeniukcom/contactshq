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

func newExporterSvc(repo *mockContactRepo) *service.ExporterService {
	ab := &domain.AddressBook{ID: "ab1", UserID: "user1"}
	abRepo := &mockAbRepo{ab: ab}
	return service.NewExporterService(repo, abRepo)
}

func TestExportCSV_IncludesTitleNoteColumns(t *testing.T) {
	repo := newMockContactRepo()
	svc := newExporterSvc(repo)
	ctx := context.Background()

	// Seed a contact
	repo.contacts["c1"] = &domain.Contact{
		ID:            "c1",
		AddressBookID: "ab1",
		UID:           "uid1",
		FirstName:     "Alice",
		LastName:      "Smith",
		Email:         "alice@example.com",
		Title:         "CEO",
		Note:          "VIP",
	}

	csv, err := svc.ExportCSV(ctx, "user1")
	require.NoError(t, err)
	assert.Contains(t, csv, "title")
	assert.Contains(t, csv, "note")
	assert.Contains(t, csv, "CEO")
	assert.Contains(t, csv, "VIP")
}

func TestExportJSON_IncludesTitleNote(t *testing.T) {
	repo := newMockContactRepo()
	svc := newExporterSvc(repo)
	ctx := context.Background()

	repo.contacts["c1"] = &domain.Contact{
		ID:            "c1",
		AddressBookID: "ab1",
		UID:           "uid1",
		FirstName:     "Bob",
		Title:         "Director",
		Note:          "Important",
	}

	jsonStr, err := svc.ExportJSON(ctx, "user1")
	require.NoError(t, err)
	assert.True(t, strings.Contains(jsonStr, `"title"`), "JSON should include title field")
	assert.True(t, strings.Contains(jsonStr, `"note"`), "JSON should include note field")
	assert.Contains(t, jsonStr, "Director")
	assert.Contains(t, jsonStr, "Important")
}
