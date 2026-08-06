package vcard

import (
	"errors"
	"io"
	"sort"
	"strings"

	gvcard "github.com/emersion/go-vcard"
)

// ErrMissingVersion reports a card without the mandatory VERSION property.
var ErrMissingVersion = errors.New("vcard: VERSION field missing")

// go-vcard escapes every value the same way — `strings.NewReplacer("\\", "\\\\", "\n",
// "\\n", ",", "\\,")` — regardless of what the property holds. RFC 6350 §3.4 does not work
// that way: whether a comma is data or a separator depends on the property's value type.
// Applying one rule to all of them corrupts three kinds of value:
//
//   - **URI values** (PHOTO, URL, KEY, …) are not TEXT and take no escaping at all. A stored
//     photo came out as `PHOTO:data:image/jpeg;base64\,/9j/…`, and a client reading that
//     literally has a base64 payload with a backslash in it. This is the visible one: it
//     breaks contact photos on iOS today.
//   - **List values** (CATEGORIES, NICKNAME) use the comma as the separator between items.
//     Escaping it turns two categories into one whose name contains a comma.
//   - **Structured values** (N, ADR, ORG, …) use the semicolon as the component separator and
//     the comma as a separator within a component. Neither may be escaped.
//
// A fourth defect is not about types: a carriage return is passed through untouched, so a
// value containing CRLF produced a bare CR mid-line and an unparseable card.
//
// # Semicolons in TEXT values
//
// RFC 6350 §3.4 also requires `;` to be escaped inside a TEXT value, and go-vcard does not do
// it. This encoder does not do it either, on purpose. go-vcard's decoder unescapes only `\\`,
// `\n` and `\,` (decoder.go:223) — it has no case for `\;`. Emitting `NOTE:a\; b` would
// therefore be read back by this very application as the literal text `a\; b`, corrupting
// every note containing a semicolon on the next read. Undoing it after decoding is not
// reliable either: a decoded `\;` is genuinely ambiguous between an escaped semicolon and a
// literal backslash followed by one. Fixing this properly means owning the decode path too;
// until then, an unescaped `;` in a single-valued TEXT property is the safer of two wrongs,
// and is what the previous encoder produced anyway.

// uriValued properties hold a URI, which RFC 6350 §3.4 exempts from escaping entirely.
var uriValued = map[string]bool{
	"PHOTO": true, "LOGO": true, "SOUND": true, "URL": true, "KEY": true,
	"SOURCE": true, "FBURL": true, "CALURI": true, "CALADRURI": true,
	"IMPP": true, "MEMBER": true, "RELATED": true, "GEO": true, "UID": true,
}

// listValued properties are a comma-separated list of TEXT values: the comma separates, it is
// not data.
var listValued = map[string]bool{
	"CATEGORIES": true, "NICKNAME": true,
}

// structuredValued properties are semicolon-separated components, each itself a
// comma-separated list. Neither separator may be escaped.
var structuredValued = map[string]bool{
	"N": true, "ADR": true, "ORG": true, "GENDER": true, "CLIENTPIDMAP": true,
}

// textEscaper escapes a single TEXT value. Same rules as go-vcard, which is what keeps output
// byte-identical for the values that were never mishandled.
var textEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, ",", `\,`)

