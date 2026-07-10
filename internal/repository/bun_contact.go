package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

func orderExpr(f ListFilters) string {
	dir := "ASC"
	if strings.EqualFold(f.SortDir, "desc") {
		dir = "DESC"
	}
	// Whitelisted sort columns to prevent SQL injection.
	switch f.SortBy {
	case "email":
		return "email " + dir
	case "org":
		return "org " + dir
	case "created_at":
		return "created_at " + dir
	case "updated_at":
		return "updated_at " + dir
	default: // "name" or unrecognized
		return fmt.Sprintf("last_name %s, first_name %s", dir, dir)
	}
}

func applyFilters(q *bun.SelectQuery, f ListFilters) *bun.SelectQuery {
	if len(f.Category) > 0 {
		q = q.Where("c.id IN (SELECT contact_id FROM contact_categories WHERE value IN (?))", bun.List(f.Category))
	}
	if f.Org != "" {
		q = q.Where("c.org = ?", f.Org)
	}
	if f.HasEmail != nil && *f.HasEmail {
		q = q.Where("(COALESCE(c.email, '') != '' OR c.id IN (SELECT contact_id FROM contact_emails WHERE COALESCE(value, '') != ''))")
	}
	if f.HasPhone != nil && *f.HasPhone {
		q = q.Where("(COALESCE(c.phone, '') != '' OR c.id IN (SELECT contact_id FROM contact_phones WHERE COALESCE(value, '') != ''))")
	}
	return q
}

type BunContactRepository struct {
	db *bun.DB
}

func NewBunContactRepository(db *bun.DB) *BunContactRepository {
	return &BunContactRepository{db: db}
}

func (r *BunContactRepository) Create(ctx context.Context, contact *domain.Contact) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		seq, err := nextChangeSeq(ctx, tx, contact.AddressBookID)
		if err != nil {
			return err
		}
		contact.ChangeSeq = seq

		if err := dropTombstones(ctx, tx, contact.AddressBookID, []string{contact.UID}); err != nil {
			return err
		}
		_, err = tx.NewInsert().Model(contact).Exec(ctx)
		return err
	})
}

func (r *BunContactRepository) GetByID(ctx context.Context, id string) (*domain.Contact, error) {
	contact := new(domain.Contact)
	err := r.db.NewSelect().Model(contact).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return contact, err
}

func (r *BunContactRepository) GetByUID(ctx context.Context, addressBookID, uid string) (*domain.Contact, error) {
	contact := new(domain.Contact)
	err := r.db.NewSelect().Model(contact).
		Where("address_book_id = ?", addressBookID).
		Where("uid = ?", uid).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return contact, err
}

func (r *BunContactRepository) Update(ctx context.Context, contact *domain.Contact) error {
	_, err := r.db.NewUpdate().Model(contact).WherePK().Exec(ctx)
	return err
}

func (r *BunContactRepository) Delete(ctx context.Context, id string) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		contact := new(domain.Contact)
		err := tx.NewSelect().Model(contact).Where("id = ?", id).Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*domain.Contact)(nil)).Where("id = ?", id).Exec(ctx); err != nil {
			return err
		}

		seq, err := nextChangeSeq(ctx, tx, contact.AddressBookID)
		if err != nil {
			return err
		}
		return recordDeletions(ctx, tx, contact.AddressBookID, []string{contact.UID}, seq)
	})
}

func (r *BunContactRepository) DeleteAll(ctx context.Context, addressBookID string) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		uids, err := uidsOf(ctx, tx, addressBookID, nil)
		if err != nil {
			return err
		}
		if _, err := tx.NewDelete().Model((*domain.Contact)(nil)).
			Where("address_book_id = ?", addressBookID).Exec(ctx); err != nil {
			return err
		}

		seq, err := nextChangeSeq(ctx, tx, addressBookID)
		if err != nil {
			return err
		}
		return recordDeletions(ctx, tx, addressBookID, uids, seq)
	})
}

// uidsOf lists the UIDs of an address book, optionally restricted to the given ids.
func uidsOf(ctx context.Context, tx bun.Tx, addressBookID string, ids []string) ([]string, error) {
	var uids []string
	q := tx.NewSelect().Model((*domain.Contact)(nil)).
		Column("uid").
		Where("address_book_id = ?", addressBookID)
	if ids != nil {
		q = q.Where("id IN (?)", bun.In(ids))
	}
	err := q.Scan(ctx, &uids)
	return uids, err
}

func (r *BunContactRepository) List(ctx context.Context, addressBookID string, limit, offset int, filters ListFilters) ([]*domain.Contact, int, error) {
	var contacts []*domain.Contact
	q := r.db.NewSelect().Model(&contacts).
		Where("c.address_book_id = ?", addressBookID)
	q = applyFilters(q, filters)
	count, err := q.OrderExpr(orderExpr(filters)).
		Limit(limit).Offset(offset).
		ScanAndCount(ctx)
	return contacts, count, err
}

