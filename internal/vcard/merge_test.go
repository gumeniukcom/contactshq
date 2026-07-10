package vcard_test

import (
	"strings"
	"testing"

	"github.com/gumeniukcom/contactshq/internal/vcard"
)

// A contact synced from Google or an iPhone carries properties this app does not model.
// Editing the name must not delete them.
const googleSyncedCard = "BEGIN:VCARD\r\n" +
	"VERSION:4.0\r\n" +
	"UID:contact-1\r\n" +
	"FN:Jane Doe\r\n" +
	"N:Doe;Jane;;;\r\n" +
	"EMAIL:jane@example.com\r\n" +
	"PHOTO:data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD\r\n" +
	"X-ABLabel:_$!<Work>!$_\r\n" +
	"X-SOCIALPROFILE;TYPE=twitter:https://twitter.com/jane\r\n" +
	"KEY:https://example.com/key.asc\r\n" +
	"PRODID:-//Google Inc//Google Contacts//EN\r\n" +
	"REV:20250101T120000Z\r\n" +
	"END:VCARD\r\n"

func parse(t *testing.T, card string) *vcard.ParsedContact {
	t.Helper()

	p, err := vcard.ParseVCard(card)
	if err != nil {
		t.Fatalf("ParseVCard: %v", err)
	}
	return p
}

func TestMergeIntoVCard_PreservesUnmodelledProperties(t *testing.T) {
	p := parse(t, googleSyncedCard)
	p.FirstName = "Janet" // the user renames the contact
	p.FN = "Janet Doe"

	merged, err := vcard.MergeIntoVCard(googleSyncedCard, p)
	if err != nil {
		t.Fatalf("MergeIntoVCard: %v", err)
	}

	if !strings.Contains(merged, "Janet") {
		t.Errorf("the edit was not applied: %q", merged)
	}

	// go-vcard uppercases property names and escapes commas on the way out, so assert on
	// the payload rather than on a byte-for-byte line.
	for _, want := range []string{
		"PHOTO:", "/9j/4AAQSkZJRgABAQAAAQABAAD",
		"X-ABLABEL:",
		"X-SOCIALPROFILE",
		"twitter.com/jane",
		"KEY:https://example.com/key.asc",
		"PRODID:",
		"REV:",
	} {
		if !strings.Contains(merged, want) {
			t.Errorf("merge dropped %q\n\ngot:\n%s", want, merged)
		}
	}
}

func TestMergeIntoVCard_ReplacesManagedProperties(t *testing.T) {
	p := parse(t, googleSyncedCard)
	p.Emails = []vcard.Field{{Value: "new@example.com"}}
	p.Note = "added a note"

	merged, err := vcard.MergeIntoVCard(googleSyncedCard, p)
	if err != nil {
		t.Fatalf("MergeIntoVCard: %v", err)
	}

	if strings.Contains(merged, "jane@example.com") {
		t.Error("the old email must be replaced, not kept alongside the new one")
	}
	if !strings.Contains(merged, "new@example.com") {
		t.Error("the new email is missing")
	}
	if !strings.Contains(merged, "added a note") {
		t.Error("NOTE was not applied")
	}
}

// Removing every email in the form must remove the EMAIL lines, not leave the old ones.
func TestMergeIntoVCard_ClearedFieldsAreRemoved(t *testing.T) {
	p := parse(t, googleSyncedCard)
	p.Emails = nil

	merged, err := vcard.MergeIntoVCard(googleSyncedCard, p)
	if err != nil {
		t.Fatalf("MergeIntoVCard: %v", err)
	}

	if strings.Contains(merged, "EMAIL") {
		t.Errorf("cleared email still present:\n%s", merged)
	}
	if !strings.Contains(merged, "PHOTO:") {
		t.Error("clearing a managed field must not disturb unmanaged ones")
	}
}

// An edit is not the place to renumber a contact.
func TestMergeIntoVCard_KeepsExistingUID(t *testing.T) {
	p := parse(t, googleSyncedCard)
	p.UID = "some-other-uid"

	merged, err := vcard.MergeIntoVCard(googleSyncedCard, p)
	if err != nil {
		t.Fatalf("MergeIntoVCard: %v", err)
	}

	if !strings.Contains(merged, "UID:contact-1") {
		t.Errorf("UID was changed by an edit:\n%s", merged)
	}
	if strings.Contains(merged, "some-other-uid") {
		t.Error("the caller's UID must not overwrite the stored one")
	}
}

func TestMergeIntoVCard_EmptyExistingBuildsFresh(t *testing.T) {
	p := &vcard.ParsedContact{UID: "u1", FirstName: "New", LastName: "Contact"}

	merged, err := vcard.MergeIntoVCard("", p)
	if err != nil {
		t.Fatalf("MergeIntoVCard: %v", err)
	}

	if !strings.Contains(merged, "UID:u1") || !strings.Contains(merged, "New") {
		t.Errorf("fresh card is wrong:\n%s", merged)
	}
}

func TestMergeIntoVCard_UnparseableExistingBuildsFresh(t *testing.T) {
	p := &vcard.ParsedContact{UID: "u1", FirstName: "New"}

	merged, err := vcard.MergeIntoVCard("this is not a vCard", p)
	if err != nil {
		t.Fatalf("MergeIntoVCard: %v", err)
	}
	if !strings.Contains(merged, "UID:u1") {
		t.Errorf("expected a freshly built card, got:\n%s", merged)
	}
}

// A round trip through parse + merge must be a no-op when nothing was edited.
func TestMergeIntoVCard_RoundTripIsStable(t *testing.T) {
	p := parse(t, googleSyncedCard)

	once, err := vcard.MergeIntoVCard(googleSyncedCard, p)
	if err != nil {
		t.Fatalf("MergeIntoVCard: %v", err)
	}
	twice, err := vcard.MergeIntoVCard(once, parse(t, once))
	if err != nil {
		t.Fatalf("MergeIntoVCard: %v", err)
	}

	if once != twice {
		t.Errorf("merge is not idempotent:\nfirst:\n%s\nsecond:\n%s", once, twice)
	}
}

func TestUnmanagedProperties(t *testing.T) {
	got := vcard.UnmanagedProperties(googleSyncedCard)

	want := map[string]bool{"PHOTO": true, "X-ABLABEL": true, "X-SOCIALPROFILE": true, "KEY": true, "PRODID": true, "REV": true}
	for _, key := range got {
		if !want[key] {
			t.Errorf("unexpected unmanaged property %q", key)
		}
		delete(want, key)
	}
	if len(want) > 0 {
		t.Errorf("these properties were not reported as unmanaged: %v", want)
	}
}
