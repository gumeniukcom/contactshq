package sync

import "context"

type SyncProvider interface {
	Name() string
	List(ctx context.Context) ([]SyncItem, error)
	Get(ctx context.Context, remoteID string) (*SyncItem, error)
	Put(ctx context.Context, item SyncItem) (newETag string, err error)
	Delete(ctx context.Context, remoteID string) error
}

type SyncItem struct {
	RemoteID    string
	ETag        string
	ContentHash string
	VCardData   string
}
