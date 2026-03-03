package carddav

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/carddav"
	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	chqvcard "github.com/gumeniukcom/contactshq/internal/vcard"
)

// contextKey is used for passing auth info through context.
type contextKey string

const userIDKey contextKey = "userID"
const userEmailKey contextKey = "userEmail"

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func GetUserID(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}

func WithUserEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, userEmailKey, email)
}

func GetUserEmail(ctx context.Context) string {
	v, _ := ctx.Value(userEmailKey).(string)
	return v
}

type Backend struct {
	userRepo    repository.UserRepository
	abRepo      repository.AddressBookRepository
	contactRepo repository.ContactRepository
	prefix      string
}

func NewBackend(userRepo repository.UserRepository, abRepo repository.AddressBookRepository, contactRepo repository.ContactRepository, prefix string) *Backend {
	return &Backend{
		userRepo:    userRepo,
		abRepo:      abRepo,
		contactRepo: contactRepo,
		prefix:      prefix,
	}
}

func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	email := GetUserEmail(ctx)
	if email == "" {
		return "", fmt.Errorf("no authenticated user")
	}
	return b.prefix + "/" + email + "/", nil
}

func (b *Backend) AddressBookHomeSetPath(ctx context.Context) (string, error) {
	email := GetUserEmail(ctx)
	if email == "" {
		return "", fmt.Errorf("no authenticated user")
	}
	return b.prefix + "/" + email + "/", nil
}

func (b *Backend) ListAddressBooks(ctx context.Context) ([]carddav.AddressBook, error) {
	userID := GetUserID(ctx)
	if userID == "" {
		return nil, fmt.Errorf("no authenticated user")
	}

	ab, err := b.abRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ab == nil {
		return nil, nil
	}

	email := GetUserEmail(ctx)
	return []carddav.AddressBook{
		{
			Path:        b.prefix + "/" + email + "/contacts/",
			Name:        ab.Name,
			Description: ab.Description,
		},
	}, nil
}

func (b *Backend) GetAddressBook(ctx context.Context, path string) (*carddav.AddressBook, error) {
	userID := GetUserID(ctx)
	if userID == "" {
		return nil, fmt.Errorf("no authenticated user")
	}

	ab, err := b.abRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ab == nil {
		return nil, fmt.Errorf("address book not found")
	}

	email := GetUserEmail(ctx)
	return &carddav.AddressBook{
		Path:        b.prefix + "/" + email + "/contacts/",
		Name:        ab.Name,
		Description: ab.Description,
	}, nil
}

func (b *Backend) CreateAddressBook(ctx context.Context, addressBook *carddav.AddressBook) error {
	return fmt.Errorf("creating additional address books is not supported")
}

func (b *Backend) DeleteAddressBook(ctx context.Context, path string) error {
	return fmt.Errorf("deleting address books is not supported")
}

func (b *Backend) GetAddressObject(ctx context.Context, path string, req *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	userID := GetUserID(ctx)
	if userID == "" {
		return nil, fmt.Errorf("no authenticated user")
	}

	ab, err := b.abRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ab == nil {
		return nil, fmt.Errorf("address book not found")
	}

	uid := extractUIDFromPath(path)
	if uid == "" {
		return nil, fmt.Errorf("invalid path")
	}

	contact, err := b.contactRepo.GetByUID(ctx, ab.ID, uid)
	if err != nil {
		return nil, err
	}
	if contact == nil {
		return nil, fmt.Errorf("contact not found")
	}

	return contactToAddressObject(contact, path)
}

func (b *Backend) ListAddressObjects(ctx context.Context, path string, req *carddav.AddressDataRequest) ([]carddav.AddressObject, error) {
	userID := GetUserID(ctx)
	if userID == "" {
		return nil, fmt.Errorf("no authenticated user")
	}

	ab, err := b.abRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ab == nil {
		return nil, nil
	}

	contacts, err := b.contactRepo.ListAll(ctx, ab.ID)
	if err != nil {
		return nil, err
	}

	email := GetUserEmail(ctx)
	objects := make([]carddav.AddressObject, 0, len(contacts))
	for _, c := range contacts {
		objPath := b.prefix + "/" + email + "/contacts/" + c.UID + ".vcf"
		obj, err := contactToAddressObject(c, objPath)
		if err != nil {
			continue
		}
		objects = append(objects, *obj)
	}

	return objects, nil
}

func (b *Backend) QueryAddressObjects(ctx context.Context, path string, query *carddav.AddressBookQuery) ([]carddav.AddressObject, error) {
	// For simplicity, return all objects and let the library filter
	return b.ListAddressObjects(ctx, path, &query.DataRequest)
}

