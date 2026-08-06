package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type mockMergeLogRepo struct {
	entries []*domain.MergeLogEntry
	err     error
}

func (m *mockMergeLogRepo) Create(_ context.Context, e *domain.MergeLogEntry) error {
	if m.err != nil {
		return m.err
	}
	m.entries = append(m.entries, e)
	return nil
}

func (m *mockMergeLogRepo) ListByUser(_ context.Context, userID string, _ int) ([]*domain.MergeLogEntry, error) {
	var out []*domain.MergeLogEntry
	for _, e := range m.entries {
		if e.UserID == userID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *mockMergeLogRepo) DeleteOlderThan(context.Context, time.Time) (int, error) { return 0, nil }

// recordingSyncStateRepo tracks which states were deleted.
type recordingSyncStateRepo struct {
	mockSyncStateRepo
	states  []*domain.SyncState
	deleted []string
}

func (r *recordingSyncStateRepo) ListAllByUser(context.Context, string) ([]*domain.SyncState, error) {
	return r.states, nil
}

func (r *recordingSyncStateRepo) Delete(_ context.Context, id string) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func TestMerge_RecordsTheMergeWithASnapshotOfTheLoser(t *testing.T) {
	svc, _, _ := setupMerge(t)
	logRepo := &mockMergeLogRepo{}
	svc.WithMergeLog(logRepo)

	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{
		WinnerID: "w1", LoserID: "l1",
		Resolution: map[string]string{"EMAIL": "loser"},
	})
	require.NoError(t, err)

	require.Len(t, logRepo.entries, 1)
	entry := logRepo.entries[0]
	require.Equal(t, "u1", entry.UserID)
	require.Equal(t, "w1", entry.WinnerID)
	require.Equal(t, "l1", entry.LoserUID)
	require.Equal(t, "Ada Lovelace", entry.LoserDisplayName)
	require.Contains(t, entry.LoserVCard, "UID:l1", "the snapshot must let the card be recreated")
	// The record keeps whichever form of choice was made, so a merge stays explainable
	// whether it came from the quick buttons or the per-value screen.
	require.JSONEq(t, `{"resolution":{"EMAIL":"loser"},"selection":null,"dup_id":""}`, entry.Resolution)
	require.False(t, entry.MergedAt.IsZero())
}

// An embedded photo is hundreds of kilobytes and adds nothing to a snapshot meant for
// recreating names and numbers by hand.
func TestMerge_SnapshotStripsThePhoto(t *testing.T) {
	svc, contactRepo, _ := setupMerge(t)
	logRepo := &mockMergeLogRepo{}
	svc.WithMergeLog(logRepo)

	contactRepo.contacts["l1"].VCardData = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:l1\r\nFN:Ada Lovelace\r\n" +
		"PHOTO:data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAA\r\nEND:VCARD\r\n"

	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{WinnerID: "w1", LoserID: "l1"})
	require.NoError(t, err)

	require.Len(t, logRepo.entries, 1)
	require.NotContains(t, logRepo.entries[0].LoserVCard, "/9j/4AAQ", "image data must not be stored")
	require.Contains(t, logRepo.entries[0].LoserVCard, "FN:Ada Lovelace", "everything else must survive")
}

// A sync state left pointing at a deleted contact makes the next export either recreate the
// contact from the remote or raise a conflict for something that no longer exists.
func TestMerge_ClearsTheLosersSyncState(t *testing.T) {
	svc, _, _ := setupMerge(t)
	syncRepo := &recordingSyncStateRepo{states: []*domain.SyncState{
		{ID: "s-loser", UserID: "u1", ProviderType: "carddav", LocalID: "l1"},
		{ID: "s-loser-google", UserID: "u1", ProviderType: "google", LocalID: "l1"},
		{ID: "s-winner", UserID: "u1", ProviderType: "carddav", LocalID: "w1"},
	}}
	svc.WithSyncStateRepo(syncRepo)

	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{WinnerID: "w1", LoserID: "l1"})
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"s-loser", "s-loser-google"}, syncRepo.deleted,
		"every provider's state for the deleted contact must go")
	require.NotContains(t, syncRepo.deleted, "s-winner",
		"the surviving contact keeps its sync state, or the next run re-uploads it as new")
}

// The merge is what the user asked for; a failure to journal it must not undo it, but it must
// not pass unnoticed either.
func TestMerge_LogFailureIsReportedAndDoesNotAbortTheMerge(t *testing.T) {
	svc, contactRepo, _ := setupMerge(t)
	core, logs := observer.New(zap.WarnLevel)
	svc.WithLogger(zap.New(core)).WithMergeLog(&mockMergeLogRepo{err: errors.New("disk on fire")})

	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{WinnerID: "w1", LoserID: "l1"})
	require.NoError(t, err, "a journalling failure must not fail the merge")
	require.NotContains(t, contactRepo.contacts, "l1", "the merge still happened")
	require.NotZero(t, logs.FilterMessage("failed to record the merge").Len())
}

// Without a log repository the service must behave exactly as before.
func TestMerge_WorksWithoutAMergeLog(t *testing.T) {
	svc, contactRepo, _ := setupMerge(t)

	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{WinnerID: "w1", LoserID: "l1"})
	require.NoError(t, err)
	require.NotContains(t, contactRepo.contacts, "l1")
}
