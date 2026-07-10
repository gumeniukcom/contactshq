package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

func setupJournal(t *testing.T) (context.Context, *repository.BunContactRepository, *repository.BunAddressBookRepository, string) {
	t.Helper()

	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	userID := uuid.New().String()
	abID := uuid.New().String()
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, userID, "j@x.com", "h")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO address_books (id, user_id, name) VALUES (?,?,?)`, abID, userID, "ab")
	require.NoError(t, err)

	return ctx, repository.NewBunContactRepository(db), repository.NewBunAddressBookRepository(db), abID
}

func saveContact(t *testing.T, ctx context.Context, repo *repository.BunContactRepository, abID, uid string) *domain.Contact {
	t.Helper()

	c := newContact(abID)
	c.UID = uid
	require.NoError(t, repo.Save(ctx, c, domain.ChildRecords{}))
	return c
}

func uidsOfChanges(changes *repository.CollectionChanges) []string {
	out := make([]string, 0, len(changes.Updated))
	for _, c := range changes.Updated {
		out = append(out, c.UID)
	}
	return out
}

// The CTag has to move on every kind of write, or a polling client concludes nothing
// changed and never asks again.
func TestChangeSeq_AdvancesOnEveryWrite(t *testing.T) {
	ctx, repo, abRepo, abID := setupJournal(t)

	start, err := abRepo.ChangeSeq(ctx, abID)
	require.NoError(t, err)

	c := saveContact(t, ctx, repo, abID, "u1")
	afterCreate, err := abRepo.ChangeSeq(ctx, abID)
	require.NoError(t, err)
	assert.Greater(t, afterCreate, start, "creating a contact must advance the collection")

	c.FirstName = "Renamed"
	require.NoError(t, repo.Save(ctx, c, domain.ChildRecords{}))
	afterUpdate, err := abRepo.ChangeSeq(ctx, abID)
	require.NoError(t, err)
	assert.Greater(t, afterUpdate, afterCreate, "updating a contact must advance the collection")

	require.NoError(t, repo.Delete(ctx, c.ID))
	afterDelete, err := abRepo.ChangeSeq(ctx, abID)
	require.NoError(t, err)
	assert.Greater(t, afterDelete, afterUpdate, "deleting a contact must advance the collection")
}

// A client with no token gets everything and nothing to delete: it has nothing to delete.
func TestChangesSince_ZeroTokenReturnsWholeCollection(t *testing.T) {
	ctx, repo, _, abID := setupJournal(t)
	saveContact(t, ctx, repo, abID, "u1")
	c2 := saveContact(t, ctx, repo, abID, "u2")
	require.NoError(t, repo.Delete(ctx, c2.ID))

	changes, err := repo.ChangesSince(ctx, abID, 0)
	require.NoError(t, err)

	assert.Equal(t, []string{"u1"}, uidsOfChanges(changes))
	assert.Empty(t, changes.DeletedUIDs, "a client that has never synced has nothing to delete")
	assert.Positive(t, changes.Seq)
}

func TestChangesSince_ReportsOnlyWhatHappenedAfterTheToken(t *testing.T) {
	ctx, repo, _, abID := setupJournal(t)
	saveContact(t, ctx, repo, abID, "u1")

	baseline, err := repo.ChangesSince(ctx, abID, 0)
	require.NoError(t, err)

	saveContact(t, ctx, repo, abID, "u2")

	changes, err := repo.ChangesSince(ctx, abID, baseline.Seq)
	require.NoError(t, err)

	assert.Equal(t, []string{"u2"}, uidsOfChanges(changes), "the contact the client already has must not repeat")
	assert.Empty(t, changes.DeletedUIDs)
}

// The whole reason tombstones exist: a deletion cannot be inferred from the contacts table.
func TestChangesSince_ReportsDeletions(t *testing.T) {
	ctx, repo, _, abID := setupJournal(t)
	c1 := saveContact(t, ctx, repo, abID, "u1")
	saveContact(t, ctx, repo, abID, "u2")

	baseline, err := repo.ChangesSince(ctx, abID, 0)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, c1.ID))

	changes, err := repo.ChangesSince(ctx, abID, baseline.Seq)
	require.NoError(t, err)

	assert.Equal(t, []string{"u1"}, changes.DeletedUIDs)
	assert.Empty(t, uidsOfChanges(changes), "a deleted contact is not also an update")
}

func TestChangesSince_ReportsBulkDeletions(t *testing.T) {
	ctx, repo, _, abID := setupJournal(t)
	c1 := saveContact(t, ctx, repo, abID, "u1")
	c2 := saveContact(t, ctx, repo, abID, "u2")
	saveContact(t, ctx, repo, abID, "u3")

	baseline, err := repo.ChangesSince(ctx, abID, 0)
	require.NoError(t, err)

	deleted, err := repo.DeleteMany(ctx, abID, []string{c1.ID, c2.ID})
	require.NoError(t, err)
	require.Equal(t, 2, deleted)

	changes, err := repo.ChangesSince(ctx, abID, baseline.Seq)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"u1", "u2"}, changes.DeletedUIDs)
}

func TestChangesSince_ReportsDeleteAll(t *testing.T) {
	ctx, repo, _, abID := setupJournal(t)
	saveContact(t, ctx, repo, abID, "u1")
	saveContact(t, ctx, repo, abID, "u2")

	baseline, err := repo.ChangesSince(ctx, abID, 0)
	require.NoError(t, err)

	require.NoError(t, repo.DeleteAll(ctx, abID))

	changes, err := repo.ChangesSince(ctx, abID, baseline.Seq)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"u1", "u2"}, changes.DeletedUIDs)
	assert.Empty(t, uidsOfChanges(changes))
}

// A contact recreated under a UID it once had is present again. Leaving the tombstone
// would tell a synchronising client to delete the contact it just received.
func TestChangesSince_RecreatedUIDHasNoTombstone(t *testing.T) {
	ctx, repo, _, abID := setupJournal(t)
	c1 := saveContact(t, ctx, repo, abID, "u1")

	baseline, err := repo.ChangesSince(ctx, abID, 0)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, c1.ID))
	saveContact(t, ctx, repo, abID, "u1")

	changes, err := repo.ChangesSince(ctx, abID, baseline.Seq)
	require.NoError(t, err)

	assert.Empty(t, changes.DeletedUIDs, "the UID exists again; it must not be reported as deleted")
	assert.Equal(t, []string{"u1"}, uidsOfChanges(changes))
}

// Deleting a contact that is not there must not invent a tombstone.
func TestDelete_UnknownContactIsANoop(t *testing.T) {
	ctx, repo, abRepo, abID := setupJournal(t)
	before, err := abRepo.ChangeSeq(ctx, abID)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, uuid.New().String()))

	after, err := abRepo.ChangeSeq(ctx, abID)
	require.NoError(t, err)
	assert.Equal(t, before, after, "nothing changed, so the collection did not change")
}
