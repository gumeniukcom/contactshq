package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunUserDedupSettingsRepository struct {
	db *bun.DB
}

func NewBunUserDedupSettingsRepository(db *bun.DB) *BunUserDedupSettingsRepository {
	return &BunUserDedupSettingsRepository{db: db}
}

func (r *BunUserDedupSettingsRepository) Get(ctx context.Context, userID string) (*domain.UserDedupSettings, error) {
	s := new(domain.UserDedupSettings)
	err := r.db.NewSelect().Model(s).Where("uds.user_id = ?", userID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *BunUserDedupSettingsRepository) Upsert(ctx context.Context, s *domain.UserDedupSettings) error {
	_, err := r.db.NewInsert().Model(s).
		On("CONFLICT (user_id) DO UPDATE").
		Set("schedule = EXCLUDED.schedule").
		Set("enabled = EXCLUDED.enabled").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (r *BunUserDedupSettingsRepository) ListAll(ctx context.Context) ([]*domain.UserDedupSettings, error) {
	var settings []*domain.UserDedupSettings
	err := r.db.NewSelect().Model(&settings).Scan(ctx)
	return settings, err
}
