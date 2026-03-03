package vcard

import (
	"strings"
	"testing"
)

const sampleV3 = `BEGIN:VCARD
VERSION:3.0
UID:test-uid-123
N:Doe;John;Michael;Dr.;Jr.
FN:Dr. John Michael Doe Jr.
EMAIL;TYPE=work:john@work.com
EMAIL;TYPE=home:john@home.com
TEL;TYPE=cell:+1-555-0100
TEL;TYPE=work:+1-555-0200
ORG:Acme Corp;Engineering
TITLE:Senior Engineer
NOTE:Test note
BDAY:19900101
END:VCARD
`

const sampleV4 = `BEGIN:VCARD
VERSION:4.0
UID:test-uid-456
N:Smith;Jane;;;
FN:Jane Smith
EMAIL;PREF=1:jane@example.com
TEL;TYPE=cell;PREF=1:+44-20-1234-5678
ADR;TYPE=home:;;123 Main St;Springfield;IL;62701;USA
ORG:Tech Inc
CATEGORIES:friend,colleague
END:VCARD
`

const multiVCard = `BEGIN:VCARD
VERSION:3.0
UID:a
FN:Alice
END:VCARD

BEGIN:VCARD
VERSION:3.0
UID:b
FN:Bob
END:VCARD
`

func TestParseVCard_V3BasicFields(t *testing.T) {
	p, err := ParseVCard(sampleV3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.UID != "test-uid-123" {
		t.Errorf("UID: got %q, want %q", p.UID, "test-uid-123")
	}
	if p.FirstName != "John" {
		t.Errorf("FirstName: got %q, want %q", p.FirstName, "John")
	}
	if p.LastName != "Doe" {
		t.Errorf("LastName: got %q, want %q", p.LastName, "Doe")
	}
	if p.MiddleName != "Michael" {
		t.Errorf("MiddleName: got %q, want %q", p.MiddleName, "Michael")
	}
	if p.NamePrefix != "Dr." {
		t.Errorf("NamePrefix: got %q, want %q", p.NamePrefix, "Dr.")
	}
	if p.NameSuffix != "Jr." {
		t.Errorf("NameSuffix: got %q, want %q", p.NameSuffix, "Jr.")
	}
	if p.Org != "Acme Corp" {
		t.Errorf("Org: got %q, want %q", p.Org, "Acme Corp")
	}
	if p.Department != "Engineering" {
		t.Errorf("Department: got %q, want %q", p.Department, "Engineering")
	}
	if p.Title != "Senior Engineer" {
		t.Errorf("Title: got %q, want %q", p.Title, "Senior Engineer")
	}
	if p.Note != "Test note" {
		t.Errorf("Note: got %q, want %q", p.Note, "Test note")
	}
}

func TestParseVCard_MultipleEmails(t *testing.T) {
	p, err := ParseVCard(sampleV3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(p.Emails) != 2 {
		t.Fatalf("Emails count: got %d, want 2", len(p.Emails))
	}
	if p.Emails[0].Value != "john@work.com" {
		t.Errorf("Emails[0].Value: got %q, want %q", p.Emails[0].Value, "john@work.com")
	}
	if p.PrimaryEmail != "john@work.com" {
		t.Errorf("PrimaryEmail: got %q, want %q", p.PrimaryEmail, "john@work.com")
	}
}

func TestParseVCard_MultiplePhones(t *testing.T) {
	p, err := ParseVCard(sampleV3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(p.Phones) != 2 {
		t.Fatalf("Phones count: got %d, want 2", len(p.Phones))
	}
	if p.PrimaryPhone != "+1-555-0100" {
		t.Errorf("PrimaryPhone: got %q, want %q", p.PrimaryPhone, "+1-555-0100")
	}
}

func TestParseVCard_Dates(t *testing.T) {
	p, err := ParseVCard(sampleV3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(p.Dates) == 0 {
		t.Fatal("expected at least one date")
	}
	bday := ""
	for _, d := range p.Dates {
		if d.Kind == "bday" {
			bday = d.Value
		}
	}
	if bday != "19900101" {
		t.Errorf("BDAY: got %q, want %q", bday, "19900101")
	}
}

func TestParseVCard_V4Categories(t *testing.T) {
	p, err := ParseVCard(sampleV4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(p.Categories) != 2 {
		t.Errorf("Categories count: got %d, want 2", len(p.Categories))
	}
}

func TestParseVCard_V4Address(t *testing.T) {
	p, err := ParseVCard(sampleV4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(p.Addresses) != 1 {
		t.Fatalf("Addresses count: got %d, want 1", len(p.Addresses))
	}
	addr := p.Addresses[0]
	if addr.Street != "123 Main St" {
		t.Errorf("Street: got %q, want %q", addr.Street, "123 Main St")
	}
	if addr.City != "Springfield" {
		t.Errorf("City: got %q, want %q", addr.City, "Springfield")
	}
	if addr.PostalCode != "62701" {
		t.Errorf("PostalCode: got %q, want %q", addr.PostalCode, "62701")
	}
	if addr.Country != "USA" {
		t.Errorf("Country: got %q, want %q", addr.Country, "USA")
	}
}

func TestParseVCard_FNFallback(t *testing.T) {
	vcf := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Alice Wonderland\r\nEND:VCARD\r\n"
	p, err := ParseVCard(vcf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.FirstName != "Alice" {
		t.Errorf("FirstName fallback: got %q, want %q", p.FirstName, "Alice")
	}
	if p.LastName != "Wonderland" {
		t.Errorf("LastName fallback: got %q, want %q", p.LastName, "Wonderland")
	}
}

func TestBuildVCard_RoundTrip(t *testing.T) {
	original, err := ParseVCard(sampleV4)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	built, err := BuildVCard(original)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if !strings.Contains(built, "BEGIN:VCARD") {
		t.Error("built vCard missing BEGIN:VCARD")
	}
	if !strings.Contains(strings.ToUpper(built), "VERSION:4.0") {
		t.Error("built vCard missing VERSION:4.0")
	}

	reparsed, err := ParseVCard(built)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}

	if reparsed.FirstName != original.FirstName {
		t.Errorf("FirstName round-trip: got %q, want %q", reparsed.FirstName, original.FirstName)
	}
	if reparsed.LastName != original.LastName {
		t.Errorf("LastName round-trip: got %q, want %q", reparsed.LastName, original.LastName)
	}
	if reparsed.PrimaryEmail != original.PrimaryEmail {
		t.Errorf("PrimaryEmail round-trip: got %q, want %q", reparsed.PrimaryEmail, original.PrimaryEmail)
	}
	if reparsed.Org != original.Org {
		t.Errorf("Org round-trip: got %q, want %q", reparsed.Org, original.Org)
	}
}

func TestBuildVCard_DeriveFN(t *testing.T) {
	p := &ParsedContact{
		UID:       "uid-1",
		FirstName: "Bob",
		LastName:  "Builder",
	}
	built, err := BuildVCard(p)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(built, "FN:Bob Builder") {
		t.Errorf("expected derived FN in built vCard, got:\n%s", built)
	}
}

func TestSplitVCards(t *testing.T) {
	cards := SplitVCards(multiVCard)
	if len(cards) != 2 {
		t.Fatalf("SplitVCards count: got %d, want 2", len(cards))
	}
	if !strings.Contains(cards[0], "Alice") {
		t.Errorf("card[0] should contain Alice, got: %s", cards[0])
	}
	if !strings.Contains(cards[1], "Bob") {
		t.Errorf("card[1] should contain Bob, got: %s", cards[1])
	}
}

func TestSplitVCards_Empty(t *testing.T) {
	cards := SplitVCards("")
	if len(cards) != 0 {
		t.Errorf("expected 0 cards for empty input, got %d", len(cards))
	}
}

func TestInjectUID_AddsMissing(t *testing.T) {
	vcf := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Test\r\nEND:VCARD\r\n"
	result := InjectUID(vcf, "new-uid-999")
	if !strings.Contains(result, "UID:new-uid-999") {
		t.Errorf("expected UID injected, got:\n%s", result)
	}
}

func TestInjectUID_PreservesExisting(t *testing.T) {
	vcf := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:existing-uid\r\nFN:Test\r\nEND:VCARD\r\n"
	result := InjectUID(vcf, "new-uid-999")
	if strings.Contains(result, "new-uid-999") {
		t.Error("InjectUID should not overwrite existing UID")
	}
	if !strings.Contains(result, "existing-uid") {
		t.Error("InjectUID should preserve existing UID")
	}
}

func TestNewFromSimple(t *testing.T) {
	p := NewFromSimple("uid-1", "Alice", "Wonder", "a@b.com", "+1234", "Corp", "CEO", "notes")
	if p.FirstName != "Alice" {
		t.Errorf("FirstName: %q", p.FirstName)
	}
	if p.LastName != "Wonder" {
		t.Errorf("LastName: %q", p.LastName)
	}
	if p.PrimaryEmail != "a@b.com" {
		t.Errorf("PrimaryEmail: %q", p.PrimaryEmail)
	}
	if p.PrimaryPhone != "+1234" {
		t.Errorf("PrimaryPhone: %q", p.PrimaryPhone)
	}
	if len(p.Emails) != 1 || p.Emails[0].Value != "a@b.com" {
		t.Errorf("Emails: %v", p.Emails)
	}
	if len(p.Phones) != 1 || p.Phones[0].Value != "+1234" {
		t.Errorf("Phones: %v", p.Phones)
	}
}
