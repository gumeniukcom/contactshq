package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

// replaceRowsIn deletes all child rows for contactID then bulk-inserts new ones, using
// whatever database handle it is given so it can join a caller's transaction.
// T must be a pointer to a Bun model struct.
func replaceRowsIn[T any](ctx context.Context, db bun.IDB, contactID string, table string, rows []T) error {
	if _, err := db.NewDelete().TableExpr(table).Where("contact_id = ?", contactID).Exec(ctx); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	_, err := db.NewInsert().Model(&rows).Exec(ctx)
	return err
}

// replaceRows runs a single child-table replacement in its own transaction.
func replaceRows[T any](ctx context.Context, db *bun.DB, contactID string, table string, rows []T) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return replaceRowsIn(ctx, tx, contactID, table, rows)
	})
}

// Save writes a contact and all of its child rows in one transaction.
//
// The contact row and its seven child tables used to be written by eight independent
// statements. A failure part-way through left a contact whose emails, phones and
// categories belonged to its previous version — or to nothing at all — which search,
// filters and duplicate detection all read from.
func (r *BunContactRepository) Save(ctx context.Context, contact *domain.Contact, children domain.ChildRecords) error {
	assignChildIDs(contact.ID, &children)

	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewUpdate().Model(contact).WherePK().Exec(ctx)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			if _, err := tx.NewInsert().Model(contact).Exec(ctx); err != nil {
				return err
			}
		}

		if err := replaceRowsIn(ctx, tx, contact.ID, "contact_emails", children.Emails); err != nil {
			return err
		}
		if err := replaceRowsIn(ctx, tx, contact.ID, "contact_phones", children.Phones); err != nil {
			return err
		}
		if err := replaceRowsIn(ctx, tx, contact.ID, "contact_addresses", children.Addresses); err != nil {
			return err
		}
		if err := replaceRowsIn(ctx, tx, contact.ID, "contact_urls", children.URLs); err != nil {
			return err
		}
		if err := replaceRowsIn(ctx, tx, contact.ID, "contact_ims", children.IMs); err != nil {
			return err
		}
		if err := replaceRowsIn(ctx, tx, contact.ID, "contact_categories", children.Categories); err != nil {
			return err
		}
		return replaceRowsIn(ctx, tx, contact.ID, "contact_dates", children.Dates)
	})
}

// assignChildIDs fills in the primary keys and owner of every child row. SQLite has no
// UUID generator, so the ids are minted here rather than by the database.
func assignChildIDs(contactID string, c *domain.ChildRecords) {
	for _, r := range c.Emails {
		r.ID, r.ContactID = childID(r.ID), contactID
	}
	for _, r := range c.Phones {
		r.ID, r.ContactID = childID(r.ID), contactID
	}
	for _, r := range c.Addresses {
		r.ID, r.ContactID = childID(r.ID), contactID
	}
	for _, r := range c.URLs {
		r.ID, r.ContactID = childID(r.ID), contactID
	}
	for _, r := range c.IMs {
		r.ID, r.ContactID = childID(r.ID), contactID
	}
	for _, r := range c.Categories {
		r.ID, r.ContactID = childID(r.ID), contactID
	}
	for _, r := range c.Dates {
		r.ID, r.ContactID = childID(r.ID), contactID
	}
}

func childID(existing string) string {
	if existing != "" {
		return existing
	}
	return uuid.New().String()
}

func (r *BunContactRepository) ReplaceEmails(ctx context.Context, contactID string, rows []*domain.ContactEmail) error {
	for _, e := range rows {
		if e.ID == "" {
			e.ID = uuid.New().String()
		}
		e.ContactID = contactID
	}
	return replaceRows(ctx, r.db, contactID, "contact_emails", rows)
}

func (r *BunContactRepository) ReplacePhones(ctx context.Context, contactID string, rows []*domain.ContactPhone) error {
	for _, p := range rows {
		if p.ID == "" {
			p.ID = uuid.New().String()
		}
		p.ContactID = contactID
	}
	return replaceRows(ctx, r.db, contactID, "contact_phones", rows)
}

func (r *BunContactRepository) ReplaceAddresses(ctx context.Context, contactID string, rows []*domain.ContactAddress) error {
	for _, a := range rows {
		if a.ID == "" {
			a.ID = uuid.New().String()
		}
		a.ContactID = contactID
	}
	return replaceRows(ctx, r.db, contactID, "contact_addresses", rows)
}

func (r *BunContactRepository) ReplaceURLs(ctx context.Context, contactID string, rows []*domain.ContactURL) error {
	for _, u := range rows {
		if u.ID == "" {
			u.ID = uuid.New().String()
		}
		u.ContactID = contactID
	}
	return replaceRows(ctx, r.db, contactID, "contact_urls", rows)
}

func (r *BunContactRepository) ReplaceIMs(ctx context.Context, contactID string, rows []*domain.ContactIM) error {
	for _, im := range rows {
		if im.ID == "" {
			im.ID = uuid.New().String()
		}
		im.ContactID = contactID
	}
	return replaceRows(ctx, r.db, contactID, "contact_ims", rows)
}

func (r *BunContactRepository) ReplaceCategories(ctx context.Context, contactID string, rows []*domain.ContactCategory) error {
	for _, c := range rows {
		if c.ID == "" {
			c.ID = uuid.New().String()
		}
		c.ContactID = contactID
	}
	return replaceRows(ctx, r.db, contactID, "contact_categories", rows)
}

