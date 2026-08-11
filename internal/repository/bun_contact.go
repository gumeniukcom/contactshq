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
	// Empty, not nil: a nil slice marshals to `null`, and the list view reads .length off the
	// response. Any filter combination that matches nothing produced it, not just a first run.
	contacts := []*domain.Contact{}
	q := r.db.NewSelect().Model(&contacts).
		Where("c.address_book_id = ?", addressBookID)
	q = applyFilters(q, filters)
	count, err := q.OrderExpr(orderExpr(filters)).
		Limit(limit).Offset(offset).
		ScanAndCount(ctx)
	return contacts, count, err
}

func (r *BunContactRepository) Search(ctx context.Context, addressBookID, query string, limit, offset int, filters ListFilters) ([]*domain.Contact, int, error) {
	contacts := []*domain.Contact{}
	// Folded, because LIKE is case-sensitive on PostgreSQL — the engine docker-compose ships —
	// while SQLite folds ASCII for it. Without this, searching "john" does not find "John Smith"
	// on the deployment this project provisions for itself.
	like := "%" + strings.ToLower(query) + "%"
	q := r.db.NewSelect().Model(&contacts).
		Where("c.address_book_id = ?", addressBookID).
		Where(`(
			LOWER(c.first_name) LIKE ? OR LOWER(c.last_name) LIKE ? OR LOWER(c.nickname) LIKE ?
			OR LOWER(c.email) LIKE ? OR LOWER(c.phone) LIKE ?
			OR LOWER(c.org) LIKE ? OR LOWER(c.department) LIKE ? OR LOWER(c.title) LIKE ? OR LOWER(c.note) LIKE ?
			OR c.id IN (
				SELECT contact_id FROM contact_emails WHERE LOWER(value) LIKE ?
				UNION SELECT contact_id FROM contact_phones WHERE LOWER(value) LIKE ?
				UNION SELECT contact_id FROM contact_addresses
				      WHERE LOWER(street) LIKE ? OR LOWER(city) LIKE ? OR LOWER(region) LIKE ? OR LOWER(country) LIKE ?
				UNION SELECT contact_id FROM contact_urls WHERE LOWER(value) LIKE ?
				UNION SELECT contact_id FROM contact_ims WHERE LOWER(value) LIKE ?
				UNION SELECT contact_id FROM contact_categories WHERE LOWER(value) LIKE ?
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
	contacts := []*domain.Contact{}
	err := r.db.NewSelect().Model(&contacts).
		Where("address_book_id = ?", addressBookID).
		Where("id IN (?)", bun.In(ids)).
		OrderExpr("last_name ASC, first_name ASC").
		Scan(ctx)
	return contacts, err
}

func (r *BunContactRepository) ListAll(ctx context.Context, addressBookID string) ([]*domain.Contact, error) {
	contacts := []*domain.Contact{}
	err := r.db.NewSelect().Model(&contacts).
		Where("address_book_id = ?", addressBookID).
		OrderExpr("last_name ASC, first_name ASC").
		Scan(ctx)
	return contacts, err
}

// ListForDedup returns only the columns duplicate detection compares.
//
// ListAll pulls every column, including vcard_data and photo_uri: on ten thousand contacts
// that is tens of megabytes read and discarded, for a scan that looks at four fields.
func (r *BunContactRepository) ListForDedup(ctx context.Context, addressBookID string) ([]*domain.Contact, error) {
	contacts := []*domain.Contact{}
	err := r.db.NewSelect().
		Model(&contacts).
		Column("id", "first_name", "last_name", "email", "phone").
		Where("address_book_id = ?", addressBookID).
		Order("id ASC").
		Scan(ctx)
	return contacts, err
}

// ListDedupValues returns every contact_emails and contact_phones value in the address book,
// each as a bare (contact_id, value) pair.
//
// Two queries rather than one join onto ListForDedup, and rather than Relation("Emails"):
//
//   - Joining contacts to contact_emails repeats every selected contact column once per email
//     and leaves the de-duplication to Go.
//   - Relation("Emails") / WithRelations loads whole contact rows — vcard_data and photo_uri
//     included — which is precisely the read cost the comment on ListForDedup exists to avoid.
//
// The plan each query wants is: drive from contacts filtered by address_book_id, which
// idx_contacts_book_seq(address_book_id, change_seq) supports (migrations/021), then look the
// child rows up by contact_id, which is idx_contact_emails_contact and
// idx_contact_phones_contact (migrations/014). The child-table index alone would not make this
// cheap — it is the pairing of the two that keeps it proportional to the address book.
//
// Only the two columns are selected. Nothing here reads a card.
func (r *BunContactRepository) ListDedupValues(ctx context.Context, addressBookID string) ([]domain.ContactValueRef, []domain.ContactValueRef, error) {
	read := func(table string) ([]domain.ContactValueRef, error) {
		var refs []domain.ContactValueRef
		err := r.db.NewSelect().
			ColumnExpr("v.contact_id AS contact_id").
			ColumnExpr("v.value AS value").
			TableExpr(table+" AS v").
			Join("JOIN contacts AS c ON c.id = v.contact_id").
			Where("c.address_book_id = ?", addressBookID).
			Scan(ctx, &refs)
		return refs, err
	}

	emails, err := read("contact_emails")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list dedup emails: %w", err)
	}
	phones, err := read("contact_phones")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list dedup phones: %w", err)
	}
	return emails, phones, nil
}
