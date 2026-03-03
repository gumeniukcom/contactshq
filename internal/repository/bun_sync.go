package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunSyncStateRepository struct {
	db *bun.DB
}

func NewBunSyncStateRepository(db *bun.DB) *BunSyncStateRepository {
	return &BunSyncStateRepository{db: db}
}

func (r *BunSyncStateRepository) Create(ctx context.Context, state *domain.SyncState) error {
	_, err := r.db.NewInsert().Model(state).Exec(ctx)
	return err
}

func (r *BunSyncStateRepository) GetByRemoteID(ctx context.Context, userID, providerType, remoteID string) (*domain.SyncState, error) {
	state := new(domain.SyncState)
	err := r.db.NewSelect().Model(state).
		Where("user_id = ?", userID).
		Where("provider_type = ?", providerType).
		Where("remote_id = ?", remoteID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return state, err
}

func (r *BunSyncStateRepository) GetByLocalID(ctx context.Context, userID, providerType, localID string) (*domain.SyncState, error) {
	state := new(domain.SyncState)
	err := r.db.NewSelect().Model(state).
		Where("user_id = ?", userID).
		Where("provider_type = ?", providerType).
		Where("local_id = ?", localID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return state, err
}

func (r *BunSyncStateRepository) ListByUser(ctx context.Context, userID, providerType string) ([]*domain.SyncState, error) {
	var states []*domain.SyncState
	err := r.db.NewSelect().Model(&states).
		Where("user_id = ?", userID).
		Where("provider_type = ?", providerType).
		Scan(ctx)
	return states, err
}

func (r *BunSyncStateRepository) Update(ctx context.Context, state *domain.SyncState) error {
	_, err := r.db.NewUpdate().Model(state).WherePK().Exec(ctx)
	return err
}

func (r *BunSyncStateRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().Model((*domain.SyncState)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *BunSyncStateRepository) DeleteByUser(ctx context.Context, userID, providerType string) error {
	_, err := r.db.NewDelete().Model((*domain.SyncState)(nil)).
		Where("user_id = ?", userID).
		Where("provider_type = ?", providerType).
		Exec(ctx)
	return err
}