func (r *BunContactRepository) Search(ctx context.Context, addressBookID, query string, limit, offset int, filters ListFilters) ([]*domain.Contact, int, error) {
	var contacts []*domain.Contact
	like := "%" + query + "%"
	q := r.db.NewSelect().Model(&contacts).
		Where("c.address_book_id = ?", addressBookID).
		Where(`(
			c.first_name LIKE ? OR c.last_name LIKE ? OR c.nickname LIKE ?
			OR c.email LIKE ? OR c.phone LIKE ?
			OR c.org LIKE ? OR c.department LIKE ? OR c.title LIKE ? OR c.note LIKE ?
			OR c.id IN (
				SELECT contact_id FROM contact_emails WHERE value LIKE ?
				UNION SELECT contact_id FROM contact_phones WHERE value LIKE ?
				UNION SELECT contact_id FROM contact_addresses
				      WHERE street LIKE ? OR city LIKE ? OR region LIKE ? OR country LIKE ?
				UNION SELECT contact_id FROM contact_urls WHERE value LIKE ?
				UNION SELECT contact_id FROM contact_ims WHERE value LIKE ?
				UNION SELECT contact_id FROM contact_categories WHERE value LIKE ?
			)
		)`,
			like, like, like,
			like, like,
			like, like, like, like,
			like, like,
			like, like, like, like,
			like,
			like,
			like,
		)
	q = applyFilters(q, filters)
	count, err := q.OrderExpr(orderExpr(filters)).
		Limit(limit).Offset(offset).
		ScanAndCount(ctx)
	return contacts, count, err
}

func (r *BunContactRepository) Facets(ctx context.Context, addressBookID string) (*ContactFacets, error) {
	facets := &ContactFacets{}

	// Total contacts
	count, err := r.db.NewSelect().Model((*domain.Contact)(nil)).
		Where("address_book_id = ?", addressBookID).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	facets.Total = count

	// Distinct categories
	err = r.db.NewSelect().
		TableExpr("contact_categories cc").
		Join("JOIN contacts c ON c.id = cc.contact_id").
		Where("c.address_book_id = ?", addressBookID).
		ColumnExpr("DISTINCT cc.value").
		OrderExpr("cc.value ASC").
		Scan(ctx, &facets.Categories)
	if err != nil {
		return nil, err
	}
	if facets.Categories == nil {
		facets.Categories = []string{}
	}

	// Distinct orgs
	err = r.db.NewSelect().Model((*domain.Contact)(nil)).
		Where("address_book_id = ?", addressBookID).
		Where("org != ''").
		ColumnExpr("DISTINCT org").
		OrderExpr("org ASC").
		Scan(ctx, &facets.Orgs)
	if err != nil {
		return nil, err
	}
	if facets.Orgs == nil {
		facets.Orgs = []string{}
	}

	return facets, nil
}

// DeleteMany removes the given contacts in one statement, scoped to the address book so
// an id from another user's book cannot be deleted by guessing it.
// It returns how many rows were actually removed.
func (r *BunContactRepository) DeleteMany(ctx context.Context, addressBookID string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var deleted int
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		uids, err := uidsOf(ctx, tx, addressBookID, ids)
		if err != nil {
			return err
		}

		res, err := tx.NewDelete().Model((*domain.Contact)(nil)).
			Where("address_book_id = ?", addressBookID).
			Where("id IN (?)", bun.In(ids)).
			Exec(ctx)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		deleted = int(affected)

		seq, err := nextChangeSeq(ctx, tx, addressBookID)
		if err != nil {
			return err
		}
		return recordDeletions(ctx, tx, addressBookID, uids, seq)
	})
	return deleted, err
}

// ListByIDs returns the named contacts of an address book, in the same order the list
// view shows them.
func (r *BunContactRepository) ListByIDs(ctx context.Context, addressBookID string, ids []string) ([]*domain.Contact, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var contacts []*domain.Contact
	err := r.db.NewSelect().Model(&contacts).
		Where("address_book_id = ?", addressBookID).
		Where("id IN (?)", bun.In(ids)).
		OrderExpr("last_name ASC, first_name ASC").
		Scan(ctx)
	return contacts, err
}

func (r *BunContactRepository) ListAll(ctx context.Context, addressBookID string) ([]*domain.Contact, error) {
	var contacts []*domain.Contact
	err := r.db.NewSelect().Model(&contacts).
		Where("address_book_id = ?", addressBookID).
		OrderExpr("last_name ASC, first_name ASC").
		Scan(ctx)
	return contacts, err
}
