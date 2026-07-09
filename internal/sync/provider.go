package sync

import "context"

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
