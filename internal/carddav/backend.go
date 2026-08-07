package carddav

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
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

// go-webdav resolves the kind of resource a request targets purely from the number of
// path segments below the prefix: 1 = user principal, 2 = address book home set,
// 3 = address book, 4 = address object. The layout below must keep those depths, or
// the handler misroutes — a collection PROPFIND lists nothing and DELETE of a contact
// is dispatched to DeleteAddressBook.
//
//	/dav/{email}/                            principal
//	/dav/{email}/addressbooks/               home set
//	/dav/{email}/addressbooks/contacts/      address book
//	/dav/{email}/addressbooks/contacts/{uid}.vcf   address object
const (
	homeSetSegment     = "addressbooks"
	addressBookSegment = "contacts"
)

// PrincipalPath is the CardDAV principal URL for a user, the value iOS configuration
// profiles point at.
func PrincipalPath(prefix, email string) string {
	return prefix + "/" + email + "/"
}

// HomeSetPath is the address book home set URL for a user.
func HomeSetPath(prefix, email string) string {
	return PrincipalPath(prefix, email) + homeSetSegment + "/"
}

// AddressBookPath is the collection URL clients sync contacts from.
func AddressBookPath(prefix, email string) string {
	return HomeSetPath(prefix, email) + addressBookSegment + "/"
}

// AddressObjectPath is the URL of a single contact within the address book.
func AddressObjectPath(prefix, email, uid string) string {
	return AddressBookPath(prefix, email) + uid + ".vcf"
}

type Backend struct {
	userRepo    repository.UserRepository
	abRepo      repository.AddressBookRepository
	contactRepo repository.ContactRepository
	prefix      string
	// maxResourceSize is advertised to clients as CARDDAV:max-resource-size. Zero means the
	// property is omitted, which is how go-webdav behaved before.
	maxResourceSize int64
}

func NewBackend(userRepo repository.UserRepository, abRepo repository.AddressBookRepository, contactRepo repository.ContactRepository, prefix string) *Backend {
	return &Backend{
		userRepo:    userRepo,
		abRepo:      abRepo,
		contactRepo: contactRepo,
		prefix:      prefix,
	}
}

// WithMaxResourceSize advertises a per-card size limit to clients.
func (b *Backend) WithMaxResourceSize(bytes int64) *Backend {
	b.maxResourceSize = bytes
	return b
}

func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	email := GetUserEmail(ctx)
	if email == "" {
		return "", fmt.Errorf("no authenticated user")
	}
	return PrincipalPath(b.prefix, email), nil
}

func (b *Backend) AddressBookHomeSetPath(ctx context.Context) (string, error) {
	email := GetUserEmail(ctx)
	if email == "" {
		return "", fmt.Errorf("no authenticated user")
	}
	return HomeSetPath(b.prefix, email), nil
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
			Path:        AddressBookPath(b.prefix, email),
			Name:        ab.Name,
			Description: ab.Description,
			// Announced so a client learns the limit before uploading rather than by being
			// refused afterwards; go-webdav renders it as CARDDAV:max-resource-size.
			MaxResourceSize: b.maxResourceSize,
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
		Path:            AddressBookPath(b.prefix, email),
		Name:            ab.Name,
		Description:     ab.Description,
		MaxResourceSize: b.maxResourceSize,
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
		objPath := AddressObjectPath(b.prefix, email, c.UID)
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

	if err := checkPreconditions(opts, existing); err != nil {
		return nil, err
	}

	contact := existing
	if contact == nil {
		contact = &domain.Contact{
			ID:            uuid.New().String(),
			AddressBookID: ab.ID,
			UID:           uid,
			CreatedAt:     now,
		}
	}
	contact.VCardData = vcardData
	contact.ETag = etag
	contact.UpdatedAt = now
	chqvcard.ApplyToContact(contact, parsed)

	if err := b.contactRepo.Save(ctx, contact, chqvcard.ChildRecordsFor(contact.ID, parsed)); err != nil {
		return nil, err
	}

	return &carddav.AddressObject{
		Path:    path,
		ModTime: now,
		// Unquoted: go-webdav quotes the value when it writes the ETag header. Quoting
		// it here produced a doubly-quoted, malformed header.
		ETag: etag,
		Card: card,
	}, nil
}

// checkPreconditions enforces the If-Match and If-None-Match headers a CardDAV client
// sends to guard against lost updates. Ignoring them meant two devices editing the same
// contact would each overwrite the other without noticing.
func checkPreconditions(opts *carddav.PutAddressObjectOptions, existing *domain.Contact) error {
	if opts == nil {
		return nil
	}

	// "If-None-Match: *" means create only — fail if the contact is already there.
	if opts.IfNoneMatch.IsSet() {
		if existing != nil {
			return webdav.NewHTTPError(http.StatusPreconditionFailed,
				fmt.Errorf("contact %s already exists", existing.UID))
		}
		return nil
	}

	if !opts.IfMatch.IsSet() {
		return nil
	}

	// "If-Match" means update only, and only if the client saw the current version.
	if existing == nil {
		return webdav.NewHTTPError(http.StatusPreconditionFailed,
			fmt.Errorf("contact does not exist"))
	}
	match, err := opts.IfMatch.MatchETag(existing.ETag)
	if err != nil {
		return webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	if !match {
		return webdav.NewHTTPError(http.StatusPreconditionFailed,
			fmt.Errorf("contact %s was modified by someone else", existing.UID))
	}
	return nil
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
	// Path format: /dav/{email}/addressbooks/contacts/{uid}.vcf
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
		ETag:          contact.ETag,
		Card:          card,
	}, nil
}

// cardToString delegates to the shared encoder; see the note in internal/vcard/encoder.go.
func cardToString(card vcard.Card) string {
	return chqvcard.CardToString(card)
}
