package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

// writeChildRecords writes multi-value child records for a contact.
func writeChildRecords(ctx context.Context, repo repository.ContactRepository, contactID string, p *vcardpkg.ParsedContact) error {
	if err := repo.ReplaceEmails(ctx, contactID, vcardpkg.ToEmails(contactID, p.Emails)); err != nil {
		return err
	}
	if err := repo.ReplacePhones(ctx, contactID, vcardpkg.ToPhones(contactID, p.Phones)); err != nil {
		return err
	}
	if err := repo.ReplaceAddresses(ctx, contactID, vcardpkg.ToAddresses(contactID, p.Addresses)); err != nil {
		return err
	}
	if err := repo.ReplaceURLs(ctx, contactID, vcardpkg.ToURLs(contactID, p.URLs)); err != nil {
		return err
	}
	if err := repo.ReplaceIMs(ctx, contactID, vcardpkg.ToIMs(contactID, p.IMs)); err != nil {
		return err
	}
	if err := repo.ReplaceCategories(ctx, contactID, vcardpkg.ToCategories(contactID, p.Categories)); err != nil {
		return err
	}
	return repo.ReplaceDates(ctx, contactID, vcardpkg.ToDates(contactID, p.Dates))
}

type InternalProvider struct {
	contactRepo repository.ContactRepository
	abRepo      repository.AddressBookRepository
	userID      string
}

func NewInternalProvider(contactRepo repository.ContactRepository, abRepo repository.AddressBookRepository, userID string) *InternalProvider {
	return &InternalProvider{
		contactRepo: contactRepo,
		abRepo:      abRepo,
		userID:      userID,
	}
}

func (p *InternalProvider) Name() string {
	return "internal"
}

func (p *InternalProvider) List(ctx context.Context) ([]SyncItem, error) {
	ab, err := p.abRepo.GetOrCreateByUserID(ctx, p.userID)
	if err != nil {
		return nil, err
	}

	contacts, err := p.contactRepo.ListAll(ctx, ab.ID)
	if err != nil {
		return nil, err
	}

	items := make([]SyncItem, 0, len(contacts))
	for _, c := range contacts {
		h := sha256.Sum256([]byte(c.VCardData))
		items = append(items, SyncItem{
			RemoteID:    c.UID,
			ETag:        c.ETag,
			ContentHash: hex.EncodeToString(h[:]),
			VCardData:   c.VCardData,
		})
	}

	return items, nil
}

func (p *InternalProvider) Get(ctx context.Context, remoteID string) (*SyncItem, error) {
	ab, err := p.abRepo.GetOrCreateByUserID(ctx, p.userID)
	if err != nil {
		return nil, err
	}

	contact, err := p.contactRepo.GetByUID(ctx, ab.ID, remoteID)
	if err != nil {
		return nil, err
	}
	if contact == nil {
		return nil, nil
	}

	h := sha256.Sum256([]byte(contact.VCardData))
	return &SyncItem{
		RemoteID:    contact.UID,
		ETag:        contact.ETag,
		ContentHash: hex.EncodeToString(h[:]),
		VCardData:   contact.VCardData,
	}, nil
}

func (p *InternalProvider) Put(ctx context.Context, item SyncItem) (string, error) {
	ab, err := p.abRepo.GetOrCreateByUserID(ctx, p.userID)
	if err != nil {
		return "", err
	}

	existing, err := p.contactRepo.GetByUID(ctx, ab.ID, item.RemoteID)
	if err != nil {
		return "", err
	}

	h := sha256.Sum256([]byte(item.VCardData))
	etag := hex.EncodeToString(h[:8])
	now := time.Now()

	parsed, _ := vcardpkg.ParseVCard(item.VCardData)
	if parsed == nil {
		parsed = &vcardpkg.ParsedContact{}
	}

	if existing != nil {
		existing.VCardData = item.VCardData
		existing.ETag = etag
		existing.UpdatedAt = now
		vcardpkg.ApplyToContact(existing, parsed)
		if err := p.contactRepo.Update(ctx, existing); err != nil {
			return "", err
		}
		_ = writeChildRecords(ctx, p.contactRepo, existing.ID, parsed)
		return etag, nil
	}

	contact := &domain.Contact{
		ID:            uuid.New().String(),
		AddressBookID: ab.ID,
		UID:           item.RemoteID,
		ETag:          etag,
		VCardData:     item.VCardData,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	vcardpkg.ApplyToContact(contact, parsed)

	if err := p.contactRepo.Create(ctx, contact); err != nil {
		return "", err
	}
	_ = writeChildRecords(ctx, p.contactRepo, contact.ID, parsed)

	return etag, nil
}

func (p *InternalProvider) Delete(ctx context.Context, remoteID string) error {
	ab, err := p.abRepo.GetOrCreateByUserID(ctx, p.userID)
	if err != nil {
		return err
	}

	contact, err := p.contactRepo.GetByUID(ctx, ab.ID, remoteID)
	if err != nil {
		return err
	}
	if contact == nil {
		return nil
	}

	return p.contactRepo.Delete(ctx, contact.ID)
}
