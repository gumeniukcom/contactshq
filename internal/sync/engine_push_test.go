package sync_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
)

// googleLike mimics the People API: a created contact comes back under a server-assigned
// resourceName that bears no relation to the id we sent.
func googleLike(name string) *memProvider {
	p := newMemProvider(name)
	n := 0
	p.idAssigner = func(chqsync.SyncItem) string {
		n++
		return fmt.Sprintf("people/c%d", n)
	}
	return p
}

// --- mock SyncConflictRepository ---

type mockConflictRepo struct {
	conflicts map[string]*domain.SyncConflict
	creates   int
	updates   int
}

func newMockConflictRepo() *mockConflictRepo {
	return &mockConflictRepo{conflicts: make(map[string]*domain.SyncConflict)}
}

func (m *mockConflictRepo) Create(_ context.Context, c *domain.SyncConflict) error {
	m.conflicts[c.ID] = c
	m.creates++
	return nil
}

func (m *mockConflictRepo) GetByID(_ context.Context, id string) (*domain.SyncConflict, error) {
	return m.conflicts[id], nil
}

func (m *mockConflictRepo) ListByUser(_ context.Context, _, _ string, _, _ int) ([]*domain.SyncConflict, int, error) {
	return nil, 0, nil
}

func (m *mockConflictRepo) ListPendingByProvider(_ context.Context, userID, pt string) ([]*domain.SyncConflict, error) {
	var out []*domain.SyncConflict
	for _, c := range m.conflicts {
		if c.UserID == userID && c.ProviderType == pt && c.Status == "pending" {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *mockConflictRepo) Update(_ context.Context, c *domain.SyncConflict) error {
	m.conflicts[c.ID] = c
	m.updates++
	return nil
}

func (m *mockConflictRepo) DeleteByProvider(_ context.Context, _, _ string) error { return nil }

func (m *mockConflictRepo) CountPending(_ context.Context, _ string) (int, error) {
	return len(m.conflicts), nil
}

func (m *mockConflictRepo) pendingCount() int {
	n := 0
	for _, c := range m.conflicts {
		if c.Status == "pending" {
			n++
		}
	}
	return n
}

// A contact created locally and pushed to Google must survive the next sync.
//
// Before the RemoteID fix, push recorded the local UID as the remote id. Google's
// listing keys by resourceName, so the following pull saw the contact as new (importing
// a duplicate) and saw the tracked local UID as missing from the remote (deleting the
// original).
func TestPush_CapturesRemoteAssignedID(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	local := newMemProvider("internal")
	remote := googleLike("google")

	local.items["local-uid-1"] = chqsync.SyncItem{
		RemoteID: "local-uid-1", ETag: "l1", VCardData: makeVCard("local-uid-1", "Alice"),
	}

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeExport)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)

	states, err := syncRepo.ListByUser(ctx, "u1", "google->internal")
	require.NoError(t, err)
	require.Len(t, states, 1)

	assert.Equal(t, "people/c1", states[0].RemoteID, "must record the id Google assigned")
	assert.Equal(t, "local-uid-1", states[0].LocalID, "must keep the local id separate")

	_, stored := remote.items["people/c1"]
	assert.True(t, stored, "contact should live under its Google resourceName")
}

// The full destructive scenario: push a local contact, then run a bidirectional sync.
// The contact must not be duplicated locally, and must not be deleted.
func TestBidirectional_DoesNotDestroyPushedContact(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	local := newMemProvider("internal")
	remote := googleLike("google")

	local.items["local-uid-1"] = chqsync.SyncItem{
		RemoteID: "local-uid-1", ETag: "l1", VCardData: makeVCard("local-uid-1", "Alice"),
	}

	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeExport)
	require.NoError(t, err)

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeTwoWay)
	require.NoError(t, err)

	assert.Equal(t, 0, res.Deleted, "the pushed contact must not be deleted locally")
	assert.Equal(t, 0, res.Created, "the same contact must not be re-imported as new")
	assert.Len(t, local.items, 1, "no duplicate contact locally")
	assert.Contains(t, local.items, "local-uid-1")
	assert.Len(t, remote.items, 1, "no duplicate contact remotely")
}

// Deleting a contact locally must delete it remotely under its remote id, not the local one.
func TestPush_DeleteUsesRemoteID(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	local := newMemProvider("internal")
	remote := googleLike("google")

	for i := 1; i <= 4; i++ {
		uid := fmt.Sprintf("local-uid-%d", i)
		local.items[uid] = chqsync.SyncItem{RemoteID: uid, ETag: "l", VCardData: makeVCard(uid, "Contact")}
	}

	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeExport)
	require.NoError(t, err)
	require.Len(t, remote.items, 4)

	delete(local.items, "local-uid-2")

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeExport)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Deleted)
	assert.Len(t, remote.items, 3, "exactly one contact removed remotely")
}

// A local edit must not silently overwrite a remote edit made since the last sync.
func TestPush_RemoteChangedSinceLastSync_QueuesConflict(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	conflictRepo := newMockConflictRepo()
	engine := chqsync.NewEngineWithAllRepos(syncRepo, nil, conflictRepo, zap.NewNop())

	local := newMemProvider("internal")
	remote := newMemProvider("carddav")

	local.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "l1", VCardData: makeVCard("uid1", "Alice")}

	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictManual, chqsync.SyncModeExport)
	require.NoError(t, err)

	// Both sides diverge from the synced base.
	local.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "l2", VCardData: makeVCard("uid1", "Alice Local")}
	remote.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "r2", VCardData: makeVCard("uid1", "Alice Remote")}

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictManual, chqsync.SyncModeExport)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Conflicts)
	assert.Equal(t, 1, res.Skipped)
	assert.Equal(t, 1, conflictRepo.pendingCount())
	assert.Contains(t, remote.items["uid1"].VCardData, "Alice Remote", "remote edit must survive")
}

