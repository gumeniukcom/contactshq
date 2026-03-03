package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

var (
	ErrContactNotFound     = errors.New("contact not found")
	ErrAddressBookNotFound = errors.New("address book not found")
)

type ContactService struct {
	contactRepo repository.ContactRepository
	abRepo      repository.AddressBookRepository
}

func NewContactService(contactRepo repository.ContactRepository, abRepo repository.AddressBookRepository) *ContactService {
	return &ContactService{
		contactRepo: contactRepo,
		abRepo:      abRepo,
	}
}

// CreateContactInput supports both a flat form (single email/phone) and a full vCard blob.
type CreateContactInput struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Org       string `json:"org"`
	Title     string `json:"title"`
	Note      string `json:"note"`
	VCardData string `json:"vcard_data,omitempty"`
}

// UpdateContactInput supports partial flat-field updates or a full vCard replacement.
type UpdateContactInput struct {
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	Email     *string `json:"email,omitempty"`
	Phone     *string `json:"phone,omitempty"`
	Org       *string `json:"org,omitempty"`
	Title     *string `json:"title,omitempty"`
	Note      *string `json:"note,omitempty"`
	VCardData *string `json:"vcard_data,omitempty"`
}

func (s *ContactService) Create(ctx context.Context, userID string, input CreateContactInput) (*domain.Contact, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	uid := uuid.New().String()
	now := time.Now()

	var parsed *vcardpkg.ParsedContact
	vcardData := input.VCardData

	if vcardData == "" {
		parsed = vcardpkg.NewFromSimple(uid, input.FirstName, input.LastName,
			input.Email, input.Phone, input.Org, input.Title, input.Note)
		vcardData, err = vcardpkg.BuildVCard(parsed)
		if err != nil {
			return nil, fmt.Errorf("build vcard: %w", err)
		}
	} else {
		parsed, err = vcardpkg.ParseVCard(vcardData)
		if err != nil {
			return nil, fmt.Errorf("parse vcard: %w", err)
		}
		if parsed.UID == "" {
			vcardData = vcardpkg.InjectUID(vcardData, uid)
			parsed.UID = uid
		} else {
			uid = parsed.UID
		}
	}

	contact := &domain.Contact{
		ID:            uuid.New().String(),
		AddressBookID: ab.ID,
		UID:           uid,
		ETag:          generateETag(vcardData),
		VCardData:     vcardData,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	vcardpkg.ApplyToContact(contact, parsed)

	if err := s.contactRepo.Create(ctx, contact); err != nil {
		return nil, err
	}
	if err := s.writeChildRecords(ctx, contact.ID, parsed); err != nil {
		return nil, err
	}

	return contact, nil
}

func (s *ContactService) GetByID(ctx context.Context, userID, contactID string) (*domain.Contact, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	contact, err := s.contactRepo.GetByIDWithRelations(ctx, contactID)
	if err != nil {
		return nil, err
	}
	if contact == nil || contact.AddressBookID != ab.ID {
		return nil, ErrContactNotFound
	}

	return contact, nil
}

func (s *ContactService) Update(ctx context.Context, userID, contactID string, input UpdateContactInput) (*domain.Contact, error) {
	// Fetch without relations — we only need the base contact for the update
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	contact, err := s.contactRepo.GetByID(ctx, contactID)
	if err != nil {
		return nil, err
	}
	if contact == nil || contact.AddressBookID != ab.ID {
		return nil, ErrContactNotFound
	}

	var parsed *vcardpkg.ParsedContact

	if input.VCardData != nil {
		// Full vCard replacement
		contact.VCardData = *input.VCardData
		parsed, err = vcardpkg.ParseVCard(*input.VCardData)
		if err != nil {
			return nil, fmt.Errorf("parse vcard: %w", err)
		}
	} else {
		// Parse existing vCard to preserve multi-value fields
		parsed, _ = vcardpkg.ParseVCard(contact.VCardData)
		if parsed == nil {
			parsed = &vcardpkg.ParsedContact{UID: contact.UID}
		}
		// Apply only provided flat fields
		if input.FirstName != nil {
			parsed.FirstName = *input.FirstName
		}
		if input.LastName != nil {
			parsed.LastName = *input.LastName
		}
		if input.Email != nil {
			if len(parsed.Emails) > 0 {
				parsed.Emails[0].Value = *input.Email
			} else {
				parsed.Emails = []vcardpkg.Field{{Value: *input.Email}}
			}
			parsed.PrimaryEmail = *input.Email
		}
		if input.Phone != nil {
			if len(parsed.Phones) > 0 {
				parsed.Phones[0].Value = *input.Phone
			} else {
				parsed.Phones = []vcardpkg.Field{{Value: *input.Phone}}
			}
			parsed.PrimaryPhone = *input.Phone
		}
		if input.Org != nil {
			parsed.Org = *input.Org
		}
		if input.Title != nil {
			parsed.Title = *input.Title
		}
		if input.Note != nil {
			parsed.Note = *input.Note
		}
		// Rebuild vCard from the updated parsed state
		contact.VCardData, err = vcardpkg.BuildVCard(parsed)
		if err != nil {
			return nil, fmt.Errorf("build vcard: %w", err)
		}
	}

	vcardpkg.ApplyToContact(contact, parsed)
	contact.ETag = generateETag(contact.VCardData)
	contact.UpdatedAt = time.Now()

	if err := s.contactRepo.Update(ctx, contact); err != nil {
		return nil, err
	}
	if err := s.writeChildRecords(ctx, contact.ID, parsed); err != nil {
		return nil, err
	}

	return contact, nil
}

func (s *ContactService) Delete(ctx context.Context, userID, contactID string) error {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if ab == nil {
		return ErrAddressBookNotFound
	}
	contact, err := s.contactRepo.GetByID(ctx, contactID)
	if err != nil {
		return err
	}
	if contact == nil || contact.AddressBookID != ab.ID {
		return ErrContactNotFound
	}
	return s.contactRepo.Delete(ctx, contact.ID)
}

func (s *ContactService) List(ctx context.Context, userID string, limit, offset int) ([]*domain.Contact, int, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}

	return s.contactRepo.ListWithRelations(ctx, ab.ID, limit, offset)
}