// atomEscaper escapes one component of a structured or list value: the separators are applied
// by the caller, so they must survive.
var atomEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`)

// normalizeNewlines folds CRLF and bare CR into LF so escaping sees one representation. Left
// alone, a CR travels into the output verbatim and produces an unparseable line.
func normalizeNewlines(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// escapeValue applies the escaping the property's value type calls for.
func escapeValue(property, value string) string {
	value = normalizeNewlines(value)
	key := strings.ToUpper(property)

	switch {
	case uriValued[key]:
		// A URI carries its own percent-encoding; backslash-escaping it is what produced
		// `base64\,` in stored photos. Newlines cannot appear in a URI, and if one does the
		// value is already broken — drop it rather than emit a bare CR.
		return strings.ReplaceAll(value, "\n", "")

	case listValued[key]:
		items := strings.Split(value, ",")
		for i, item := range items {
			items[i] = atomEscaper.Replace(item)
		}
		return strings.Join(items, ",")

	case structuredValued[key]:
		components := strings.Split(value, ";")
		for i, component := range components {
			items := strings.Split(component, ",")
			for j, item := range items {
				items[j] = atomEscaper.Replace(item)
			}
			components[i] = strings.Join(items, ",")
		}
		return strings.Join(components, ";")

	default:
		return textEscaper.Replace(value)
	}
}

// EncodeCard writes a card the way go-vcard would, with the escaping corrected.
//
// The layout is deliberately identical to go-vcard's encoder — BEGIN, VERSION, remaining keys
// in sorted order, END, CRLF endings, sorted parameters — so that a card with nothing to fix
// serialises byte for byte as before. Anything else would change every stored ETag and make
// every CardDAV client re-download the whole address book.
func EncodeCard(w io.Writer, card gvcard.Card) error {
	if _, err := io.WriteString(w, "BEGIN:VCARD\r\n"); err != nil {
		return err
	}

	version := card.Get(gvcard.FieldVersion)
	if version == nil {
		return ErrMissingVersion
	}
	if _, err := io.WriteString(w, formatLine(gvcard.FieldVersion, version)+"\r\n"); err != nil {
		return err
	}

	keys := make([]string, 0, len(card))
	for k := range card {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if strings.EqualFold(k, gvcard.FieldVersion) {
			continue
		}
		for _, f := range card[k] {
			if _, err := io.WriteString(w, formatLine(k, f)+"\r\n"); err != nil {
				return err
			}
		}
	}

	_, err := io.WriteString(w, "END:VCARD\r\n")
	return err
}

// photoPlaceholder marks where an image was removed from a stored snapshot.
const photoPlaceholder = "[stripped]"

// StripPhoto replaces embedded image data with a marker, leaving everything else intact.
//
// A merge log keeps a copy of the discarded card so it can be recreated by hand. An embedded
// PHOTO is typically hundreds of kilobytes and contributes nothing to that: the point of the
// snapshot is the names, numbers and addresses. Input that cannot be decoded is returned
// unchanged — a snapshot is better than nothing.
func StripPhoto(card string) string {
	if card == "" {
		return ""
	}

	decoded, err := gvcard.NewDecoder(strings.NewReader(card)).Decode()
	if err != nil {
		return card
	}

	stripped := false
	for _, property := range []string{gvcard.FieldPhoto, "LOGO", "SOUND"} {
		for _, f := range decoded[property] {
			if f.Value != "" {
				f.Value = photoPlaceholder
				stripped = true
			}
		}
	}
	if !stripped {
		return card
	}

	var sb strings.Builder
	if err := EncodeCard(&sb, decoded); err != nil {
		return card
	}
	return sb.String()
}

func formatLine(key string, field *gvcard.Field) string {
	var sb strings.Builder

	if field.Group != "" {
		sb.WriteString(field.Group)
		sb.WriteByte('.')
	}
	sb.WriteString(key)

	paramKeys := make([]string, 0, len(field.Params))
	for k := range field.Params {
		paramKeys = append(paramKeys, k)
	}
	sort.Strings(paramKeys)

	for _, pk := range paramKeys {
		for _, pv := range field.Params[pk] {
			sb.WriteByte(';')
			sb.WriteString(pk)
			sb.WriteByte('=')
			// Parameter values keep go-vcard's treatment: they are a separate grammar
			// (RFC 6350 §3.3) and nothing in this codebase was mis-serialising them.
			sb.WriteString(textEscaper.Replace(normalizeNewlines(pv)))
		}
	}

	sb.WriteByte(':')
	sb.WriteString(escapeValue(key, field.Value))
	return sb.String()
}
