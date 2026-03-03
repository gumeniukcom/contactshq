package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunAddressBookRepository struct {
	db *bun.DB
}

func NewBunAddressBookRepository(db *bun.DB) *BunAddressBookRepository {
	return &BunAddressBookRepository{db: db}
}

func (r *BunAddressBookRepository) Create(ctx context.Context, ab *domain.AddressBook) error {
	_, err := r.db.NewInsert().Model(ab).Exec(ctx)
	return err
}

func (r *BunAddressBookRepository) GetByID(ctx context.Context, id string) (*domain.AddressBook, error) {
	ab := new(domain.AddressBook)
	err := r.db.NewSelect().Model(ab).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ab, err
}

func (r *BunAddressBookRepository) GetByUserID(ctx context.Context, userID string) (*domain.AddressBook, error) {
	ab := new(domain.AddressBook)
	err := r.db.NewSelect().Model(ab).Where("user_id = ?", userID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ab, err
}

func (r *BunAddressBookRepository) GetOrCreateByUserID(ctx context.Context, userID string) (*domain.AddressBook, error) {
	ab, err := r.GetByUserID(ctx, userID)
	if err != nil || ab != nil {
		return ab, err
	}
	// Address book doesn't exist — create one.
	now := time.Now()
	ab = &domain.AddressBook{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      "Contacts",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.Create(ctx, ab); err != nil {
		// Race condition: another request may have created it concurrently.
		if ab2, err2 := r.GetByUserID(ctx, userID); err2 == nil && ab2 != nil {
			return ab2, nil
		}
		return nil, err
	}
	return ab, nil
}

func (r *BunAddressBookRepository) Update(ctx context.Context, ab *domain.AddressBook) error {
	_, err := r.db.NewUpdate().Model(ab).WherePK().Exec(ctx)
	return err
}

func (r *BunAddressBookRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().Model((*domain.AddressBook)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}
