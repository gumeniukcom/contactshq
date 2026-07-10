package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

type ExporterService struct {
	contactRepo repository.ContactRepository
	abRepo      repository.AddressBookRepository
}

func NewExporterService(contactRepo repository.ContactRepository, abRepo repository.AddressBookRepository) *ExporterService {
	return &ExporterService{
		contactRepo: contactRepo,
		abRepo:      abRepo,
	}
}

func (s *ExporterService) ExportVCard(ctx context.Context, userID string) (string, error) {
	return s.ExportVCardByIDs(ctx, userID, nil)
}

// ExportVCardByIDs exports the named contacts, or the whole address book when ids is empty.
//
// The list view used to fetch each selected contact with its own request and quietly drop
// the ones that failed, so the downloaded file could be missing contacts it claimed to
// contain.
func (s *ExporterService) ExportVCardByIDs(ctx context.Context, userID string, ids []string) (string, error) {
	if len(ids) > MaxBulkIDs {
		return "", fmt.Errorf("%w: %d (max %d)", ErrTooManyIDs, len(ids), MaxBulkIDs)
	}

	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return "", err
	}

	var contacts []*domain.Contact
	if len(ids) == 0 {
		contacts, err = s.contactRepo.ListAll(ctx, ab.ID)
	} else {
		contacts, err = s.contactRepo.ListByIDs(ctx, ab.ID, dedupe(ids))
	}
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	for _, c := range contacts {
		buf.WriteString(c.VCardData)
	}

	return buf.String(), nil
}

func (s *ExporterService) ExportCSV(ctx context.Context, userID string) (string, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return "", err
	}

	contacts, err := s.contactRepo.ListAll(ctx, ab.ID)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	_ = w.Write([]string{"first_name", "last_name", "email", "phone", "org", "title", "note"})

	for _, c := range contacts {
		_ = w.Write([]string{c.FirstName, c.LastName, c.Email, c.Phone, c.Org, c.Title, c.Note})
	}

	w.Flush()
	return buf.String(), nil
}

type contactExport struct {
	ID        string `json:"id"`
	UID       string `json:"uid"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Org       string `json:"org"`
	Title     string `json:"title"`
	Note      string `json:"note"`
	VCardData string `json:"vcard_data"`
}

func (s *ExporterService) ExportJSON(ctx context.Context, userID string) (string, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return "", err
	}

	contacts, err := s.contactRepo.ListAll(ctx, ab.ID)
	if err != nil {
		return "", err
	}

	exports := make([]contactExport, 0, len(contacts))
	for _, c := range contacts {
		exports = append(exports, contactExport{
			ID:        c.ID,
			UID:       c.UID,
			FirstName: c.FirstName,
			LastName:  c.LastName,
			Email:     c.Email,
			Phone:     c.Phone,
			Org:       c.Org,
			Title:     c.Title,
			Note:      c.Note,
			VCardData: c.VCardData,
		})
	}

	data, err := json.MarshalIndent(exports, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}
