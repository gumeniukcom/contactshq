package sync_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
)

// --- incremental provider fake ---

// incProvider is a memProvider that also serves deltas. It records the cursors it is
// asked with, so a test can prove the engine passes the last one back.
type incProvider struct {
	*memProvider
	deltas      map[string]chqsync.Delta // cursor -> what to return for it
	askedWith   []string
	expireOnce  bool // return ErrCursorExpired the first time a non-empty cursor is used
	expiredSeen bool
}

func newIncProvider(name string) *incProvider {
	return &incProvider{memProvider: newMemProvider(name), deltas: map[string]chqsync.Delta{}}
}

func (p *incProvider) ListChanges(_ context.Context, cursor string) (chqsync.Delta, error) {
	p.askedWith = append(p.askedWith, cursor)

	if p.expireOnce && cursor != "" && !p.expiredSeen {
		p.expiredSeen = true
		return chqsync.Delta{}, chqsync.ErrCursorExpired
	}

	if d, ok := p.deltas[cursor]; ok {
		return d, nil
	}
	// Default: an empty cursor yields the whole collection; anything else, nothing new.
	if cursor == "" {
		items := make([]chqsync.SyncItem, 0, len(p.items))
		for _, v := range p.items {
			items = append(items, v)
		}
		return chqsync.Delta{Updated: items, Full: true, Cursor: "c1"}, nil
	}
	return chqsync.Delta{Cursor: cursor}, nil
}

var _ chqsync.IncrementalProvider = (*incProvider)(nil)

// --- cursor store fake ---

type memCursorStore struct {
	cursors map[string]string // key -> cursor
}

func newMemCursorStore() *memCursorStore {
	return &memCursorStore{cursors: map[string]string{}}
}

func key(userID, providerType string) string { return userID + "|" + providerType }

func (s *memCursorStore) Get(_ context.Context, userID, providerType string) (string, error) {
	return s.cursors[key(userID, providerType)], nil
}
func (s *memCursorStore) Set(_ context.Context, userID, providerType, cursor string) error {
	s.cursors[key(userID, providerType)] = cursor
	return nil
}
func (s *memCursorStore) Delete(_ context.Context, userID, providerType string) error {
	delete(s.cursors, key(userID, providerType))
	return nil
}

func newIncEngine(store chqsync.CursorStore) (*chqsync.Engine, *mockSyncStateRepo) {
	repo := newMockSyncStateRepo()
	return chqsync.NewEngine(repo, zap.NewNop()).WithCursorStore(store), repo
}

// The first sync has no cursor, so the provider returns everything and the engine stores
// the fresh cursor for next time.
func TestIncremental_FirstSyncIsFullAndStoresCursor(t *testing.T) {
	ctx := context.Background()
	store := newMemCursorStore()
	engine, _ := newIncEngine(store)

	remote := newIncProvider("google")
	local := newMemProvider("internal")
	remote.items["r1"] = chqsync.SyncItem{RemoteID: "r1", ETag: "e1", VCardData: makeVCard("r1", "Alice")}
	remote.items["r2"] = chqsync.SyncItem{RemoteID: "r2", ETag: "e1", VCardData: makeVCard("r2", "Bob")}

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)

	assert.Equal(t, 2, res.Created)
	assert.Len(t, local.items, 2)
	assert.Equal(t, []string{""}, remote.askedWith, "the first call uses an empty cursor")

	stored, _ := store.Get(ctx, "u1", "google->internal")
	assert.Equal(t, "c1", stored, "the fresh cursor must be stored for next time")
}

// A delta must apply only what it names and delete only what it names — not reconcile
// against a full list.
func TestIncremental_DeltaAppliesOnlyReportedChanges(t *testing.T) {
	ctx := context.Background()
	store := newMemCursorStore()
	engine, syncRepo := newIncEngine(store)

	remote := newIncProvider("google")
	local := newMemProvider("internal")
	remote.items["r1"] = chqsync.SyncItem{RemoteID: "r1", ETag: "e1", VCardData: makeVCard("r1", "Alice")}
	remote.items["r2"] = chqsync.SyncItem{RemoteID: "r2", ETag: "e1", VCardData: makeVCard("r2", "Bob")}

	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)
	require.Len(t, local.items, 2)

	// The next sync from cursor c1 adds r3 and deletes r1, and says nothing about r2.
	remote.deltas["c1"] = chqsync.Delta{
		Updated: []chqsync.SyncItem{{RemoteID: "r3", ETag: "e1", VCardData: makeVCard("r3", "Carol")}},
		Deleted: []string{"r1"},
		Cursor:  "c2",
	}

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Created, "only r3 is new")
	assert.Equal(t, 1, res.Deleted, "only r1 is deleted")
	assert.Len(t, local.items, 2, "r2 and r3 remain")

	states, _ := syncRepo.ListByUser(ctx, "u1", "google->internal")
	remoteIDs := map[string]bool{}
	for _, s := range states {
		remoteIDs[s.RemoteID] = true
	}
	assert.True(t, remoteIDs["r2"] && remoteIDs["r3"])
	assert.False(t, remoteIDs["r1"], "the deleted contact's state is gone")

	stored, _ := store.Get(ctx, "u1", "google->internal")
	assert.Equal(t, "c2", stored)
}

