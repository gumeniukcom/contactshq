package service

import (
	"context"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

type AddressBookService struct {
	abRepo repository.AddressBookRepository
}

func NewAddressBookService(abRepo repository.AddressBookRepository) *AddressBookService {
	return &AddressBookService{abRepo: abRepo}
}

func (s *AddressBookService) GetByUserID(ctx context.Context, userID string) (*domain.AddressBook, error) {
	ab, err := s.abRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ab == nil {
		return nil, ErrAddressBookNotFound
	}
	return ab, nil
}
