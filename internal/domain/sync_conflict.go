package domain

import (
	"time"

	"github.com/uptrace/bun"
)

// SyncConflict represents a sync conflict that requires user resolution.
// Created when both local and remote versions of a contact changed since
// the last sync and auto-merge was not possible.
type SyncConflict struct {
	bun.BaseModel `bun:"table:sync_conflicts,alias:sc"`

	ID             string     `bun:",pk,type:text"                          json:"id"`
	UserID         string     `bun:",notnull"                               json:"user_id"`
	ProviderType   string     `bun:",notnull"                               json:"provider_type"`
	RemoteID       string     `bun:",notnull"                               json:"remote_id"`
	LocalContactID string     `bun:",notnull,default:''"                    json:"local_contact_id"`
	BaseVCard      string     `bun:"base_vcard,notnull,default:''"          json:"base_vcard"`
	LocalVCard     string     `bun:"local_vcard,notnull,default:''"         json:"local_vcard"`
	RemoteVCard    string     `bun:"remote_vcard,notnull,default:''"        json:"remote_vcard"`
	FieldDiffs     string     `bun:"field_diffs,notnull,default:'[]'"       json:"field_diffs"`
	Status         string     `bun:",notnull,default:'pending'"             json:"status"`
	Resolution     string     `bun:",notnull,default:''"                    json:"resolution"`
	ResolvedVCard  string     `bun:"resolved_vcard,notnull,default:''"      json:"resolved_vcard"`
	CreatedAt      time.Time  `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	ResolvedAt     *time.Time `bun:",nullzero"                              json:"resolved_at,omitempty"`
}

// FieldDiff describes a per-field conflict between local and remote vCard versions.
type FieldDiff struct {
	Field  string `json:"field"`
	Base   string `json:"base"`
	Local  string `json:"local"`
	Remote string `json:"remote"`
}
