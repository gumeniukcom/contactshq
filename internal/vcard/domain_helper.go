package vcard

import (
	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
)

// ApplyToContact copies scalar fields from a ParsedContact into a domain.Contact.
// It does NOT touch child relations (Emails/Phones/etc.) or VCardData/ETag.
func ApplyToContact(c *domain.Contact, p *ParsedContact) {
	c.FirstName = p.FirstName
	c.LastName = p.LastName
	c.MiddleName = p.MiddleName
	c.NamePrefix = p.NamePrefix
	c.NameSuffix = p.NameSuffix
	c.Nickname = p.Nickname
	c.Email = p.PrimaryEmail
	c.Phone = p.PrimaryPhone
	c.Org = p.Org
	c.Department = p.Department
	c.Title = p.Title
	c.Role = p.Role
	c.Note = p.Note
	c.Gender = p.Gender
	c.TZ = p.TZ
	c.Geo = p.Geo
	c.PhotoURI = p.PhotoURI
	c.Rev = p.Rev

	// Derive bday and anniversary from Dates slice
	c.Bday = ""
	c.Anniversary = ""
	for _, d := range p.Dates {
		switch d.Kind {
		case "bday":
			c.Bday = d.Value
		case "anniversary":
			c.Anniversary = d.Value
		}
	}
}

// ToEmails converts parsed email fields to domain records.
func ToEmails(contactID string, fields []Field) []*domain.ContactEmail {
	out := make([]*domain.ContactEmail, 0, len(fields))
	for _, f := range fields {
		out = append(out, &domain.ContactEmail{
			ID:        uuid.New().String(),
			ContactID: contactID,
			Value:     f.Value,
			Type:      f.Type,
			Pref:      f.Pref,
			Label:     f.Label,
		})
	}
	return out
}

// ToPhones converts parsed phone fields to domain records.
func ToPhones(contactID string, fields []Field) []*domain.ContactPhone {
	out := make([]*domain.ContactPhone, 0, len(fields))
	for _, f := range fields {
		out = append(out, &domain.ContactPhone{
			ID:        uuid.New().String(),
			ContactID: contactID,
			Value:     f.Value,
			Type:      f.Type,
			Pref:      f.Pref,
			Label:     f.Label,
		})
	}
	return out
}

// ToAddresses converts parsed address fields to domain records.
func ToAddresses(contactID string, addrs []Address) []*domain.ContactAddress {
	out := make([]*domain.ContactAddress, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, &domain.ContactAddress{
			ID:         uuid.New().String(),
			ContactID:  contactID,
			Type:       a.Type,
			Pref:       a.Pref,
			Label:      a.Label,
			POBox:      a.POBox,
			Extended:   a.Extended,
			Street:     a.Street,
			City:       a.City,
			Region:     a.Region,
			PostalCode: a.PostalCode,
			Country:    a.Country,
		})
	}
	return out
}

// ToURLs converts parsed URL fields to domain records.
func ToURLs(contactID string, fields []Field) []*domain.ContactURL {
	out := make([]*domain.ContactURL, 0, len(fields))
	for _, f := range fields {
		out = append(out, &domain.ContactURL{
			ID:        uuid.New().String(),
			ContactID: contactID,
			Value:     f.Value,
			Type:      f.Type,
			Pref:      f.Pref,
		})
	}
	return out
}

// ToIMs converts parsed IM fields to domain records.
func ToIMs(contactID string, fields []Field) []*domain.ContactIM {
	out := make([]*domain.ContactIM, 0, len(fields))
	for _, f := range fields {
		out = append(out, &domain.ContactIM{
			ID:        uuid.New().String(),
			ContactID: contactID,
			Value:     f.Value,
			Type:      f.Type,
			Pref:      f.Pref,
		})
	}
	return out
}

// ToCategories converts parsed category strings to domain records.
func ToCategories(contactID string, cats []string) []*domain.ContactCategory {
	out := make([]*domain.ContactCategory, 0, len(cats))
	for _, v := range cats {
		out = append(out, &domain.ContactCategory{
			ID:        uuid.New().String(),
			ContactID: contactID,
			Value:     v,
		})
	}
	return out
}

// ToDates converts parsed date fields to domain records.
func ToDates(contactID string, dates []Date) []*domain.ContactDate {
	out := make([]*domain.ContactDate, 0, len(dates))
	for _, d := range dates {
		out = append(out, &domain.ContactDate{
			ID:        uuid.New().String(),
			ContactID: contactID,
			Kind:      d.Kind,
			Value:     d.Value,
			Label:     d.Label,
		})
	}
	return out
}
