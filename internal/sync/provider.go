package sync

import (
	"context"
	"errors"
)

type SyncProvider interface {
	Name() string
	List(ctx context.Context) ([]SyncItem, error)
	Get(ctx context.Context, remoteID string) (*SyncItem, error)
	Put(ctx context.Context, item SyncItem) (PutResult, error)
	Delete(ctx context.Context, remoteID string) error
}

type SyncItem struct {
	RemoteID    string
	ETag        string
	ContentHash string
	VCardData   string
}

// Delta is what a provider changed since a cursor.
//
// A pull that only knows a full listing has to infer deletions from absence, which is
// indistinguishable from a truncated or expired response — the reason the mass-deletion
// guard exists. A provider that reports deletions explicitly removes that guesswork.
type Delta struct {
	// Updated holds contacts created or modified since the cursor.
	Updated []SyncItem
	// Deleted holds the remote ids removed since the cursor. It is only consulted when
	// Full is false; a full listing names no deletions.
	Deleted []string
	// Cursor is the opaque token to pass next time. Empty means the provider does not do
	// incremental sync, so nothing is stored.
	Cursor string
	// Full is true when Updated is the entire collection rather than a delta. The engine
	// then reconciles deletions against it, exactly as a plain List would.
	Full bool
}

// IncrementalProvider fetches only what changed since a cursor.
//
// Optional: a provider that does not implement it is driven by List, and every sync is a
// full listing. The engine checks for this interface and falls back automatically.
type IncrementalProvider interface {
	SyncProvider

	// ListChanges returns the collection's changes since cursor. An empty cursor — a
	// first sync, or one after the cursor was invalidated — must return the whole
	// collection with Full set, and a fresh cursor to continue from.
	//
	// A provider whose cursor the server rejected as too old returns ErrCursorExpired, so
	// the engine discards it and asks again from empty.
	ListChanges(ctx context.Context, cursor string) (Delta, error)
}

// ErrCursorExpired signals that a stored cursor is no longer honoured and the caller must
// resynchronise from an empty cursor. Google answers 410 EXPIRED_SYNC_TOKEN this way, and
// CardDAV a 403 valid-sync-token precondition.
var ErrCursorExpired = errors.New("sync cursor expired")

// ConditionalWriter writes a contact only if the remote still holds the version the caller
// last saw. It lets push detect a concurrent edit without downloading the whole remote
// collection to compare ETags first.
type ConditionalWriter interface {
	// PutIfMatch writes item only if the remote's current ETag equals ifMatch. An empty
	// ifMatch means "create only, must not already exist". A mismatch returns
	// ErrPreconditionFailed and writes nothing.
	PutIfMatch(ctx context.Context, item SyncItem, ifMatch string) (PutResult, error)
}

// ErrPreconditionFailed means the remote moved on since the ETag the caller passed: some
// other client edited the contact. Push turns this into a conflict instead of clobbering
// their change.
var ErrPreconditionFailed = errors.New("remote precondition failed")

// PutResult reports where the provider actually stored the item.
//
// RemoteID matters because a provider may assign its own identifier: Google People
// returns a fresh resourceName when a contact is created, and it bears no relation to
// the id we sent. Recording the id we sent instead of the one we got back makes the
// next sync see the remote contact as brand new and the tracked one as deleted — it
// then deletes the original and re-imports a copy.
type PutResult struct {
	RemoteID string
	ETag     string
}