// A contact untouched since the last sync must not be re-processed just because a full
// listing would still contain it.
func TestIncremental_UnchangedContactsAreNotTouched(t *testing.T) {
	ctx := context.Background()
	store := newMemCursorStore()
	engine, _ := newIncEngine(store)

	remote := newIncProvider("google")
	local := newMemProvider("internal")
	remote.items["r1"] = chqsync.SyncItem{RemoteID: "r1", ETag: "e1", VCardData: makeVCard("r1", "Alice")}

	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)

	// An empty delta: nothing changed.
	remote.deltas["c1"] = chqsync.Delta{Cursor: "c1"}

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)

	assert.Equal(t, 0, res.Created)
	assert.Equal(t, 0, res.Updated)
	assert.Equal(t, 0, res.Deleted)
	assert.Len(t, local.items, 1)
}

// An expired cursor is dropped and the collection re-listed in full, which reconciles the
// deletion the delta could no longer report.
func TestIncremental_ExpiredCursorResyncsFully(t *testing.T) {
	ctx := context.Background()
	store := newMemCursorStore()
	engine, _ := newIncEngine(store)

	remote := newIncProvider("google")
	local := newMemProvider("internal")
	remote.items["r1"] = chqsync.SyncItem{RemoteID: "r1", ETag: "e1", VCardData: makeVCard("r1", "Alice")}
	remote.items["r2"] = chqsync.SyncItem{RemoteID: "r2", ETag: "e1", VCardData: makeVCard("r2", "Bob")}
	remote.items["r3"] = chqsync.SyncItem{RemoteID: "r3", ETag: "e1", VCardData: makeVCard("r3", "Carol")}
	remote.items["r4"] = chqsync.SyncItem{RemoteID: "r4", ETag: "e1", VCardData: makeVCard("r4", "Dave")}

	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)
	require.Len(t, local.items, 4)

	// The provider now rejects the stored cursor once. Meanwhile r1 was removed remotely.
	remote.expireOnce = true
	delete(remote.items, "r1")

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)

	assert.True(t, remote.expiredSeen, "the stored cursor was tried and rejected")
	assert.Contains(t, remote.askedWith, "", "the engine retried from an empty cursor")
	assert.Equal(t, 1, res.Deleted, "the full re-list reconciles the removed contact")
	assert.Len(t, local.items, 3)
}

// The mass-deletion guard stays active in delta mode: a delta claiming most of the
// address book is gone must be refused, because a parsing bug looks exactly like this.
func TestIncremental_MassDeletionInDeltaIsRefused(t *testing.T) {
	ctx := context.Background()
	store := newMemCursorStore()
	engine, _ := newIncEngine(store)

	remote := newIncProvider("google")
	local := newMemProvider("internal")
	for _, id := range []string{"r1", "r2", "r3", "r4", "r5", "r6"} {
		remote.items[id] = chqsync.SyncItem{RemoteID: id, ETag: "e1", VCardData: makeVCard(id, id)}
	}

	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)
	require.Len(t, local.items, 6)

	remote.deltas["c1"] = chqsync.Delta{
		Deleted: []string{"r1", "r2", "r3", "r4", "r5"},
		Cursor:  "c2",
	}

	_, err = engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.ErrorIs(t, err, chqsync.ErrMassDeletion)
	assert.Len(t, local.items, 6, "nothing is deleted when the guard trips")

	stored, _ := store.Get(ctx, "u1", "google->internal")
	assert.Equal(t, "c1", stored, "the cursor must not advance past a refused delta")
}

// A provider that does not implement IncrementalProvider still syncs, the full way, even
// with a cursor store present.
func TestIncremental_NonIncrementalProviderStillFullSyncs(t *testing.T) {
	ctx := context.Background()
	store := newMemCursorStore()
	engine, _ := newIncEngine(store)

	remote := newMemProvider("carddav") // plain SyncProvider
	local := newMemProvider("internal")
	remote.items["r1"] = chqsync.SyncItem{RemoteID: "r1", ETag: "e1", VCardData: makeVCard("r1", "Alice")}

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictAuto, chqsync.SyncModeImport)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)

	stored, _ := store.Get(ctx, "u1", "carddav->internal")
	assert.Empty(t, stored, "a non-incremental provider stores no cursor")
}

