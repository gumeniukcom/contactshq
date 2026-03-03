package service

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

type ImporterService struct {
	contactRepo repository.ContactRepository
	abRepo      repository.AddressBookRepository
}

func NewImporterService(contactRepo repository.ContactRepository, abRepo repository.AddressBookRepository) *ImporterService {
	return &ImporterService{
		contactRepo: contactRepo,
		abRepo:      abRepo,
	}
}

type ImportResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Errors   int `json:"errors"`
}

func (s *ImporterService) ImportVCard(ctx context.Context, userID string, data string) (*ImportResult, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := &ImportResult{}
	cards := vcardpkg.SplitVCards(data)

	for _, card := range cards {
		card = strings.TrimSpace(card)
		if card == "" {
			continue
		}

		parsed, err := vcardpkg.ParseVCard(card)
		if err != nil {
			result.Errors++
			continue
		}

		uid := parsed.UID
		if uid == "" {
			uid = uuid.New().String()
			card = vcardpkg.InjectUID(card, uid)
			parsed.UID = uid
		}

		existing, err := s.contactRepo.GetByUID(ctx, ab.ID, uid)
		if err != nil {
			result.Errors++
			continue
		}

		if existing != nil {
			existing.VCardData = card
			existing.ETag = generateETag(card)
			vcardpkg.ApplyToContact(existing, parsed)
			existing.UpdatedAt = time.Now()
			if err := s.contactRepo.Update(ctx, existing); err != nil {
				result.Errors++
				continue
			}
			if err := writeChildRecords(ctx, s.contactRepo, existing.ID, parsed); err != nil {
				result.Errors++
				continue
			}
			result.Imported++
			continue
		}

		now := time.Now()
		contact := &domain.Contact{
			ID:            uuid.New().String(),
			AddressBookID: ab.ID,
			UID:           uid,
			ETag:          generateETag(card),
			VCardData:     card,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		vcardpkg.ApplyToContact(contact, parsed)

		if err := s.contactRepo.Create(ctx, contact); err != nil {
			result.Errors++
			continue
		}
		if err := writeChildRecords(ctx, s.contactRepo, contact.ID, parsed); err != nil {
			result.Errors++
			continue
		}
		result.Imported++
	}

	return result, nil
}

func (s *ImporterService) ImportCSV(ctx context.Context, userID string, data string) (*ImportResult, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := &ImportResult{}
	reader := csv.NewReader(strings.NewReader(data))

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.ToLower(strings.TrimSpace(col))] = i
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors++
			continue
		}

		firstName := getCSVField(record, colMap, "first_name", "firstname", "first name")
		lastName := getCSVField(record, colMap, "last_name", "lastname", "last name")
		email := getCSVField(record, colMap, "email", "e-mail")
		phone := getCSVField(record, colMap, "phone", "telephone", "tel")
		org := getCSVField(record, colMap, "org", "organization", "company")
		title := getCSVField(record, colMap, "title", "job_title")
		note := getCSVField(record, colMap, "note", "notes", "description")

		uid := uuid.New().String()
		p := vcardpkg.NewFromSimple(uid, firstName, lastName, email, phone, org, title, note)
		vcardData, err := vcardpkg.BuildVCard(p)
		if err != nil {
			result.Errors++
			continue
		}
		now := time.Now()

		contact := &domain.Contact{
			ID:            uuid.New().String(),
			AddressBookID: ab.ID,
			UID:           uid,
			ETag:          generateETag(vcardData),
			VCardData:     vcardData,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		vcardpkg.ApplyToContact(contact, p)

		if err := s.contactRepo.Create(ctx, contact); err != nil {
			result.Errors++
			continue
		}
		if err := writeChildRecords(ctx, s.contactRepo, contact.ID, p); err != nil {
			result.Errors++
			continue
		}
		result.Imported++
	}

	return result, nil
}

// writeChildRecords writes multi-value child records for a contact.
// It is a package-level helper shared by ImporterService and BackupService.
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

func getCSVField(record []string, colMap map[string]int, names ...string) string {
	for _, name := range names {
		if idx, ok := colMap[name]; ok && idx < len(record) {
			return strings.TrimSpace(record[idx])
		}
	}
	return ""
}