func (s *ContactService) Search(ctx context.Context, userID, query string, limit, offset int) ([]*domain.Contact, int, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}

	return s.contactRepo.SearchWithRelations(ctx, ab.ID, query, limit, offset)
}

func (s *ContactService) DeleteAll(ctx context.Context, userID string) error {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if ab == nil {
		return ErrAddressBookNotFound
	}
	return s.contactRepo.DeleteAll(ctx, ab.ID)
}

func (s *ContactService) ListAll(ctx context.Context, userID string) ([]*domain.Contact, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.contactRepo.ListAll(ctx, ab.ID)
}

// writeChildRecords writes all multi-value child records for a contact.
func (s *ContactService) writeChildRecords(ctx context.Context, contactID string, p *vcardpkg.ParsedContact) error {
	if err := s.contactRepo.ReplaceEmails(ctx, contactID, vcardpkg.ToEmails(contactID, p.Emails)); err != nil {
		return fmt.Errorf("replace emails: %w", err)
	}
	if err := s.contactRepo.ReplacePhones(ctx, contactID, vcardpkg.ToPhones(contactID, p.Phones)); err != nil {
		return fmt.Errorf("replace phones: %w", err)
	}
	if err := s.contactRepo.ReplaceAddresses(ctx, contactID, vcardpkg.ToAddresses(contactID, p.Addresses)); err != nil {
		return fmt.Errorf("replace addresses: %w", err)
	}
	if err := s.contactRepo.ReplaceURLs(ctx, contactID, vcardpkg.ToURLs(contactID, p.URLs)); err != nil {
		return fmt.Errorf("replace urls: %w", err)
	}
	if err := s.contactRepo.ReplaceIMs(ctx, contactID, vcardpkg.ToIMs(contactID, p.IMs)); err != nil {
		return fmt.Errorf("replace ims: %w", err)
	}
	if err := s.contactRepo.ReplaceCategories(ctx, contactID, vcardpkg.ToCategories(contactID, p.Categories)); err != nil {
		return fmt.Errorf("replace categories: %w", err)
	}
	if err := s.contactRepo.ReplaceDates(ctx, contactID, vcardpkg.ToDates(contactID, p.Dates)); err != nil {
		return fmt.Errorf("replace dates: %w", err)
	}
	return nil
}

func generateETag(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:8])
}