func (r *BunContactRepository) ReplaceDates(ctx context.Context, contactID string, rows []*domain.ContactDate) error {
	for _, d := range rows {
		if d.ID == "" {
			d.ID = uuid.New().String()
		}
		d.ContactID = contactID
	}
	return replaceRows(ctx, r.db, contactID, "contact_dates", rows)
}

func (r *BunContactRepository) GetByIDWithRelations(ctx context.Context, id string) (*domain.Contact, error) {
	contact := &domain.Contact{}
	err := r.db.NewSelect().Model(contact).
		Relation("Emails").Relation("Phones").Relation("Addresses").
		Relation("URLs").Relation("IMs").Relation("Categories").Relation("Dates").
		Where("c.id = ?", id).
		Scan(ctx)
	// A missing contact is not a failure. Returning sql.ErrNoRows made the API answer
	// 500 to a request for an id that simply does not exist.
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return contact, nil
}

func (r *BunContactRepository) GetByUIDWithRelations(ctx context.Context, addressBookID, uid string) (*domain.Contact, error) {
	contact := &domain.Contact{}
	err := r.db.NewSelect().Model(contact).
		Relation("Emails").Relation("Phones").Relation("Addresses").
		Relation("URLs").Relation("IMs").Relation("Categories").Relation("Dates").
		Where("c.address_book_id = ? AND c.uid = ?", addressBookID, uid).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return contact, nil
}

// loadChildren fetches every child row belonging to the given contacts. Bun derives the
// table from T.
func loadChildren[T any](ctx context.Context, db *bun.DB, ids []string) ([]T, error) {
	var rows []T
	err := db.NewSelect().Model(&rows).
		Where("contact_id IN (?)", bun.In(ids)).
		Scan(ctx)
	return rows, err
}

func groupByContactID[T any](rows []T, contactID func(T) string) map[string][]T {
	byID := make(map[string][]T, len(rows))
	for _, row := range rows {
		id := contactID(row)
		byID[id] = append(byID[id], row)
	}
	return byID
}

// loadRelations attaches child rows to contacts that have already been selected, ordered
// and paginated.
//
// It must not re-select the parents. Bun's slice scan resets the destination and refills
// it in whatever order the rows arrive, so a second query without an ORDER BY silently
// discarded the caller's sort — the contacts list ignored its sort controls and paged
// through an unstable sequence.
func loadRelations(ctx context.Context, db *bun.DB, contacts []*domain.Contact) error {
	if len(contacts) == 0 {
		return nil
	}

	ids := make([]string, len(contacts))
	for i, c := range contacts {
		ids[i] = c.ID
	}

	emails, err := loadChildren[*domain.ContactEmail](ctx, db, ids)
	if err != nil {
		return err
	}
	phones, err := loadChildren[*domain.ContactPhone](ctx, db, ids)
	if err != nil {
		return err
	}
	addresses, err := loadChildren[*domain.ContactAddress](ctx, db, ids)
	if err != nil {
		return err
	}
	urls, err := loadChildren[*domain.ContactURL](ctx, db, ids)
	if err != nil {
		return err
	}
	ims, err := loadChildren[*domain.ContactIM](ctx, db, ids)
	if err != nil {
		return err
	}
	categories, err := loadChildren[*domain.ContactCategory](ctx, db, ids)
	if err != nil {
		return err
	}
	dates, err := loadChildren[*domain.ContactDate](ctx, db, ids)
	if err != nil {
		return err
	}

	emailsByID := groupByContactID(emails, func(r *domain.ContactEmail) string { return r.ContactID })
	phonesByID := groupByContactID(phones, func(r *domain.ContactPhone) string { return r.ContactID })
	addressesByID := groupByContactID(addresses, func(r *domain.ContactAddress) string { return r.ContactID })
	urlsByID := groupByContactID(urls, func(r *domain.ContactURL) string { return r.ContactID })
	imsByID := groupByContactID(ims, func(r *domain.ContactIM) string { return r.ContactID })
	categoriesByID := groupByContactID(categories, func(r *domain.ContactCategory) string { return r.ContactID })
	datesByID := groupByContactID(dates, func(r *domain.ContactDate) string { return r.ContactID })

	for _, c := range contacts {
		c.Emails = emailsByID[c.ID]
		c.Phones = phonesByID[c.ID]
		c.Addresses = addressesByID[c.ID]
		c.URLs = urlsByID[c.ID]
		c.IMs = imsByID[c.ID]
		c.Categories = categoriesByID[c.ID]
		c.Dates = datesByID[c.ID]
	}

	return nil
}

func (r *BunContactRepository) ListWithRelations(ctx context.Context, addressBookID string, limit, offset int, filters ListFilters) ([]*domain.Contact, int, error) {
	contacts, total, err := r.List(ctx, addressBookID, limit, offset, filters)
	if err != nil || len(contacts) == 0 {
		return contacts, total, err
	}
	if err := loadRelations(ctx, r.db, contacts); err != nil {
		return nil, 0, err
	}
	return contacts, total, nil
}

func (r *BunContactRepository) SearchWithRelations(ctx context.Context, addressBookID, query string, limit, offset int, filters ListFilters) ([]*domain.Contact, int, error) {
	contacts, total, err := r.Search(ctx, addressBookID, query, limit, offset, filters)
	if err != nil || len(contacts) == 0 {
		return contacts, total, err
	}
	if err := loadRelations(ctx, r.db, contacts); err != nil {
		return nil, 0, err
	}
	return contacts, total, nil
}
