package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

// MinPasswordLen matches what the HTTP handler enforces, so a password set through the CLI
// cannot be weaker than one set through the API.
const MinPasswordLen = 8

// ErrPasswordTooShort reports a password below MinPasswordLen.
var ErrPasswordTooShort = errors.New("password is too short")

// CredentialInvalidator is called after a credential stops being valid.
//
// It exists as a function type rather than an interface on the carddav package because
// service must not import carddav: that import cycle is precisely why there are two argon2id
// verifiers in this codebase already.
type CredentialInvalidator func(email string)

type UserService struct {
	userRepo repository.UserRepository

	// invalidate drops any cached authentication verdict for a user. Without it a password
	// change takes up to authCachePositiveTTL (5 minutes) to reach CardDAV, so the old
	// password keeps working after the user was told it no longer does.
	invalidate CredentialInvalidator
}

func NewUserService(userRepo repository.UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
}

// WithCredentialInvalidator wires the CardDAV verdict cache into credential changes. Nil is
// accepted: tests and the CLI run without a CardDAV server.
func (s *UserService) WithCredentialInvalidator(fn CredentialInvalidator) *UserService {
	s.invalidate = fn
	return s
}

func (s *UserService) invalidateCredentials(email string) {
	if s.invalidate != nil && email != "" {
		s.invalidate(email)
	}
}

func (s *UserService) GetByID(ctx context.Context, id string) (*domain.User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, id, displayName, email string) (*domain.User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	if email != "" && email != user.Email {
		existing, err := s.userRepo.GetByEmail(ctx, email)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, ErrEmailTaken
		}
		user.Email = email
	}

	if displayName != "" {
		user.DisplayName = displayName
	}

	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) ChangePassword(ctx context.Context, id, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	if !verifyPassword(oldPassword, user.PasswordHash) {
		return ErrInvalidCredentials
	}

	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		return err
	}

	s.invalidateCredentials(user.Email)
	return nil
}

// SetPassword replaces a user's password without knowing the old one. It backs the
// `set-password` subcommand, which is the supported way out of a forgotten password.
//
// It deliberately does not create the account and does not touch the role: recovering access
// must not be a way to grant yourself one.
func (s *UserService) SetPassword(ctx context.Context, email, newPassword string) error {
	if len(newPassword) < MinPasswordLen {
		return fmt.Errorf("%w: at least %d characters required", ErrPasswordTooShort, MinPasswordLen)
	}

	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		return err
	}

	s.invalidateCredentials(user.Email)
	return nil
}

func (s *UserService) Delete(ctx context.Context, id string) error {
	return s.userRepo.Delete(ctx, id)
}

func (s *UserService) List(ctx context.Context, limit, offset int) ([]*domain.User, int, error) {
	return s.userRepo.List(ctx, limit, offset)
}

func (s *UserService) UpdateRole(ctx context.Context, id, role string) error {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}
	user.Role = role
	user.UpdatedAt = time.Now()
	return s.userRepo.Update(ctx, user)
}
