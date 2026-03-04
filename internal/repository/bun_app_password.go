package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunAppPasswordRepository struct {
	db *bun.DB
}

func NewBunAppPasswordRepository(db *bun.DB) *BunAppPasswordRepository {
	return &BunAppPasswordRepository{db: db}
}

func (r *BunAppPasswordRepository) Create(ctx context.Context, ap *domain.AppPassword) error {
	_, err := r.db.NewInsert().Model(ap).Exec(ctx)
	return err
}

func (r *BunAppPasswordRepository) ListByUser(ctx context.Context, userID string) ([]domain.AppPassword, error) {
	var passwords []domain.AppPassword
	err := r.db.NewSelect().Model(&passwords).
		Where("ap.user_id = ?", userID).
		OrderExpr("ap.created_at DESC").
		Scan(ctx)
	return passwords, err
}

func (r *BunAppPasswordRepository) GetByID(ctx context.Context, id string) (*domain.AppPassword, error) {
	ap := new(domain.AppPassword)
	err := r.db.NewSelect().Model(ap).Where("ap.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return ap, nil
}

func (r *BunAppPasswordRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().Model((*domain.AppPassword)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *BunAppPasswordRepository) ListAllByUser(ctx context.Context, userID string) ([]domain.AppPassword, error) {
	return r.ListByUser(ctx, userID)
}

func (r *BunAppPasswordRepository) UpdateLastUsed(ctx context.Context, id string) error {
	now := time.Now()
	_, err := r.db.NewUpdate().Model((*domain.AppPassword)(nil)).
		Set("last_used_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	return err
}
