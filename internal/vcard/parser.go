package vcard

import (
	"strconv"
	"strings"

	gvcard "github.com/emersion/go-vcard"
)

// ParseVCard parses a raw vCard string (version 3.0 or 4.0) into a ParsedContact.
// All known properties are extracted; unknown/X- properties are ignored.
// The caller should keep the original raw string as the canonical vcard_data;
// call BuildVCard to reconstruct a 4.0-compliant string from ParsedContact.
func ParseVCard(data string) (*ParsedContact, error) {
	card, err := gvcard.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return nil, err
	}
	return cardToParsed(card), nil
}

func cardToParsed(card gvcard.Card) *ParsedContact {
	p := &ParsedContact{}

	// UID
	p.UID = strings.TrimSpace(card.Value(gvcard.FieldUID))

	// FN — formatted/display name
	if fn := card.Get(gvcard.FieldFormattedName); fn != nil {
		p.FN = strings.TrimSpace(fn.Value)
	}

	// N — structured name (5 components)
	if name := card.Name(); name != nil {
		p.LastName = strings.TrimSpace(name.FamilyName)
		p.FirstName = strings.TrimSpace(name.GivenName)
		p.MiddleName = strings.TrimSpace(name.AdditionalName)
		p.NamePrefix = strings.TrimSpace(name.HonorificPrefix)
		p.NameSuffix = strings.TrimSpace(name.HonorificSuffix)
	}
	// FN fallback: when N produced empty first+last name, derive from FN
	if p.FirstName == "" && p.LastName == "" && p.FN != "" {
		if idx := strings.LastIndex(p.FN, " "); idx >= 0 {
			p.FirstName = strings.TrimSpace(p.FN[:idx])
			p.LastName = strings.TrimSpace(p.FN[idx+1:])
		} else {
			p.FirstName = strings.TrimSpace(p.FN)
		}
	}

	// NICKNAME — take first comma-separated value
	if nick := card.Get(gvcard.FieldNickname); nick != nil {
		parts := strings.SplitN(nick.Value, ",", 2)
		p.Nickname = strings.TrimSpace(parts[0])
	}

	// EMAIL — all values; preferred one becomes PrimaryEmail
	prefEmail := card.Preferred(gvcard.FieldEmail)
	for _, f := range card[gvcard.FieldEmail] {
		field := extractField(f)
		p.Emails = append(p.Emails, field)
		if f == prefEmail || p.PrimaryEmail == "" {
			p.PrimaryEmail = field.Value
		}
	}

	// TEL — all values; preferred one becomes PrimaryPhone
	prefTel := card.Preferred(gvcard.FieldTelephone)
	for _, f := range card[gvcard.FieldTelephone] {
		field := extractField(f)
		p.Phones = append(p.Phones, field)
		if f == prefTel || p.PrimaryPhone == "" {
			p.PrimaryPhone = field.Value
		}
	}

	// ADR — all addresses (structured 7-component field)
	for _, adr := range card.Addresses() {
		a := Address{
			Type:       joinTypes(adr.Params.Types()),
			Pref:       parsePref(adr.Field),
			Label:      adr.Params.Get("LABEL"),
			POBox:      strings.TrimSpace(adr.PostOfficeBox),
			Extended:   strings.TrimSpace(adr.ExtendedAddress),
			Street:     strings.TrimSpace(adr.StreetAddress),
			City:       strings.TrimSpace(adr.Locality),
			Region:     strings.TrimSpace(adr.Region),
			PostalCode: strings.TrimSpace(adr.PostalCode),
			Country:    strings.TrimSpace(adr.Country),
		}
		p.Addresses = append(p.Addresses, a)
	}

	// URL
	for _, f := range card[gvcard.FieldURL] {
		p.URLs = append(p.URLs, extractField(f))
	}

	// IMPP — instant messaging
	for _, f := range card[gvcard.FieldIMPP] {
		p.IMs = append(p.IMs, extractField(f))
	}

	// ORG — first component = company, second = department
	if org := card.Get(gvcard.FieldOrganization); org != nil {
		parts := strings.SplitN(org.Value, ";", 2)
		p.Org = strings.TrimSpace(parts[0])
		if len(parts) > 1 {
			p.Department = strings.TrimSpace(parts[1])
		}
	}

	// TITLE, ROLE
	if f := card.Get(gvcard.FieldTitle); f != nil {
		p.Title = strings.TrimSpace(f.Value)
	}
	if f := card.Get(gvcard.FieldRole); f != nil {
		p.Role = strings.TrimSpace(f.Value)
	}

	// NOTE
	if f := card.Get(gvcard.FieldNote); f != nil {
		p.Note = strings.TrimSpace(f.Value)
	}

	// GENDER (vCard 4.0: sex;identity)
	sex, identity := card.Gender()
	p.Gender = string(sex)
	p.GenderText = identity

	// TZ, GEO
	if f := card.Get(gvcard.FieldTimezone); f != nil {
		p.TZ = strings.TrimSpace(f.Value)
	}
	if f := card.Get(gvcard.FieldGeolocation); f != nil {
		p.Geo = strings.TrimSpace(f.Value)
	}

	// PHOTO — URI only (binary/base64 not stored)
	if f := card.Get(gvcard.FieldPhoto); f != nil {
		// Skip data URIs that are just base64 blobs
		v := strings.TrimSpace(f.Value)
		if !strings.HasPrefix(strings.ToLower(v), "data:") {
			p.PhotoURI = v
		}
	}

	// REV, PRODID
	p.Rev = strings.TrimSpace(card.Value(gvcard.FieldRevision))
	p.ProdID = strings.TrimSpace(card.Value(gvcard.FieldProductID))

	// CATEGORIES — comma-separated tag list
	if f := card.Get(gvcard.FieldCategories); f != nil && f.Value != "" {
		for _, c := range strings.Split(f.Value, ",") {
			if v := strings.TrimSpace(c); v != "" {
				p.Categories = append(p.Categories, v)
			}
		}
	}

	// BDAY
	if f := card.Get(gvcard.FieldBirthday); f != nil && f.Value != "" {
		p.Dates = append(p.Dates, Date{Kind: "bday", Value: strings.TrimSpace(f.Value)})
	}
	// ANNIVERSARY
	if f := card.Get(gvcard.FieldAnniversary); f != nil && f.Value != "" {
		p.Dates = append(p.Dates, Date{Kind: "anniversary", Value: strings.TrimSpace(f.Value)})
	}

	return p
}

// extractField converts a go-vcard Field into our Field type.
func extractField(f *gvcard.Field) Field {
	return Field{
		Value: strings.TrimSpace(f.Value),
		Type:  joinTypes(f.Params.Types()),
		Pref:  parsePref(f),
		Label: f.Params.Get("LABEL"),
	}
}

// parsePref extracts the numeric preference from a vCard field.
// vCard 4.0 uses PREF=1..100 (1 = most preferred).
// vCard 3.0 uses TYPE=pref (treated as PREF=1).
func parsePref(f *gvcard.Field) int {
	if f == nil {
		return 0
	}
	if pref := f.Params.Get(gvcard.ParamPreferred); pref != "" {
		if n, err := strconv.Atoi(pref); err == nil {
			return n
		}
	}
	if f.Params.HasType("pref") {
		return 1
	}
	return 0
}

// joinTypes joins type values, filtering out the vCard 3.0 "pref" pseudo-type
// (preference is handled via the Pref field instead).
func joinTypes(types []string) string {
	filtered := make([]string, 0, len(types))
	for _, t := range types {
		if t != "pref" {
			filtered = append(filtered, t)
		}
	}
	return strings.Join(filtered, ",")
}
