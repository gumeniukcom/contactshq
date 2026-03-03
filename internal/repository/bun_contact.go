package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunContactRepository struct {
	db *bun.DB
}

func NewBunContactRepository(db *bun.DB) *BunContactRepository {
	return &BunContactRepository{db: db}
}

func (r *BunContactRepository) Create(ctx context.Context, contact *domain.Contact) error {
	_, err := r.db.NewInsert().Model(contact).Exec(ctx)
	return err
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
	_, err := r.db.NewDelete().Model((*domain.Contact)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *BunContactRepository) DeleteAll(ctx context.Context, addressBookID string) error {
	_, err := r.db.NewDelete().Model((*domain.Contact)(nil)).Where("address_book_id = ?", addressBookID).Exec(ctx)
	return err
}

func (r *BunContactRepository) List(ctx context.Context, addressBookID string, limit, offset int) ([]*domain.Contact, int, error) {
	var contacts []*domain.Contact
	count, err := r.db.NewSelect().Model(&contacts).
		Where("address_book_id = ?", addressBookID).
		OrderExpr("last_name ASC, first_name ASC").
		Limit(limit).Offset(offset).
		ScanAndCount(ctx)
	return contacts, count, err
}

func (r *BunContactRepository) Search(ctx context.Context, addressBookID, query string, limit, offset int) ([]*domain.Contact, int, error) {
	var contacts []*domain.Contact
	like := "%" + query + "%"
	count, err := r.db.NewSelect().Model(&contacts).
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
		).
		OrderExpr("c.last_name ASC, c.first_name ASC").
		Limit(limit).Offset(offset).
		ScanAndCount(ctx)
	return contacts, count, err
}

func (r *BunContactRepository) ListAll(ctx context.Context, addressBookID string) ([]*domain.Contact, error) {
	var contacts []*domain.Contact
	err := r.db.NewSelect().Model(&contacts).
		Where("address_book_id = ?", addressBookID).
		OrderExpr("last_name ASC, first_name ASC").
		Scan(ctx)
	return contacts, err
}
