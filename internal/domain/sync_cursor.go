package domain

import (
	"time"

	"github.com/uptrace/bun"
)

// SyncCursor is a provider's incremental-sync token for one pipeline.
//
// It marks how far a collection has been synchronised, so the next run asks only for what
// changed since. One row per (user, provider_type).
type SyncCursor struct {
	bun.BaseModel `bun:"table:sync_cursors,alias:scur"`

	ID           string    `bun:",pk,type:text" json:"id"`
	UserID       string    `bun:",notnull" json:"user_id"`
	ProviderType string    `bun:",notnull" json:"provider_type"`
	Cursor       string    `bun:",notnull,default:''" json:"cursor"`
	UpdatedAt    time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
