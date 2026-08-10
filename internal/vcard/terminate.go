package vcard

import "strings"

// Terminated returns card with the trailing CRLF a stored vCard must keep.
//
// Cards written by EncodeCard already end that way. Cards that arrive through import or a
// backup restore did not: both paths ran strings.TrimSpace over the card before storing it,
// which strips the terminator that separates one card from the next. Concatenating those for
// export produced `END:VCARDBEGIN:VCARD` on one physical line, and SplitVCards — which tests
// for a line STARTING with BEGIN:VCARD — silently dropped every card after the first. An
// exported address book then re-imported as one contact, or none.
//
// Applied on write as well as on store, so a database already holding trimmed cards exports
// correctly without a data migration.
func Terminated(card string) string {
	card = strings.TrimRight(card, "\r\n")
	if card == "" {
		return ""
	}
	return card + "\r\n"
}
