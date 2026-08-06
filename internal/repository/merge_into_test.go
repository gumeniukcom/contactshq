package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

// MergeInto has to leave the change journal describing both halves of the merge, or CardDAV
// clients never learn the loser disappeared and keep it on the phone forever.
func TestMergeInto_JournalsBothTheUpdateAndTheDeletion(t *testing.T) {
	db := newPairDB(t)
	ctx := context.Background()
	seedPair(t, db)

	repo := repository.NewBunContactRepository(db)

	// Advance the collection counter first: ChangesSince treats sinceSeq==0 as "the client
	// has never seen this collection" and deliberately withholds tombstones, so a merge
	// measured from zero would look tombstone-free for a reason unrelated to the merge.
	seed, err := repo.GetByID(ctx, "a")
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, seed, domain.ChildRecords{}))

	before, err := repository.NewBunAddressBookRepository(db).ChangeSeq(ctx, "ab1")
	require.NoError(t, err)
	require.Positive(t, before)

	winner, err := repo.GetByID(ctx, "a")
	require.NoError(t, err)
	winner.VCardData = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:a\r\nFN:Merged\r\nEND:VCARD\r\n"
	winner.FirstName = "Merged"

	require.NoError(t, repo.MergeInto(ctx, winner, domain.ChildRecords{
		Emails: []*domain.ContactEmail{{Value: "merged@example.com"}},
	}, "b"))

	// The loser is gone, the winner is updated.
	gone, err := repo.GetByID(ctx, "b")
	require.NoError(t, err)
	require.Nil(t, gone)

	survivor, err := repo.GetByIDWithRelations(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, "Merged", survivor.FirstName)
	require.Len(t, survivor.Emails, 1)
	require.Equal(t, "merged@example.com", survivor.Emails[0].Value)

	// A client syncing from before the merge must see exactly two things.
	changes, err := repo.ChangesSince(ctx, "ab1", before)
	require.NoError(t, err)

	require.Len(t, changes.DeletedUIDs, 1, "the removed card must be tombstoned")
	require.Equal(t, "b", changes.DeletedUIDs[0], "the tombstone carries the loser's UID")

	var changedUIDs []string
	for _, c := range changes.Updated {
		changedUIDs = append(changedUIDs, c.UID)
	}
	require.Equal(t, []string{"a"}, changedUIDs, "only the surviving card changed")
}

// Both halves share one sequence number: a client reading the journal sees one event, not a
// window in which the contact existed twice or not at all.
func TestMergeInto_UsesOneSequenceForBothChanges(t *testing.T) {
	db := newPairDB(t)
	ctx := context.Background()
	seedPair(t, db)

	repo := repository.NewBunContactRepository(db)
	winner, err := repo.GetByID(ctx, "a")
	require.NoError(t, err)

	require.NoError(t, repo.MergeInto(ctx, winner, domain.ChildRecords{}, "b"))

	survivor, err := repo.GetByID(ctx, "a")
	require.NoError(t, err)

	seq, err := repository.NewBunAddressBookRepository(db).ChangeSeq(ctx, "ab1")
	require.NoError(t, err)
	require.Equal(t, seq, survivor.ChangeSeq,
		"the winner's change_seq must be the collection's current one")

	// Nothing is visible from that sequence onwards; the merge is entirely below it.
	changes, err := repo.ChangesSince(ctx, "ab1", seq)
	require.NoError(t, err)
	require.Empty(t, changes.Updated)
	require.Empty(t, changes.DeletedUIDs)
}

// A merge is one transaction: a winner saved without the loser being removed is the state the
// separate-calls version could leave behind, and the next sync reads it as two contacts.
func TestMergeInto_UnknownLoserStillSavesTheWinner(t *testing.T) {
	db := newPairDB(t)
	ctx := context.Background()
	seedPair(t, db)

	repo := repository.NewBunContactRepository(db)
	winner, err := repo.GetByID(ctx, "a")
	require.NoError(t, err)
	winner.FirstName = "Still Saved"

	require.NoError(t, repo.MergeInto(ctx, winner, domain.ChildRecords{}, "does-not-exist"))

	got, err := repo.GetByID(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, "Still Saved", got.FirstName)
}
