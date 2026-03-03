package service

import (
	"context"
	"time"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

type UserService struct {
	userRepo repository.UserRepository
}

func NewUserService(userRepo repository.UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
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
	return s.userRepo.Update(ctx, user)
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
