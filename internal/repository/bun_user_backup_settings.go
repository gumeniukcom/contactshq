package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunUserBackupSettingsRepository struct {
	db *bun.DB
}

func NewBunUserBackupSettingsRepository(db *bun.DB) *BunUserBackupSettingsRepository {
	return &BunUserBackupSettingsRepository{db: db}
}

func (r *BunUserBackupSettingsRepository) Get(ctx context.Context, userID string) (*domain.UserBackupSettings, error) {
	s := new(domain.UserBackupSettings)
	err := r.db.NewSelect().Model(s).Where("ubs.user_id = ?", userID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *BunUserBackupSettingsRepository) Upsert(ctx context.Context, s *domain.UserBackupSettings) error {
	_, err := r.db.NewInsert().Model(s).
		On("CONFLICT (user_id) DO UPDATE").
		Set("schedule = EXCLUDED.schedule").
		Set("retention = EXCLUDED.retention").
		Set("enabled = EXCLUDED.enabled").
		Set("compress = EXCLUDED.compress").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (r *BunUserBackupSettingsRepository) ListAll(ctx context.Context) ([]*domain.UserBackupSettings, error) {
	var settings []*domain.UserBackupSettings
	err := r.db.NewSelect().Model(&settings).Scan(ctx)
	return settings, err
}
