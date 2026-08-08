package service

import (
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

// ContactFields is the structured payload the contact form sends.
//
// The form used to build a whole vCard in the browser and post it as a replacement,
// which deleted every property the form did not know about — photos and custom fields on
// any contact synced from Google or a phone. Sending the fields instead lets the server
// merge them into the stored card.
type ContactFields struct {
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	MiddleName string `json:"middle_name"`
	NamePrefix string `json:"name_prefix"`
	NameSuffix string `json:"name_suffix"`
	Nickname   string `json:"nickname"`

	Emails []ContactFieldValue `json:"emails"`
	Phones []ContactFieldValue `json:"phones"`
	URLs   []ContactFieldValue `json:"urls"`
	IMs    []ContactFieldValue `json:"ims"`

	Addresses []ContactAddressValue `json:"addresses"`

	Org        string `json:"org"`
	Department string `json:"department"`
	Title      string `json:"title"`
	Role       string `json:"role"`

	Note        string   `json:"note"`
	Categories  []string `json:"categories"`
	Birthday    string   `json:"bday"`
	Anniversary string   `json:"anniversary"`
	Gender      string   `json:"gender"`
	TZ          string   `json:"tz"`
	// Geo has no control in the web form; the form reads it off the contact and posts it
	// back unchanged. GEO is a managed property, so leaving it out of this struct made
	// every form edit write an empty GEO over a real one.
	Geo string `json:"geo"`
}

type ContactFieldValue struct {
	Value string `json:"value"`
	Type  string `json:"type"`
	Label string `json:"label"`
	Pref  int    `json:"pref"`
}

type ContactAddressValue struct {
	Type       string `json:"type"`
	Label      string `json:"label"`
	POBox      string `json:"po_box"`
	Extended   string `json:"extended"`
	Street     string `json:"street"`
	City       string `json:"city"`
	Region     string `json:"region"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
	Pref       int    `json:"pref"`
}

func toVCardFields(in []ContactFieldValue) []vcardpkg.Field {
	out := make([]vcardpkg.Field, 0, len(in))
	for _, f := range in {
		if f.Value == "" {
			continue
		}
		out = append(out, vcardpkg.Field{Value: f.Value, Type: f.Type, Label: f.Label, Pref: f.Pref})
	}
	return out
}

// ToParsed converts the form payload into the canonical in-memory contact. It fills only
// the properties the form owns; MergeIntoVCard carries the rest over from the stored card.
func (f ContactFields) ToParsed(uid string) *vcardpkg.ParsedContact {
	p := &vcardpkg.ParsedContact{
		UID:        uid,
		FirstName:  f.FirstName,
		LastName:   f.LastName,
		MiddleName: f.MiddleName,
		NamePrefix: f.NamePrefix,
		NameSuffix: f.NameSuffix,
		Nickname:   f.Nickname,
		Org:        f.Org,
		Department: f.Department,
		Title:      f.Title,
		Role:       f.Role,
		Note:       f.Note,
		Gender:     f.Gender,
		TZ:         f.TZ,
		Geo:        f.Geo,
		Emails:     toVCardFields(f.Emails),
		Phones:     toVCardFields(f.Phones),
		URLs:       toVCardFields(f.URLs),
		IMs:        toVCardFields(f.IMs),
	}

	for _, c := range f.Categories {
		if c != "" {
			p.Categories = append(p.Categories, c)
		}
	}

	for _, a := range f.Addresses {
		if a.Street == "" && a.City == "" && a.Region == "" && a.PostalCode == "" &&
			a.Country == "" && a.POBox == "" && a.Extended == "" {
			continue
		}
		p.Addresses = append(p.Addresses, vcardpkg.Address{
			Type:       a.Type,
			Label:      a.Label,
			Pref:       a.Pref,
			POBox:      a.POBox,
			Extended:   a.Extended,
			Street:     a.Street,
			City:       a.City,
			Region:     a.Region,
			PostalCode: a.PostalCode,
			Country:    a.Country,
		})
	}

	if f.Birthday != "" {
		p.Dates = append(p.Dates, vcardpkg.Date{Kind: "bday", Value: f.Birthday})
	}
	if f.Anniversary != "" {
		p.Dates = append(p.Dates, vcardpkg.Date{Kind: "anniversary", Value: f.Anniversary})
	}

	if len(p.Emails) > 0 {
		p.PrimaryEmail = p.Emails[0].Value
	}
	if len(p.Phones) > 0 {
		p.PrimaryPhone = p.Phones[0].Value
	}

	return p
}
