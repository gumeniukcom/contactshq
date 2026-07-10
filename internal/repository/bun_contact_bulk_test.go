package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

// setupBulkTest creates two address books so that cross-tenant leakage is testable.
func setupBulkTest(t *testing.T) (context.Context, *repository.BunContactRepository, string, string) {
	t.Helper()

	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))
	repo := repository.NewBunContactRepository(db)

	mine := newBook(t, ctx, db, "mine@x.com")
	theirs := newBook(t, ctx, db, "theirs@x.com")

	return ctx, repo, mine, theirs
}

func newBook(t *testing.T, ctx context.Context, db *bun.DB, email string) string {
	t.Helper()

	userID := uuid.New().String()
	abID := uuid.New().String()
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, userID, email, "h")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO address_books (id, user_id, name) VALUES (?,?,?)`, abID, userID, "ab")
	require.NoError(t, err)
	return abID
}

func addContact(t *testing.T, ctx context.Context, repo *repository.BunContactRepository, abID, last string) *domain.Contact {
	t.Helper()

	c := &domain.Contact{
		ID:            uuid.New().String(),
		AddressBookID: abID,
		UID:           uuid.New().String(),
		FirstName:     "Test",
		LastName:      last,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, repo.Create(ctx, c))
	return c
}

func TestDeleteMany_RemovesOnlyTheNamedContacts(t *testing.T) {
	ctx, repo, abID, _ := setupBulkTest(t)
	a := addContact(t, ctx, repo, abID, "Anderson")
	b := addContact(t, ctx, repo, abID, "Brown")
	keep := addContact(t, ctx, repo, abID, "Keeper")

	deleted, err := repo.DeleteMany(ctx, abID, []string{a.ID, b.ID})
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)

	remaining, total, err := repo.List(ctx, abID, 10, 0, repository.ListFilters{})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, remaining, 1)
	assert.Equal(t, keep.ID, remaining[0].ID)
}

// An id from another user's address book must not be deletable by guessing it.
func TestDeleteMany_IgnoresContactsFromAnotherAddressBook(t *testing.T) {
	ctx, repo, mine, theirs := setupBulkTest(t)
	ours := addContact(t, ctx, repo, mine, "Mine")
	foreign := addContact(t, ctx, repo, theirs, "Theirs")

	deleted, err := repo.DeleteMany(ctx, mine, []string{ours.ID, foreign.ID})
	require.NoError(t, err)
	assert.Equal(t, 1, deleted, "only the contact in the caller's address book counts")

	still, err := repo.GetByID(ctx, foreign.ID)
	require.NoError(t, err)
	assert.NotNil(t, still, "the other address book must be untouched")
}

func TestDeleteMany_UnknownIDsAreNotAnError(t *testing.T) {
	ctx, repo, abID, _ := setupBulkTest(t)
	a := addContact(t, ctx, repo, abID, "Anderson")

	deleted, err := repo.DeleteMany(ctx, abID, []string{a.ID, uuid.New().String()})
	require.NoError(t, err)
	assert.Equal(t, 1, deleted, "the count reports what actually existed")
}

func TestDeleteMany_EmptyIDsIsANoop(t *testing.T) {
	ctx, repo, abID, _ := setupBulkTest(t)
	addContact(t, ctx, repo, abID, "Anderson")

	deleted, err := repo.DeleteMany(ctx, abID, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, deleted)

	_, total, err := repo.List(ctx, abID, 10, 0, repository.ListFilters{})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "an empty id list must never mean 'everything'")
}

func TestListByIDs_ReturnsOnlyOwnContactsInSortOrder(t *testing.T) {
	ctx, repo, mine, theirs := setupBulkTest(t)
	z := addContact(t, ctx, repo, mine, "Zimmerman")
	a := addContact(t, ctx, repo, mine, "Anderson")
	foreign := addContact(t, ctx, repo, theirs, "Theirs")

	got, err := repo.ListByIDs(ctx, mine, []string{z.ID, a.ID, foreign.ID})
	require.NoError(t, err)

	require.Len(t, got, 2, "a foreign id must not leak a contact")
	assert.Equal(t, "Anderson", got[0].LastName, "results follow the list view's sort order")
	assert.Equal(t, "Zimmerman", got[1].LastName)
}

func TestListByIDs_EmptyIDsReturnsNothing(t *testing.T) {
	ctx, repo, abID, _ := setupBulkTest(t)
	addContact(t, ctx, repo, abID, "Anderson")

	got, err := repo.ListByIDs(ctx, abID, nil)
	require.NoError(t, err)
	assert.Empty(t, got, "an empty id list must never mean 'everything'")
}
