package vcard

import (
	"fmt"
	"strings"

	gvcard "github.com/emersion/go-vcard"
)

// BuildVCard generates a RFC 6350 vCard 4.0 string from a ParsedContact.
func BuildVCard(p *ParsedContact) (string, error) {
	card := make(gvcard.Card)

	// VERSION — always 4.0
	card.SetValue(gvcard.FieldVersion, "4.0")

	// UID
	if p.UID != "" {
		card.SetValue(gvcard.FieldUID, p.UID)
	}

	// N — structured name
	hasName := p.LastName != "" || p.FirstName != "" ||
		p.MiddleName != "" || p.NamePrefix != "" || p.NameSuffix != ""
	if hasName {
		card.SetName(&gvcard.Name{
			FamilyName:      p.LastName,
			GivenName:       p.FirstName,
			AdditionalName:  p.MiddleName,
			HonorificPrefix: p.NamePrefix,
			HonorificSuffix: p.NameSuffix,
		})
	}

	// FN — required in vCard 4.0; derive from N if not explicitly set
	fn := p.FN
	if fn == "" {
		parts := make([]string, 0, 5)
		if p.NamePrefix != "" {
			parts = append(parts, p.NamePrefix)
		}
		if p.FirstName != "" {
			parts = append(parts, p.FirstName)
		}
		if p.MiddleName != "" {
			parts = append(parts, p.MiddleName)
		}
		if p.LastName != "" {
			parts = append(parts, p.LastName)
		}
		if p.NameSuffix != "" {
			parts = append(parts, p.NameSuffix)
		}
		fn = strings.Join(parts, " ")
	}
	if fn == "" && p.Org != "" {
		fn = p.Org
	}
	if fn == "" {
		fn = p.UID // last resort to satisfy RFC requirement
	}
	card.SetValue(gvcard.FieldFormattedName, fn)

	// NICKNAME
	if p.Nickname != "" {
		card.SetValue(gvcard.FieldNickname, p.Nickname)
	}

	// EMAIL — all values with TYPE and PREF
	for i, e := range p.Emails {
		f := &gvcard.Field{Value: e.Value, Params: gvcard.Params{}}
		if e.Type != "" {
			for _, t := range strings.Split(e.Type, ",") {
				if t = strings.TrimSpace(t); t != "" {
					f.Params.Add(gvcard.ParamType, t)
				}
			}
		}
		if i == 0 || e.Pref == 1 {
			f.Params.Set(gvcard.ParamPreferred, "1")
		}
		card.Add(gvcard.FieldEmail, f)
	}

	// TEL — all values with TYPE and PREF
	for i, ph := range p.Phones {
		f := &gvcard.Field{Value: ph.Value, Params: gvcard.Params{}}
		if ph.Type != "" {
			for _, t := range strings.Split(ph.Type, ",") {
				if t = strings.TrimSpace(t); t != "" {
					f.Params.Add(gvcard.ParamType, t)
				}
			}
		}
		if i == 0 || ph.Pref == 1 {
			f.Params.Set(gvcard.ParamPreferred, "1")
		}
		card.Add(gvcard.FieldTelephone, f)
	}

	// ADR — all addresses
	for _, addr := range p.Addresses {
		a := &gvcard.Address{
			Field:           &gvcard.Field{Params: gvcard.Params{}},
			PostOfficeBox:   addr.POBox,
			ExtendedAddress: addr.Extended,
			StreetAddress:   addr.Street,
			Locality:        addr.City,
			Region:          addr.Region,
			PostalCode:      addr.PostalCode,
			Country:         addr.Country,
		}
		if addr.Type != "" {
			for _, t := range strings.Split(addr.Type, ",") {
				if t = strings.TrimSpace(t); t != "" {
					a.Field.Params.Add(gvcard.ParamType, t)
				}
			}
		}
		if addr.Label != "" {
			a.Field.Params.Set("LABEL", addr.Label)
		}
		card.AddAddress(a)
	}

	// URL
	for _, u := range p.URLs {
		f := &gvcard.Field{Value: u.Value, Params: gvcard.Params{}}
		if u.Type != "" {
			f.Params.Set(gvcard.ParamType, u.Type)
		}
		card.Add(gvcard.FieldURL, f)
	}

	// IMPP
	for _, im := range p.IMs {
		f := &gvcard.Field{Value: im.Value, Params: gvcard.Params{}}
		if im.Type != "" {
			f.Params.Set(gvcard.ParamType, im.Type)
		}
		card.Add(gvcard.FieldIMPP, f)
	}

	// ORG — company;department
	if p.Org != "" || p.Department != "" {
		orgVal := p.Org
		if p.Department != "" {
			orgVal += ";" + p.Department
		}
		card.SetValue(gvcard.FieldOrganization, orgVal)
	}

	// TITLE, ROLE
	if p.Title != "" {
		card.SetValue(gvcard.FieldTitle, p.Title)
	}
	if p.Role != "" {
		card.SetValue(gvcard.FieldRole, p.Role)
	}

	// NOTE
	if p.Note != "" {
		card.SetValue(gvcard.FieldNote, p.Note)
	}

	// GENDER
	if p.Gender != "" || p.GenderText != "" {
		card.SetGender(gvcard.Sex(p.Gender), p.GenderText)
	}

	// TZ, GEO
	if p.TZ != "" {
		card.SetValue(gvcard.FieldTimezone, p.TZ)
	}
	if p.Geo != "" {
		card.SetValue(gvcard.FieldGeolocation, p.Geo)
	}

	// PHOTO
	if p.PhotoURI != "" {
		card.SetValue(gvcard.FieldPhoto, p.PhotoURI)
	}

	// CATEGORIES
	if len(p.Categories) > 0 {
		card.SetCategories(p.Categories)
	}

	// Dates: BDAY, ANNIVERSARY
	for _, d := range p.Dates {
		switch d.Kind {
		case "bday":
			card.SetValue(gvcard.FieldBirthday, d.Value)
		case "anniversary":
			card.SetValue(gvcard.FieldAnniversary, d.Value)
		}
	}

	var sb strings.Builder
	if err := gvcard.NewEncoder(&sb).Encode(card); err != nil {
		return "", fmt.Errorf("encode vcard: %w", err)
	}
	return sb.String(), nil
}

// NewFromSimple creates a ParsedContact from minimal flat fields.
// Used when creating contacts without a pre-existing vCard (e.g., from UI form
// or CSV import with only single-value fields).
func NewFromSimple(uid, firstName, lastName, email, phone, org, title, note string) *ParsedContact {
	p := &ParsedContact{
		UID:       uid,
		FirstName: firstName,
		LastName:  lastName,
		Org:       org,
		Title:     title,
		Note:      note,
	}
	if email != "" {
		p.Emails = []Field{{Value: email}}
		p.PrimaryEmail = email
	}
	if phone != "" {
		p.Phones = []Field{{Value: phone}}
		p.PrimaryPhone = phone
	}
	return p
}
