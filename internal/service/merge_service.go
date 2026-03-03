package service

import (
	"context"
	"errors"
	"time"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

var (
	ErrDuplicateNotFound = errors.New("duplicate record not found")
	ErrSameContact       = errors.New("winner and loser must be different contacts")
)

// MergeService merges two contacts into one, preserving field-level choices.
type MergeService struct {
	contactRepo repository.ContactRepository
	abRepo      repository.AddressBookRepository
	dupRepo     repository.PotentialDuplicateRepository
	syncRepo    repository.SyncStateRepository
}

func NewMergeService(
	contactRepo repository.ContactRepository,
	abRepo repository.AddressBookRepository,
	dupRepo repository.PotentialDuplicateRepository,
	syncRepo repository.SyncStateRepository,
) *MergeService {
	return &MergeService{
		contactRepo: contactRepo,
		abRepo:      abRepo,
		dupRepo:     dupRepo,
		syncRepo:    syncRepo,
	}
}

// MergeInput specifies which contact wins and the per-field resolution choices.
type MergeInput struct {
	WinnerID   string            `json:"winner_id"`
	LoserID    string            `json:"loser_id"`
	Resolution map[string]string `json:"resolution"` // vCard field type → "winner"|"loser"
}

// Merge combines loser into winner, deletes loser, and updates potential_duplicate status.
func (s *MergeService) Merge(ctx context.Context, userID string, input MergeInput) (*domain.Contact, error) {
	if input.WinnerID == input.LoserID {
		return nil, ErrSameContact
	}

	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	winner, err := s.contactRepo.GetByID(ctx, input.WinnerID)
	if err != nil || winner == nil || winner.AddressBookID != ab.ID {
		return nil, ErrContactNotFound
	}
	loser, err := s.contactRepo.GetByID(ctx, input.LoserID)
	if err != nil || loser == nil || loser.AddressBookID != ab.ID {
		return nil, ErrContactNotFound
	}

	// Map "winner"/"loser" resolution choices to "local"/"remote" for ApplyResolution.
	// Winner's vCard plays "local", loser's vCard plays "remote".
	vcardRes := make(map[string]string, len(input.Resolution))
	for field, choice := range input.Resolution {
		if choice == "loser" {
			vcardRes[field] = "remote"
		} else {
			vcardRes[field] = "local"
		}
	}

	mergedVCard, err := chqsync.ApplyResolution("", winner.VCardData, loser.VCardData, vcardRes)
	if err != nil {
		return nil, err
	}

	// Re-extract structured fields from the merged vCard.
	mergedParsed, _ := vcardpkg.ParseVCard(mergedVCard)
	if mergedParsed == nil {
		mergedParsed = &vcardpkg.ParsedContact{}
	}
	winner.VCardData = mergedVCard
	winner.ETag = generateETag(mergedVCard)
	winner.FirstName = mergedParsed.FirstName
	winner.LastName = mergedParsed.LastName
	winner.Email = mergedParsed.PrimaryEmail
	winner.Phone = mergedParsed.PrimaryPhone
	winner.Org = mergedParsed.Org
	winner.Title = mergedParsed.Title
	winner.Note = mergedParsed.Note
	winner.UpdatedAt = time.Now()

	if err := s.contactRepo.Update(ctx, winner); err != nil {
		return nil, err
	}

	// Delete loser and its duplicate records.
	if err := s.dupRepo.DeleteByContact(ctx, loser.ID); err != nil {
		// non-fatal: log but continue
		_ = err
	}
	if err := s.contactRepo.Delete(ctx, loser.ID); err != nil {
		return nil, err
	}

	// Also clean up duplicate records involving winner (both sides may have dupes).
	_ = s.dupRepo.DeleteByContact(ctx, winner.ID)

	return winner, nil
}
