package sync_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
	"go.uber.org/zap"
)

// --- mock SyncProvider ---

type memProvider struct {
	name  string
	items map[string]chqsync.SyncItem
}

func newMemProvider(name string) *memProvider {
	return &memProvider{name: name, items: make(map[string]chqsync.SyncItem)}
}

func (p *memProvider) Name() string { return p.name }

func (p *memProvider) List(_ context.Context) ([]chqsync.SyncItem, error) {
	out := make([]chqsync.SyncItem, 0, len(p.items))
	for _, v := range p.items {
		out = append(out, v)
	}
	return out, nil
}

func (p *memProvider) Get(_ context.Context, id string) (*chqsync.SyncItem, error) {
	if item, ok := p.items[id]; ok {
		return &item, nil
	}
	return nil, nil
}

func (p *memProvider) Put(_ context.Context, item chqsync.SyncItem) (string, error) {
	h := sha256.Sum256([]byte(item.VCardData))
	item.ETag = hex.EncodeToString(h[:8])
	item.ContentHash = hex.EncodeToString(h[:])
	p.items[item.RemoteID] = item
	return item.ETag, nil
}

func (p *memProvider) Delete(_ context.Context, id string) error {
	delete(p.items, id)
	return nil
}

// --- mock SyncStateRepository ---

type mockSyncStateRepo struct {
	states map[string]*domain.SyncState
}

func newMockSyncStateRepo() *mockSyncStateRepo {
	return &mockSyncStateRepo{states: make(map[string]*domain.SyncState)}
}

func (m *mockSyncStateRepo) Create(_ context.Context, s *domain.SyncState) error {
	m.states[s.ID] = s
	return nil
}

func (m *mockSyncStateRepo) GetByRemoteID(_ context.Context, userID, pt, remoteID string) (*domain.SyncState, error) {
	return nil, nil
}

func (m *mockSyncStateRepo) GetByLocalID(_ context.Context, userID, pt, localID string) (*domain.SyncState, error) {
	return nil, nil
}

func (m *mockSyncStateRepo) ListByUser(_ context.Context, userID, pt string) ([]*domain.SyncState, error) {
	var out []*domain.SyncState
	for _, s := range m.states {
		if s.UserID == userID && s.ProviderType == pt {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *mockSyncStateRepo) Update(_ context.Context, s *domain.SyncState) error {
	m.states[s.ID] = s
	return nil
}

func (m *mockSyncStateRepo) Delete(_ context.Context, id string) error {
	delete(m.states, id)
	return nil
}

func (m *mockSyncStateRepo) DeleteByUser(_ context.Context, userID, pt string) error {
	for id, s := range m.states {
		if s.UserID == userID && s.ProviderType == pt {
			delete(m.states, id)
		}
	}
	return nil
}

// --- mock SyncRunRepository ---

type mockSyncRunRepo struct {
	runs []*domain.SyncRun
}

func (m *mockSyncRunRepo) Create(_ context.Context, run *domain.SyncRun) error {
	m.runs = append(m.runs, run)
	return nil
}

func (m *mockSyncRunRepo) Update(_ context.Context, _ *domain.SyncRun) error { return nil }

func (m *mockSyncRunRepo) ListByUser(_ context.Context, _ string, _ int) ([]*domain.SyncRun, error) {
	return m.runs, nil
}

func (m *mockSyncRunRepo) ListActiveByUser(_ context.Context, _ string) ([]*domain.SyncRun, error) {
	var active []*domain.SyncRun
	for _, r := range m.runs {
		if r.Status == "running" {
			active = append(active, r)
		}
	}
	return active, nil
}

func (m *mockSyncRunRepo) ListByPipeline(_ context.Context, _, pipelineID string, _ int) ([]*domain.SyncRun, error) {
	var result []*domain.SyncRun
	for _, r := range m.runs {
		if r.PipelineID == pipelineID {
			result = append(result, r)
		}
	}
	return result, nil
}

// verify mockSyncRunRepo implements repository.SyncRunRepository
var _ repository.SyncRunRepository = (*mockSyncRunRepo)(nil)

// --- helpers ---

func makeVCard(uid, name string) string {
	return "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + uid + "\r\nFN:" + name + "\r\nEND:VCARD\r\n"
}

// --- tests ---

func TestSync_NewItems_CreatedInDest(t *testing.T) {
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")

	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e1", VCardData: makeVCard("uid1", "Alice")}

	result, err := engine.Sync(context.Background(), "u1", "", src, dst, chqsync.ConflictSourceWins)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 0, result.Updated)
	assert.Contains(t, dst.items, "uid1")
}

func TestSync_DeletedFromSource_DeletedFromDest(t *testing.T) {
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")

	// Initial sync: uid1 in source
	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e1", VCardData: makeVCard("uid1", "Alice")}
	_, err := engine.Sync(context.Background(), "u1", "", src, dst, chqsync.ConflictSourceWins)
	require.NoError(t, err)
	require.Contains(t, dst.items, "uid1")

	// Remove from source
	delete(src.items, "uid1")
	result, err := engine.Sync(context.Background(), "u1", "", src, dst, chqsync.ConflictSourceWins)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Deleted)
	assert.NotContains(t, dst.items, "uid1")
}

