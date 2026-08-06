package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

var (
	// ErrDuplicateNotFound reports a duplicate pair that does not exist or is not the
	// caller's.
	ErrDuplicateNotFound = errors.New("duplicate record not found")
	ErrSameContact       = errors.New("winner and loser must be different contacts")
)

// MergeService merges two contacts into one, preserving field-level choices.
type MergeService struct {
	contactRepo repository.ContactRepository
	abRepo      repository.AddressBookRepository
	dupRepo     repository.PotentialDuplicateRepository
	syncRepo    repository.SyncStateRepository

	// mergeLogRepo is optional so existing callers and tests keep working; when set, every
	// merge leaves a record that survives the deletion of both contacts.
	mergeLogRepo repository.MergeLogRepository
	logger       *zap.Logger
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
		logger:      zap.NewNop(),
	}
}

// WithMergeLog records merges to the given repository.
func (s *MergeService) WithMergeLog(repo repository.MergeLogRepository) *MergeService {
	s.mergeLogRepo = repo
	return s
}

// WithSyncStateRepo overrides the sync state repository. Present so a test can observe what a
// merge cleans up.
func (s *MergeService) WithSyncStateRepo(repo repository.SyncStateRepository) *MergeService {
	s.syncRepo = repo
	return s
}

// WithLogger supplies a logger for the failures a merge survives but should not hide.
func (s *MergeService) WithLogger(logger *zap.Logger) *MergeService {
	if logger != nil {
		s.logger = logger
	}
	return s
}

