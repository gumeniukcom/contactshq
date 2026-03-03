package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunProviderConnectionRepository struct {
	db *bun.DB
}

func NewBunProviderConnectionRepository(db *bun.DB) *BunProviderConnectionRepository {
	return &BunProviderConnectionRepository{db: db}
}

func (r *BunProviderConnectionRepository) Create(ctx context.Context, c *domain.ProviderConnection) error {
	_, err := r.db.NewInsert().Model(c).Exec(ctx)
	return err
}

func (r *BunProviderConnectionRepository) GetByID(ctx context.Context, id string) (*domain.ProviderConnection, error) {
	c := new(domain.ProviderConnection)
	err := r.db.NewSelect().Model(c).Where("pc.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *BunProviderConnectionRepository) ListByUser(ctx context.Context, userID string) ([]*domain.ProviderConnection, error) {
	var conns []*domain.ProviderConnection
	err := r.db.NewSelect().Model(&conns).
		Where("pc.user_id = ?", userID).
		OrderExpr("pc.created_at ASC").
		Scan(ctx)
	return conns, err
}

func (r *BunProviderConnectionRepository) GetByUserAndType(ctx context.Context, userID, providerType string) (*domain.ProviderConnection, error) {
	c := new(domain.ProviderConnection)
	err := r.db.NewSelect().Model(c).
		Where("pc.user_id = ?", userID).
		Where("pc.provider_type = ?", providerType).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *BunProviderConnectionRepository) Update(ctx context.Context, c *domain.ProviderConnection) error {
	_, err := r.db.NewUpdate().Model(c).WherePK().Exec(ctx)
	return err
}

func (r *BunProviderConnectionRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().Model((*domain.ProviderConnection)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}
