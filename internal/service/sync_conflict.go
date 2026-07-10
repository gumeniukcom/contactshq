package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

var (
	ErrConflictNotFound     = errors.New("conflict not found")
	ErrConflictNotPending   = errors.New("conflict already resolved")
	ErrConflictForbidden    = errors.New("conflict belongs to another user")
	ErrConflictContactGone  = errors.New("the contact this conflict refers to no longer exists")
	ErrConflictStateMissing = errors.New("no sync state for this conflict")
)

// SyncConflictService applies a user's conflict resolution.
//
// Storing the merged vCard on the conflict row is not a resolution: nothing reads it
// back. A resolution has to land on the contact itself and advance the sync state, or
// the next run re-detects the same divergence and the user's choice evaporates.
type SyncConflictService struct {
	conflictRepo  repository.SyncConflictRepository
	syncStateRepo repository.SyncStateRepository
	contactRepo   repository.ContactRepository
	abRepo        repository.AddressBookRepository
}

func NewSyncConflictService(
	conflictRepo repository.SyncConflictRepository,
	syncStateRepo repository.SyncStateRepository,
	contactRepo repository.ContactRepository,
	abRepo repository.AddressBookRepository,
) *SyncConflictService {
	return &SyncConflictService{
		conflictRepo:  conflictRepo,
		syncStateRepo: syncStateRepo,
		contactRepo:   contactRepo,
		abRepo:        abRepo,
	}
}

// Resolve merges the user's per-field choices, writes the result to the local contact,
// and leaves the sync state so the next run pushes it to the remote instead of flagging
// the same conflict again.
func (s *SyncConflictService) Resolve(ctx context.Context, userID, conflictID string, resolution map[string]string) (*domain.SyncConflict, error) {
	conflict, err := s.load(ctx, userID, conflictID)
	if err != nil {
		return nil, err
	}

	resolved, err := chqsync.ApplyResolution(conflict.BaseVCard, conflict.LocalVCard, conflict.RemoteVCard, resolution)
	if err != nil {
		return nil, fmt.Errorf("apply resolution: %w", err)
	}

	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	contact, err := s.contactRepo.GetByUID(ctx, ab.ID, conflict.LocalContactID)
	if err != nil {
		return nil, err
	}
	if contact == nil {
		return nil, fmt.Errorf("%w: uid %s", ErrConflictContactGone, conflict.LocalContactID)
	}

	parsed, err := vcardpkg.ParseVCard(resolved)
	if err != nil {
		return nil, fmt.Errorf("parse resolved vcard: %w", err)
	}

	now := time.Now()
	etag := etagOf(resolved)

	contact.VCardData = resolved
	contact.ETag = etag
	contact.UpdatedAt = now
	vcardpkg.ApplyToContact(contact, parsed)

	if err := s.contactRepo.Save(ctx, contact, vcardpkg.ChildRecordsFor(contact.ID, parsed)); err != nil {
		return nil, fmt.Errorf("save contact: %w", err)
	}

	if err := s.advanceSyncState(ctx, userID, conflict, resolved); err != nil {
		return nil, err
	}

	conflict.Status = "resolved"
	conflict.ResolvedVCard = resolved
	conflict.ResolvedAt = &now
	if err := s.conflictRepo.Update(ctx, conflict); err != nil {
		return nil, fmt.Errorf("save resolution: %w", err)
	}

	return conflict, nil
}

// advanceSyncState records the resolved vCard as the new merge base and adopts the
// remote ETag seen when the conflict arose, so the remote no longer looks changed.
// LocalETag is deliberately cleared: the local copy now differs from what the remote
// holds, and a cleared ETag is what makes the next push carry the resolution outward.
func (s *SyncConflictService) advanceSyncState(ctx context.Context, userID string, conflict *domain.SyncConflict, resolved string) error {
	state, err := s.syncStateRepo.GetByRemoteID(ctx, userID, conflict.ProviderType, conflict.RemoteID)
	if err != nil {
		return fmt.Errorf("load sync state: %w", err)
	}
	if state == nil {
		// The pipeline may have been deleted between detection and resolution. The
		// contact is already updated; there is simply no state left to advance.
		return nil
	}

	state.BaseVCard = resolved
	state.ContentHash = contentHashOf(resolved)
	state.RemoteETag = conflict.RemoteETag
	state.LocalETag = ""
	state.LastSyncedAt = time.Now()

	if err := s.syncStateRepo.Update(ctx, state); err != nil {
		return fmt.Errorf("update sync state: %w", err)
	}
	return nil
}

// Dismiss marks a conflict as reviewed without changing any contact.
func (s *SyncConflictService) Dismiss(ctx context.Context, userID, conflictID string) error {
	conflict, err := s.load(ctx, userID, conflictID)
	if err != nil {
		return err
	}

	now := time.Now()
	conflict.Status = "dismissed"
	conflict.ResolvedAt = &now
	return s.conflictRepo.Update(ctx, conflict)
}

func (s *SyncConflictService) load(ctx context.Context, userID, conflictID string) (*domain.SyncConflict, error) {
	conflict, err := s.conflictRepo.GetByID(ctx, conflictID)
	if err != nil || conflict == nil {
		return nil, ErrConflictNotFound
	}
	if conflict.UserID != userID {
		return nil, ErrConflictForbidden
	}
	if conflict.Status != "pending" {
		return nil, ErrConflictNotPending
	}
	return conflict, nil
}

func etagOf(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:8])
}

func contentHashOf(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
