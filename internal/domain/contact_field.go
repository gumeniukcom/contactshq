package domain

import "github.com/uptrace/bun"

// ContactEmail represents one EMAIL entry on a contact.
type ContactEmail struct {
	bun.BaseModel `bun:"table:contact_emails,alias:ce"`

	ID        string `bun:",pk,type:text"  json:"id"`
	ContactID string `bun:",notnull"       json:"contact_id"`
	Value     string `bun:",notnull"       json:"value"`
	Type      string `bun:",default:''"    json:"type"`
	Pref      int    `bun:",default:0"     json:"pref"`
	Label     string `bun:",default:''"    json:"label"`
}

// ContactPhone represents one TEL entry on a contact.
type ContactPhone struct {
	bun.BaseModel `bun:"table:contact_phones,alias:cp"`

	ID        string `bun:",pk,type:text"  json:"id"`
	ContactID string `bun:",notnull"       json:"contact_id"`
	Value     string `bun:",notnull"       json:"value"`
	Type      string `bun:",default:''"    json:"type"`
	Pref      int    `bun:",default:0"     json:"pref"`
	Label     string `bun:",default:''"    json:"label"`
}

// ContactAddress represents one ADR entry on a contact.
type ContactAddress struct {
	bun.BaseModel `bun:"table:contact_addresses,alias:ca"`

	ID         string `bun:",pk,type:text"  json:"id"`
	ContactID  string `bun:",notnull"       json:"contact_id"`
	Type       string `bun:",default:''"    json:"type"`
	Pref       int    `bun:",default:0"     json:"pref"`
	Label      string `bun:",default:''"    json:"label"`
	POBox      string `bun:"po_box,default:''"    json:"po_box"`
	Extended   string `bun:",default:''"    json:"extended"`
	Street     string `bun:",default:''"    json:"street"`
	City       string `bun:",default:''"    json:"city"`
	Region     string `bun:",default:''"    json:"region"`
	PostalCode string `bun:"postal_code,default:''" json:"postal_code"`
	Country    string `bun:",default:''"    json:"country"`
}

// ContactURL represents one URL entry on a contact.
type ContactURL struct {
	bun.BaseModel `bun:"table:contact_urls,alias:cu"`

	ID        string `bun:",pk,type:text"  json:"id"`
	ContactID string `bun:",notnull"       json:"contact_id"`
	Value     string `bun:",notnull"       json:"value"`
	Type      string `bun:",default:''"    json:"type"`
	Pref      int    `bun:",default:0"     json:"pref"`
}

// ContactIM represents one IMPP entry on a contact.
type ContactIM struct {
	bun.BaseModel `bun:"table:contact_ims,alias:ci"`

	ID        string `bun:",pk,type:text"  json:"id"`
	ContactID string `bun:",notnull"       json:"contact_id"`
	Value     string `bun:",notnull"       json:"value"`
	Type      string `bun:",default:''"    json:"type"`
	Pref      int    `bun:",default:0"     json:"pref"`
}

// ContactCategory represents one CATEGORIES tag on a contact.
type ContactCategory struct {
	bun.BaseModel `bun:"table:contact_categories,alias:ccat"`

	ID        string `bun:",pk,type:text" json:"id"`
	ContactID string `bun:",notnull"      json:"contact_id"`
	Value     string `bun:",notnull"      json:"value"`
}

// ContactDate represents BDAY, ANNIVERSARY, or other date fields.
type ContactDate struct {
	bun.BaseModel `bun:"table:contact_dates,alias:cd"`

	ID        string `bun:",pk,type:text" json:"id"`
	ContactID string `bun:",notnull"      json:"contact_id"`
	Kind      string `bun:",notnull"      json:"kind"`  // "bday", "anniversary", "other"
	Value     string `bun:",notnull"      json:"value"` // raw vCard date string
	Label     string `bun:",default:''"   json:"label"`
}
