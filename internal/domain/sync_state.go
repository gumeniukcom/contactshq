package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type SyncState struct {
	bun.BaseModel `bun:"table:sync_states,alias:ss"`

	ID           string    `bun:",pk,type:text" json:"id"`
	UserID       string    `bun:",notnull" json:"user_id"`
	ProviderType string    `bun:",notnull" json:"provider_type"`
	ProviderURI  string    `bun:"provider_uri,default:''" json:"provider_uri"`
	RemoteID     string    `bun:"remote_id,default:''" json:"remote_id"`
	LocalID      string    `bun:"local_id,default:''" json:"local_id"`
	RemoteETag   string    `bun:"remote_etag,default:''" json:"remote_etag"`
	LocalETag    string    `bun:"local_etag,default:''" json:"local_etag"`
	ContentHash  string    `bun:",default:''" json:"content_hash"`
	BaseVCard    string    `bun:"base_vcard,default:''" json:"base_vcard"`
	LastSyncedAt time.Time `bun:",nullzero" json:"last_synced_at"`
	SyncToken    string    `bun:",default:''" json:"sync_token"`
}
