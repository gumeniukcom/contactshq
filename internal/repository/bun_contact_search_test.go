package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

func TestContactSearch_ByChildEmail(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	uid := uuid.New().String()
	abID := uuid.New().String()
	cID := uuid.New().String()

	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, uid, "owner@x.com", "h")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO address_books (id, user_id, name) VALUES (?,?,?)`, abID, uid, "ab")
	require.NoError(t, err)

	now := time.Now()
	repo := repository.NewBunContactRepository(db)

	contact := &domain.Contact{
		ID: cID, AddressBookID: abID, UID: uuid.New().String(),
		ETag: "e", VCardData: "BEGIN:VCARD\nVERSION:4.0\nFN:Hidden Person\nEND:VCARD",
		FirstName: "Hidden", LastName: "Person",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, contact))

	// Add a child email not reflected in the flat email column.
	emails := []*domain.ContactEmail{{Value: "secret@work.org", Type: "work"}}
	require.NoError(t, repo.ReplaceEmails(ctx, cID, emails))

	// Search by the child email — should find the contact.
	results, total, err := repo.Search(ctx, abID, "secret@work.org", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, cID, results[0].ID)

	// Partial match also works.
	results, total, err = repo.Search(ctx, abID, "secret", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, results, 1)

	// Unrelated query returns nothing.
	results, total, err = repo.Search(ctx, abID, "nobody@nowhere.com", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, results)
}

func TestContactSearch_ByChildPhone(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	uid := uuid.New().String()
	abID := uuid.New().String()
	cID := uuid.New().String()

	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, uid, "owner2@x.com", "h")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO address_books (id, user_id, name) VALUES (?,?,?)`, abID, uid, "ab")
	require.NoError(t, err)

	now := time.Now()
	repo := repository.NewBunContactRepository(db)

	contact := &domain.Contact{
		ID: cID, AddressBookID: abID, UID: uuid.New().String(),
		ETag: "e2", VCardData: "BEGIN:VCARD\nVERSION:4.0\nFN:Phone Test\nEND:VCARD",
		FirstName: "Phone", LastName: "Test",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, contact))

	phones := []*domain.ContactPhone{{Value: "+49 30 987654321", Type: "work"}}
	require.NoError(t, repo.ReplacePhones(ctx, cID, phones))

	results, total, err := repo.Search(ctx, abID, "987654", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, cID, results[0].ID)
}

func TestContactSearch_ByChildCategory(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	uid := uuid.New().String()
	abID := uuid.New().String()
	cID := uuid.New().String()

	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, uid, "owner3@x.com", "h")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO address_books (id, user_id, name) VALUES (?,?,?)`, abID, uid, "ab")
	require.NoError(t, err)

	now := time.Now()
	repo := repository.NewBunContactRepository(db)

	contact := &domain.Contact{
		ID: cID, AddressBookID: abID, UID: uuid.New().String(),
		ETag: "e3", VCardData: "BEGIN:VCARD\nVERSION:4.0\nFN:Cat Test\nEND:VCARD",
		FirstName: "Cat", LastName: "Test",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, contact))

	cats := []*domain.ContactCategory{{Value: "vip-customer"}}
	require.NoError(t, repo.ReplaceCategories(ctx, cID, cats))

	results, total, err := repo.Search(ctx, abID, "vip", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, cID, results[0].ID)
}

func TestContactSearchWithRelations_LoadsChildRows(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	uid := uuid.New().String()
	abID := uuid.New().String()
	cID := uuid.New().String()

	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, uid, "owner4@x.com", "h")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO address_books (id, user_id, name) VALUES (?,?,?)`, abID, uid, "ab")
	require.NoError(t, err)

	now := time.Now()
	repo := repository.NewBunContactRepository(db)

	contact := &domain.Contact{
		ID: cID, AddressBookID: abID, UID: uuid.New().String(),
		ETag: "e4", VCardData: "BEGIN:VCARD\nVERSION:4.0\nFN:Multi Email\nEND:VCARD",
		FirstName: "Multi", LastName: "Email",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, contact))

	emails := []*domain.ContactEmail{
		{Value: "primary@test.com", Type: "work"},
		{Value: "secondary@test.com", Type: "home"},
	}
	require.NoError(t, repo.ReplaceEmails(ctx, cID, emails))

	results, total, err := repo.SearchWithRelations(ctx, abID, "Multi", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)

	// Relations should be loaded.
	require.NotNil(t, results[0].Emails)
	assert.Len(t, results[0].Emails, 2)
}
