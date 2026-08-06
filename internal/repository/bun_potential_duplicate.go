package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunPotentialDuplicateRepository struct {
	db *bun.DB
}

func NewBunPotentialDuplicateRepository(db *bun.DB) *BunPotentialDuplicateRepository {
	return &BunPotentialDuplicateRepository{db: db}
}

func (r *BunPotentialDuplicateRepository) Create(ctx context.Context, d *domain.PotentialDuplicate) error {
	_, err := r.db.NewInsert().Model(d).Exec(ctx)
	return err
}

// CreateIfAbsent inserts a pair unless it is already recorded, reporting whether it was new.
//
// The conflict target is named explicitly. Without it PostgreSQL picks whichever unique
// constraint it likes, which on a table that also has a primary key means the clause can
// swallow a genuine id collision instead of the pair collision it was written for.
func (r *BunPotentialDuplicateRepository) CreateIfAbsent(ctx context.Context, d *domain.PotentialDuplicate) (bool, error) {
	res, err := r.db.NewInsert().
		Model(d).
		On("CONFLICT (user_id, contact_a_id, contact_b_id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *BunPotentialDuplicateRepository) GetByID(ctx context.Context, id string) (*domain.PotentialDuplicate, error) {
	d := new(domain.PotentialDuplicate)
	err := r.db.NewSelect().Model(d).
		Relation("ContactA").
		Relation("ContactB").
		Where("pd.id = ?", id).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		// Returning a half-populated model alongside an error invites callers to use it.
		return nil, err
	}
	return d, nil
}

// GetByIDWithContacts loads one pair with both contacts and all of their child collections.
//
// Ownership is part of the query rather than a check the caller is trusted to remember: a
// pair belonging to someone else must not be readable, and "the handler compares user ids
// afterwards" is one forgotten line away from an authorisation hole.
func (r *BunPotentialDuplicateRepository) GetByIDWithContacts(ctx context.Context, userID, id string) (*domain.PotentialDuplicate, error) {
	d := new(domain.PotentialDuplicate)
	err := r.db.NewSelect().Model(d).
		Relation("ContactA").
		Relation("ContactB").
		Where("pd.id = ?", id).
		Where("pd.user_id = ?", userID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Bun will not assemble a two-level belongs-to → has-many in one statement here, so the
	// collections are loaded per contact through the path that already knows how.
	for _, c := range []**domain.Contact{&d.ContactA, &d.ContactB} {
		if *c == nil {
			continue
		}
		full, err := r.loadRelationsFor(ctx, *c)
		if err != nil {
			return nil, err
		}
		*c = full
	}

	return d, nil
}

// loadRelationsFor fills a contact's child collections in place.
func (r *BunPotentialDuplicateRepository) loadRelationsFor(ctx context.Context, c *domain.Contact) (*domain.Contact, error) {
	full := new(domain.Contact)
	err := r.db.NewSelect().Model(full).
		Relation("Emails").
		Relation("Phones").
		Relation("Addresses").
		Relation("URLs").
		Relation("IMs").
		Relation("Categories").
		Relation("Dates").
		// The model's bun alias is "c" (domain.Contact), not the table name.
		Where("c.id = ?", c.ID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	return full, nil
}

// StatusAll asks ListByUser for every status rather than one.
const StatusAll = "all"

// ListByUser returns a page of pairs with both contacts, but WITHOUT their child collections:
// the list renders four fields per pair, and joining seven collections for each of two sides
// would add fourteen joins per page for data nothing displays.
//
// What it does carry is the cheap question the list has to answer: is one side's set of
// values contained in the other's? That decides whether a one-click "keep this one" is
// lossless, and it is computed in SQL rather than by loading the values.
func (r *BunPotentialDuplicateRepository) ListByUser(ctx context.Context, userID, status string, limit, offset int) ([]*domain.PotentialDuplicate, int, error) {
	var dups []*domain.PotentialDuplicate
	q := r.db.NewSelect().Model(&dups).
		Relation("ContactA").
		Relation("ContactB").
		ColumnExpr("pd.*").
		ColumnExpr(subsetExpr("contact_b_id", "contact_a_id")+" AS b_subset_of_a").
		ColumnExpr(subsetExpr("contact_a_id", "contact_b_id")+" AS a_subset_of_b").
		Where("pd.user_id = ?", userID)

	// An explicit "all" is how a caller asks for every status; an empty string used to mean
	// the same thing by accident, which made the filter impossible to clear from the UI.
	if status != "" && status != StatusAll {
		q = q.Where("pd.status = ?", status)
	}

	total, err := q.OrderExpr("pd.score DESC, pd.created_at DESC").
		Limit(limit).Offset(offset).
		ScanAndCount(ctx)
	return dups, total, err
}

// subsetExpr builds the SQL asking "is every email and phone of `from` also present on
// `to`?". Comparison is case-insensitive for emails and digits-only for phones, matching how
// the detector decides two contacts are the same person.
func subsetExpr(from, to string) string {
	return `(NOT EXISTS (
		SELECT 1 FROM contact_emails fe
		WHERE fe.contact_id = pd.` + from + `
		  AND NOT EXISTS (
			SELECT 1 FROM contact_emails te
			WHERE te.contact_id = pd.` + to + ` AND lower(te.value) = lower(fe.value))
	) AND NOT EXISTS (
		SELECT 1 FROM contact_phones fp
		WHERE fp.contact_id = pd.` + from + `
		  AND NOT EXISTS (
			SELECT 1 FROM contact_phones tp
			WHERE tp.contact_id = pd.` + to + ` AND ` + digitsOnly("tp.value") + ` = ` + digitsOnly("fp.value") + `)
	))`
}

// digitsOnly strips the characters people put in phone numbers. Written with nested REPLACE
// because SQLite has no translate() and no regexp by default, and this has to run unchanged
// on both supported databases.
func digitsOnly(column string) string {
	expr := column
	for _, ch := range []string{" ", "-", "(", ")", "+", ".", " "} {
		expr = "REPLACE(" + expr + ", '" + ch + "', '')"
	}
	return expr
}

// GetByContacts finds the row for an unordered pair.
//
// Pairs are stored with the smaller contact id first, so one comparison suffices. The
// previous OR over both orderings would have hidden a regression in that normalisation — the
// very thing the unique index in migration 024 depends on.
func (r *BunPotentialDuplicateRepository) GetByContacts(ctx context.Context, userID, aID, bID string) (*domain.PotentialDuplicate, error) {
	if aID > bID {
		aID, bID = bID, aID
	}

	d := new(domain.PotentialDuplicate)
	err := r.db.NewSelect().Model(d).
		Where("pd.user_id = ?", userID).
		Where("pd.contact_a_id = ?", aID).
		Where("pd.contact_b_id = ?", bID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (r *BunPotentialDuplicateRepository) Update(ctx context.Context, d *domain.PotentialDuplicate) error {
	_, err := r.db.NewUpdate().Model(d).WherePK().Exec(ctx)
	return err
}

func (r *BunPotentialDuplicateRepository) DeleteByContact(ctx context.Context, contactID string) error {
	_, err := r.db.NewDelete().Model((*domain.PotentialDuplicate)(nil)).
		Where("contact_a_id = ? OR contact_b_id = ?", contactID, contactID).
		Exec(ctx)
	return err
}

func (r *BunPotentialDuplicateRepository) CountPending(ctx context.Context, userID string) (int, error) {
	return r.db.NewSelect().Model((*domain.PotentialDuplicate)(nil)).
		Where("user_id = ?", userID).
		Where("status = ?", "pending").
		Count(ctx)
}
