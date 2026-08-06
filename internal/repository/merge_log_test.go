package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

// The whole reason merge_log has no foreign key to contacts: potential_duplicates cascades on
// contact deletion, so history tied to a contact would vanish exactly when it becomes the
// only remaining evidence of the merge.
func TestMergeLog_SurvivesDeletionOfBothContacts(t *testing.T) {
	db := newPairDB(t)
	ctx := context.Background()
	seedPair(t, db)

	repo := repository.NewBunMergeLogRepository(db)
	require.NoError(t, repo.Create(ctx, &domain.MergeLogEntry{
		ID: "m1", UserID: "u1",
		WinnerID: "a", WinnerDisplayName: "Ada Lovelace",
		LoserUID: "b", LoserDisplayName: "Ada L",
		LoserVCard: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:b\r\nEND:VCARD\r\n",
		Resolution: `{"EMAIL":"loser"}`,
	}))

	// Delete both contacts — the pair row cascades away with them.
	_, err := db.NewDelete().Model((*domain.Contact)(nil)).Where("id IN (?, ?)", "a", "b").Exec(ctx)
	require.NoError(t, err)

	entries, err := repo.ListByUser(ctx, "u1", 50)
	require.NoError(t, err)
	require.Len(t, entries, 1, "merge history must outlive the contacts it describes")
	require.Equal(t, "Ada L", entries[0].LoserDisplayName)
	require.Contains(t, entries[0].LoserVCard, "UID:b", "the snapshot must survive too")
}

// Bun's default column mapping turns WinnerID into winner_i_d; the explicit tags are what
// keep the round trip honest.
func TestMergeLog_ColumnNamesRoundTrip(t *testing.T) {
	db := newPairDB(t)
	ctx := context.Background()
	seedPair(t, db)

	repo := repository.NewBunMergeLogRepository(db)
	require.NoError(t, repo.Create(ctx, &domain.MergeLogEntry{
		ID: "m1", UserID: "u1", WinnerID: "a", LoserUID: "uid-b",
		WinnerDisplayName: "W", LoserDisplayName: "L", Resolution: "{}",
	}))

	entries, err := repo.ListByUser(ctx, "u1", 50)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "a", entries[0].WinnerID)
	require.Equal(t, "uid-b", entries[0].LoserUID)
	require.False(t, entries[0].MergedAt.IsZero(), "merged_at must be populated")
}

func TestMergeLog_ListIsScopedToTheUserAndNewestFirst(t *testing.T) {
	db := newPairDB(t)
	ctx := context.Background()
	seedPair(t, db)

	repo := repository.NewBunMergeLogRepository(db)
	base := time.Now()
	for _, e := range []*domain.MergeLogEntry{
		{ID: "old", UserID: "u1", MergedAt: base.Add(-2 * time.Hour), Resolution: "{}"},
		{ID: "new", UserID: "u1", MergedAt: base, Resolution: "{}"},
		{ID: "theirs", UserID: "u2", MergedAt: base, Resolution: "{}"},
	} {
		require.NoError(t, repo.Create(ctx, e))
	}

	entries, err := repo.ListByUser(ctx, "u1", 50)
	require.NoError(t, err)
	require.Len(t, entries, 2, "another user's merges must not be visible")
	require.Equal(t, "new", entries[0].ID, "newest first")
	require.Equal(t, "old", entries[1].ID)
}

// Every row keeps a card snapshot, so retention is not optional book-keeping.
func TestMergeLog_DeleteOlderThanPrunesOnlyPastTheCutoff(t *testing.T) {
	db := newPairDB(t)
	ctx := context.Background()
	seedPair(t, db)

	repo := repository.NewBunMergeLogRepository(db)
	now := time.Now()
	for _, e := range []*domain.MergeLogEntry{
		{ID: "ancient", UserID: "u1", MergedAt: now.AddDate(0, 0, -90), Resolution: "{}"},
		{ID: "old", UserID: "u1", MergedAt: now.AddDate(0, 0, -31), Resolution: "{}"},
		{ID: "recent", UserID: "u1", MergedAt: now.AddDate(0, 0, -1), Resolution: "{}"},
	} {
		require.NoError(t, repo.Create(ctx, e))
	}

	removed, err := repo.DeleteOlderThan(ctx, now.AddDate(0, 0, -30))
	require.NoError(t, err)
	require.Equal(t, 2, removed)

	entries, err := repo.ListByUser(ctx, "u1", 50)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "recent", entries[0].ID)
}
