package repository

import (
	"context"

	"github.com/uptrace/bun"
)

// BumpChangeSeq advances an address book's change counter and returns the new value.
//
// It exists for maintenance commands that rewrite contacts outside the repository's own write
// methods. The counter IS the collection's CTag, so a rewrite that skips it is invisible: a
// CTag-polling client never issues the PROPFIND that would reveal the new ETags, and a
// sync-collection client filtering on contacts.change_seq never sees the rows either.
//
// Call it inside the same transaction as the rows it accounts for, so a client can never read
// a CTag no contact carries yet.
func BumpChangeSeq(ctx context.Context, db bun.IDB, addressBookID string) (int64, error) {
	return nextChangeSeq(ctx, db, addressBookID)
}
