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

func setupSaveTest(t *testing.T) (context.Context, *repository.BunContactRepository, string) {
	t.Helper()

	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))
	repo := repository.NewBunContactRepository(db)

	userID := uuid.New().String()
	abID := uuid.New().String()
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, userID, "save@x.com", "h")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO address_books (id, user_id, name) VALUES (?,?,?)`, abID, userID, "ab")
	require.NoError(t, err)

	return ctx, repo, abID
}

func newContact(abID string) *domain.Contact {
	now := time.Now()
	return &domain.Contact{
		ID:            uuid.New().String(),
		AddressBookID: abID,
		UID:           uuid.New().String(),
		FirstName:     "Jane",
		LastName:      "Doe",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func TestSave_InsertsContactAndChildren(t *testing.T) {
	ctx, repo, abID := setupSaveTest(t)
	c := newContact(abID)

	err := repo.Save(ctx, c, domain.ChildRecords{
		Emails:     []*domain.ContactEmail{{Value: "jane@example.com", Type: "work"}},
		Phones:     []*domain.ContactPhone{{Value: "+15551234567", Type: "cell"}},
		Categories: []*domain.ContactCategory{{Value: "vip"}},
	})
	require.NoError(t, err)

	got, err := repo.GetByIDWithRelations(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "Jane", got.FirstName)
	require.Len(t, got.Emails, 1)
	assert.Equal(t, "jane@example.com", got.Emails[0].Value)
	assert.Equal(t, c.ID, got.Emails[0].ContactID)
	assert.NotEmpty(t, got.Emails[0].ID, "child rows need generated primary keys")
	require.Len(t, got.Phones, 1)
	require.Len(t, got.Categories, 1)
}

func TestSave_UpdatesExistingContactAndReplacesChildren(t *testing.T) {
	ctx, repo, abID := setupSaveTest(t)
	c := newContact(abID)

	require.NoError(t, repo.Save(ctx, c, domain.ChildRecords{
		Emails: []*domain.ContactEmail{
			{Value: "old-a@example.com"},
			{Value: "old-b@example.com"},
		},
	}))

	c.FirstName = "Janet"
	require.NoError(t, repo.Save(ctx, c, domain.ChildRecords{
		Emails: []*domain.ContactEmail{{Value: "new@example.com"}},
	}))

	got, err := repo.GetByIDWithRelations(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "Janet", got.FirstName)
	require.Len(t, got.Emails, 1, "old child rows must be replaced, not accumulated")
	assert.Equal(t, "new@example.com", got.Emails[0].Value)

	all, total, err := repo.List(ctx, abID, 10, 0, repository.ListFilters{})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "save must update in place, not insert a second row")
	assert.Len(t, all, 1)
}

// Writing the contact and its children in separate statements left a contact behind when
// a later child write failed, invisible to search because its rows never landed.
func TestSave_RollsBackContactWhenChildWriteFails(t *testing.T) {
	ctx, repo, abID := setupSaveTest(t)
	c := newContact(abID)

	// Two child rows sharing a primary key make the bulk insert fail.
	dupID := uuid.New().String()
	err := repo.Save(ctx, c, domain.ChildRecords{
		Emails: []*domain.ContactEmail{
			{ID: dupID, Value: "a@example.com"},
			{ID: dupID, Value: "b@example.com"},
		},
	})
	require.Error(t, err, "a failing child write must fail the whole save")

	got, err := repo.GetByID(ctx, c.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "the contact must not survive a rolled-back save")
}

// The same guarantee on the update path: a failed save must not leave the contact with
// half of its previous children.
func TestSave_RollsBackUpdateWhenChildWriteFails(t *testing.T) {
	ctx, repo, abID := setupSaveTest(t)
	c := newContact(abID)

	require.NoError(t, repo.Save(ctx, c, domain.ChildRecords{
		Emails: []*domain.ContactEmail{{Value: "original@example.com"}},
		Phones: []*domain.ContactPhone{{Value: "+15550000000"}},
	}))

	c.FirstName = "Mutated"
	dupID := uuid.New().String()
	err := repo.Save(ctx, c, domain.ChildRecords{
		Emails: []*domain.ContactEmail{{Value: "replacement@example.com"}},
		Phones: []*domain.ContactPhone{
			{ID: dupID, Value: "+15551111111"},
			{ID: dupID, Value: "+15552222222"},
		},
	})
	require.Error(t, err)

	got, err := repo.GetByIDWithRelations(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "Jane", got.FirstName, "the contact row must keep its previous value")
	require.Len(t, got.Emails, 1)
	assert.Equal(t, "original@example.com", got.Emails[0].Value, "children must keep their previous values")
	require.Len(t, got.Phones, 1)
	assert.Equal(t, "+15550000000", got.Phones[0].Value)
}

func TestSave_EmptyChildrenClearsExistingRows(t *testing.T) {
	ctx, repo, abID := setupSaveTest(t)
	c := newContact(abID)

	require.NoError(t, repo.Save(ctx, c, domain.ChildRecords{
		Emails: []*domain.ContactEmail{{Value: "gone@example.com"}},
	}))
	require.NoError(t, repo.Save(ctx, c, domain.ChildRecords{}))

	got, err := repo.GetByIDWithRelations(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.Emails, "removing every email from a vCard must remove its rows")
}

// An unknown id is a 404, not a 500: the getters must report absence rather than
// surfacing sql.ErrNoRows.
func TestGetWithRelations_UnknownIDReturnsNilWithoutError(t *testing.T) {
	ctx, repo, abID := setupSaveTest(t)

	byID, err := repo.GetByIDWithRelations(ctx, uuid.New().String())
	require.NoError(t, err)
	assert.Nil(t, byID)

	byUID, err := repo.GetByUIDWithRelations(ctx, abID, "no-such-uid")
	require.NoError(t, err)
	assert.Nil(t, byUID)
}
