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
	return d, err
}

func (r *BunPotentialDuplicateRepository) ListByUser(ctx context.Context, userID, status string, limit, offset int) ([]*domain.PotentialDuplicate, int, error) {
	var dups []*domain.PotentialDuplicate
	q := r.db.NewSelect().Model(&dups).
		Relation("ContactA").
		Relation("ContactB").
		Where("pd.user_id = ?", userID)
	if status != "" {
		q = q.Where("pd.status = ?", status)
	}
	total, err := q.OrderExpr("pd.score DESC, pd.created_at DESC").
		Limit(limit).Offset(offset).
		ScanAndCount(ctx)
	return dups, total, err
}

func (r *BunPotentialDuplicateRepository) GetByContacts(ctx context.Context, userID, aID, bID string) (*domain.PotentialDuplicate, error) {
	d := new(domain.PotentialDuplicate)
	err := r.db.NewSelect().Model(d).
		Where("pd.user_id = ?", userID).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				Where("(pd.contact_a_id = ? AND pd.contact_b_id = ?)", aID, bID).
				WhereOr("(pd.contact_a_id = ? AND pd.contact_b_id = ?)", bID, aID)
		}).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return d, err
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