func TestSync_ConflictSourceWins(t *testing.T) {
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")

	vcard1 := makeVCard("uid1", "Alice")
	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e1", VCardData: vcard1}
	dst.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e1", VCardData: vcard1}

	// Initial sync to record state
	_, err := engine.Sync(context.Background(), "u1", "", src, dst, chqsync.ConflictSourceWins)
	require.NoError(t, err)

	// Both sides change
	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e2", VCardData: makeVCard("uid1", "Alice Updated")}
	dst.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e3", VCardData: makeVCard("uid1", "Alice Conflicting")}

	result, err := engine.Sync(context.Background(), "u1", "", src, dst, chqsync.ConflictSourceWins)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Conflicts)
	// Source wins: dest should have source's data
	assert.Contains(t, dst.items["uid1"].VCardData, "Alice Updated")
}

func TestSync_ConflictDestWins_Skips(t *testing.T) {
	syncRepo := newMockSyncStateRepo()
	engine := chqsync.NewEngine(syncRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")

	vcard1 := makeVCard("uid1", "Alice")
	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e1", VCardData: vcard1}
	dst.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e1", VCardData: vcard1}

	// Initial sync
	_, err := engine.Sync(context.Background(), "u1", "", src, dst, chqsync.ConflictDestWins)
	require.NoError(t, err)

	// Both sides change
	srcVCard := makeVCard("uid1", "Alice Source")
	dstVCard := makeVCard("uid1", "Alice Dest")
	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e2", VCardData: srcVCard}
	dst.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e3", VCardData: dstVCard}

	result, err := engine.Sync(context.Background(), "u1", "", src, dst, chqsync.ConflictDestWins)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Conflicts)
	assert.Equal(t, 1, result.Skipped)
	// Dest wins: dest still has its own data
	assert.Contains(t, dst.items["uid1"].VCardData, "Alice Dest")
}

func TestSync_RecordsSyncRun(t *testing.T) {
	syncStateRepo := newMockSyncStateRepo()
	runRepo := &mockSyncRunRepo{}
	engine := chqsync.NewEngineWithRunRepo(syncStateRepo, runRepo, zap.NewNop())

	src := newMemProvider("source")
	dst := newMemProvider("dest")
	src.items["uid1"] = chqsync.SyncItem{RemoteID: "uid1", ETag: "e1", VCardData: makeVCard("uid1", "Bob")}

	result, err := engine.Sync(context.Background(), "u1", "", src, dst, chqsync.ConflictSourceWins)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)

	require.Len(t, runRepo.runs, 1)
	run := runRepo.runs[0]
	assert.Equal(t, "u1", run.UserID)
	assert.NotEmpty(t, run.ProviderType)
	// Status should be "completed" after successful sync
	assert.Equal(t, "completed", run.Status)
	assert.NotNil(t, run.FinishedAt)
	assert.True(t, run.FinishedAt.After(time.Time{}))
}
