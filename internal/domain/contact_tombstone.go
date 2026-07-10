package domain

import (
	"time"

	"github.com/uptrace/bun"
)

// ContactTombstone remembers that a contact was deleted.
//
// RFC 6578 sync-collection must name deleted resources explicitly, and nothing about the
// contacts table can reconstruct what is no longer there.
type ContactTombstone struct {
	bun.BaseModel `bun:"table:contact_tombstones,alias:ct"`

	ID            string    `bun:",pk,type:text" json:"id"`
	AddressBookID string    `bun:",notnull" json:"address_book_id"`
	UID           string    `bun:"uid,notnull" json:"uid"`
	ChangeSeq     int64     `bun:"change_seq,notnull" json:"change_seq"`
	DeletedAt     time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"deleted_at"`
}
