package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

// nextChangeSeq bumps an address book's change counter and returns the new value.
//
// It runs inside the caller's transaction, so the sequence a write claims and the write
// itself land together: a client that reads a CTag can never observe a value that no
// contact carries yet.
func nextChangeSeq(ctx context.Context, db bun.IDB, addressBookID string) (int64, error) {
	var seq int64
	err := db.NewUpdate().
		Model((*domain.AddressBook)(nil)).
		Set("change_seq = change_seq + 1").
		Where("id = ?", addressBookID).
		Returning("change_seq").
		Scan(ctx, &seq)
	return seq, err
}

// recordDeletions writes a tombstone per deleted contact at the given sequence.
func recordDeletions(ctx context.Context, db bun.IDB, addressBookID string, uids []string, seq int64) error {
	if len(uids) == 0 {
		return nil
	}

	now := time.Now()
	rows := make([]*domain.ContactTombstone, 0, len(uids))
	for _, uid := range uids {
		rows = append(rows, &domain.ContactTombstone{
			ID:            uuid.New().String(),
			AddressBookID: addressBookID,
			UID:           uid,
			ChangeSeq:     seq,
			DeletedAt:     now,
		})
	}

	_, err := db.NewInsert().Model(&rows).Exec(ctx)
	return err
}

// dropTombstones removes tombstones for the given UIDs. A contact recreated under a UID
// it once had is present again; leaving the tombstone would tell a synchronising client
// to delete it.
func dropTombstones(ctx context.Context, db bun.IDB, addressBookID string, uids []string) error {
	if len(uids) == 0 {
		return nil
	}
	_, err := db.NewDelete().
		Model((*domain.ContactTombstone)(nil)).
		Where("address_book_id = ?", addressBookID).
		Where("uid IN (?)", bun.In(uids)).
		Exec(ctx)
	return err
}

// CollectionChanges is what a client needs to catch up with a collection.
type CollectionChanges struct {
	// Updated holds contacts created or modified after the requested sequence.
	Updated []*domain.Contact
	// DeletedUIDs holds contacts removed after it.
	DeletedUIDs []string
	// Seq is the collection's current change sequence: the token to come back with.
	Seq int64
}

// ChangeSeq returns an address book's current change sequence, which is its CTag.
func (r *BunAddressBookRepository) ChangeSeq(ctx context.Context, addressBookID string) (int64, error) {
	var seq int64
	err := r.db.NewSelect().
		Model((*domain.AddressBook)(nil)).
		Column("change_seq").
		Where("id = ?", addressBookID).
		Scan(ctx, &seq)
	return seq, err
}

// ChangesSince reports everything that happened to a collection after sinceSeq.
//
// A zero sinceSeq means "I have nothing": every contact comes back and no tombstone does,
// because a client that has never seen the collection has nothing to delete.
func (r *BunContactRepository) ChangesSince(ctx context.Context, addressBookID string, sinceSeq int64) (*CollectionChanges, error) {
	changes := &CollectionChanges{}

	var current int64
	if err := r.db.NewSelect().
		Model((*domain.AddressBook)(nil)).
		Column("change_seq").
		Where("id = ?", addressBookID).
		Scan(ctx, &current); err != nil {
		return nil, err
	}
	changes.Seq = current

	q := r.db.NewSelect().Model(&changes.Updated).
		Where("address_book_id = ?", addressBookID)
	if sinceSeq > 0 {
		q = q.Where("change_seq > ?", sinceSeq)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}

	if sinceSeq == 0 {
		return changes, nil
	}

	var tombstones []*domain.ContactTombstone
	if err := r.db.NewSelect().Model(&tombstones).
		Where("address_book_id = ?", addressBookID).
		Where("change_seq > ?", sinceSeq).
		Scan(ctx); err != nil {
		return nil, err
	}
	for _, t := range tombstones {
		changes.DeletedUIDs = append(changes.DeletedUIDs, t.UID)
	}

	return changes, nil
}
