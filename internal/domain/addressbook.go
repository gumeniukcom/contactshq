package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type AddressBook struct {
	bun.BaseModel `bun:"table:address_books,alias:ab"`

	ID          string `bun:",pk,type:text" json:"id"`
	UserID      string `bun:",notnull" json:"user_id"`
	Name        string `bun:",notnull,default:'Contacts'" json:"name"`
	Description string `bun:",default:''" json:"description"`
	SyncToken   string `bun:",default:''" json:"sync_token,omitempty"`
	// ChangeSeq is bumped on every write to the book's contacts. It is the collection's
	// CTag and the basis of RFC 6578 sync tokens.
	ChangeSeq int64     `bun:"change_seq,notnull,default:0" json:"change_seq"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	User     *User      `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty"`
	Contacts []*Contact `bun:"rel:has-many,join:id=address_book_id" json:"contacts,omitempty"`
}
