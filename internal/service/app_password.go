package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

var ErrAppPasswordNotFound = errors.New("app password not found")

type AppPasswordService struct {
	repo repository.AppPasswordRepository
}

func NewAppPasswordService(repo repository.AppPasswordRepository) *AppPasswordService {
	return &AppPasswordService{repo: repo}
}

// Create generates a new app-specific password. Returns the plaintext token (shown once) and the stored record.
func (s *AppPasswordService) Create(ctx context.Context, userID, label string) (string, *domain.AppPassword, error) {
	token, err := generateToken(32)
	if err != nil {
		return "", nil, err
	}

	hash, err := hashPassword(token)
	if err != nil {
		return "", nil, err
	}

	ap := &domain.AppPassword{
		ID:           uuid.New().String(),
		UserID:       userID,
		Label:        label,
		PasswordHash: hash,
	}

	if err := s.repo.Create(ctx, ap); err != nil {
		return "", nil, err
	}

	return token, ap, nil
}

func (s *AppPasswordService) List(ctx context.Context, userID string) ([]domain.AppPassword, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *AppPasswordService) Delete(ctx context.Context, userID, id string) error {
	ap, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if ap == nil || ap.UserID != userID {
		return ErrAppPasswordNotFound
	}
	return s.repo.Delete(ctx, id)
}

// Verify checks the plaintext password against all app passwords for a user.
// Returns the matching AppPassword and true if found, or nil and false.
func (s *AppPasswordService) Verify(ctx context.Context, userID, plaintext string) (*domain.AppPassword, bool) {
	passwords, err := s.repo.ListAllByUser(ctx, userID)
	if err != nil || len(passwords) == 0 {
		return nil, false
	}

	for i := range passwords {
		if verifyPassword(plaintext, passwords[i].PasswordHash) {
			// Update last used (fire and forget)
			_ = s.repo.UpdateLastUsed(ctx, passwords[i].ID)
			return &passwords[i], true
		}
	}

	return nil, false
}

func generateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
