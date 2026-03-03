package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunUserRepository struct {
	db *bun.DB
}

func NewBunUserRepository(db *bun.DB) *BunUserRepository {
	return &BunUserRepository{db: db}
}

func (r *BunUserRepository) Create(ctx context.Context, user *domain.User) error {
	_, err := r.db.NewInsert().Model(user).Exec(ctx)
	return err
}

func (r *BunUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	user := new(domain.User)
	err := r.db.NewSelect().Model(user).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return user, err
}

func (r *BunUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	user := new(domain.User)
	err := r.db.NewSelect().Model(user).Where("email = ?", email).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return user, err
}

func (r *BunUserRepository) Update(ctx context.Context, user *domain.User) error {
	_, err := r.db.NewUpdate().Model(user).WherePK().Exec(ctx)
	return err
}

func (r *BunUserRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().Model((*domain.User)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *BunUserRepository) List(ctx context.Context, limit, offset int) ([]*domain.User, int, error) {
	var users []*domain.User
	count, err := r.db.NewSelect().Model(&users).
		OrderExpr("created_at DESC").
		Limit(limit).Offset(offset).
		ScanAndCount(ctx)
	return users, count, err
}

func (r *BunUserRepository) ListAllIDs(ctx context.Context) ([]string, error) {
	var ids []string
	rows, err := r.db.QueryContext(ctx, "SELECT id FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