// dest_wins means the local copy is authoritative, so the push proceeds.
func TestPush_DestWinsOverwritesRemote(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	local := newMemProvider("internal")
	remote := newMemProvider("carddav")

	local.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "l1", VCardData: makeVCard("uid1", "Alice")}
	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictDestWins, chqsync.SyncModeExport)
	require.NoError(t, err)

	local.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "l2", VCardData: makeVCard("uid1", "Alice Local")}
	remote.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "r2", VCardData: makeVCard("uid1", "Alice Remote")}

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictDestWins, chqsync.SyncModeExport)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Updated)
	assert.Contains(t, remote.items["uid1"].VCardData, "Alice Local")
}

// An empty or truncated provider listing must abort the run rather than delete everything.
func TestPull_EmptySourceListingAbortsInsteadOfDeleting(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")

	for i := 1; i <= 6; i++ {
		uid := fmt.Sprintf("uid%d", i)
		src.items[uid] = chqsync.SyncItem{RemoteID: uid, ETag: "e", VCardData: makeVCard(uid, "Contact")}
	}

	_, err := engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)
	require.Len(t, dst.items, 6)

	// The provider now answers with nothing — an expired token, not six deletions.
	src.items = map[string]chqsync.SyncItem{}

	_, err = engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.ErrorIs(t, err, chqsync.ErrMassDeletion)
	assert.Len(t, dst.items, 6, "local contacts must be untouched")
}

// A plausible number of deletions still propagates.
func TestPull_SmallDeletionPropagates(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")

	for i := 1; i <= 6; i++ {
		uid := fmt.Sprintf("uid%d", i)
		src.items[uid] = chqsync.SyncItem{RemoteID: uid, ETag: "e", VCardData: makeVCard(uid, "Contact")}
	}
	_, err := engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)

	delete(src.items, "uid3")

	res, err := engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Deleted)
	assert.Len(t, dst.items, 5)
}

// Below the minimum tracked count the guard stays out of the way, or a user with two
// contacts could never delete one.
func TestPull_DeletionGuardIgnoresTinyAddressBooks(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")

	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e", VCardData: makeVCard("uid1", "A")}
	src.items["uid2"] = chqsync.SyncItem{RemoteID: "uid2", ETag: "e", VCardData: makeVCard("uid2", "B")}
	_, err := engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)

	src.items = map[string]chqsync.SyncItem{}

	res, err := engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Deleted)
	assert.Empty(t, dst.items)
}

// Repeated runs over an unresolved conflict must update one row, not append a new one
// on every scheduler tick.
func TestPull_ConflictIsDedupedAcrossRuns(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	conflictRepo := newMockConflictRepo()
	engine := chqsync.NewEngineWithAllRepos(syncRepo, nil, conflictRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")

	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e1", VCardData: makeVCard("uid1", "Alice")}
	_, err := engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictManual, chqsync.SyncModeImport)
	require.NoError(t, err)

	// Diverge both sides on the same field.
	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e2", VCardData: makeVCard("uid1", "Alice Remote")}
	dst.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "d2", VCardData: makeVCard("uid1", "Alice Local")}

	for run := 0; run < 3; run++ {
		_, err = engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictManual, chqsync.SyncModeImport)
		require.NoError(t, err)
	}

	assert.Equal(t, 1, conflictRepo.pendingCount(), "one pending conflict, not one per run")
	assert.Equal(t, 1, conflictRepo.creates)
}

// "manual" must never resolve a conflict on its own, even when a merge would succeed.
func TestPull_ManualModeDoesNotAutoMerge(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	conflictRepo := newMockConflictRepo()
	engine := chqsync.NewEngineWithAllRepos(syncRepo, nil, conflictRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")

	base := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:uid1\r\nFN:Alice\r\nEND:VCARD\r\n"
	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e1", VCardData: base}
	_, err := engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictManual, chqsync.SyncModeImport)
	require.NoError(t, err)

	// Disjoint edits — a three-way merge would combine them without conflict.
	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e2",
		VCardData: "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:uid1\r\nFN:Alice\r\nEMAIL:a@remote.com\r\nEND:VCARD\r\n"}
	dst.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "d2",
		VCardData: "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:uid1\r\nFN:Alice\r\nTEL:+15551234567\r\nEND:VCARD\r\n"}

	res, err := engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictManual, chqsync.SyncModeImport)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Skipped, "manual mode must defer to the user")
	assert.Equal(t, 0, res.Updated)
	assert.Equal(t, 1, conflictRepo.pendingCount())
	assert.Contains(t, dst.items["uid1"].VCardData, "+15551234567", "local copy untouched")
}

// source_wins resolves without asking, and must not leave a pending conflict behind.
func TestPull_SourceWinsLeavesNoPendingConflict(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	conflictRepo := newMockConflictRepo()
	engine := chqsync.NewEngineWithAllRepos(syncRepo, nil, conflictRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")

	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e1", VCardData: makeVCard("uid1", "Alice")}
	_, err := engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictSourceWins, chqsync.SyncModeImport)
	require.NoError(t, err)

	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e2", VCardData: makeVCard("uid1", "Alice Remote")}
	dst.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "d2", VCardData: makeVCard("uid1", "Alice Local")}

	res, err := engine.Sync(ctx, "u1", "p1", src, dst, chqsync.ConflictSourceWins, chqsync.SyncModeImport)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Updated)
	assert.Equal(t, 0, conflictRepo.pendingCount(), "source_wins resolves; nothing to review")
	assert.Contains(t, dst.items["uid1"].VCardData, "Alice Remote")
}
