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

// ErrTooManyAppPasswords reports that an account has reached MaxAppPasswordsPerUser.
var ErrTooManyAppPasswords = errors.New("too many app passwords")

// MaxAppPasswordsPerUser caps how many app passwords one account may hold.
//
// Every failed CardDAV Basic-auth attempt hashes once per stored password, so an unbounded
// list turns a single request into arbitrarily much argon2id work. Twenty is far above any
// plausible number of devices.
const MaxAppPasswordsPerUser = 20

type AppPasswordService struct {
	repo repository.AppPasswordRepository

	// invalidate drops cached CardDAV verdicts so a deleted app password stops working now
	// rather than in five minutes.
	invalidate CredentialInvalidator
	// userRepo resolves the account's email, which is what the verdict cache is keyed on.
	userRepo repository.UserRepository
}

func NewAppPasswordService(repo repository.AppPasswordRepository) *AppPasswordService {
	return &AppPasswordService{repo: repo}
}

// WithCredentialInvalidator wires the CardDAV verdict cache into deletion. Both arguments may
// be nil; the service then simply skips invalidation.
func (s *AppPasswordService) WithCredentialInvalidator(userRepo repository.UserRepository, fn CredentialInvalidator) *AppPasswordService {
	s.userRepo = userRepo
	s.invalidate = fn
	return s
}

// Create generates a new app-specific password. Returns the plaintext token (shown once) and the stored record.
func (s *AppPasswordService) Create(ctx context.Context, userID, label string) (string, *domain.AppPassword, error) {
	existing, err := s.repo.ListAllByUser(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	if len(existing) >= MaxAppPasswordsPerUser {
		return "", nil, ErrTooManyAppPasswords
	}

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
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// The verdict cache holds a positive for (email, sha256(app password)), so without this
	// the revoked password keeps working for up to five minutes.
	if s.invalidate != nil && s.userRepo != nil {
		if user, uerr := s.userRepo.GetByID(ctx, userID); uerr == nil && user != nil {
			s.invalidate(user.Email)
		}
	}
	return nil
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