// MergeInput specifies which contact wins and what survives of the other.
type MergeInput struct {
	WinnerID string `json:"winner_id"`
	LoserID  string `json:"loser_id"`

	// Resolution is the older whole-property form: vCard property name → "winner"|"loser".
	// Kept because the quick "keep this one" buttons still send it and it needs no per-value
	// knowledge.
	Resolution map[string]string `json:"resolution,omitempty"`

	// Selection is the per-value form: vCard property name → the value ids to keep. When
	// present it takes precedence, and it is the only way to express "the work address from
	// one card and the home address from the other".
	Selection vcardpkg.Selection `json:"selection,omitempty"`

	// DupID optionally names the duplicate pair this merge resolves. Ownership is verified
	// and the id is recorded with the merge.
	DupID string `json:"dup_id,omitempty"`
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

	// A pair id from someone else's account must not be accepted, even though the contacts
	// themselves are checked below: it would end up recorded against this merge.
	if err := s.verifyDuplicateOwnership(ctx, userID, input.DupID); err != nil {
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

	mergedVCard, err := s.mergedCard(winner, loser, input)
	if err != nil {
		return nil, err
	}

	// Re-extract structured fields from the merged vCard.
	//
	// The error is returned rather than discarded: substituting an empty ParsedContact — as
	// this used to — blanked the winner's name, email, phone, org, title and note, turning an
	// unreadable merge into silent data loss.
	mergedParsed, err := vcardpkg.ParseVCard(mergedVCard)
	if err != nil {
		return nil, fmt.Errorf("parse merged vcard: %w", err)
	}
	if mergedParsed == nil {
		return nil, fmt.Errorf("parse merged vcard: no contact data")
	}

	// ApplyToContact sets all 18 modelled fields. The seven that used to be copied by hand
	// here left the rest — nickname, department, birthday, the address components — holding
	// the pre-merge values.
	vcardpkg.ApplyToContact(winner, mergedParsed)
	winner.VCardData = mergedVCard
	winner.ETag = generateETag(mergedVCard)
	winner.UpdatedAt = time.Now()

	// Record the merge before it happens: afterwards the loser is gone and the pair row has
	// been cascaded away by the database, leaving nothing to describe.
	s.recordMerge(ctx, userID, winner, loser, input)

	// Duplicate rows referencing the loser would be cascaded by the delete anyway; removing
	// them first keeps the intent visible and the failure reportable.
	if err := s.dupRepo.DeleteByContact(ctx, loser.ID); err != nil {
		s.logger.Warn("failed to clear duplicate records for the merged contact",
			zap.String("contact_id", loser.ID), zap.Error(err))
	}

	// One transaction: writing the winner and deleting the loser separately left, on a
	// failure between them, an updated winner beside a loser that should have been gone —
	// which the next sync reads as two contacts again.
	if err := s.contactRepo.MergeInto(ctx, winner, vcardpkg.ChildRecordsFor(winner.ID, mergedParsed), loser.ID); err != nil {
		return nil, fmt.Errorf("merge contacts: %w", err)
	}

	// The loser's sync state must go with it. Left behind, the next export or two-way run
	// reads it as a contact that vanished locally and either recreates it from the remote or
	// raises a conflict for a contact that no longer exists.
	s.clearSyncState(ctx, userID, loser.ID)

	// Also clean up duplicate records involving winner (both sides may have dupes).
	if err := s.dupRepo.DeleteByContact(ctx, winner.ID); err != nil {
		s.logger.Warn("failed to clear duplicate records for the surviving contact",
			zap.String("contact_id", winner.ID), zap.Error(err))
	}

	return winner, nil
}

// mergedCard produces the surviving card from whichever form of choice the caller sent.
//
// Selection is per-value and is what the merge screen sends; Resolution is the older
// whole-property form the quick "keep this one" buttons still use. Neither is translated into
// the other: the per-value merge cannot be expressed as property swaps, and the whole-property
// path needs no value identifiers.
func (s *MergeService) mergedCard(winner, loser *domain.Contact, input MergeInput) (string, error) {
	if len(input.Selection) > 0 {
		return vcardpkg.MergeCards(winner.VCardData, loser.VCardData, input.Selection)
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
	return chqsync.ApplyResolution("", winner.VCardData, loser.VCardData, vcardRes)
}

// verifyDuplicateOwnership checks that a referenced pair exists and belongs to the caller.
func (s *MergeService) verifyDuplicateOwnership(ctx context.Context, userID, dupID string) error {
	if dupID == "" {
		return nil
	}
	dup, err := s.dupRepo.GetByIDWithContacts(ctx, userID, dupID)
	if err != nil {
		return err
	}
	if dup == nil {
		return ErrDuplicateNotFound
	}
	return nil
}

// recordMerge writes the audit entry. A failure here does not undo the merge — the merge is
// the operation the user asked for — but it must not pass unnoticed either.
func (s *MergeService) recordMerge(ctx context.Context, userID string, winner, loser *domain.Contact, input MergeInput) {
	if s.mergeLogRepo == nil {
		return
	}

	choices := map[string]any{"resolution": input.Resolution, "selection": input.Selection, "dup_id": input.DupID}
	encoded, err := json.Marshal(choices)
	if err != nil {
		encoded = []byte("{}")
	}

	entry := &domain.MergeLogEntry{
		ID:                uuid.New().String(),
		UserID:            userID,
		WinnerID:          winner.ID,
		WinnerDisplayName: displayName(winner),
		LoserUID:          loser.UID,
		LoserDisplayName:  displayName(loser),
		LoserVCard:        vcardpkg.StripPhoto(loser.VCardData),
		Resolution:        string(encoded),
		MergedAt:          time.Now(),
	}

	if err := s.mergeLogRepo.Create(ctx, entry); err != nil {
		s.logger.Warn("failed to record the merge",
			zap.String("winner_id", winner.ID), zap.String("loser_id", loser.ID), zap.Error(err))
	}
}

// clearSyncState removes every provider's sync state for a contact that no longer exists.
func (s *MergeService) clearSyncState(ctx context.Context, userID, contactID string) {
	if s.syncRepo == nil {
		return
	}

	states, err := s.syncRepo.ListAllByUser(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to read sync state after a merge",
			zap.String("contact_id", contactID), zap.Error(err))
		return
	}

	for _, st := range states {
		if st.LocalID != contactID {
			continue
		}
		if err := s.syncRepo.Delete(ctx, st.ID); err != nil {
			s.logger.Warn("failed to clear sync state for a merged contact",
				zap.String("contact_id", contactID),
				zap.String("provider", st.ProviderType),
				zap.Error(err))
		}
	}
}

func displayName(c *domain.Contact) string {
	name := strings.TrimSpace(c.FirstName + " " + c.LastName)
	if name != "" {
		return name
	}
	if c.Email != "" {
		return c.Email
	}
	return c.UID
}
