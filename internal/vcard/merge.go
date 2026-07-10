package vcard

import (
	"strings"

	gvcard "github.com/emersion/go-vcard"
)

// managedProperties are the vCard properties ParsedContact fully represents, and which
// an edit therefore replaces. Everything else in a stored card is data this application
// does not model — an embedded PHOTO, a KEY, an X-ABLabel written by iOS — and it has to
// survive an edit rather than be silently dropped when the card is rebuilt.
var managedProperties = map[string]bool{
	gvcard.FieldFormattedName: true,
	gvcard.FieldName:          true,
	gvcard.FieldNickname:      true,
	gvcard.FieldEmail:         true,
	gvcard.FieldTelephone:     true,
	gvcard.FieldAddress:       true,
	gvcard.FieldURL:           true,
	gvcard.FieldIMPP:          true,
	gvcard.FieldOrganization:  true,
	gvcard.FieldTitle:         true,
	gvcard.FieldRole:          true,
	gvcard.FieldNote:          true,
	gvcard.FieldGender:        true,
	gvcard.FieldTimezone:      true,
	gvcard.FieldGeolocation:   true,
	gvcard.FieldCategories:    true,
	gvcard.FieldBirthday:      true,
	gvcard.FieldAnniversary:   true,
}

// MergeIntoVCard applies the edited fields of p onto an existing vCard.
//
// Properties the form owns are replaced wholesale; every other property is carried over
// untouched. Rebuilding the card from ParsedContact alone — which is what a client-side
// vCard builder amounts to — deletes the photo of every contact synced from Google and
// every custom property written by another CardDAV client.
//
// The existing card's VERSION and UID win: an edit is not the place to renumber a
// contact or migrate it between vCard versions.
func MergeIntoVCard(existing string, p *ParsedContact) (string, error) {
	if existing == "" {
		return BuildVCard(p)
	}

	existingCard, err := gvcard.NewDecoder(strings.NewReader(existing)).Decode()
	if err != nil {
		// An unparseable stored card has nothing worth preserving.
		return BuildVCard(p)
	}

	merged := buildCard(p)

	for key, fields := range existingCard {
		if managedProperties[key] {
			continue
		}
		merged[key] = fields
	}

	// UID identifies the contact everywhere; never let an edit change it.
	if uid, ok := existingCard[gvcard.FieldUID]; ok && len(uid) > 0 && uid[0].Value != "" {
		merged[gvcard.FieldUID] = uid
	}

	return encodeCard(merged)
}

// UnmanagedProperties lists the properties of a card that MergeIntoVCard preserves.
// Exported for tests and diagnostics.
func UnmanagedProperties(card string) []string {
	parsed, err := gvcard.NewDecoder(strings.NewReader(card)).Decode()
	if err != nil {
		return nil
	}
	var out []string
	for key := range parsed {
		if key == gvcard.FieldVersion || key == gvcard.FieldUID {
			continue
		}
		if !managedProperties[key] {
			out = append(out, key)
		}
	}
	return out
}