func (b *Backend) PutAddressObject(ctx context.Context, path string, card vcard.Card, opts *carddav.PutAddressObjectOptions) (*carddav.AddressObject, error) {
	userID := GetUserID(ctx)
	if userID == "" {
		return nil, fmt.Errorf("no authenticated user")
	}

	ab, err := b.abRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ab == nil {
		return nil, fmt.Errorf("address book not found")
	}

	uid := extractUIDFromPath(path)
	if uid == "" {
		// Try getting UID from card
		if uidField := card.Get(vcard.FieldUID); uidField != nil {
			uid = uidField.Value
		}
		if uid == "" {
			uid = uuid.New().String()
		}
	}

	vcardData := cardToString(card)
	h := sha256.Sum256([]byte(vcardData))
	etag := hex.EncodeToString(h[:8])

	now := time.Now()

	parsed, parseErr := chqvcard.ParseVCard(vcardData)
	if parseErr != nil {
		parsed = &chqvcard.ParsedContact{}
	}

	existing, err := b.contactRepo.GetByUID(ctx, ab.ID, uid)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		existing.VCardData = vcardData
		existing.ETag = etag
		existing.UpdatedAt = now
		chqvcard.ApplyToContact(existing, parsed)

		if err := b.contactRepo.Update(ctx, existing); err != nil {
			return nil, err
		}
		_ = writeChildRecords(ctx, b.contactRepo, existing.ID, parsed)

		return &carddav.AddressObject{
			Path:    path,
			ModTime: now,
			ETag:    `"` + etag + `"`,
			Card:    card,
		}, nil
	}

	contact := &domain.Contact{
		ID:            uuid.New().String(),
		AddressBookID: ab.ID,
		UID:           uid,
		ETag:          etag,
		VCardData:     vcardData,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	chqvcard.ApplyToContact(contact, parsed)

	if err := b.contactRepo.Create(ctx, contact); err != nil {
		return nil, err
	}
	_ = writeChildRecords(ctx, b.contactRepo, contact.ID, parsed)

	return &carddav.AddressObject{
		Path:    path,
		ModTime: now,
		ETag:    `"` + etag + `"`,
		Card:    card,
	}, nil
}

func (b *Backend) DeleteAddressObject(ctx context.Context, path string) error {
	userID := GetUserID(ctx)
	if userID == "" {
		return fmt.Errorf("no authenticated user")
	}

	ab, err := b.abRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if ab == nil {
		return fmt.Errorf("address book not found")
	}

	uid := extractUIDFromPath(path)
	if uid == "" {
		return fmt.Errorf("invalid path")
	}

	contact, err := b.contactRepo.GetByUID(ctx, ab.ID, uid)
	if err != nil {
		return err
	}
	if contact == nil {
		return fmt.Errorf("contact not found")
	}

	return b.contactRepo.Delete(ctx, contact.ID)
}

func extractUIDFromPath(path string) string {
	// Path format: /dav/{email}/contacts/{uid}.vcf
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	return strings.TrimSuffix(last, ".vcf")
}

func contactToAddressObject(contact *domain.Contact, path string) (*carddav.AddressObject, error) {
	card, err := vcard.NewDecoder(strings.NewReader(contact.VCardData)).Decode()
	if err != nil {
		// If we can't parse, return a minimal card
		card = make(vcard.Card)
		card.SetValue(vcard.FieldUID, contact.UID)
		card.SetValue(vcard.FieldVersion, "3.0")
		card.SetValue(vcard.FieldFormattedName, contact.FirstName+" "+contact.LastName)
	}

	return &carddav.AddressObject{
		Path:          path,
		ModTime:       contact.UpdatedAt,
		ContentLength: int64(len(contact.VCardData)),
		ETag:          `"` + contact.ETag + `"`,
		Card:          card,
	}, nil
}

func cardToString(card vcard.Card) string {
	var sb strings.Builder
	enc := vcard.NewEncoder(&sb)
	_ = enc.Encode(card)
	return sb.String()
}

// writeChildRecords writes multi-value child records for a contact.
func writeChildRecords(ctx context.Context, repo repository.ContactRepository, contactID string, p *chqvcard.ParsedContact) error {
	if err := repo.ReplaceEmails(ctx, contactID, chqvcard.ToEmails(contactID, p.Emails)); err != nil {
		return err
	}
	if err := repo.ReplacePhones(ctx, contactID, chqvcard.ToPhones(contactID, p.Phones)); err != nil {
		return err
	}
	if err := repo.ReplaceAddresses(ctx, contactID, chqvcard.ToAddresses(contactID, p.Addresses)); err != nil {
		return err
	}
	if err := repo.ReplaceURLs(ctx, contactID, chqvcard.ToURLs(contactID, p.URLs)); err != nil {
		return err
	}
	if err := repo.ReplaceIMs(ctx, contactID, chqvcard.ToIMs(contactID, p.IMs)); err != nil {
		return err
	}
	if err := repo.ReplaceCategories(ctx, contactID, chqvcard.ToCategories(contactID, p.Categories)); err != nil {
		return err
	}
	return repo.ReplaceDates(ctx, contactID, chqvcard.ToDates(contactID, p.Dates))
}
