package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"

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
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return "", err
	}

	contacts, err := s.contactRepo.ListAll(ctx, ab.ID)
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
