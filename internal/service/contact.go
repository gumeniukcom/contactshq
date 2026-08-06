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

	// Fields carries the full structured contact from the web form. When present it
	// takes precedence over the flat fields above.
	Fields *ContactFields `json:"fields,omitempty"`
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

	// VCardData replaces the stored card wholesale. Only clients that own the whole card
	// — an import, another CardDAV server — should send it.
	VCardData *string `json:"vcard_data,omitempty"`

	// Fields carries the structured contact from the web form. It is merged into the
	// stored vCard, so properties the form does not model survive the edit.
	Fields *ContactFields `json:"fields,omitempty"`
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

	if input.Fields != nil {
		parsed = input.Fields.ToParsed(uid)
		vcardData, err = vcardpkg.BuildVCard(parsed)
		if err != nil {
			return nil, fmt.Errorf("build vcard: %w", err)
		}
	} else if vcardData == "" {
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

	if err := s.contactRepo.Save(ctx, contact, vcardpkg.ChildRecordsFor(contact.ID, parsed)); err != nil {
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

	if input.Fields != nil {
		parsed = input.Fields.ToParsed(contact.UID)
		contact.VCardData, err = vcardpkg.MergeIntoVCard(contact.VCardData, parsed)
		if err != nil {
			return nil, fmt.Errorf("merge vcard: %w", err)
		}
		// The merge may have kept properties (a photo, a REV) the form never saw; re-read
		// the card so the flat columns reflect what was actually stored.
		if reparsed, perr := vcardpkg.ParseVCard(contact.VCardData); perr == nil {
			parsed = reparsed
		}
	} else if input.VCardData != nil {
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

	if err := s.contactRepo.Save(ctx, contact, vcardpkg.ChildRecordsFor(contact.ID, parsed)); err != nil {
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

func (s *ContactService) List(ctx context.Context, userID string, limit, offset int, filters repository.ListFilters) ([]*domain.Contact, int, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}

	return s.contactRepo.ListWithRelations(ctx, ab.ID, limit, offset, filters)
}

func (s *ContactService) Search(ctx context.Context, userID, query string, limit, offset int, filters repository.ListFilters) ([]*domain.Contact, int, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}

	return s.contactRepo.SearchWithRelations(ctx, ab.ID, query, limit, offset, filters)
}

func (s *ContactService) Facets(ctx context.Context, userID string) (*repository.ContactFacets, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.contactRepo.Facets(ctx, ab.ID)
}

// MaxBulkIDs bounds a single bulk request. Selection happens a page at a time, so this
// is well above anything the UI can produce, and it keeps a hostile payload from turning
// into an unbounded IN clause.
const MaxBulkIDs = 500

var ErrTooManyIDs = errors.New("too many contact ids in one request")

// DeleteMany removes several contacts at once and reports how many existed.
//
// The list view used to issue one DELETE per contact in a loop; a failure halfway
// through left an unknown subset deleted and reported nothing.
func (s *ContactService) DeleteMany(ctx context.Context, userID string, ids []string) (int, error) {
	if len(ids) > MaxBulkIDs {
		return 0, fmt.Errorf("%w: %d (max %d)", ErrTooManyIDs, len(ids), MaxBulkIDs)
	}

	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}
	if ab == nil {
		return 0, ErrAddressBookNotFound
	}

	return s.contactRepo.DeleteMany(ctx, ab.ID, dedupe(ids))
}

// dedupe keeps the caller from inflating the row count with repeated ids.
func dedupe(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
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

func generateETag(data string) string {
	return ContactETag(data)
}

// ContactETag derives a contact's ETag from its vCard text. Exported so a repair command can
// recompute exactly what the write paths would have stored — a hand-rolled second copy of
// this would drift and hand CardDAV clients an ETag that never matches.
func ContactETag(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:8])
}
