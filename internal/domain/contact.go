package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type Contact struct {
	bun.BaseModel `bun:"table:contacts,alias:c"`

	ID            string `bun:",pk,type:text" json:"id"`
	AddressBookID string `bun:",notnull" json:"address_book_id"`
	UID           string `bun:",notnull" json:"uid"`
	ETag          string `bun:"etag,notnull" json:"etag"`
	VCardData     string `bun:"vcard_data,type:text,notnull" json:"vcard_data,omitempty"`
	FirstName     string `bun:",default:''" json:"first_name"`
	LastName      string `bun:",default:''" json:"last_name"`
	MiddleName    string `bun:",default:''" json:"middle_name"`
	NamePrefix    string `bun:",default:''" json:"name_prefix"`
	NameSuffix    string `bun:",default:''" json:"name_suffix"`
	Nickname      string `bun:",default:''" json:"nickname"`

	// Primary (pref=1 or first) — denormalised for fast display
	Email string `bun:",default:''" json:"email"`
	Phone string `bun:",default:''" json:"phone"`

	Org        string `bun:",default:''" json:"org"`
	Department string `bun:",default:''" json:"department"`
	Title      string `bun:",default:''" json:"title"`
	Role       string `bun:",default:''" json:"role"`
	Note       string `bun:",default:''" json:"note"`

	Bday        string `bun:",default:''" json:"bday"`
	Anniversary string `bun:",default:''" json:"anniversary"`
	Gender      string `bun:",default:''" json:"gender"`
	TZ          string `bun:"tz,default:''" json:"tz"`
	Geo         string `bun:",default:''" json:"geo"`
	PhotoURI    string `bun:"photo_uri,default:''" json:"photo_uri"`
	Rev         string `bun:",default:''" json:"rev"`

	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations (loaded on demand via WithRelations queries)
	AddressBook *AddressBook `bun:"rel:belongs-to,join:address_book_id=id" json:"address_book,omitempty"`

	Emails     []*ContactEmail    `bun:"rel:has-many,join:id=contact_id" json:"emails,omitempty"`
	Phones     []*ContactPhone    `bun:"rel:has-many,join:id=contact_id" json:"phones,omitempty"`
	Addresses  []*ContactAddress  `bun:"rel:has-many,join:id=contact_id" json:"addresses,omitempty"`
	URLs       []*ContactURL      `bun:"rel:has-many,join:id=contact_id" json:"urls,omitempty"`
	IMs        []*ContactIM       `bun:"rel:has-many,join:id=contact_id" json:"ims,omitempty"`
	Categories []*ContactCategory `bun:"rel:has-many,join:id=contact_id" json:"categories,omitempty"`
	Dates      []*ContactDate     `bun:"rel:has-many,join:id=contact_id" json:"dates,omitempty"`
}