// --- conditional writer fake ---

// condProvider is a memProvider that writes conditionally: a stale If-Match is rejected,
// like a CardDAV server honouring the header or Google enforcing an etag.
type condProvider struct {
	*memProvider
	listCalls int
}

func newCondProvider(name string) *condProvider {
	return &condProvider{memProvider: newMemProvider(name)}
}

func (p *condProvider) List(ctx context.Context) ([]chqsync.SyncItem, error) {
	p.listCalls++
	return p.memProvider.List(ctx)
}

func (p *condProvider) PutIfMatch(_ context.Context, item chqsync.SyncItem, ifMatch string) (chqsync.PutResult, error) {
	existing, ok := p.items[item.RemoteID]
	if ifMatch == "" {
		if ok {
			return chqsync.PutResult{}, chqsync.ErrPreconditionFailed
		}
	} else if !ok || existing.ETag != ifMatch {
		return chqsync.PutResult{}, chqsync.ErrPreconditionFailed
	}

	h := sha256.Sum256([]byte(item.VCardData))
	item.ETag = hex.EncodeToString(h[:8])
	item.ContentHash = hex.EncodeToString(h[:])
	p.items[item.RemoteID] = item
	return chqsync.PutResult{RemoteID: item.RemoteID, ETag: item.ETag}, nil
}

var _ chqsync.ConditionalWriter = (*condProvider)(nil)

// A conditional writer lets push skip the full remote listing entirely.
func TestConditionalPush_DoesNotListRemote(t *testing.T) {
	ctx := context.Background()
	engine, _ := newIncEngine(nil)

	local := newMemProvider("internal")
	remote := newCondProvider("carddav")
	local.items["u1"] = chqsync.SyncItem{RemoteID: "u1", ETag: "l1", VCardData: makeVCard("u1", "Alice")}

	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictManual, chqsync.SyncModeExport)
	require.NoError(t, err)
	assert.Equal(t, 0, remote.listCalls, "a conditional writer is never listed during push")
	assert.Contains(t, remote.items, "u1")
}

// A concurrent remote edit is caught by the failed precondition, not by comparing lists.
func TestConditionalPush_RemoteChangedIsAConflict(t *testing.T) {
	ctx := context.Background()
	syncRepo := newMockSyncStateRepo()
	conflictRepo := newMockConflictRepo()
	engine := chqsync.NewEngineWithAllRepos(syncRepo, nil, conflictRepo, zap.NewNop())

	local := newMemProvider("internal")
	remote := newCondProvider("carddav")
	local.items["u1"] = chqsync.SyncItem{RemoteID: "u1", ETag: "l1", VCardData: makeVCard("u1", "Alice")}

	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictManual, chqsync.SyncModeExport)
	require.NoError(t, err)

	// Both sides diverge: local edits, and the remote is changed out of band.
	local.items["u1"] = chqsync.SyncItem{RemoteID: "u1", ETag: "l2", VCardData: makeVCard("u1", "Alice Local")}
	remote.items["u1"] = chqsync.SyncItem{RemoteID: "u1", ETag: "r-moved", VCardData: makeVCard("u1", "Alice Remote")}

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictManual, chqsync.SyncModeExport)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Conflicts)
	assert.Equal(t, 1, res.Skipped)
	assert.Equal(t, 1, conflictRepo.pendingCount())
	assert.Contains(t, remote.items["u1"].VCardData, "Alice Remote", "the remote edit must survive")
}

// dest_wins overwrites the remote even when its precondition fails.
func TestConditionalPush_DestWinsOverwrites(t *testing.T) {
	ctx := context.Background()
	engine, _ := newIncEngine(nil)

	local := newMemProvider("internal")
	remote := newCondProvider("carddav")
	local.items["u1"] = chqsync.SyncItem{RemoteID: "u1", ETag: "l1", VCardData: makeVCard("u1", "Alice")}
	_, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictDestWins, chqsync.SyncModeExport)
	require.NoError(t, err)

	local.items["u1"] = chqsync.SyncItem{RemoteID: "u1", ETag: "l2", VCardData: makeVCard("u1", "Alice Local")}
	remote.items["u1"] = chqsync.SyncItem{RemoteID: "u1", ETag: "r-moved", VCardData: makeVCard("u1", "Alice Remote")}

	res, err := engine.Sync(ctx, "u1", "p1", remote, local, chqsync.ConflictDestWins, chqsync.SyncModeExport)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Updated)
	assert.Contains(t, remote.items["u1"].VCardData, "Alice Local")
}
