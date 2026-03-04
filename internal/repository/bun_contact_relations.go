package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

// replaceRows is a generic helper: delete all child rows for contactID then bulk-insert new ones.
// T must be a pointer to a Bun model struct.
func replaceRows[T any](ctx context.Context, db *bun.DB, contactID string, table string, rows []T) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().TableExpr(table).Where("contact_id = ?", contactID).Exec(ctx); err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		_, err := tx.NewInsert().Model(&rows).Exec(ctx)
		return err
	})
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
	if err != nil {
		return nil, err
	}
	return contact, nil
}

func loadRelations(ctx context.Context, db *bun.DB, contacts []*domain.Contact) error {
	ids := make([]string, len(contacts))
	for i, c := range contacts {
		ids[i] = c.ID
	}
	return db.NewSelect().Model(&contacts).
		Where("c.id IN (?)", bun.In(ids)). //nolint:staticcheck
		Relation("Emails").Relation("Phones").Relation("Addresses").
		Relation("URLs").Relation("IMs").Relation("Categories").Relation("Dates").
		Scan(ctx)
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
